package proj

import (
	"errors"
	"math"
	"testing"
)

// impl_geos_test.go — 参照值来自 probe9.py / probe9b.py
// (pyproj 3.8.0 / PROJ 9.8.1 @ 2026-10-17)。golden 侧(椭球双 sweep)由
// xcheck 覆盖;本文件补 golden 未覆盖的球体路径/错误面/口径特例。
const (
	geosFwdTol = 1e-6 // m
	geosInvTol = 1e-9 // 度
)

func mustGeos(t *testing.T, crs string) *geosProj {
	t.Helper()
	c, err := Parse(crs)
	if err != nil {
		t.Fatalf("Parse(%q): %v", crs, err)
	}
	pr, err := c.NewProjection()
	if err != nil {
		t.Fatalf("NewProjection(%q): %v", crs, err)
	}
	g, ok := pr.(*geosProj)
	if !ok {
		t.Fatalf("NewProjection(%q) 未产出 geosProj", crs)
	}
	return g
}

// sphere grid(lon = 140.7+dlon,lat ∈ ±60):全部在视界内,forward/inverse 双向参照(probe9b)
func TestGeosSphereGrid(t *testing.T) {
	p := mustGeos(t, "+proj=geos +h=35785863 +a=6378137 +b=6378137 +lon_0=140.7")
	var maxF, maxI float64
	for _, c := range []struct {
		lon, lat, x, y, invLon, invLat float64
	}{
		{85.69999999999999, -60.0, -2314480.587764809, -4860320.641971195, 85.69999999999916, -60.00000000000025},
		{85.69999999999999, 0.0, -4826154.359574757, 0.0, 85.70000000000009, 0.0},
		{110.69999999999999, -30.0, -2639208.3450803654, -3037418.315156671, 110.69999999999986, -30.000000000000092},
		{110.69999999999999, 60.0, -1447398.521750382, 4980167.114393057, 110.7000000000006, 59.999999999999524},
		{140.7, -60.0, 0.0, -5038113.707798288, 140.7, -59.99999999999999},
		{140.7, 0.0, 0.0, 0.0, 140.7, 0.0},
		{140.7, 60.0, 0.0, 5038113.707798288, 140.7, 59.99999999999999},
		{170.7, -30.0, 2639208.3450803636, -3037418.315156671, 170.70000000000013, -30.000000000000092},
		{170.7, 60.0, 1447398.5217503805, 4980167.114393057, 170.6999999999994, 59.99999999999954},
		{195.7, 0.0, 4826154.359574757, 0.0, -164.30000000000007, 0.0}, // 195.7 ≡ −164.3(环绕)
	} {
		x, y, err := p.Forward(c.lon, c.lat)
		if err != nil {
			t.Fatalf("Forward(%v,%v): %v", c.lon, c.lat, err)
		}
		eF := math.Abs(x-c.x) + math.Abs(y-c.y)
		if eF > geosFwdTol {
			t.Errorf("sphere Forward(%v,%v) = (%v,%v) want (%v,%v) 偏差 %g", c.lon, c.lat, x, y, c.x, c.y, eF)
		}
		maxF = math.Max(maxF, eF)
		gl, ga, err := p.Inverse(x, y)
		if err != nil {
			t.Fatalf("Inverse(%v,%v): %v", x, y, err)
		}
		eI := circDist(gl, c.invLon) + math.Abs(ga-c.invLat)
		if eI > geosInvTol {
			t.Errorf("sphere Inverse(%v,%v) = (%v,%v) want (%v,%v) 偏差 %g", x, y, gl, ga, c.invLon, c.invLat, eI)
		}
		maxI = math.Max(maxI, eI)
	}
	t.Logf("sphere 网格最大偏差: forward %.3g m | inverse %.3g°", maxF, maxI)
}

// sphere sweep=x forward 参照(probe9;注意球体 forward 无可见性检查 ——
// 界外点也给出确定性输出,与 PROJ 一致,故逆不建议判等)
func TestGeosSphereSweepXForward(t *testing.T) {
	px := mustGeos(t, "+proj=geos +h=35785863 +a=6378137 +b=6378137 +lon_0=140.7 +sweep=x")
	for _, c := range []struct {
		lon, lat, x, y float64
	}{
		{0, 0, -3061899.3578497567, 0.0},
		{140.7, 0, 0.0, 0.0},
		{30.5, -45.25, -3420958.908898224, -3694915.1416018396},
		{-100.0, 60.0, 2255206.277624175, 4496917.9222248895},
		{60.0, 80.0, -921154.2241654263, 5314388.908035341},
	} {
		x, y, err := px.Forward(c.lon, c.lat)
		if err != nil {
			t.Fatalf("Forward(%v,%v): %v", c.lon, c.lat, err)
		}
		if e := math.Abs(x-c.x) + math.Abs(y-c.y); e > geosFwdTol {
			t.Errorf("sweep=x sphere Forward(%v,%v) 偏差 %g", c.lon, c.lat, e)
		}
	}
	// sweep=y 同点(差异 huge,确认 flip 真的换了轴)
	py := mustGeos(t, "+proj=geos +h=35785863 +a=6378137 +b=6378137 +lon_0=140.7 +sweep=y")
	x1, y1, _ := px.Forward(30.5, -45.25)
	x2, y2, _ := py.Forward(30.5, -45.25)
	if x1 == x2 && y1 == y2 {
		t.Errorf("swEEP=x 与 sweep=y 同点同输出,flip 轴配置未生效")
	}
}

// sweep 缺省 === y(PROJ≥6,决策 D1;probe9 实测)
func TestGeosSweepDefault(t *testing.T) {
	pd := mustGeos(t, "+proj=geos +h=35785863 +a=6378137 +b=6378137 +lon_0=140.7")
	py := mustGeos(t, "+proj=geos +h=35785863 +a=6378137 +b=6378137 +lon_0=140.7 +sweep=y")
	if pd.flip != false || pd.flip != py.flip {
		t.Errorf("sweep 缺省应等价 sweep=y")
	}
	x1, y1, _ := pd.Forward(30.5, -45.25)
	x2, y2, _ := py.Forward(30.5, -45.25)
	if x1 != x2 || y1 != y2 {
		t.Errorf("缺省 sweep 与 sweep=y 输出不一致: (%v,%v) vs (%v,%v)", x1, y1, x2, y2)
	}
}

// 椭球域外:forward 可见性 + inverse det(probe9:sweep=y lon_0=0 东西超界 → inf)
func TestGeosEllipsoidDomain(t *testing.T) {
	p := mustGeos(t, "+proj=geos +h=35785863 +ellps=WGS84 +lon_0=0 +sweep=y")
	for _, pt := range [][2]float64{{105.2, 30.0}, {170.0, 10.0}} {
		x, y, err := p.Forward(pt[0], pt[1])
		if !math.IsNaN(x) || !math.IsNaN(y) || !errors.Is(err, ErrDomain) {
			t.Errorf("域外 Forward(%v,%v) 期望 (NaN,NaN,ErrDomain), got (%v,%v,%v)", pt[0], pt[1], x, y, err)
		}
	}
	// 域内点(probe9)
	x, y, err := p.Forward(-30.5, -60.0)
	if err != nil {
		t.Fatalf("Forward(-30.5,-60): %v", err)
	}
	if e := math.Abs(x+1472629.351973165) + math.Abs(y+4958537.734914743); e > geosFwdTol {
		t.Errorf("域内 Forward(-30.5,-60) = (%v,%v) 偏差 %g", x, y, e)
	}
	// inverse 远值(golden special 同测,此处仅 main 路径)
	lon, lat, err := p.Inverse(9e9, -9e9)
	if !math.IsNaN(lon) || !math.IsNaN(lat) || !errors.Is(err, ErrDomain) {
		t.Errorf("Inverse(9e9,-9e9) 期望 (NaN,NaN,ErrDomain), got (%v,%v,%v)", lon, lat, err)
	}
}

// 错误面:h 缺省/非法、sweep 非法、输入域错误
func TestGeosErrors(t *testing.T) {
	for _, crs := range []string{
		"+proj=geos +a=6378137 +b=6378137",            // h 缺省 0 → rg1≤0,PROJ 亦报错(probe9)
		"+proj=geos +h=0 +a=6378137 +b=6378137",       // h=0
		"+proj=geos +h=-1e10 +a=6378137 +b=6378137",   // 负
		"+proj=geos +sweep=q +h=35785863 +a=6378137",  // sweep 非法
		"+proj=geos +sweep=yy +h=35785863 +a=6378137", // sweep 多字符
	} {
		c, err := Parse(crs)
		if err == nil {
			if _, perr := c.NewProjection(); perr == nil {
				t.Errorf("%q NewProjection 应报错", crs)
			}
		}
	}
	p := mustGeos(t, "+proj=geos +h=35785863 +ellps=WGS84 +lon_0=0")
	for _, pt := range [][2]float64{
		{360.1, 0}, {0, 90.1}, {math.NaN(), 0}, {0, math.Inf(1)},
	} {
		_, _, err := p.Forward(pt[0], pt[1])
		if err == nil || errors.Is(err, ErrDomain) {
			t.Errorf("Forward(%v,%v) 应报非 ErrDomain 错误", pt[0], pt[1])
		}
	}
	if _, _, err := p.Inverse(math.NaN(), 0); err == nil {
		t.Errorf("Inverse(NaN) 应报错")
	}
}

// TestGeosFalseOrigin geos 的 +x_0/+y_0 同样是末端仿射偏移。
// 参照值 = pyproj 3.8.0 / PROJ 9.8.1 实测(2026-10-22):
//
//	+proj=geos +h=35785863 +lon_0=140.7 +x_0=1000 +y_0=2000 +ellps=WGS84
//	(140,20) → (-71475.87148593215, 2144190.675610925)
func TestGeosFalseOrigin(t *testing.T) {
	p := mustGeos(t, "+proj=geos +h=35785863 +lon_0=140.7 +x_0=1000 +y_0=2000 +ellps=WGS84")
	x, y, err := p.Forward(140, 20)
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	const wantX, wantY = -71475.87148593215, 2144190.675610925
	if math.Abs(x-wantX) > geosFwdTol || math.Abs(y-wantY) > geosFwdTol {
		t.Errorf("Forward(140,20) = (%.10f, %.10f), want (%.10f, %.10f)", x, y, wantX, wantY)
	}
	lon, lat, err := p.Inverse(x, y)
	if err != nil {
		t.Fatalf("Inverse: %v", err)
	}
	if math.Abs(lon-140) > geosInvTol || math.Abs(lat-20) > geosInvTol {
		t.Errorf("Inverse 往返 = (%.12f, %.12f), want (140, 20)", lon, lat)
	}
}
