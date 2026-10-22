package proj

import (
	"errors"
	"math"
	"testing"
)

// impl_eqc_test.go — 参照值全部来自 probe7.py / probe8.py
// (pyproj 3.8.0 / PROJ 9.8.1 @ 2026-10-17),按 COORDINATION 数值门槛断言:
// forward ≤ 1e-6 m、inverse ≤ 1e-9 度。
// 参照点保留机器满精度 repr;断言用门值而非 bit 相等 —— pyproj 与本实现
// 的最后一次乘法次序差在 1e-10 m 量级,远低于门槛。

const (
	eqcFwdTol   = 1e-6 // m
	eqcInvTol   = 1e-9 // 度
	eqcRoundTol = 1e-9 // 度
)

// eqcAcc 偏差记账(probeReport 汇总打点)。
type eqcAcc struct {
	maxFwd float64
	maxInv float64
	maxRnd float64
}

func (a *eqcAcc) noteFwd(e float64) { a.maxFwd = math.Max(a.maxFwd, e) }
func (a *eqcAcc) noteInv(e float64) { a.maxInv = math.Max(a.maxInv, e) }
func (a *eqcAcc) noteRnd(e float64) { a.maxRnd = math.Max(a.maxRnd, e) }

// mustEqc 解析并构造 eqc 投影实例。
func mustEqc(t *testing.T, crs string) *eqcProj {
	t.Helper()
	c, err := Parse(crs)
	if err != nil {
		t.Fatalf("Parse(%q): %v", crs, err)
	}
	pr, err := c.NewProjection()
	if err != nil {
		t.Fatalf("NewProjection(%q): %v", crs, err)
	}
	e, ok := pr.(*eqcProj)
	if !ok {
		t.Fatalf("NewProjection(%q) 未产出 eqcProj", crs)
	}
	return e
}

// eqcCheck 对照单点 forward + (以参照 x/y 为输入的)inverse + 往返。
func eqcCheck(t *testing.T, name string, p *eqcProj, lon, lat, xW, yW, invLonW, invLatW float64, acc *eqcAcc) {
	t.Helper()
	x, y, err := p.Forward(lon, lat)
	if err != nil {
		t.Fatalf("%s Forward(%v,%v): %v", name, lon, lat, err)
	}
	eF := math.Abs(x-xW) + math.Abs(y-yW)
	acc.noteFwd(eF)
	if eF > eqcFwdTol {
		t.Errorf("%s Forward(%v,%v) = (%v,%v) want (%v,%v) 偏差 %g", name, lon, lat, x, y, xW, yW, eF)
	}
	gl, ga, err := p.Inverse(xW, yW)
	if err != nil {
		t.Fatalf("%s Inverse(%v,%v): %v", name, xW, yW, err)
	}
	eI := circDist(gl, invLonW) + math.Abs(ga-invLatW)
	acc.noteInv(eI)
	if eI > eqcInvTol {
		t.Errorf("%s Inverse(%v,%v) = (%v,%v) want (%v,%v) 偏差 %g", name, xW, yW, gl, ga, invLonW, invLatW, eI)
	}
	// 往返:Forward 输出再逆回(用本实现 x,短路参照 xW 的舍入差)
	gl2, ga2, err := p.Inverse(x, y)
	if err != nil {
		t.Fatalf("%s 往返 Inverse: %v", name, err)
	}
	eR := circDist(gl2, lon) + math.Abs(ga2-lat)
	acc.noteRnd(eR)
	if eR > eqcRoundTol {
		t.Errorf("%s 往返(%v,%v) → (%v,%v) 偏差 %g", name, lon, lat, gl2, ga2, eR)
	}
}

func probeReport(t *testing.T, name string, acc *eqcAcc) {
	t.Helper()
	t.Logf("%s 最大偏差: forward %.3g m(门槛 %.0e m)|inverse %.3g°|往返 %.3g°",
		name, acc.maxFwd, eqcFwdTol, acc.maxInv, acc.maxRnd)
	if acc.maxFwd > eqcFwdTol || acc.maxInv > eqcInvTol {
		t.Errorf("%s 触数值门槛", name)
	}
}

func TestEqcForwardGoldenCRS(t *testing.T) {
	// golden_eqc.json 的 CRS(lat_ts=0,lon_0=180,WGS84)
	p := mustEqc(t, "+proj=eqc +lat_ts=0 +lat_0=0 +lon_0=180 +x_0=0 +y_0=0 +ellps=WGS84 +units=m")
	var acc eqcAcc
	// probe7 主表
	for _, c := range []struct {
		lon, lat, x, y, invLon, invLat float64
	}{
		{0, 0, -20037508.342789244, 0.0, 0, 0},
		{30.5, 45.25, -16642263.8735944, 5012727.933035933, 30.50000000000001, 45.25},
		{-102.0, -89.0, 8682920.281875337, -9890271.86439852, -102.00000000000001, -88.99999999999999},
		{85.3, 89.999, -10541955.778123006, 10001854.035333164, 85.30000000000001, 89.999},
		{90.0, 90.0, -10018754.171394622, 10001965.729312722, 90.0, 89.99999999999999},
		{200.0, 10.0, 2226389.8158654687, 1105854.833234372, -160.00000000000003, 9.999999999999998},
	} {
		eqcCheck(t, "goldenCRS", p, c.lon, c.lat, c.x, c.y, c.invLon, c.invLat, &acc)
	}
	// probe8 Δλ 环绕边界
	for _, c := range []struct{ lon, x float64 }{
		{-180, 0.0}, {180, 0.0}, {-90, 10018754.171394622}, {90, -10018754.171394622},
		{270, 10018754.171394622}, {-270, -10018754.171394622}, {89.9999, -10018765.3033437},
		{-89.9999, 10018765.303343698},
	} {
		x, _, err := p.Forward(c.lon, 0)
		if err != nil {
			t.Fatalf("Forward(%v,0): %v", c.lon, err)
		}
		e := math.Abs(x - c.x)
		acc.noteFwd(e)
		if e > eqcFwdTol {
			t.Errorf("Δλ 环绕 Forward(%v) x=%v want %v 偏差 %g", c.lon, x, c.x, e)
		}
	}
	// probe8 纬向补值(lat_ts=0 → y 与 lon_0 无关)
	for _, c := range []struct{ lat, y float64 }{
		{90, 10001965.729312722}, {-90, -10001965.729312722},
		{85, 9443510.140666245}, {-85, -9443510.140666245}, {33.7, 3730385.5380052268},
	} {
		_, y, err := p.Forward(89.9, c.lat)
		if err != nil {
			t.Fatalf("Forward(89.9,%v): %v", c.lat, err)
		}
		e := math.Abs(y - c.y)
		acc.noteFwd(e)
		if e > eqcFwdTol {
			t.Errorf("纬向 Forward(89.9,%v) y=%v want %v 偏差 %g", c.lat, y, c.y, e)
		}
	}
	probeReport(t, "goldenCRS", &acc)
}

func TestEqcEllipsoidLatTs45(t *testing.T) {
	// lat_ts/lat_0 非零:同时锻炼 nu1·cosφ_ts 与 m0/inv_mlfn 全链路
	p := mustEqc(t, "+proj=eqc +lat_ts=45.25 +lat_0=10 +lon_0=0 +x_0=0 +y_0=0 +ellps=WGS84 +units=m")
	var acc eqcAcc
	for _, c := range []struct {
		lon, lat, x, y, invLon, invLat float64
	}{
		{0, 0, 0.0, -1105854.833234372, 0, 1.600995009688936e-15},
		{10, 30, 785032.0177484065, 2214258.5647060103, 9.999999999999998, 29.999999999999996},
		{-170, 85, -13345544.301722914, 8337655.307431873, -170.00000000000003, 85.0},
		{120, -60, 9420384.21298088, -7759927.652724883, 119.99999999999999, -59.99999999999999},
	} {
		eqcCheck(t, "lat_ts45", p, c.lon, c.lat, c.x, c.y, c.invLon, c.invLat, &acc)
	}
	probeReport(t, "lat_ts45", &acc)
}

func TestEqcSphere(t *testing.T) {
	// 球体(a=b;lat_ts≠0 走 s_forward:y = a·(φ−φ0))
	p := mustEqc(t, "+proj=eqc +lat_ts=45 +a=6378137 +b=6378137 +lon_0=0 +units=m")
	var acc eqcAcc
	for _, c := range []struct {
		lon, lat, x, y, invLon, invLat float64
	}{
		{30, 60, 2361443.0045447154, 6679169.447596414, 29.999999999999993, 59.99999999999999},
		{0, 0, 0.0, 0.0, 0, 0},
		{179.9, -85, 14160786.550586479, -9462156.717428254, 179.89999999999998, -85.0},
	} {
		eqcCheck(t, "sphere", p, c.lon, c.lat, c.x, c.y, c.invLon, c.invLat, &acc)
	}
	probeReport(t, "sphere", &acc)
}

func TestEqcK0Ignored(t *testing.T) {
	// PROJ 语义:eqc 忽略 k_0(probe8 实测 k_0=3 与无 k_0 全同)
	p1 := mustEqc(t, "+proj=eqc +lat_ts=0 +k_0=3 +a=6378137 +b=6356752.3 +lon_0=0")
	p2 := mustEqc(t, "+proj=eqc +lat_ts=0 +a=6378137 +b=6356752.3 +lon_0=0")
	for _, pt := range [][2]float64{{30, 45}, {-123.7, -22.2}} {
		x1, y1, err := p1.Forward(pt[0], pt[1])
		if err != nil {
			t.Fatalf("k_0=3 Forward: %v", err)
		}
		x2, y2, _ := p2.Forward(pt[0], pt[1])
		if x1 != x2 || y1 != y2 {
			t.Errorf("k_0 应被忽略: eqc(%v,%v) k_0=3 → (%v,%v),无 k_0 → (%v,%v)",
				pt[0], pt[1], x1, y1, x2, y2)
		}
	}
}

func TestEqcErrors(t *testing.T) {
	// |lat_ts| = 90 → rc=0 除零,PROJ 拒收
	for _, crs := range []string{
		"+proj=eqc +lat_ts=90 +ellps=WGS84",
		"+proj=eqc +lat_ts=-90 +ellps=WGS84",
	} {
		c, err := Parse(crs)
		if err != nil {
			t.Errorf("%q Parse 应成功, got %v", crs, err)
			continue
		}
		if _, err := c.NewProjection(); err == nil {
			t.Errorf("%q NewProjection 应报错(|lat_ts|=90)", crs)
		}
	}
	// 输入域错误(非 ErrDomain):经度/纬度出界、非有限
	p := mustEqc(t, "+proj=eqc +lat_ts=0 +lon_0=180 +ellps=WGS84")
	for _, pt := range [][2]float64{
		{360.1, 0}, {-360.1, 0}, {0, 90.1}, {0, -90.1},
		{math.NaN(), 0}, {0, math.NaN()}, {math.Inf(1), 0}, {0, math.Inf(-1)},
	} {
		_, _, err := p.Forward(pt[0], pt[1])
		if err == nil || errors.Is(err, ErrDomain) {
			t.Errorf("Forward(%v,%v) 应报非 ErrDomain 错误, got %v", pt[0], pt[1], err)
		}
	}
	// Inverse:非有限输入报错;有限输入无定义域限制(eqc 全平面可逆)
	if _, _, err := p.Inverse(math.NaN(), 0); err == nil {
		t.Errorf("Inverse(NaN,…) 应报错")
	}
	if _, _, err := p.Inverse(3e10, -9e9); err != nil {
		t.Errorf("Inverse(远值) 不应报错: %v", err)
	}
}

func TestEqcRoundtripGrid(t *testing.T) {
	// 网格往返(含两极、跨经度圈),门槛 1e-9 度
	p := mustEqc(t, "+proj=eqc +lat_ts=0 +lat_0=0 +lon_0=180 +ellps=WGS84")
	var maxE float64
	for lon := -180.0; lon <= 180.0; lon += 22.5 {
		for _, lat := range []float64{-90, -85, -45.25, 0, 45.25, 85, 90} {
			x, y, err := p.Forward(lon, lat)
			if err != nil {
				t.Fatalf("(%v,%v): %v", lon, lat, err)
			}
			gl, ga, err := p.Inverse(x, y)
			if err != nil {
				t.Fatalf("inverse(%v,%v): %v", x, y, err)
			}
			e := circDist(gl, lon) + math.Abs(ga-lat)
			if e > maxE {
				maxE = e
			}
			if e > eqcRoundTol {
				t.Errorf("往返(%v,%v) → (%v,%v) 偏差 %g", lon, lat, gl, ga, e)
			}
		}
	}
	probeReport(t, "roundtrip-grid", &eqcAcc{maxRnd: maxE})
}

// TestEqcFalseOrigin +x_0/+y_0 是 PROJ pj_fwd 末端的仿射偏移(米)。
// 参照值 = pyproj 3.8.0 / PROJ 9.8.1 实测(2026-10-22):
//
//	Transformer.from_crs(EPSG:4326, CRS.from_proj4(...))  (10,20) → 下列值
//
// 改造前 eqc 把 x_0/y_0 静默丢掉,这里锁死语义。
func TestEqcFalseOrigin(t *testing.T) {
	p := mustEqc(t, "+proj=eqc +lat_ts=0 +lon_0=0 +x_0=500000 +y_0=1000000 +ellps=WGS84")
	x, y, err := p.Forward(10, 20)
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	const wantX, wantY = 1613194.9079327357, 3212366.254171633
	if math.Abs(x-wantX) > eqcFwdTol || math.Abs(y-wantY) > eqcFwdTol {
		t.Errorf("Forward(10,20) = (%.10f, %.10f), want (%.10f, %.10f)", x, y, wantX, wantY)
	}
	lon, lat, err := p.Inverse(x, y)
	if err != nil {
		t.Fatalf("Inverse: %v", err)
	}
	if math.Abs(lon-10) > eqcInvTol || math.Abs(lat-20) > eqcInvTol {
		t.Errorf("Inverse 往返 = (%.12f, %.12f), want (10, 20)", lon, lat)
	}
	// 偏移是纯加法:与零偏移实例逐位相差恰好 (x_0, y_0)。
	base := mustEqc(t, "+proj=eqc +lat_ts=0 +lon_0=0 +ellps=WGS84")
	bx, by, _ := base.Forward(10, 20)
	if x-bx != 500000 || y-by != 1000000 {
		t.Errorf("偏移量不符: dx=%v dy=%v", x-bx, y-by)
	}
}
