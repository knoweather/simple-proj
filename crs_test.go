package proj

import (
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
)

// crsSame 比较两次 Parse 的 CRS 语义字段相等(往返测试用)。
func crsSame(t *testing.T, name string, a, b *CRS) {
	t.Helper()
	if a.Proj != b.Proj {
		t.Errorf("%s: Proj 不一致 %q vs %q", name, a.Proj, b.Proj)
	}
	if !reflect.DeepEqual(a.Params, b.Params) {
		t.Errorf("%s: Params 不一致:\n  %#v\n  %#v", name, a.Params, b.Params)
	}
	if !reflect.DeepEqual(a.UnknownParams, b.UnknownParams) {
		t.Errorf("%s: UnknownParams 不一致\n  %#v\n  %#v", name, a.UnknownParams, b.UnknownParams)
	}
	if a.Ellipsoid != b.Ellipsoid {
		t.Errorf("%s: Ellipsoid 不一致:\n  %#v\n  %#v", name, a.Ellipsoid, b.Ellipsoid)
	}
}

func TestParseBasic(t *testing.T) {
	c, err := Parse("+proj=geos +lon_0=140.7 +h=35785863 +ellps=WGS84 +units=m +no_defs")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if c.Proj != "geos" {
		t.Errorf("Proj: %q", c.Proj)
	}
	for k, want := range map[string]float64{
		"lon_0": 140.7, "h": 35785863, "x_0": 0, "y_0": 0, "lat_0": 0, "k_0": 1,
	} {
		if got := c.Params[k]; got != want {
			t.Errorf("Params[%s]=%v want %v", k, got, want)
		}
	}
	if c.UnknownParams["no_defs"] != "" {
		t.Errorf("no_defs 旗标应存 UnknownParams 为空串, got %q", c.UnknownParams["no_defs"])
	}
	c2, err := Parse(c.String())
	if err != nil {
		t.Fatalf("Parse(String()): %v", err)
	}
	crsSame(t, "geos 往返", c, c2)
}

func TestParseAliases(t *testing.T) {
	a, err := Parse("+proj=tmerc +k=0.9996 +lon_0=105 +ellps=WGS84")
	if err != nil {
		t.Fatalf("k 别名: %v", err)
	}
	if a.Params["k_0"] != 0.9996 {
		t.Errorf("k→k_0 归一失败: %v", a.Params["k_0"])
	}
	b, err := Parse("+proj=tmerc +k0=0.9996 +lon0=105 +ellps=WGS84")
	if err != nil {
		t.Fatalf("k0/lon0 别名: %v", err)
	}
	crsSame(t, "k vs k0", a, b)
	c, err := Parse("+proj=lcc +lat1=30 +lat_2=60 +ellps=WGS84")
	if err != nil {
		t.Fatalf("lat1 别名: %v", err)
	}
	if c.Params["lat_1"] != 30 || c.Params["lat_2"] != 60 {
		t.Errorf("lat1/lat_2: %v", c.Params)
	}
	d, err := Parse("+proj=eqc +latts=33.5 +ellps=WGS84")
	if err != nil {
		t.Fatalf("latts 别名: %v", err)
	}
	if d.Params["lat_ts"] != 33.5 {
		t.Errorf("latts: %v", d.Params)
	}
}

func TestParseLonglatAlias(t *testing.T) {
	a, err := Parse("+proj=longlat +ellps=WGS84")
	if err != nil {
		t.Fatalf("%v", err)
	}
	if a.Proj != "latlong" {
		t.Errorf("longlat 应常规化为 latlong, got %q", a.Proj)
	}
	b, err := Parse("+proj=latlong +ellps=WGS84")
	if err != nil {
		t.Fatalf("%v", err)
	}
	crsSame(t, "longlat/latlong", a, b)
	if a.String() != "+proj=longlat +ellps=WGS84" {
		t.Errorf("String() 必须保留原始串, got %q", a.String())
	}
}

func TestParseRadianSuffix(t *testing.T) {
	c, err := Parse("+proj=geos +lon_0=1.570796r +ellps=WGS84")
	if err != nil {
		t.Fatalf("%v", err)
	}
	// 与实现同口径:运行期 float64 运算(变量形式,不做编译期常量精确折叠,
	// 否则与 double 双舍入差 O(1ulp) ≈ 2e-14 度)。
	rad := 1.570796
	want := rad * 180 / math.Pi
	if c.Params["lon_0"] != want {
		t.Errorf("'r' 后缀: got %v want %v", c.Params["lon_0"], want)
	}
	// 非度数参数拒收 'r' 后缀
	if _, err := Parse("+proj=geos +h=1.5r"); err == nil {
		t.Errorf("非角度参数 +h=1.5r 应报错")
	}
}

func TestParseEPSG(t *testing.T) {
	t4326, err := Parse("EPSG:4326")
	if err != nil {
		t.Fatalf("EPSG:4326: %v", err)
	}
	if t4326.Proj != "latlong" || !near(t4326.Ellipsoid.A, 6378137) {
		t.Errorf("EPSG:4326 形态: %#v", t4326)
	}
	if t4326.EPSG() != 4326 {
		t.Errorf("EPSG(): %d", t4326.EPSG())
	}
	// String() 回放规范 proj 串且可再吸入
	if s := t4326.String(); s != "+proj=latlong +ellps=WGS84" {
		t.Errorf("String(): %q", s)
	}
	t2, err := Parse(t4326.String())
	if err != nil {
		t.Fatalf("Parse(String(EPSG:4326)): %v", err)
	}
	if t2.Ellipsoid != t4326.Ellipsoid || t2.Proj != t4326.Proj {
		t.Errorf("EPSG 规范串往返: %#v", t2)
	}
	// 往返稳定:二次 String 化后同串
	if t4326.String() != t2.String() {
		t.Errorf("String 往返不稳定: %q vs %q", t4326.String(), t2.String())
	}

	t3857, err := Parse("epsg:3857")
	if err != nil {
		t.Fatalf("epsg:3857: %v", err)
	}
	if t3857.Proj != "merc" || !t3857.Ellipsoid.Sphere || !near(t3857.Ellipsoid.A, 6378137) {
		t.Errorf("EPSG:3857 形态: %#v", t3857)
	}
	if t3857.EPSG() != 3857 {
		t.Errorf("EPSG(): %d", t3857.EPSG())
	}
	// 等价写法特征识别(非字面量路径)
	t3, err := Parse("+proj=merc +a=6378137 +b=6378137 +lat_ts=0 +lon_0=0 +x_0=0 +y_0=0 +units=m")
	if err != nil {
		t.Fatalf("%v", err)
	}
	if t3.EPSG() != 3857 {
		t.Errorf("merc 球体应识别为 3857, got %d", t3.EPSG())
	}
	// NAD83 → 4269
	t4269, err := Parse("+proj=latlong +ellps=GRS80")
	if err != nil {
		t.Fatalf("%v", err)
	}
	if t4269.EPSG() != 4269 {
		t.Errorf("latlong+GRS80 应识别为 4269, got %d", t4269.EPSG())
	}
	// 不支持的码报错
	if _, err := Parse("EPSG:2180"); err == nil {
		t.Errorf("EPSG:2180 应报错(v0 未收录)")
	}
}

func TestParseGoldenCRSEnd2End(t *testing.T) {
	// xcheck 侧将要消费的 9 条 CRS 字符串必须全部可解析(此处不含 mer 参数等
	// B 轨专属投影的注册检查 —— NewProjection 属于各自轨道的测试)。
	for _, crs := range []string{
		"+proj=latlong +ellps=WGS84",
		"+proj=eqc +lat_ts=0 +lat_0=0 +lon_0=180 +x_0=0 +y_0=0 +ellps=WGS84 +units=m",
		"+proj=geos +lon_0=140.7 +h=35785863 +x_0=0 +y_0=0 +a=6378137 +b=6356752.3 +units=m +sweep=x",
		"+proj=geos +lon_0=0 +h=35785863 +ellps=WGS84 +sweep=y",
		"+proj=stere +lat_0=90 +lat_ts=60 +lon_0=-100 +ellps=WGS84",
		"+proj=laea +lat_0=45 +lon_0=10 +ellps=WGS84 +x_0=0 +y_0=0",
		"+proj=lcc +lat_1=30 +lat_2=60 +lat_0=0 +lon_0=100 +ellps=WGS84",
		"+proj=merc +a=6378137 +b=6378137 +lat_ts=0.0 +lon_0=0.0 +x_0=0 +y_0=0 +units=m",
		"+proj=tmerc +lat_0=0 +lon_0=105 +k=0.9996 +ellps=WGS84",
	} {
		c, err := Parse(crs)
		if err != nil {
			t.Fatalf("Parse(%q): %v", crs, err)
		}
		c2, err := Parse(c.String())
		if err != nil {
			t.Fatalf("Parse(String(%q)): %v", crs, err)
		}
		crsSame(t, crs, c, c2)
	}
}

func TestParseSweepStorage(t *testing.T) {
	c, err := Parse("+proj=geos +sweep=x +ellps=WGS84")
	if err != nil {
		t.Fatalf("%v", err)
	}
	if c.UnknownParams["sweep"] != "x" {
		t.Errorf("sweep 应存 UnknownParams(决策 D1), got %+v", c.UnknownParams)
	}
	if got := paramString(c, "sweep", "y"); got != "x" {
		t.Errorf("paramString(sweep): %q", got)
	}
	c2, _ := Parse("+proj=geos +ellps=WGS84")
	if got := paramString(c2, "sweep", "y"); got != "y" {
		t.Errorf("sweep 缺省应按 PROJ>=6 口径回落 y, got %q", got)
	}
}

func TestParseUnknownParams(t *testing.T) {
	// 真未知键(PROJ 也不认识)→ 宽容保留,便于未来扩展。
	c, err := Parse("+proj=geos +wktext +my_future_flag=42 +ellps=WGS84")
	if err != nil {
		t.Fatalf("%v", err)
	}
	if c.UnknownParams["wktext"] != "" || c.UnknownParams["my_future_flag"] != "42" {
		t.Errorf("未知参数应原样保留: %#v", c.UnknownParams)
	}
}

// TestParseRejectsUnsupportedSemantics 有语义但未实现的键必须 fail-loud:
// 静默忽略 +pm/+towgs84/+nadgrids 之类会让坐标算错却不报错(本次改造的核心)。
func TestParseRejectsUnsupportedSemantics(t *testing.T) {
	cases := []string{
		"+proj=merc +pm=paris +ellps=WGS84",       // 本初子午线
		"+proj=merc +pm=paris",                    // 缺值同罪
		"+proj=merc +axis=neu +ellps=WGS84",       // 轴序
		"+proj=merc +nadgrids=conus +ellps=WGS84", // 网格基准
		"+proj=longlat +datum=NAD27",              // 需要基准变换
		"+proj=merc +R_A +ellps=WGS84",            // authalic 半径旗标
		"+proj=merc +R_V +ellps=WGS84",
		"+proj=merc +over +ellps=WGS84", // over-range
		"+proj=merc +geoc +ellps=WGS84", // 地心纬度
		"+proj=merc +geoidgrids=egm96.gtx +ellps=WGS84",
		"+init=epsg:4326", // +init 语法
	}
	for _, s := range cases {
		if _, err := Parse(s); err == nil {
			t.Errorf("Parse(%q) 应报错(有语义但未实现,不许静默忽略)", s)
		}
	}
}

// TestTowgs84NeedsTransformer +towgs84 现在由 Parse 收下(供 NewTransformer 做
// 基准变换),但**单投影**必须拒收 —— 投影只在自己椭球上映射,平移无处安放。
func TestTowgs84NeedsTransformer(t *testing.T) {
	cases := []string{
		"+proj=merc +towgs84=1,2,3 +ellps=WGS84",
		"+proj=merc +towgs84=0,0,0,1,2,3,4 +ellps=WGS84",
		"+proj=longlat +ellps=bessel +towgs84=598.1,73.7,418.2",
	}
	for _, s := range cases {
		c, err := Parse(s)
		if err != nil {
			t.Fatalf("Parse(%q) 应收下 towgs84: %v", s, err)
		}
		if len(c.Towgs84) != 3 && len(c.Towgs84) != 7 {
			t.Errorf("%q: Towgs84 = %v, want 3 或 7 项", s, c.Towgs84)
		}
		if _, err := c.NewProjection(); err == nil {
			t.Errorf("%q: 单投影带 towgs84 应报错", s)
		}
	}
	// 带 towgs84 的 CRS 不能被特征回识别成 WGS84 系的码(差一个基准)
	{
		c, err := Parse("+proj=longlat +ellps=WGS84 +towgs84=1,2,3")
		if err != nil {
			t.Fatal(err)
		}
		if got := c.EPSG(); got != 0 {
			t.Errorf("带 towgs84 的 CRS 的 EPSG() 应为 0, got %d", got)
		}
	}

	// 全零 towgs84 = 无变换,单投影照常可用
	c, err := Parse("+proj=merc +towgs84=0,0,0 +ellps=WGS84")
	if err != nil {
		t.Fatal(err)
	}
	if c.Towgs84 != nil {
		t.Errorf("全零 towgs84 应视为无基准参数, got %v", c.Towgs84)
	}
	if _, err := c.NewProjection(); err != nil {
		t.Errorf("全零 towgs84 不该妨碍单投影: %v", err)
	}
}

// TestParseRejectsBareFlags 裸着写的 +ellps/+units/+towgs84 等于没说,
// PROJ 也报错(pyproj 实测);此前会被静默收进 UnknownParams。
func TestParseRejectsBareFlags(t *testing.T) {
	for _, s := range []string{
		"+proj=merc +ellps",
		"+proj=merc +ellps=WGS84 +units",
		"+proj=merc +towgs84 +ellps=WGS84",
		"+proj=merc +ellps=",
		"+proj=merc +units=",
		"+proj=merc +towgs84=",
	} {
		if _, err := Parse(s); err == nil {
			t.Errorf("Parse(%q) 应报错(裸旗标缺值)", s)
		}
	}
	// +sweep 裸写 PROJ 放行(对 merc 无意义),我们也不报错;
	// +zone 是数值键,裸写按「缺值」报错(比 PROJ 的静默忽略更严,可接受)
	if _, err := Parse("+proj=merc +sweep +ellps=WGS84"); err != nil {
		t.Errorf("+sweep 裸写不该报错: %v", err)
	}
}

// TestParseAcceptsNoopValues 恒等值放行:+nadgrids=@null、+towgs84=0,0,0、
// +datum=WGS84、+type=crs 在 PROJ 里等于不做变换。
func TestParseAcceptsNoopValues(t *testing.T) {
	cases := []string{
		"+proj=longlat +datum=WGS84",
		"+proj=merc +nadgrids=@null +ellps=WGS84",
		"+proj=merc +towgs84=0,0,0 +ellps=WGS84",
		"+proj=merc +towgs84=0,0,0,0,0,0,0 +ellps=WGS84",
		"+proj=longlat +ellps=WGS84 +type=crs",
		"+proj=merc +a=6378137 +b=6378137 +no_defs",
	}
	for _, s := range cases {
		if _, err := Parse(s); err != nil {
			t.Errorf("Parse(%q) 应放行(恒等值): %v", s, err)
		}
	}
	// +datum=WGS84 等价于默认椭球,数值结果须与 +ellps=WGS84 一致。
	a, err := Parse("+proj=merc +datum=WGS84")
	if err != nil {
		t.Fatal(err)
	}
	if a.Ellipsoid.A != 6378137.0 {
		t.Errorf("+datum=WGS84 应落到 WGS84 椭球, got a=%v", a.Ellipsoid.A)
	}
}

func TestParseErrors(t *testing.T) {
	cases := []string{
		"",
		"   ",
		"+lat_0=10 +ellps=WGS84",            // 缺 +proj
		"+proj=abc +ellps=WGS84",            // 未知投影
		"+proj=geos +lon_0=abc",             // 数值错
		"+proj=geos +h",                     // 数值缺值
		"+proj=geos +units=furlong",         // 未知单位(km 已支持)
		"+proj=eqc +lat_ts=95 +ellps=WGS84", // 纬度出界
		"+proj=geos +lon_0=500",             // 经度出界
		"+proj=geos +k_0=0",                 // k_0 非正
		"+proj=merc +ellps=未知椭球",            // 未知椭球
		"+proj=merc +a=0",                   // 长半轴非法(由 ellipsoid 报)
	}
	for _, s := range cases {
		if _, err := Parse(s); err == nil {
			t.Errorf("Parse(%q) 应报错", s)
		}
	}
}

func TestParseDuplicatedKeyLastWins(t *testing.T) {
	c, err := Parse("+proj=geos +lon_0=10 +lon_0=20 +ellps=WGS84")
	if err != nil {
		t.Fatalf("%v", err)
	}
	if c.Params["lon_0"] != 20 {
		t.Errorf("同键后者胜, got %v", c.Params["lon_0"])
	}
}

func TestNewProjectionDispatch(t *testing.T) {
	// 已注册(latlong)
	c, _ := Parse("+proj=latlong +ellps=WGS84")
	projection, err := c.NewProjection()
	if err != nil {
		t.Fatalf("latlong NewProjection: %v", err)
	}
	if projection.CRS() != c {
		t.Errorf("CRS() 应回传绑定实例")
	}
	// 未注册投影(+proj 合法但 impl 未登记于 factories)应报错不 panic
	// (B 轨 stere 已注册,故用合成名;不能用 Parse 构造,knownProj 会拒收)
	c2 := &CRS{Proj: "pseudo_no_impl"}
	_, err = c2.NewProjection()
	if err == nil || errors.Is(err, ErrDomain) {
		t.Errorf("未注册投影应报非 ErrDomain 错误: %v", err)
	}
}

// TestParseEdgeErrors 参数级边界:空名/缺值/非有限/错误后缀/坏 EPSG 码。
func TestParseEdgeErrors(t *testing.T) {
	cases := []struct{ src, wantSub string }{
		{"+proj=merc +towgs84=1,abc,3", "towgs84"},
		{"+proj=merc +=1", "空参数名"},
		{"+proj", "+proj 必须带投影名"},
		{"+proj=merc +lat_0=NaN", "非有限"},
		{"+proj=merc +lat_0=Inf", "非有限"},
		{"+proj=merc +k_0=1.5r", "'r' 弧度后缀"},
		{"EPSG:abc", "缺少 +proj"},
		{"EPSG:99999", "不支持的 EPSG"},
	}
	for _, c := range cases {
		_, err := Parse(c.src)
		if err == nil {
			t.Errorf("Parse(%q) 应报错", c.src)
			continue
		}
		if !strings.Contains(err.Error(), c.wantSub) {
			t.Errorf("Parse(%q) 的报错应含 %q, got %v", c.src, c.wantSub, err)
		}
	}
}

// TestEPSGFeatureMatch UTM 用 proj 串写(无 EPSG 字面量)也要能特征回识别。
func TestEPSGFeatureMatch(t *testing.T) {
	cases := []struct {
		crs  string
		want int
	}{
		{"+proj=utm +zone=50 +ellps=WGS84", 32650},
		{"+proj=utm +zone=50 +south +ellps=WGS84", 32750},
		{"+proj=utm +zone=1 +ellps=WGS84", 32601},
		{"+proj=utm +zone=60 +south +ellps=WGS84", 32760},
		{"+proj=utm +zone=50 +ellps=GRS80", 0},           // 非 WGS84 轴
		{"+proj=utm +zone=50 +units=km +ellps=WGS84", 0}, // 非米单位
	}
	for _, c := range cases {
		crs, err := Parse(c.crs)
		if err != nil {
			t.Fatalf("Parse(%q): %v", c.crs, err)
		}
		if got := crs.EPSG(); got != c.want {
			t.Errorf("%q: EPSG() = %d, want %d", c.crs, got, c.want)
		}
	}
	// nil 接收者
	var nilCRS *CRS
	if got := nilCRS.EPSG(); got != 0 {
		t.Errorf("nil CRS 的 EPSG() 应为 0, got %d", got)
	}
}
