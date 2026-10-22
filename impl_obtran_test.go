package proj

// impl_obtran_test.go — ob_tran（旋转极点）与 pyproj 的对拍。
//
// 参照值全部由 pyproj 3.8.0 / PROJ 9.8.1 实测得到（2026-10-22）：
//   Transformer.from_crs(CRS.from_proj4(ob_tran 串), EPSG:4326, always_xy=True)
//   .transform(rotatedLon, rotatedLat)  →  (geographicLon, geographicLat)
// 所以下面的 x/y 字段是**未旋转系**的经纬度（度），不是米。

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const obTranTol = 1e-11

type obTranCase struct {
	crs string
	pts []struct{ lon, lat, x, y float64 }
}

var obTranRef = map[string]obTranCase{
	// 北极点 30°N，极点经度 0。注意旋转北极（rotLat=90）落到 (180, 30)。
	"np30": {
		crs: "+proj=ob_tran +o_proj=longlat +o_lat_p=30 +lon_0=0 +ellps=WGS84",
		pts: []struct{ lon, lat, x, y float64 }{
			{0, 0, 0.0, 60.00000000000001},
			{10, 20, 44.420564925534535, 76.5182767751997},
			{-30, -60, -14.50247051634759, -3.3257502125432117},
			{0, 90, 180.0, 30.000000000000004},
			{90, 0, 90.0, 3.0383242517508457e-15},
			{180, 0, 180.0, -60.00000000000001},
			{0, -90, 0.0, -29.999999999999993},
			{45, 80, 171.18154495571787, 36.77979826752316},
		},
	},
	// 绕旋转极轴再转 40°：等价于先把旋转经度平移 −40° 再套上面的极点旋转。
	"np30_lonp40": {
		crs: "+proj=ob_tran +o_proj=longlat +o_lat_p=30 +o_lon_p=40 +lon_0=0 +ellps=WGS84",
		pts: []struct{ lon, lat, x, y float64 }{
			{0, 0, -59.21026697116786, 41.56076257015933},
			{10, 20, -76.74230673519315, 61.137368271806594},
			{-30, -60, -29.351227252786305, -16.553686237663754},
			{0, 90, -180.0, 30.000000000000004},
		},
	},
	// ECMWF 业务产品的极点（grid_north_pole = 39.25N, 162W）：
	// 旋转北极必须正好落在 (−162, 39.25) —— 这条是 GRIB2 模板 3.1 的验收点。
	"ecmwf": {
		crs: "+proj=ob_tran +o_proj=longlat +o_lat_p=39.25 +lon_0=18 +ellps=WGS84",
		pts: []struct{ lon, lat, x, y float64 }{
			{0, 0, 18.0, 50.75},
			{10, 20, 44.970549018927045, 68.91277174593492},
			{-30, -60, 3.1760513365316587, -12.275761513700756},
			{0, 90, -162.0, 39.25},
		},
	},
	// 南半球极点（o_lat_p 取负）+ 跨日界线极点经度。
	"sp30": {
		crs: "+proj=ob_tran +o_proj=longlat +o_lat_p=-30 +lon_0=-170 +ellps=WGS84",
		pts: []struct{ lon, lat, x, y float64 }{
			{0, 0, 9.99999999999999, 60.00000000000001},
			{10, 20, -2.1346761491518427, 39.081425761316765},
			{0, 90, 9.99999999999999, -29.999999999999993},
			{0, -90, -170.0, 30.000000000000004},
		},
	},
	// lon_0 与 o_lon_p 是可加的：170 + (−59.21) = 110.79。
	"np30_lon170": {
		crs: "+proj=ob_tran +o_proj=longlat +o_lat_p=30 +o_lon_p=40 +lon_0=170 +ellps=WGS84",
		pts: []struct{ lon, lat, x, y float64 }{
			{0, 0, 110.78973302883215, 41.56076257015933},
			{10, 20, 93.25769326480686, 61.137368271806594},
		},
	},
}

func TestObTranParity(t *testing.T) {
	for name, c := range obTranRef {
		p := mustProj(t, c.crs)
		for _, pt := range c.pts {
			x, y, err := p.Forward(pt.lon, pt.lat)
			if err != nil {
				t.Fatalf("%s Forward(%v,%v): %v", name, pt.lon, pt.lat, err)
			}
			// 经度按角度差比:本库的 normLonDeg 归一到 [−180,180),所以反子午线
			// 上会给出 −180 而 pyproj 给 +180 —— 同一个位置,不是误差。
			if angDiffDeg(x, pt.x) > obTranTol || math.Abs(y-pt.y) > obTranTol {
				t.Errorf("%s Forward(%v,%v) = (%.15g, %.15g), want (%.15g, %.15g)",
					name, pt.lon, pt.lat, x, y, pt.x, pt.y)
			}
			// 往返：旋转系 → 未旋转系 → 旋转系。旋转极点(|lat|=90)上经度退化
			// (所有经线汇于一点),只比纬度。
			lon, lat, err := p.Inverse(x, y)
			if err != nil {
				t.Fatalf("%s Inverse: %v", name, err)
			}
			if math.Abs(lat-pt.lat) > obTranTol {
				t.Errorf("%s 往返纬度(%v,%v) = %.15g", name, pt.lon, pt.lat, lat)
			}
			if math.Abs(pt.lat) < 90 && angDiffDeg(lon, pt.lon) > obTranTol {
				t.Errorf("%s 往返经度(%v,%v) = %.15g", name, pt.lon, pt.lat, lon)
			}
		}
	}
}

// TestObTranRoundTripGrid 全域网格往返：旋转是正交变换，除极点经度退化外
// 必须精确可逆（含跨日界线、高纬、南北半球）。
func TestObTranRoundTripGrid(t *testing.T) {
	cases := []string{
		"+proj=ob_tran +o_proj=longlat +o_lat_p=30 +lon_0=0 +ellps=WGS84",
		"+proj=ob_tran +o_proj=longlat +o_lat_p=-45 +o_lon_p=25 +lon_0=-170 +ellps=WGS84",
		"+proj=ob_tran +o_proj=longlat +o_lat_p=90 +lon_0=0 +ellps=WGS84", // 极点极点:退化成平移
	}
	for _, src := range cases {
		p := mustProj(t, src)
		maxDLon, maxDLat := 0.0, 0.0
		for lon := -180.0; lon <= 180; lon += 15 {
			for lat := -85.0; lat <= 85; lat += 5 {
				x, y, err := p.Forward(lon, lat)
				if err != nil {
					t.Fatalf("%s Forward(%v,%v): %v", src, lon, lat, err)
				}
				lon2, lat2, err := p.Inverse(x, y)
				if err != nil {
					t.Fatalf("%s Inverse: %v", src, err)
				}
				if d := math.Abs(angDiffDeg(lon2, lon)); d > maxDLon {
					maxDLon = d
				}
				if d := math.Abs(lat2 - lat); d > maxDLat {
					maxDLat = d
				}
			}
		}
		if maxDLon > 1e-9 || maxDLat > 1e-9 {
			t.Errorf("%s 网格往返漂移 Δlon=%.3e Δlat=%.3e 度", src, maxDLon, maxDLat)
		}
	}
}

// TestObTranPoleIdentity +o_lat_p=90 是纯经度平移：φ 不变、λ 平移 lon_0。
func TestObTranPoleIdentity(t *testing.T) {
	p := mustProj(t, "+proj=ob_tran +o_proj=longlat +o_lat_p=90 +lon_0=30 +o_lon_p=10 +ellps=WGS84")
	x, y, err := p.Forward(20, 40)
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	// λ = lon_0 + (λr − o_lon_p) = 30 + 10 = 40；φ 不变
	if math.Abs(x-40) > 1e-12 || math.Abs(y-40) > 1e-12 {
		t.Errorf("极点平移 = (%.12f, %.12f), want (40, 40)", x, y)
	}
}

// TestObTranGRIB31Mapping GRIB2 模板 3.1 的"南极点"(LaSP, LoSP) → PROJ 串：
//
//	+o_lat_p = −LaSP      （北极纬度）
//	+lon_0   =  LoSP      （南极点经度，也就是北极经度 + 180）
//
// 用 ECMWF 的业务极点验证：grid_north_pole = (39.25N, 162W) ⇒ 南极点
// (−39.25, 18) ⇒ o_lat_p=39.25、lon_0=18。此时旋转北极必须正好落在
// (−162, 39.25)。注意 lon_0 **不是** LoSP+180（那是北极经度，写成它会把整幅
// 网格沿经线错 180°——静默错位，正是这个投影最容易踩的坑）。
func TestObTranGRIB31Mapping(t *testing.T) {
	fromSouth := mustProj(t, "+proj=ob_tran +o_proj=longlat +o_lat_p=39.25 +lon_0=18 +ellps=WGS84")
	for _, pt := range [][2]float64{{0, 0}, {10, 20}, {-30, -60}, {120, 45}} {
		x, y, err := fromSouth.Forward(pt[0], pt[1])
		if err != nil {
			t.Fatalf("Forward: %v", err)
		}
		// 旋转北极落在 ECMWF 的 (39.25N, 162W)
		if pt[0] == 0 && pt[1] == 0 {
			if math.Abs(x-18) > 1e-12 || math.Abs(y-50.75) > 1e-12 {
				t.Errorf("旋转原点 = (%.12f, %.12f), want (18, 50.75)", x, y)
			}
		}
	}
	// 旋转北极本身（rlat=90）必须正好是极点位置
	x, y, _ := fromSouth.Forward(0, 90)
	if math.Abs(x-(-162)) > 1e-9 || math.Abs(y-39.25) > 1e-9 {
		t.Errorf("旋转北极 = (%.9f, %.9f), want (-162, 39.25)", x, y)
	}
}

// TestObTranValidation 参数校验（都落在 NewProjection,与 utm 的 +zone 同口径）。
func TestObTranValidation(t *testing.T) {
	bad := []struct{ src, wantSub string }{
		{"+proj=ob_tran +ellps=WGS84", "需要 +o_proj"},
		{"+proj=ob_tran +o_proj=longlat +ellps=WGS84", "需要 +o_lat_p"},
		{"+proj=ob_tran +o_proj=longlat +o_lat_p=95 +ellps=WGS84", "出界 [-90,90]"},
		{"+proj=ob_tran +o_proj=longlat +o_lat_p=-91 +ellps=WGS84", "出界 [-90,90]"},
		{"+proj=ob_tran +o_proj=merc +o_lat_p=30 +ellps=WGS84", "只支持 +o_proj=longlat"},
		{"+proj=ob_tran +o_proj=nosuch +o_lat_p=30 +ellps=WGS84", "只支持 +o_proj=longlat"},
	}
	// 数值域(+o_lat_p 越界)在 Parse 就拦下(与 +lat_0 同口径),投影级校验
	// (缺 o_proj / 缺 o_lat_p / 内层不是 longlat)落在 NewProjection。两处拦都算过。
	for _, c := range bad {
		crs, err := Parse(c.src)
		if err != nil {
			if !strings.Contains(err.Error(), c.wantSub) {
				t.Errorf("Parse(%q) 报错应含 %q, got %v", c.src, c.wantSub, err)
			}
			continue
		}
		if _, err := crs.NewProjection(); err == nil {
			t.Errorf("NewProjection(%q) 应报错", c.src)
		} else if !strings.Contains(err.Error(), c.wantSub) {
			t.Errorf("NewProjection(%q) 报错应含 %q, got %v", c.src, c.wantSub, err)
		}
	}

	// 'r' 弧度后缀对角度参数生效(与 +lat_0 同口径):π/6 rad = 30°
	crs, err := Parse("+proj=ob_tran +o_proj=longlat +o_lat_p=0.5235987755982988r +ellps=WGS84")
	if err != nil {
		t.Fatalf("弧度后缀: %v", err)
	}
	if got := crs.Params["o_lat_p"]; math.Abs(got-30) > 1e-12 {
		t.Errorf("π/6 rad 应换算出 30°, got %v", got)
	}

	// PROJ 的另一套参数化本库不实现 → Parse 阶段就拒收（否则会静默算错）
	for _, key := range []string{"o_alpha", "o_lat_c", "o_lon_c", "o_lon_w", "o_lat_1", "o_lon_1", "o_lat_2", "o_lon_2"} {
		src := "+proj=ob_tran +o_proj=longlat +o_lat_p=30 +" + key + "=10 +ellps=WGS84"
		if _, err := Parse(src); err == nil {
			t.Errorf("Parse(%q) 应拒收未实现的参数化", src)
		} else if !strings.Contains(err.Error(), key) {
			t.Errorf("报错应点名 %s, got %v", key, err)
		}
	}
}

// TestObTranAngularOutput 输出是度:Units()=="degree"、+units 不生效、
// IsGeographic() 仍为 false(旋转系不是地理坐标系)。
func TestObTranAngularOutput(t *testing.T) {
	crs, err := Parse("+proj=ob_tran +o_proj=longlat +o_lat_p=30 +units=km +ellps=WGS84")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := crs.Units(); got != "degree" {
		t.Errorf("Units() = %q, want degree", got)
	}
	if crs.IsGeographic() {
		t.Error("ob_tran 不该算地理坐标系")
	}
	p, err := crs.NewProjection()
	if err != nil {
		t.Fatalf("NewProjection: %v", err)
	}
	// pyproj 实测 +units=km 对 ob_tran+longlat 无影响:(0,0) → (0, 60)
	x, y, err := p.Forward(0, 0)
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	if math.Abs(x-0) > 1e-12 || math.Abs(y-60) > 1e-12 {
		t.Errorf("+units=km 不该影响角度输出, got (%.12f, %.12f)", x, y)
	}
	// 没有 EPSG 码(不该被特征识别成 4326 —— 输出虽是度,但坐标系不同)
	if got := crs.EPSG(); got != 0 {
		t.Errorf("EPSG() = %d, want 0", got)
	}
	// String() 原样回放,再解析行为一致
	crs2, err := Parse(crs.String())
	if err != nil {
		t.Fatalf("String() 往返解析失败: %v", err)
	}
	if !crs.Equals(crs2) {
		t.Errorf("String() 往返后 Equals 失败: %q vs %q", crs.String(), crs2.String())
	}
	// Info 元数据走表里的条目
	info, err := Info(p)
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Name != "ob_tran" || info.Units != "度 → 度(旋转系经纬 → 未旋转系经纬)" || !info.HasInverse {
		t.Errorf("Info = %+v", info)
	}
}

// TestObTranInputGuards 输入域校验与 latlong 同口径(§2.5)。
func TestObTranInputGuards(t *testing.T) {
	p := mustProj(t, "+proj=ob_tran +o_proj=longlat +o_lat_p=30 +ellps=WGS84")
	if _, _, err := p.Forward(400, 0); err == nil {
		t.Error("|lon|>360 应报错")
	}
	if _, _, err := p.Forward(0, 91); err == nil {
		t.Error("|lat|>90 应报错")
	}
	if _, _, err := p.Inverse(math.NaN(), 0); err == nil {
		t.Error("NaN 应报错")
	}
	// 输入经度允许 [−360,360] 的放宽区间,输出归一化到 [−180,180)
	x, _, err := p.Forward(350, 0)
	if err != nil {
		t.Fatalf("lon=350 应放行(§2.5): %v", err)
	}
	if x < -180 || x >= 180 {
		t.Errorf("输出经度未归一: %v", x)
	}
}

// TestObTranBatch 批量路径与单点一致(含 ErrDomain 语义:ob_tran 全域可解,
// 只有坏输入整批失败)。
func TestObTranBatch(t *testing.T) {
	p := mustProj(t, "+proj=ob_tran +o_proj=longlat +o_lat_p=30 +lon_0=0 +ellps=WGS84")
	in := []float64{0, 0, 10, 20, -30, -60}
	out, err := ForwardBatch(p, in)
	if err != nil {
		t.Fatalf("ForwardBatch: %v", err)
	}
	for i := 0; i < len(in); i += 2 {
		x, y, _ := p.Forward(in[i], in[i+1])
		if out[i] != x || out[i+1] != y {
			t.Errorf("批量[%d] = (%.15g,%.15g), 单点 (%.15g,%.15g)", i/2, out[i], out[i+1], x, y)
		}
	}
	if _, err := ForwardBatch(p, []float64{math.Inf(1), 0}); err == nil {
		t.Error("Inf 输入应整批失败")
	}
}

// ---- 大样本对拍(xcheck/obtran_ref.json,生成器 xcheck/gen_obtran.py)---------
//
// 手写表只有 5 组 × 几个点;这里再跑 7 组 × 上百点的结构化 + 种子随机点,
// 两个方向都验(Forward 吃 rotated、Inverse 吃 geographic),并把最大偏差打进日志。

// N.B. 一个字段只能有一个 json 名:写成 `json:"rlon,rlat"` 会被解析成
// 名字 "rlon" + 未知选项 "rlat",第二个字段永远收不到值(踩过一次)。
type obTranRotRec struct {
	RLon float64 `json:"rlon"`
	RLat float64 `json:"rlat"`
	Lon  float64 `json:"lon"`
	Lat  float64 `json:"lat"`
}

type obTranGeoRec struct {
	Lon  float64 `json:"lon"`
	Lat  float64 `json:"lat"`
	RLon float64 `json:"rlon"`
	RLat float64 `json:"rlat"`
}

type obTranRefCase struct {
	Name         string         `json:"name"`
	Source       string         `json:"source"`
	RotatedToGeo []obTranRotRec `json:"rotated_to_geo"`
	GeoToRotated []obTranGeoRec `json:"geo_to_rotated"`
}

type obTranRefFile struct {
	Generator string          `json:"generator"`
	Cases     []obTranRefCase `json:"cases"`
}

func TestObTranParitySweep(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(file), "xcheck", "obtran_ref.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read obtran_ref: %v", err)
	}
	var r obTranRefFile
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatalf("parse obtran_ref: %v", err)
	}
	if len(r.Cases) == 0 {
		t.Fatal("obtran_ref.json 里没有 case(重跑 python xcheck/gen_obtran.py)")
	}
	const tol = 1e-11
	for _, c := range r.Cases {
		p := mustProj(t, c.Source)
		maxFwd, maxInv := 0.0, 0.0
		for _, v := range c.RotatedToGeo {
			x, y, err := p.Forward(v.RLon, v.RLat)
			if err != nil {
				t.Fatalf("%s Forward(%v,%v): %v", c.Name, v.RLon, v.RLat, err)
			}
			d := math.Max(angDiffDeg(x, v.Lon), math.Abs(y-v.Lat))
			if d > maxFwd {
				maxFwd = d
			}
			if d > tol {
				t.Errorf("%s Forward(%v,%v) = (%.15g,%.15g), 参照 (%.15g,%.15g)",
					c.Name, v.RLon, v.RLat, x, y, v.Lon, v.Lat)
			}
		}
		for _, v := range c.GeoToRotated {
			lon, lat, err := p.Inverse(v.Lon, v.Lat)
			if err != nil {
				t.Fatalf("%s Inverse(%v,%v): %v", c.Name, v.Lon, v.Lat, err)
			}
			d := math.Max(angDiffDeg(lon, v.RLon), math.Abs(lat-v.RLat))
			if d > maxInv {
				maxInv = d
			}
			if d > tol {
				t.Errorf("%s Inverse(%v,%v) = (%.15g,%.15g), 参照 (%.15g,%.15g)",
					c.Name, v.Lon, v.Lat, lon, lat, v.RLon, v.RLat)
			}
		}
		t.Logf("%-13s %3d 正解 Δ≤%.2e / %3d 反解 Δ≤%.2e 度",
			c.Name, len(c.RotatedToGeo), maxFwd, len(c.GeoToRotated), maxInv)
	}
}

// TestObTranConstructorGuards 工厂的第二道守卫(Parse 通常已先拦下,这里
// 直接喂手工构造的 CRS,与 TestConstructorGuards 同口径)以及 CRS() getter。
func TestObTranConstructorGuards(t *testing.T) {
	if _, err := newObTran(nil); err == nil {
		t.Error("newObTran(nil) 应报错")
	}
	// +o_lat_p 越界:Parse 会先拒,这里绕过 Parse 直接构造
	bad := &CRS{
		Proj:          "ob_tran",
		Params:        map[string]float64{"o_lat_p": 95},
		Ellipsoid:     wgs84Ellipsoid,
		UnknownParams: map[string]string{"o_proj": "longlat"},
	}
	if _, err := newObTran(bad); err == nil {
		t.Error("o_lat_p=95 应报错")
	}
	p := mustProj(t, "+proj=ob_tran +o_proj=longlat +o_lat_p=30 +ellps=WGS84")
	if c := p.CRS(); c == nil || c.Proj != "ob_tran" {
		t.Error("CRS() 应回传绑定的 CRS")
	}
}

// TestObTranConcurrent shares one obTranProj instance between goroutines and
// requires Forward/Inverse to stay bit-identical to the serial result. ob_tran
// is not part of the sweep fixture (it has its own obtran_ref.json), so the
// generic TestThreadSafety cannot cover it — this test closes that gap for the
// 5 reference configurations in obTranRef (north/south pole, o_lon_p shift,
// ECMWF pole, shifted pole longitude).
func TestObTranConcurrent(t *testing.T) {
	const workers = 8
	for name, c := range obTranRef {
		p := mustProj(t, c.crs)
		type ref struct{ x, y, lon, lat float64 }
		refs := make([]ref, len(c.pts))
		for i, v := range c.pts {
			x, y, err := p.Forward(v.lon, v.lat)
			if err != nil {
				t.Fatalf("%s Forward(%v,%v): %v", name, v.lon, v.lat, err)
			}
			lon, lat, err := p.Inverse(x, y)
			if err != nil {
				t.Fatalf("%s Inverse(%v,%v): %v", name, x, y, err)
			}
			refs[i] = ref{x, y, lon, lat}
		}
		errs := make(chan error, workers)
		for w := 0; w < workers; w++ {
			go func() {
				for n := 0; n < 200; n++ {
					for i, v := range c.pts {
						x, y, err := p.Forward(v.lon, v.lat)
						if err != nil {
							errs <- fmt.Errorf("%s Forward(%v,%v): %w", name, v.lon, v.lat, err)
							return
						}
						if x != refs[i].x || y != refs[i].y {
							errs <- fmt.Errorf("%s Forward(%v,%v) 并发漂移", name, v.lon, v.lat)
							return
						}
						lon, lat, err := p.Inverse(refs[i].x, refs[i].y)
						if err != nil {
							errs <- fmt.Errorf("%s Inverse(%v,%v): %w", name, refs[i].x, refs[i].y, err)
							return
						}
						if lon != refs[i].lon || lat != refs[i].lat {
							errs <- fmt.Errorf("%s Inverse(%v,%v) 并发漂移", name, refs[i].x, refs[i].y)
							return
						}
					}
				}
				errs <- nil
			}()
		}
		for w := 0; w < workers; w++ {
			if err := <-errs; err != nil {
				t.Fatalf("%s: %v", name, err)
			}
		}
	}
}
