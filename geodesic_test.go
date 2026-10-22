package proj

// geodesic_test.go — 大地线求解器(geodesic.go)与 pyproj/GeographicLib 的对拍。
//
// 为什么单独测:aeqd 的椭球斜轴/赤道方面完全由大地线解定义,如果 aeqd 对不上,
// 必须先能区分「投影写错」还是「大地线写错」。参照数据由 xcheck/gen_geodesic.py
// 生成(xcheck/geodesic_ref.json),金标 = pyproj 3.8.0 / PROJ 9.8.1 的 Geod。
//
// 口径:azi2 存的是**点 2 的前向方位**(GeographicLib geod_inverse 的约定);
// pyproj 的 Geod.inv 返回的是反方位,生成器已 +180 归一。

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

type geodInvRec struct {
	Lat1, Lon1, Lat2, Lon2 float64
	S12                    float64
	Azi1, Azi2             float64
}

type geodDirRec struct {
	Lat1, Lon1, Azi1, S12 float64
	Lat2, Lon2, Azi2      float64
}

type geodRef struct {
	Generator string `json:"generator"`
	Generated string `json:"generated"`
	Ellipsoid struct {
		A  float64 `json:"a"`
		RF float64 `json:"rf"`
	} `json:"ellipsoid"`
	Inverse []geodInvRec `json:"inverse"`
	Direct  []geodDirRec `json:"direct"`
}

func loadGeodRef(t *testing.T) geodRef {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(file), "xcheck", "geodesic_ref.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read geodesic_ref: %v (run: python xcheck/gen_geodesic.py)", err)
	}
	var r geodRef
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatalf("parse geodesic_ref: %v", err)
	}
	return r
}

// angDiffDeg 角度差,按 360° 圆周折返(方位角在 ±180 处等价)。
func angDiffDeg(a, b float64) float64 {
	d := math.Mod(a-b, 360)
	if d > 180 {
		d -= 360
	}
	if d < -180 {
		d += 360
	}
	return math.Abs(d)
}

const (
	geodDistTol = 1e-6 // m(COORDINATION §2.5 同口径)
	geodAzTol   = 1e-9 // 度
)

func TestGeodesicInverseParity(t *testing.T) {
	r := loadGeodRef(t)
	g := newGeod(r.Ellipsoid.A, 1/r.Ellipsoid.RF)
	t.Logf("reference: %s (%s), %d inverse cases", r.Generator, r.Generated, len(r.Inverse))

	var maxS, maxA float64
	for _, c := range r.Inverse {
		s12, azi1, azi2, ok := g.inverse(c.Lat1, c.Lon1, c.Lat2, c.Lon2)
		if !ok {
			t.Fatalf("inverse(%v,%v,%v,%v) 失败", c.Lat1, c.Lon1, c.Lat2, c.Lon2)
		}
		ds := math.Abs(s12 - c.S12)
		da := math.Max(angDiffDeg(azi1, c.Azi1), angDiffDeg(azi2, c.Azi2))
		if ds > maxS {
			maxS = ds
		}
		if da > maxA {
			maxA = da
		}
		if ds > geodDistTol || da > geodAzTol {
			t.Errorf("inverse(%v,%v → %v,%v): s12=%.9f(参照 %.9f, Δ%.3e m) azi1=%.12f azi2=%.12f(参照 %.12f/%.12f, Δ%.3e°)",
				c.Lat1, c.Lon1, c.Lat2, c.Lon2, s12, c.S12, ds, azi1, azi2, c.Azi1, c.Azi2, da)
		}
	}
	t.Logf("inverse max: Δs12 = %.3e m, Δazi = %.3e 度", maxS, maxA)
}

func TestGeodesicDirectParity(t *testing.T) {
	r := loadGeodRef(t)
	g := newGeod(r.Ellipsoid.A, 1/r.Ellipsoid.RF)
	t.Logf("%d direct cases", len(r.Direct))

	var maxP, maxA float64
	for _, c := range r.Direct {
		lat2, lon2, azi2, ok := g.direct(c.Lat1, c.Lon1, c.Azi1, c.S12)
		if !ok {
			t.Fatalf("direct(%v,%v,%v,%v) 失败", c.Lat1, c.Lon1, c.Azi1, c.S12)
		}
		dp := math.Max(math.Abs(lat2-c.Lat2), angDiffDeg(lon2, c.Lon2))
		da := angDiffDeg(azi2, c.Azi2)
		if dp > maxP {
			maxP = dp
		}
		if da > maxA {
			maxA = da
		}
		if dp > geodAzTol || da > geodAzTol {
			t.Errorf("direct(%v,%v,azi=%v,s=%v): (%.12f,%.12f) 参照 (%.12f,%.12f), Δ%.3e°",
				c.Lat1, c.Lon1, c.Azi1, c.S12, lat2, lon2, c.Lat2, c.Lon2, dp)
		}
	}
	t.Logf("direct max: Δ位置 = %.3e 度, Δ方位 = %.3e 度", maxP, maxA)
}

// TestGeodesicRoundTrip 自洽性:反解→正解应回到原点(不依赖参照数据)。
func TestGeodesicRoundTrip(t *testing.T) {
	g := newGeod(6378137.0, 1/298.257223563)
	for _, c := range [][4]float64{
		{0, 0, 45, 45}, {-33.9, 151.2, 51.5, -0.13}, {89, 0, -89, 179},
		{0, 0, 0.5, 179.9}, {12.3, -45.6, -78.9, 123.4},
	} {
		s12, azi1, _, ok := g.inverse(c[0], c[1], c[2], c[3])
		if !ok {
			t.Fatalf("inverse 失败: %v", c)
		}
		lat2, lon2, _, ok := g.direct(c[0], c[1], azi1, s12)
		if !ok {
			t.Fatalf("direct 失败: %v", c)
		}
		if math.Abs(lat2-c[2]) > 1e-9 || angDiffDeg(lon2, c[3]) > 1e-9 {
			t.Errorf("%v 往返 → (%.12f, %.12f), 误差 (%.3e, %.3e)",
				c, lat2, lon2, math.Abs(lat2-c[2]), angDiffDeg(lon2, c[3]))
		}
	}
}

// ---- 长球体(prolate,f<0)--------------------------------------------------
//
// 本库的**投影**构造不出 f<0(Parse 拒收负的 +rf/+f),但 geodesic.go 是通用的
// Karney 实现,移植进来的 f<0 分支必须验证 —— 否则那部分代码只是"看起来对"。
// pyproj 的 Geod 接受 f<0,拿它当金标(见 gen_geodesic.py 的 PROLATE)。

type geodProlate struct {
	Name    string       `json:"name"`
	A       float64      `json:"a"`
	F       float64      `json:"f"`
	Inverse []geodInvRec `json:"inverse"`
	Direct  []geodDirRec `json:"direct"`
}

func TestGeodesicProlate(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(file), "xcheck", "geodesic_ref.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read geodesic_ref: %v", err)
	}
	var r struct {
		Prolate []geodProlate `json:"prolate"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatalf("parse geodesic_ref: %v", err)
	}
	if len(r.Prolate) == 0 {
		t.Fatal("geodesic_ref.json 里没有 prolate 段(重跑 python xcheck/gen_geodesic.py)")
	}
	for _, c := range r.Prolate {
		g := newGeod(c.A, c.F)
		var maxS, maxA, maxP float64
		for _, v := range c.Inverse {
			s12, azi1, azi2, ok := g.inverse(v.Lat1, v.Lon1, v.Lat2, v.Lon2)
			if !ok {
				t.Fatalf("%s: inverse(%v,%v,%v,%v) 失败", c.Name, v.Lat1, v.Lon1, v.Lat2, v.Lon2)
			}
			ds := math.Abs(s12 - v.S12)
			da := math.Max(angDiffDeg(azi1, v.Azi1), angDiffDeg(azi2, v.Azi2))
			maxS = math.Max(maxS, ds)
			maxA = math.Max(maxA, da)
			if ds > geodDistTol || da > geodAzTol {
				t.Errorf("%s inverse(%v,%v → %v,%v): Δs12=%.3e m Δazi=%.3e°", c.Name, v.Lat1, v.Lon1, v.Lat2, v.Lon2, ds, da)
			}
		}
		for _, v := range c.Direct {
			lat2, lon2, azi2, ok := g.direct(v.Lat1, v.Lon1, v.Azi1, v.S12)
			if !ok {
				t.Fatalf("%s: direct 失败", c.Name)
			}
			dp := math.Max(math.Abs(lat2-v.Lat2), angDiffDeg(lon2, v.Lon2))
			maxP = math.Max(maxP, dp)
			if dp > geodAzTol || angDiffDeg(azi2, v.Azi2) > geodAzTol {
				t.Errorf("%s direct(%v,%v): (%.12f,%.12f) 参照 (%.12f,%.12f)", c.Name, v.Lat1, v.Lon1, lat2, lon2, v.Lat2, v.Lon2)
			}
		}
		t.Logf("%s (a=%g f=%g): %d 反解 Δs12 max %.3e m / Δazi max %.3e°,%d 正解 Δ位置 max %.3e°",
			c.Name, c.A, c.F, len(c.Inverse), maxS, maxA, len(c.Direct), maxP)
	}
}

// TestGeodesicInputGuards 非有限/越界入参走 ok=false,不 panic。
func TestGeodesicInputGuards(t *testing.T) {
	g := newGeod(6378137.0, 1/298.257223563)
	if _, _, _, ok := g.inverse(math.NaN(), 0, 0, 0); ok {
		t.Error("NaN 纬度应 ok=false")
	}
	if _, _, _, ok := g.inverse(0, 0, 91, 0); ok {
		t.Error("|lat|>90 应 ok=false")
	}
	if _, _, _, ok := g.direct(math.NaN(), 0, 0, 1000); ok {
		t.Error("NaN 入参应 ok=false")
	}
	if _, _, _, ok := g.direct(0, 0, 0, math.NaN()); ok {
		t.Error("NaN 距离应 ok=false")
	}
	// 极点:不 panic,给出 ±90。输入距离是四分之一子午线的近似值
	// (精确值 10001965.729312722),所以纬度门放宽到 1e-7 度。
	lat2, _, _, ok := g.direct(0, 0, 0, 10001965.729312722)
	if !ok || math.Abs(lat2-90) > 1e-7 {
		t.Errorf("沿子午线走到北极应得 90, got %.12f (ok=%v)", lat2, ok)
	}
}
