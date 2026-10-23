package proj

// impl_aeqd_test.go — aeqd 四方面(极轴 N/S、赤道、斜轴)× 球/椭球。
//
// 参照值全部来自 pyproj 3.8.0 / PROJ 9.8.1 实测(2026-10-22),门槛:
// forward ≤ 1e-6 m、inverse ≤ 1e-9 度。
// 椭球斜轴/赤道走 geodesic.go 的 Karney 解,与 pyproj 的距离差见
// geodesic_test.go(实测 ≤ 4e-9 m)。

import (
	"errors"
	"math"
	"testing"
)

const (
	aeqdFwdTol = 1e-6
	aeqdInvTol = 1e-9
)

func mustAeqd(t *testing.T, crs string) Projection {
	t.Helper()
	c, err := Parse(crs)
	if err != nil {
		t.Fatalf("Parse(%q): %v", crs, err)
	}
	p, err := c.NewProjection()
	if err != nil {
		t.Fatalf("NewProjection(%q): %v", crs, err)
	}
	return p
}

// TestAeqdAspects 四种方面在 (lon=30, lat=60) 上的 forward 参照值。
func TestAeqdAspects(t *testing.T) {
	cases := []struct {
		crs    string
		x, y   float64
		aspect string
	}{
		{"+proj=aeqd +lat_0=90 +lon_0=0 +ellps=WGS84", 1673946.454911, -2899360.309056, "椭球 北极"},
		{"+proj=aeqd +lat_0=-90 +lon_0=0 +ellps=WGS84", 8328019.274402, 14424552.509676, "椭球 南极"},
		{"+proj=aeqd +lat_0=0 +lon_0=0 +ellps=WGS84", 1989689.287278, 6856935.544171, "椭球 赤道"},
		{"+proj=aeqd +lat_0=45 +lon_0=10 +ellps=WGS84", 1114090.835319, 1820237.488474, "椭球 斜轴"},
		{"+proj=aeqd +lat_0=45 +lon_0=10 +R=6371000", 1110079.897537, 1818482.412366, "球体 斜轴"},
	}
	for _, c := range cases {
		p := mustAeqd(t, c.crs)
		x, y, err := p.Forward(30, 60)
		if err != nil {
			t.Fatalf("%s: Forward: %v", c.aspect, err)
		}
		if math.Abs(x-c.x) > aeqdFwdTol || math.Abs(y-c.y) > aeqdFwdTol {
			t.Errorf("%s: Forward(30,60) = (%.6f, %.6f), want (%.6f, %.6f)", c.aspect, x, y, c.x, c.y)
		}
		lon, lat, err := p.Inverse(x, y)
		if err != nil {
			t.Fatalf("%s: Inverse: %v", c.aspect, err)
		}
		if math.Abs(lon-30) > aeqdInvTol || math.Abs(lat-60) > aeqdInvTol {
			t.Errorf("%s: 往返 = (%.12f, %.12f), want (30, 60)", c.aspect, lon, lat)
		}
	}
}

// TestAeqdFalseOriginAndK0 x_0/y_0 生效;k_0 被忽略(方位类投影的 PROJ 口径)。
func TestAeqdFalseOriginAndK0(t *testing.T) {
	base := mustAeqd(t, "+proj=aeqd +lat_0=45 +lon_0=10 +ellps=WGS84")
	bx, by, _ := base.Forward(30, 60)

	off := mustAeqd(t, "+proj=aeqd +lat_0=45 +lon_0=10 +x_0=1000 +y_0=2000 +ellps=WGS84")
	ox, oy, _ := off.Forward(30, 60)
	if math.Abs(ox-(bx+1000)) > aeqdFwdTol || math.Abs(oy-(by+2000)) > aeqdFwdTol {
		t.Errorf("x_0/y_0 未生效: (%.6f, %.6f) vs 基准+偏移 (%.6f, %.6f)", ox, oy, bx+1000, by+2000)
	}

	for _, k := range []string{"0.9", "2"} {
		p := mustAeqd(t, "+proj=aeqd +lat_0=45 +lon_0=10 +k_0="+k+" +ellps=WGS84")
		x, y, _ := p.Forward(30, 60)
		if x != bx || y != by {
			t.Errorf("k_0=%s 应被 aeqd 忽略, got (%.6f, %.6f) vs (%.6f, %.6f)", k, x, y, bx, by)
		}
	}
}

// TestAeqdDomain 球体两处退化:斜轴的对跖点、极轴的对侧极点。
// 椭球模式全域可解(大地线/子午线弧没有洞),不该报域错。
func TestAeqdDomain(t *testing.T) {
	sph := mustAeqd(t, "+proj=aeqd +lat_0=45 +lon_0=10 +R=6371000")
	if _, _, err := sph.Forward(-170, -45); !errors.Is(err, ErrDomain) {
		t.Errorf("球体斜轴对跖点应 ErrDomain, got %v", err)
	}
	var de *DomainError
	if _, _, err := sph.Forward(-170, -45); !errors.As(err, &de) || de.Reason != "antipode" {
		t.Errorf("应对跖点原因, got %v", err)
	}
	polar := mustAeqd(t, "+proj=aeqd +lat_0=90 +lon_0=0 +R=6371000")
	if _, _, err := polar.Forward(0, -90); !errors.Is(err, ErrDomain) {
		t.Errorf("球体极轴对侧极点应 ErrDomain, got %v", err)
	}

	// 椭球同样两个位置都是合法点
	ell := mustAeqd(t, "+proj=aeqd +lat_0=45 +lon_0=10 +ellps=WGS84")
	if _, _, err := ell.Forward(-170, -45); err != nil {
		t.Errorf("椭球斜轴对跖点应可解, got %v", err)
	}
	ellPolar := mustAeqd(t, "+proj=aeqd +lat_0=90 +lon_0=0 +ellps=WGS84")
	if _, _, err := ellPolar.Forward(0, -90); err != nil {
		t.Errorf("椭球极轴对侧极点应可解, got %v", err)
	}
}

// TestAeqdCenterAndKnifeEdge
//   - 中心点 → (0,0),逆变换回到中心(不会踩 atan2(0,0) 的歧义);
//   - (lat_0=45, lon_0=10) 投 (100,45) 是 PROJ 的刀刃点:aeqd_s_inverse 里
//     `xy.y == 0 ? lam = 0` 的短路会因 1e-16 的残差舍成 0 而丢掉 90° 经度。
//     本库直接 atan2(x, 0) = 90°,往返正确 —— 这是与 PROJ 唯一有意的差异
//     (见 impl_aeqd.go 与 README「与 pyproj 的已知差异」)。
func TestAeqdCenterAndKnifeEdge(t *testing.T) {
	p := mustAeqd(t, "+proj=aeqd +lat_0=45 +lon_0=10 +R=6371000")
	x, y, err := p.Forward(10, 45)
	if err != nil {
		t.Fatalf("中心点 Forward: %v", err)
	}
	if x != 0 || y != 0 {
		t.Errorf("中心点应为 (0,0), got (%.6g, %.6g)", x, y)
	}
	lon, lat, err := p.Inverse(0, 0)
	if err != nil || math.Abs(lon-10) > aeqdInvTol || math.Abs(lat-45) > aeqdInvTol {
		t.Errorf("Inverse(0,0) = (%.12f, %.12f, %v), want (10, 45)", lon, lat, err)
	}

	// 刀刃点:本库往返正确(pyproj 在此返回中心经度 10)
	x, y, err = p.Forward(100, 45)
	if err != nil {
		t.Fatalf("刀刃点 Forward: %v", err)
	}
	lon, lat, err = p.Inverse(x, y)
	if err != nil || math.Abs(lon-100) > aeqdInvTol || math.Abs(lat-45) > aeqdInvTol {
		t.Errorf("刀刃点往返 = (%.12f, %.12f, %v), want (100, 45)", lon, lat, err)
	}
}

// TestAeqdInputValidation 输入越界与 NaN 走普通 error(非 ErrDomain)。
func TestAeqdInputValidation(t *testing.T) {
	p := mustAeqd(t, "+proj=aeqd +lat_0=45 +lon_0=10 +ellps=WGS84")
	if _, _, err := p.Forward(10, 91); err == nil || errors.Is(err, ErrDomain) {
		t.Errorf("lat=91 应报普通输入错误, got %v", err)
	}
	if _, _, err := p.Forward(400, 10); err == nil || errors.Is(err, ErrDomain) {
		t.Errorf("lon=400 应报普通输入错误, got %v", err)
	}
	if _, _, err := p.Inverse(math.Inf(1), 0); err == nil {
		t.Error("Inf 输入应报错")
	}
}

// TestAeqdSphereAspects 球体三个方面(极轴 N/S、赤道)的正反变换。
// 参照值 = pyproj 3.8.0 / PROJ 9.8.1 实测(2026-10-22),(lon=30, lat=60)。
// 极轴与赤道分支 pyproj 的 sweep 用例未覆盖,这里单独定住。
func TestAeqdSphereAspects(t *testing.T) {
	cases := []struct {
		crs    string
		x, y   float64
		aspect string
	}{
		{"+proj=aeqd +lat_0=90 +lon_0=0 +R=6371000", 1667923.899668, -2888928.937384, "球体 北极"},
		{"+proj=aeqd +lat_0=-90 +lon_0=0 +R=6371000", 8339619.498342, 14444644.686920, "球体 南极"},
		{"+proj=aeqd +lat_0=0 +lon_0=0 +R=6371000", 1984274.428674, 6873728.253246, "球体 赤道"},
	}
	for _, c := range cases {
		p := mustAeqd(t, c.crs)
		x, y, err := p.Forward(30, 60)
		if err != nil {
			t.Fatalf("%s: %v", c.aspect, err)
		}
		if math.Abs(x-c.x) > aeqdFwdTol || math.Abs(y-c.y) > aeqdFwdTol {
			t.Errorf("%s: (30,60) → (%.6f, %.6f), want (%.6f, %.6f)", c.aspect, x, y, c.x, c.y)
		}
		lon, lat, err := p.Inverse(x, y)
		if err != nil || math.Abs(lon-30) > aeqdInvTol || math.Abs(lat-60) > aeqdInvTol {
			t.Errorf("%s: 往返 = (%.12f, %.12f, %v)", c.aspect, lon, lat, err)
		}
	}
}

// TestAeqdSphereRadiusEdge 球面逆变换的半径边界:
// |rho| 略超 π 会被夹到 π(PROJ 同款),远超 π 才报域外。
func TestAeqdSphereRadiusEdge(t *testing.T) {
	p := mustAeqd(t, "+proj=aeqd +lat_0=0 +lon_0=0 +R=6371000")
	// 半径恰好 π·R 是中心的对跖点:赤道方面下即 (180°, 0°)
	x, y := math.Pi*6371000, 0.0
	lon, lat, err := p.Inverse(x, y)
	if err != nil {
		t.Fatalf("ρ=π·R 应可解: %v", err)
	}
	if math.Abs(math.Abs(lon)-180) > 1e-9 || math.Abs(lat) > 1e-6 {
		t.Errorf("ρ=π·R 的反变换 = (%.9f, %.9f), want (±180, 0)", lon, lat)
	}
	// 略超 π(1e-12 以内)仍夹回 π
	if _, _, err := p.Inverse(math.Pi*6371000+1e-6, 0); err != nil {
		t.Errorf("略超 π 应夹回而不是报错: %v", err)
	}
	// 远超 π → ErrDomain
	if _, _, err := p.Inverse(4*6371000, 0); !errors.Is(err, ErrDomain) {
		t.Errorf("ρ>π·R 应 ErrDomain, got %v", err)
	}
}

// TestAeqdParamValidation lat_0 越界要报错(通用数值域校验在 Parse 就拦下,
// 投影级校验在 NewProjection;两处拦都算过)。
func TestAeqdParamValidation(t *testing.T) {
	c, err := Parse("+proj=aeqd +lat_0=95 +ellps=WGS84")
	if err != nil {
		return // Parse 拦下也算过
	}
	if _, err := c.NewProjection(); err == nil {
		t.Error("lat_0=95 应报错")
	}
}

// TestClampUnit asin 参数夹取(球面逆变换的数值兜底)。
func TestClampUnit(t *testing.T) {
	for _, c := range []struct{ in, want float64 }{
		{0.5, 0.5}, {1.0000000000000002, 1}, {-1.0000000000000002, -1}, {2, 1}, {-2, -1},
	} {
		if got := clampUnit(c.in); got != c.want {
			t.Errorf("clampUnit(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}
