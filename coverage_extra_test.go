package proj

// coverage_extra_test.go — 边角与防御分支的定向测试。
//
// 这些用例覆盖的是「正常调用路径走不到、但确实是公开行为一部分」的代码:
//   - WKT 词法/结构错误路径(用户贴畸形 WKT 时会撞上);
//   - 各投影适配层的域外返回(极端输入);
//   - nil 守卫与纯函数(矩阵求逆、DomainError 文案、椭球构造)的非法入参。
// 内核里"永不 panic 的最后一层保险"(适配层已先校验)不在此列 —— 那部分见
// 文件末尾的说明。

import (
	"errors"
	"math"
	"strings"
	"testing"
)

// TestWKTLexicalEdges WKT 解析器的错误路径与回退分支。
func TestWKTLexicalEdges(t *testing.T) {
	// 空串:value() 在 EOF → "意外结束"
	if _, err := ParseWKT(""); err == nil || !strings.Contains(err.Error(), "意外结束") {
		t.Errorf("空 WKT 应报「意外结束」, got %v", err)
	}

	// WKT2 把线单位直接写在 CS 里(没有 AXIS)→ 走 CS 级 LENGTHUNIT 回退
	kmCS := `PROJCRS["cs-unit",BASEGEOGCRS["W",DATUM["D",ELLIPSOID["E",6378137,298.257223563]]],` +
		`CONVERSION["c",METHOD["Transverse Mercator"],` +
		`PARAMETER["Longitude of natural origin",0,ANGLEUNIT["degree",0.0174532925199433]]],` +
		`CS[Cartesian,2,LENGTHUNIT["kilometre",1000]]]`
	crs, err := Parse(kmCS)
	if err != nil {
		t.Fatalf("CS 级 LENGTHUNIT: %v", err)
	}
	if crs.Params["to_meter"] != 1000 {
		t.Errorf("CS 级 LENGTHUNIT 应得 to_meter=1000, got %v", crs.Params["to_meter"])
	}

	// ANGLEUNIT 名字不是 degree 且没有系数 → 报错
	if _, err := Parse(`GEOGCRS["g",DATUM["d",ELLIPSOID["e",6378137,298.257223563]],ANGLEUNIT["gon"]]`); err == nil ||
		!strings.Contains(err.Error(), "ANGLEUNIT 缺换算系数") {
		t.Errorf("缺系数的 ANGLEUNIT 应报错, got %v", err)
	}

	// 名字不是 degree,但系数恰好等于 π/180 → 按度收(浮点兜底分支)
	if _, err := Parse(`GEOGCRS["g",DATUM["d",ELLIPSOID["e",6378137,298.257223563]],` +
		`ANGLEUNIT["dd",0.0174532925199433]]`); err != nil {
		t.Errorf("系数为 π/180 的角单位应按度收: %v", err)
	}

	// 地理 CRS 缺椭球
	if _, err := Parse(`GEOGCRS["g",DATUM["d"]]`); err == nil ||
		!strings.Contains(err.Error(), "找不到 ELLIPSOID") {
		t.Errorf("缺椭球的地理 CRS 应报错, got %v", err)
	}

	// 地理 CRS 的角单位系数非法 → wktAngularDegrees 报错
	if _, err := Parse(`GEOGCRS["g",DATUM["d",ELLIPSOID["e",6378137,298.257223563]],ANGLEUNIT["x",-1]]`); err == nil {
		t.Error("负系数的 ANGLEUNIT 应报错")
	}

	// 投影 CRS 的参数自带非法 ANGLEUNIT:全局换算先撞上 → 报错
	badUnit := `PROJCRS["x",BASEGEOGCRS["W",DATUM["D",ELLIPSOID["E",6378137,298.257223563]]],` +
		`CONVERSION["c",METHOD["Transverse Mercator"],` +
		`PARAMETER["Longitude of natural origin",117,ANGLEUNIT["x",-1]]],CS[Cartesian,2]]`
	if _, err := Parse(badUnit); err == nil {
		t.Error("参数里的非法 ANGLEUNIT 应报错")
	}

	// 无名 PARAMETER(第一个参数不是字符串)→ 跳过,其余照常解析
	nameless := `PROJCRS["x",BASEGEOGCRS["W",DATUM["D",ELLIPSOID["E",6378137,298.257223563]]],` +
		`CONVERSION["c",METHOD["Transverse Mercator"],PARAMETER[1,2],` +
		`PARAMETER["Longitude of natural origin",117,ANGLEUNIT["degree",0.0174532925199433]]],CS[Cartesian,2]]`
	c2, err := Parse(nameless)
	if err != nil {
		t.Fatalf("无名 PARAMETER 应被跳过: %v", err)
	}
	if c2.Proj != "tmerc" || c2.Params["lon_0"] != 117 {
		t.Errorf("跳过无名参数后应正常解析, got proj=%q lon_0=%v", c2.Proj, c2.Params["lon_0"])
	}

	// 参数级 LENGTHUNIT 缺系数 → 报错(不能当米算)
	badLen := `PROJCRS["x",BASEGEOGCRS["W",DATUM["D",ELLIPSOID["E",6378137,298.257223563]]],` +
		`CONVERSION["c",METHOD["Transverse Mercator"],` +
		`PARAMETER["False easting",500000,LENGTHUNIT["metre"]]],CS[Cartesian,2]]`
	if _, err := Parse(badLen); err == nil || !strings.Contains(err.Error(), "LENGTHUNIT 缺换算系数") {
		t.Errorf("参数级 LENGTHUNIT 缺系数应报错, got %v", err)
	}
}

// TestNilGuards 内部取值助手对 nil CRS 的守卫(公开 API 不会传 nil,但它们是
// 包内共用的工具函数,不该 panic)。
func TestNilGuards(t *testing.T) {
	var nilCRS *CRS
	if _, ok := paramFloat(nilCRS, "lat_0", 7); ok {
		t.Error("paramFloat(nil) 应回 (def,false)")
	}
	if got, _ := paramFloat(nilCRS, "lat_0", 7); got != 7 {
		t.Errorf("paramFloat(nil) 应回默认值 7, got %v", got)
	}
	if got := paramString(nilCRS, "units", "m"); got != "m" {
		t.Errorf("paramString(nil) 应回默认值, got %q", got)
	}
	if hasFlag(nilCRS, "south") {
		t.Error("hasFlag(nil) 应为 false")
	}
	// 空 Params/UnknownParams 的 CRS(零值)同样安全
	zero := &CRS{}
	if _, ok := paramFloat(zero, "lat_0", 0); ok {
		t.Error("空 Params 应回 false")
	}
	if got := paramString(zero, "units", "m"); got != "m" {
		t.Error("空 UnknownParams 应回默认值")
	}
	if hasFlag(zero, "south") {
		t.Error("空 UnknownParams 的 hasFlag 应为 false")
	}
}

// TestMat3InverseSingular 奇异矩阵返回 ok=false(调用方走兜底)。
func TestMat3InverseSingular(t *testing.T) {
	var zero [3][3]float64
	if _, ok := mat3Inverse(zero); ok {
		t.Error("零矩阵应不可逆")
	}
	// 秩 1 矩阵(两行成比例)也不可逆
	m := [3][3]float64{{1, 2, 3}, {2, 4, 6}, {1, 1, 1}}
	if _, ok := mat3Inverse(m); ok {
		t.Error("秩 1 矩阵应不可逆")
	}
	// 可逆矩阵:验证 M·M⁻¹ = I
	id := [3][3]float64{{2, 0, 1}, {1, 3, 2}, {1, 0, 0}}
	inv, ok := mat3Inverse(id)
	if !ok {
		t.Fatal("可逆矩阵被误判")
	}
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			var sum float64
			for k := 0; k < 3; k++ {
				sum += id[i][k] * inv[k][j]
			}
			want := 0.0
			if i == j {
				want = 1
			}
			if math.Abs(sum-want) > 1e-12 {
				t.Errorf("M·M⁻¹[%d][%d] = %v, want %v", i, j, sum, want)
			}
		}
	}
}

// TestDomainErrorText DomainError 的三条文案分支(含 nil 接收者)。
func TestDomainErrorText(t *testing.T) {
	var nilErr *DomainError
	if got := nilErr.Error(); got != ErrDomain.Error() {
		t.Errorf("nil DomainError 应回 sentinel 文案, got %q", got)
	}
	short := domainErr("far-pole", "")
	if got, want := short.Error(), ErrDomain.Error()+" [far-pole]"; got != want {
		t.Errorf("无 Detail 的文案 = %q, want %q", got, want)
	}
	long := domainErr("ce-limit", "|Ce|=3")
	if got := long.Error(); !strings.Contains(got, "ce-limit") || !strings.Contains(got, "|Ce|=3") {
		t.Errorf("有 Detail 时文案应含原因与细节, got %q", got)
	}
	if !errors.Is(long, ErrDomain) {
		t.Error("DomainError 应 Unwrap 到 ErrDomain")
	}
}

// TestEllipsoidInvalidAxes 长/短半轴的合法性守卫(绕过 Parse 的数值域检查)。
func TestEllipsoidInvalidAxes(t *testing.T) {
	for _, c := range []struct {
		a, b float64
	}{{0, 1}, {-1, 1}, {math.NaN(), 1}, {1, 0}, {1, -1}, {1, math.NaN()}, {1, 2}} {
		if _, err := newEllipsoid(c.a, c.b); err == nil {
			t.Errorf("newEllipsoid(%v,%v) 应报错", c.a, c.b)
		}
	}
	// +f=0 走正球体分支
	e, err := fFromFlat(6378137, 0)
	if err != nil || !e.Sphere {
		t.Errorf("fFromFlat(a,0) 应为正球: %+v %v", e, err)
	}
}

// TestProjectionDomainEdges 各投影适配层的域外返回:错误必须是 ErrDomain、
// 坐标必须是 NaN,且 DomainError 的原因要对。
func TestProjectionDomainEdges(t *testing.T) {
	cases := []struct {
		name    string
		crs     string
		inverse bool
		x, y    float64
		reason  string
	}{
		{"stere 斜轴对跖点", "+proj=stere +lat_0=0 +lon_0=0 +ellps=WGS84", false, 180, 0, "antipode"},
		{"stere 极轴对跖点", "+proj=stere +lat_0=90 +lat_ts=60 +lon_0=0 +ellps=WGS84", false, 0, -90, "antipode"},
		{"laea 赤道对跖点", "+proj=laea +lat_0=0 +lon_0=0 +ellps=WGS84", false, 180, 0, "antipode"},
		{"laea 反变换超远", "+proj=laea +lat_0=0 +lon_0=0 +ellps=WGS84", true, 1e9, 0, "outside"},
		{"merc 反变换超远", "+proj=merc +ellps=WGS84", true, 1e12, 1e12, "outside"},
		{"tmerc 反变换超远", "+proj=tmerc +lon_0=0 +ellps=WGS84", true, 1e12, 1e12, "ce-limit"},
		// 球面 tmerc 反变换:h = exp(x/a/esp) 下溢成 0
		// 球面 tmerc 的域外原因统一是 degenerate(|cosφ·sinΔλ|=1 那条边)
		{"球面 tmerc 反变换下溢", "+proj=tmerc +a=6371000 +b=6371000 +lon_0=0", true, -1e10, 0, "degenerate"},
		// laea 反变换:|asin 参数| > 1(斜轴与极轴各一条)
		{"laea 斜轴反变换超远", "+proj=laea +lat_0=45 +lon_0=10 +ellps=WGS84", true, 1e9, 1e9, "outside"},
		{"laea 极轴反变换超远", "+proj=laea +lat_0=90 +lon_0=0 +ellps=WGS84", true, 1e9, 1e9, "outside"},
	}
	for _, c := range cases {
		p := mustProj(t, c.crs)
		var err error
		if c.inverse {
			_, _, err = p.Inverse(c.x, c.y)
		} else {
			_, _, err = p.Forward(c.x, c.y)
		}
		if !errors.Is(err, ErrDomain) {
			t.Errorf("%s: 应 ErrDomain, got %v", c.name, err)
			continue
		}
		var de *DomainError
		if !errors.As(err, &de) || de.Reason != c.reason {
			t.Errorf("%s: 原因应为 %q, got %v", c.name, c.reason, err)
		}
	}
}

// TestLccTangentAndApex lcc 的两种边角:
//   - 只给 +lat_ts(单标准纬线)按切锥处理,结果应与 lat_1=lat_2=lat_ts 等价;
//   - 反变换落在圆锥顶点(ρ=0)时给出 (lon_0, ±90)。
func TestLccTangentAndApex(t *testing.T) {
	tangent := mustProj(t, "+proj=lcc +lat_ts=45 +lon_0=0 +ellps=WGS84")
	explicit := mustProj(t, "+proj=lcc +lat_1=45 +lat_2=45 +lon_0=0 +ellps=WGS84")
	x1, y1, err := tangent.Forward(10, 50)
	if err != nil {
		t.Fatalf("切锥 Forward: %v", err)
	}
	x2, y2, _ := explicit.Forward(10, 50)
	if math.Abs(x1-x2) > 1e-9 || math.Abs(y1-y2) > 1e-9 {
		t.Errorf("只给 lat_ts 应与 lat_1=lat_2 等价: (%.6f,%.6f) vs (%.6f,%.6f)", x1, y1, x2, y2)
	}

	// 顶点:直接喂内核的 (0, rho0)
	k := newLccKernel(6378137, 6378137*(1-1/298.257223563), 0, 0, geoDegToRad(30), geoDegToRad(60), 1)
	lon, lat, ok := k.inverse(0, k.rho0)
	if !ok || math.Abs(lon) > 1e-12 || math.Abs(math.Abs(lat)-math.Pi/2) > 1e-12 {
		t.Errorf("顶点反变换 = (%.6f, %.6f, %v), want (0, ±π/2)", lon, lat, ok)
	}
}

// TestKernelEllipsoidClamp 内核构造器对 b>a 的处理:适配层会拒收(见
// TestConstructorGuards),内核自己把 e² 夹到 0,不该产生 NaN。
func TestKernelEllipsoidClamp(t *testing.T) {
	const a, b = 6378137.0, 7000000.0 // b > a
	check := func(name string, f func() (float64, float64)) {
		x, y := f()
		if math.IsNaN(x) || math.IsNaN(y) {
			t.Errorf("%s: b>a 时内核应夹 e²=0 并给出有限值, got (NaN,NaN)", name)
		}
	}
	check("merc", func() (float64, float64) { return newMercKernel(a, b, 0, 0).forward(0.1, 0.2) })
	check("laea", func() (float64, float64) { return newLaeaKernel(a, b, 0.5, 0).forward(0.1, 0.2) })
	check("lcc", func() (float64, float64) {
		return newLccKernel(a, b, 0, 0, 0.5, 1.0, 1).forward(0.1, 0.2)
	})
	check("tmerc", func() (float64, float64) { return newTmercKernel(a, b, 0, 0, 1).forward(0.1, 0.2) })
	check("stere", func() (float64, float64) { return newStereKernel(a, b, math.Pi/2, 0, 1).forward(0.1, 0.2) })
}

// TestConstructorGuards 适配层构造期的第二道守卫(Parse 通常已先拦下)。
func TestConstructorGuards(t *testing.T) {
	wgs := wgs84Ellipsoid
	mk := func(proj string, p map[string]float64) *CRS {
		return &CRS{Proj: proj, Params: p, Ellipsoid: wgs, UnknownParams: map[string]string{}}
	}
	if _, err := newAeqd(nil); err == nil {
		t.Error("newAeqd(nil) 应报错")
	}
	if _, err := newUtm(nil); err == nil {
		t.Error("newUtm(nil) 应报错")
	}
	if _, err := newAeqd(mk("aeqd", map[string]float64{"lat_0": 95})); err == nil {
		t.Error("aeqd lat_0=95 应报错")
	}
	if _, err := newLaea(mk("laea", map[string]float64{"lat_0": 95})); err == nil {
		t.Error("laea lat_0=95 应报错")
	}
	// utm 的派生 tmerc 失败时错误要透出(椭球 b>a)
	bad := &CRS{Proj: "utm", Params: map[string]float64{"zone": 50},
		Ellipsoid: Ellipsoid{A: 1, B: 2}, UnknownParams: map[string]string{}}
	if _, err := newUtm(bad); err == nil {
		t.Error("utm 派生 tmerc 失败时应透出错误")
	}
	// CRS() getter(新投影)
	ap := mustProj(t, "+proj=aeqd +lat_0=45 +lon_0=10 +ellps=WGS84")
	if ap.CRS() == nil || ap.CRS().Proj != "aeqd" {
		t.Error("aeqd CRS() 应回传绑定 CRS")
	}
	up := mustProj(t, "+proj=utm +zone=50 +ellps=WGS84")
	if up.CRS() == nil || up.CRS().Proj != "utm" {
		t.Error("utm CRS() 应回传绑定 CRS")
	}
}

// stubProj 测试用的假 Projection,只为打 Info 的防御分支。
type stubProj struct {
	crs *CRS
}

func (s stubProj) CRS() *CRS { return s.crs }
func (s stubProj) Forward(lon, lat float64) (float64, float64, error) {
	return lon, lat, nil
}
func (s stubProj) Inverse(x, y float64) (float64, float64, error) { return x, y, nil }

// TestInfoDefensiveBranches Info 的两条防御分支:CRS() 为 nil、proj 名不在表里。
func TestInfoDefensiveBranches(t *testing.T) {
	if _, err := Info(stubProj{crs: nil}); err == nil {
		t.Error("CRS() 为 nil 时应报错")
	}
	if _, err := Info(stubProj{crs: &CRS{Proj: "not-a-proj"}}); err == nil {
		t.Error("未知投影名应报错")
	}
}

// TestHelmertSingularFallback 尺度退化成 0(矩阵奇异)时走兜底除法,不 panic。
func TestHelmertSingularFallback(t *testing.T) {
	h := &helmert{s: 0}
	x, y, z := h.inverse(1, 2, 3)
	if !math.IsInf(x, 1) || !math.IsInf(y, 1) || !math.IsInf(z, 1) {
		t.Errorf("奇异矩阵兜底应得 +Inf, got (%v,%v,%v)", x, y, z)
	}
}

// TestTransformerErrorSides 构造期/执行期的两侧报错与批量非域外错误。
func TestTransformerErrorSides(t *testing.T) {
	wgs, err := Parse("EPSG:4326")
	if err != nil {
		t.Fatal(err)
	}
	badSrc, err := Parse("+proj=utm +ellps=WGS84") // 缺 zone
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewTransformer(badSrc, wgs); err == nil {
		t.Error("源 CRS 建不出投影时应报错")
	} else if !strings.Contains(err.Error(), "源 CRS") {
		t.Errorf("报错应指出源侧: %v", err)
	}

	tr, err := NewTransformer(wgs, wgs)
	if err != nil {
		t.Fatal(err)
	}
	// 批量里的非域外错误(经度 400)整批失败
	if _, err := tr.TransformBatch([]float64{400, 0}); err == nil {
		t.Error("批量非域外错误应整批失败")
	}
	// InverseBatch 里的非域外错误(latlong 的 x=400)
	pw, err := wgs.NewProjection()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := InverseBatch(pw, []float64{400, 0}); err == nil {
		t.Error("InverseBatch 非域外错误应报错")
	}
}

// TestParseNumberEdge 数值解析的边角。
func TestParseNumberEdge(t *testing.T) {
	if _, err := Parse("+proj=merc +lat_0=abc"); err == nil ||
		!strings.Contains(err.Error(), "不是有效数字") {
		t.Errorf("+lat_0=abc 应报「不是有效数字」, got %v", err)
	}
	// 零值 CRS 的 String() 回空串(projString 为空且不是 EPSG 码)
	var zero CRS
	if got := zero.String(); got != "" {
		t.Errorf("零值 CRS 的 String() 应为空, got %q", got)
	}
}

// TestSphericalTmercEdges 球面 tmerc 的三条边角:cosφ=1 时 |Δλ|>90° 给 y=π、
// 南半球取反、以及 b≥1 时判域外。
func TestSphericalTmercEdges(t *testing.T) {
	p := mustProj(t, "+proj=tmerc +a=6371000 +b=6371000 +lon_0=0")
	// 赤道、|Δλ|=100° > 90° → y = π(a 归一)
	x, y, err := p.Forward(100, 0)
	if err != nil {
		t.Fatalf("Forward(100,0): %v", err)
	}
	if math.Abs(x) < 1e6 || math.Abs(y-math.Pi*6371000) > 1 {
		t.Errorf("赤道 |Δλ|>90° 应给 y=πa, got (%.6f, %.6f)", x, y)
	}
	// 赤道、Δλ=0 → y=0
	if _, y0, err := p.Forward(0, 0); err != nil || y0 != 0 {
		t.Errorf("赤道 Δλ=0 应给 y=0, got (%v, %v)", y0, err)
	}
	// 南半球 → y 取反
	_, yS, err := p.Forward(10, -20)
	if err != nil {
		t.Fatalf("Forward(10,-20): %v", err)
	}
	if yS >= 0 {
		t.Errorf("南半球 y 应为负, got %v", yS)
	}
	// 域界:|b| → 1(Δλ 逼近 ±90°)时报域外
	if _, _, err := p.Forward(90, 0); !errors.Is(err, ErrDomain) {
		t.Errorf("|Δλ|=90° 应 ErrDomain, got %v", err)
	}
}

// TestSphericalStere 球体极射赤面的四个方面。
//
// 这里补的是 iterateGeodetic 的 e==0 分支:原来它直接返回 phiL,而极轴分支的
// phiL = π/2 − 2·atan(tp) 是"镜像"纬度 —— 球体反变换会给出 180°−φ
// (lat=60 回来变 120)。椭球走迭代式所以看不出来,golden/sweep 又只有椭球极轴,
// 这个 bug 一直到 2026-10-22 补球体用例才暴露。
// 参照值 = pyproj 3.8.0 / PROJ 9.8.1 实测。
func TestSphericalStere(t *testing.T) {
	cases := []struct {
		name string
		crs  string
		lon  float64
		lat  float64
		x, y float64
	}{
		{"球体 北极", "+proj=stere +lat_0=90 +R=6371000", 30, 60, 1707104.304979, -2956791.390043},
		{"球体 南极", "+proj=stere +lat_0=-90 +R=6371000", 30, 60, 23776895.695021, 41182791.390043},
		{"球体 斜轴", "+proj=stere +lat_0=45 +lon_0=10 +R=6371000", 30, 60, 1120541.967166, 1835620.899119},
		{"球体 赤道", "+proj=stere +lat_0=0 +lon_0=0 +R=6371000", 30, 60, 2222939.123843, 7700487.009257},
	}
	for _, c := range cases {
		p := mustProj(t, c.crs)
		x, y, err := p.Forward(c.lon, c.lat)
		if err != nil {
			t.Fatalf("%s: Forward: %v", c.name, err)
		}
		if math.Abs(x-c.x) > 1e-6 || math.Abs(y-c.y) > 1e-6 {
			t.Errorf("%s: (%.0f,%.0f) → (%.6f, %.6f), want (%.6f, %.6f)", c.name, c.lon, c.lat, x, y, c.x, c.y)
		}
		lon, lat, err := p.Inverse(x, y)
		if err != nil || math.Abs(lon-c.lon) > 1e-9 || math.Abs(lat-c.lat) > 1e-9 {
			t.Errorf("%s: 往返 = (%.12f, %.12f, %v), want (%.0f, %.0f)", c.name, lon, lat, err, c.lon, c.lat)
		}
	}
}

// TestSouthPolarStere 南半球极射赤面(变体 B):lat_0=-90,经度用 atan2(x, y)。
func TestSouthPolarStere(t *testing.T) {
	p := mustProj(t, "+proj=stere +lat_0=-90 +lat_ts=-70 +lon_0=0 +ellps=WGS84")
	x, y, err := p.Forward(20, -60)
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	lon, lat, err := p.Inverse(x, y)
	if err != nil || math.Abs(lon-20) > 1e-9 || math.Abs(lat+60) > 1e-9 {
		t.Errorf("南半球极射往返 = (%.12f, %.12f, %v)", lon, lat, err)
	}
}

// TestLaeaKernelBadLat0 内核收到 |lat_0|>90 时置 mode=-1(适配层已先拒收)。
func TestLaeaKernelBadLat0(t *testing.T) {
	k := newLaeaKernel(6378137, 6378137*(1-1/298.257223563), 95*deg2rad, 0)
	if k.mode != -1 {
		t.Errorf("|lat_0|>90 的内核 mode 应为 -1, got %d", k.mode)
	}
}

// TestDomainReasonHelpers 两个原因判别助手的边角分支。
func TestDomainReasonHelpers(t *testing.T) {
	sp := &laeaKernel{mode: laeaSPole}
	if got := laeaDomainReason(sp, 10); got != "far-pole" {
		t.Errorf("南冠 + 北纬应 far-pole, got %q", got)
	}
	if got := lccDomainReason(10, 0.5); got != "outside" {
		t.Errorf("φ·n > 0 应 outside, got %q", got)
	}
}

// TestBadEllipsoidConstructors 各适配层的椭球校验(Parse 已先拦,这里打第二道)。
func TestBadEllipsoidConstructors(t *testing.T) {
	bad := &CRS{Proj: "x", Params: map[string]float64{}, Ellipsoid: Ellipsoid{A: 1, B: 2},
		UnknownParams: map[string]string{}}
	if _, err := newStere(bad); err == nil {
		t.Error("stere 应拒收 b>a")
	}
	if _, err := newLaea(bad); err == nil {
		t.Error("laea 应拒收 b>a")
	}
	if _, err := newLcc(bad); err == nil {
		t.Error("lcc 应拒收 b>a")
	}
	if _, err := newMerc(bad); err == nil {
		t.Error("merc 应拒收 b>a")
	}
	// tmerc 的 lat_0 越界守卫
	badLat := &CRS{Proj: "tmerc", Params: map[string]float64{"lat_0": 95},
		Ellipsoid: wgs84Ellipsoid, UnknownParams: map[string]string{}}
	if _, err := newTmerc(badLat); err == nil {
		t.Error("tmerc lat_0=95 应报错")
	}
	// lcc 的 |lat|>90 守卫
	badLcc := &CRS{Proj: "lcc", Params: map[string]float64{"lat_1": 95},
		Ellipsoid: wgs84Ellipsoid, UnknownParams: map[string]string{}}
	if _, err := newLcc(badLcc); err == nil {
		t.Error("lcc lat_1=95 应报错")
	}
}

// TestParseRadianSuffixEdges 'r' 后缀的边角:非角度键拒收、后缀里的数字非法。
func TestParseRadianSuffixEdges(t *testing.T) {
	if _, err := Parse("+proj=merc +k_0=1r"); err == nil {
		t.Error("+k_0 不接受 'r' 后缀")
	}
	if _, err := Parse("+proj=merc +lat_0=abcr"); err == nil ||
		!strings.Contains(err.Error(), "不是有效数字") {
		t.Errorf("+lat_0=abcr 应报数字非法, got %v", err)
	}
	// 正常弧度后缀:π/2 rad = 90°
	crs, err := Parse("+proj=merc +lat_0=1.5707963267948966r +ellps=WGS84")
	if err != nil {
		t.Fatalf("弧度后缀: %v", err)
	}
	if math.Abs(crs.Params["lat_0"]-90) > 1e-9 {
		t.Errorf("π/2 rad 应换算出 90°, got %v", crs.Params["lat_0"])
	}
}

// TestAstroidZeroY astroid 在 y==0、|x|≤1 时直接返回 0(四次方程的退化根)。
func TestAstroidZeroY(t *testing.T) {
	if got := geodAstroid(0.5, 0); got != 0 {
		t.Errorf("astroid(0.5, 0) 应为 0, got %v", got)
	}
	// 常规点:正根
	if got := geodAstroid(2, 1); !(got > 0) {
		t.Errorf("astroid(2,1) 应为正根, got %v", got)
	}
}
