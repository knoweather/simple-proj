package proj

// impl_info_test.go — tests for the metadata-introspection surface:
// Info() metadata introspection and reason-carrying DomainError.

import (
	"errors"
	"math"
	"testing"
)

func TestInfo(t *testing.T) {
	for _, crs := range []string{
		"+proj=latlong +ellps=WGS84",
		"+proj=eqc +lat_ts=0 +lat_0=0 +lon_0=180 +ellps=WGS84 +units=m",
		"+proj=geos +lon_0=0 +h=35785863 +ellps=WGS84 +sweep=y",
		"+proj=stere +lat_0=90 +lat_ts=60 +lon_0=-100 +ellps=WGS84",
		"+proj=laea +lat_0=45 +lon_0=10 +ellps=WGS84",
		"+proj=lcc +lat_1=30 +lat_2=60 +lat_0=0 +lon_0=100 +ellps=WGS84",
		"+proj=merc +a=6378137 +b=6378137 +lat_ts=0 +lon_0=0 +units=m",
		"+proj=tmerc +lat_0=0 +lon_0=105 +k=0.9996 +ellps=WGS84",
	} {
		c, err := Parse(crs)
		if err != nil {
			t.Fatalf("Parse(%q): %v", crs, err)
		}
		p, err := c.NewProjection()
		if err != nil {
			t.Fatalf("NewProjection(%q): %v", crs, err)
		}
		info, err := Info(p)
		if err != nil {
			t.Fatalf("Info(%q): %v", crs, err)
		}
		if info.Name != c.Proj || info.Description == "" || info.Units == "" ||
			info.Domain == "" || info.Accuracy == "" || info.CRS == "" {
			t.Fatalf("%s: 元数据不完整: %+v", crs, info)
		}
		if !info.HasInverse {
			t.Fatalf("%s: 本库所有已注册投影都应有逆变换", crs)
		}
		spherical := c.Ellipsoid.A == c.Ellipsoid.B
		if info.Spherical != spherical || info.Ellipsoidal == info.Spherical {
			t.Fatalf("%s: 球/椭球标志错: %+v", crs, info)
		}
	}

	// EPSG 特征回识别进 Info。
	c, err := Parse("EPSG:4326")
	if err != nil {
		t.Fatalf("Parse(EPSG:4326): %v", err)
	}
	p, _ := c.NewProjection()
	if info, err := Info(p); err != nil || info.EPSG != 4326 || info.Name != "latlong" {
		t.Fatalf("EPSG:4326 Info: %+v %v", info, err)
	}

	// nil 与未知投影防御分支。
	if _, err := Info(nil); err == nil {
		t.Fatal("Info(nil) 应报错")
	}
}

func TestDomainErrorReasons(t *testing.T) {
	type want struct {
		crs    string
		fwd    [2]float64 // lon, lat
		inv    [2]float64 // x, y (0,0 = 不测)
		reason string
	}
	for _, w := range []want{
		// lcc 远极点(golden_lcc 的 n > 0,南极在远端)。
		{"+proj=lcc +lat_1=30 +lat_2=60 +lat_0=0 +lon_0=100 +ellps=WGS84", [2]float64{100, -90}, [2]float64{}, "far-pole"},
		// laea 斜轴中心对跖点。
		{"+proj=laea +lat_0=45 +lon_0=10 +ellps=WGS84", [2]float64{-170, -45}, [2]float64{}, "antipode"},
		// laea 北冠状的远极点(南极)。
		{"+proj=laea +lat_0=90 +ellps=WGS84", [2]float64{0, -90}, [2]float64{}, "far-pole"},
		// tmerc |Ce| 超限(赤道上偏离中央子午线 82.5°)。
		{"+proj=tmerc +lat_0=0 +lon_0=105 +k=0.9996 +ellps=WGS84", [2]float64{105 + 82.5, 0}, [2]float64{9e9, -9e9}, "ce-limit"},
		// 球面 tmerc 的退化边(Δλ = 90°,|b| = 1)。
		{"+proj=tmerc +lat_0=0 +lon_0=105 +k=0.9996 +a=6371000 +b=6371000", [2]float64{105 + 90, 0}, [2]float64{}, "degenerate"},
		// 注:stere 斜轴对跖点不在表内 —— kernel 的 denom == 0 是精确比较,
		// antipode 处 FP 残差 ~1e-16 永不精确为 0,故给巨大有限值而非域错
		// (PROJ 9.8.1 stere.cpp 同为精确比较,行为一致;只有 laea 用 EPS10 拦)。
	} {
		c, err := Parse(w.crs)
		if err != nil {
			t.Fatalf("Parse(%q): %v", w.crs, err)
		}
		p, err := c.NewProjection()
		if err != nil {
			t.Fatalf("NewProjection(%q): %v", w.crs, err)
		}
		_, _, err = p.Forward(w.fwd[0], w.fwd[1])
		if !errors.Is(err, ErrDomain) {
			t.Fatalf("%s Forward(%v,%v): 应 errors.Is ErrDomain, got %v", w.crs, w.fwd[0], w.fwd[1], err)
		}
		var de *DomainError
		if !errors.As(err, &de) || de.Reason != w.reason {
			t.Fatalf("%s Forward: reason = %v, want %q", w.crs, de, w.reason)
		}
		if msg := err.Error(); msg == "" || msg == ErrDomain.Error() {
			t.Fatalf("%s: Error() 应带原因: %q", w.crs, msg)
		}
		if w.inv != [2]float64{} {
			_, _, err = p.Inverse(w.inv[0], w.inv[1])
			var de2 *DomainError
			if !errors.Is(err, ErrDomain) || !errors.As(err, &de2) || de2.Reason != w.reason {
				t.Fatalf("%s Inverse(%v,%v): %v (reason %v)", w.crs, w.inv[0], w.inv[1], err, de2)
			}
		}
	}
}

// TestBatchDomainErrorCompat 验证 DomainError 不破坏 proj.go 的批量
// 语义:单点域外写 NaN 继续、批末裸 ErrDomain(not DomainError)、有效点完好。
func TestBatchDomainErrorCompat(t *testing.T) {
	c, err := Parse("+proj=tmerc +lat_0=0 +lon_0=105 +k=0.9996 +ellps=WGS84")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	p, err := c.NewProjection()
	if err != nil {
		t.Fatalf("NewProjection: %v", err)
	}
	lonLat := []float64{105, 45, 105 + 82.5, 0, 120, 30} // 第 2 点域外
	xy, err := ForwardBatch(p, lonLat)
	if !errors.Is(err, ErrDomain) {
		t.Fatalf("批末应 errors.Is ErrDomain, got %v", err)
	}
	if err != ErrDomain {
		t.Fatalf("批末应返回 ErrDomain sentinel 本体(proj.go:84), got %T", err)
	}
	if !math.IsNaN(xy[2]) || !math.IsNaN(xy[3]) {
		t.Fatalf("域外点应写 (NaN,NaN): %v", xy[2:])
	}
	x0, y0 := xy[0], xy[1]
	if math.IsNaN(x0) || y0 < 4e6 || y0 > 6e6 {
		t.Fatalf("域内点结果异常: (%v, %v)", x0, y0)
	}
}

// TestInfoUnits Info 的 Units 字段要跟着 +units/+to_meter 走(否则调用方
// 按「度 → 米」理解 km 制坐标会差 1000 倍)。
func TestInfoUnits(t *testing.T) {
	cases := []struct{ crs, want string }{
		{"+proj=merc +a=6378137 +b=6378137", "度 → 米"},
		{"+proj=merc +a=6378137 +b=6378137 +units=km", "度 → km"},
		{"+proj=merc +a=6378137 +b=6378137 +units=us-ft", "度 → us-ft"},
		{"+proj=merc +a=6378137 +b=6378137 +to_meter=1000", "度 → 米/1000(自定义 +to_meter)"},
		// latlong 输出是度,单位层不参与,Units 保持原样
		{"+proj=latlong +ellps=WGS84 +units=km", "度 → 度"},
	}
	for _, c := range cases {
		p := mustProj(t, c.crs)
		info, err := Info(p)
		if err != nil {
			t.Fatalf("%q: Info: %v", c.crs, err)
		}
		if info.Units != c.want {
			t.Errorf("%q: Units = %q, want %q", c.crs, info.Units, c.want)
		}
	}
	if _, err := Info(nil); err == nil {
		t.Error("Info(nil) 应报错")
	}
}
