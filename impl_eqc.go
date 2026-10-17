package proj

import (
	"fmt"
	"math"
)

// impl_eqc.go — Equidistant Cylindrical(Plate Carrée,EPSG:1028/1029)。
// Owner: AI-A。公式口径 PROJ 9.8 eqc.cpp(参考实现,非逐字拷贝;重写为 Go):
//
//	球体(EPSG:1029):x = R·cos(lat_ts)·λ,        y = R·(φ − φ0)
//	椭球(EPSG:1028):x = nu1·cos(lat_ts)·λ(×a),  y = M(φ) − M(φ0)
//	  其中 nu1 = 1/√(1−es·sin²lat_ts)(a-归一),M 为子午线弧。
//
// 尺度口径:坐标在投影内先 a-归一,乘 a 在本实现显式完成(对应 PROJ pj_fwd 的
// ×P->a);k_0 被 eqc 忽略(probe8 实测 pyproj 3.8 k_0=2/3 前后相同,PROJ 语义);
// Δλ 与输出经度都用 normLonDeg 归一(对应 adjlon);|lat_ts| = 90 时 rc = 0
// 会除零,PROJ 报错,这里同样拒绝。
// 子午线弧 M 用 mlfn.go(Karney 六阶级数,与 PROJ 9.8 mlfn.cpp/latitudes.cpp
// 同构:en[0] = 矩形半径 = 四分子午/(π/2·a))。

// deg2rad / rad2deg 角度-弧度换算(本包各 impl 共用)。
const (
	deg2rad = math.Pi / 180
	rad2deg = 180 / math.Pi
)

// eqcProj eqc 投影实例(不可变,线程安全)。
type eqcProj struct {
	crs    *CRS
	a      float64              // 长半轴(米)
	rc     float64              // x = rc·λ(λ rad):椭球 a·nu1·cosφ_ts;球 a·cosφ_ts
	m0     float64              // lat_0 处 a-归一子午线弧 mlfn(φ0)(椭球);球体为 0
	en0    float64              // 矩形半径(a-归一;球体 0)
	F      [auxLatOrder]float64 // φ→μ 系数
	Finv   [auxLatOrder]float64 // μ→φ 系数
	phi0   float64              // lat_0(rad)(球体 y 用)
	lon0   float64              // lon_0(度)
	sphere bool
}

// newEqc 工厂(经 register("eqc") 供 NewProjection 分发)。
func newEqc(c *CRS) (Projection, error) {
	if c == nil {
		return nil, fmt.Errorf("proj: 零值 CRS 不可用,只能由 Parse 构造")
	}
	latTs, _ := paramFloat(c, "lat_ts", 0) // PROJ 默认 0(Plate Carrée)
	if math.Abs(latTs) >= 90 {
		return nil, fmt.Errorf("proj: eqc +lat_ts=%v: |lat_ts| 应 < 90 度(cosφ_ts=0 会除零,PROJ 语义)", latTs)
	}
	// x_0/y_0 偏移不在此处理:v0 契约 Forward/Inverse 不含仿射偏移
	// (DESIGN §1.1;golden CRS 中 x_0/y_0 恒 0)。
	lon0, _ := paramFloat(c, "lon_0", 0)
	phi0, _ := paramFloat(c, "lat_0", 0)
	el := c.Ellipsoid
	p := &eqcProj{
		crs:  c,
		a:    el.A,
		phi0: phi0 * deg2rad,
		lon0: lon0,
	}

	if el.Sphere {
		p.sphere = true
		p.rc = el.A * math.Cos(latTs*deg2rad)
		return p, nil
	}

	// 椭球(EPSG:1028)
	n := el.F / (2 - el.F) // 第三扁率
	p.en0 = rectifyingRadius(n)
	p.F = mlCoeffs(n)
	sphiTs, cphiTs := math.Sincos(latTs * deg2rad)
	// rc = a·nu1·cosφ_ts,nu1 = 1/√(1−es·sin²φ_ts)
	p.rc = el.A * cphiTs / math.Sqrt(1-el.ES*sphiTs*sphiTs)
	p.m0 = mlfn(phi0*deg2rad, p.F, p.en0)
	p.Finv = mlInvCoeffs(n)
	return p, nil
}

// CRS 应回传绑定实例(契约 §2.3)。
func (p *eqcProj) CRS() *CRS { return p.crs }

// Forward lon/lat(度)→ x, y(米)。
func (p *eqcProj) Forward(lon, lat float64) (x, y float64, err error) {
	if !validLonLat(lon, lat) {
		return math.NaN(), math.NaN(), invalidLonLatErr(lon, lat)
	}
	dlam := normLonDeg(lon-p.lon0) * deg2rad
	phi := lat * deg2rad
	if p.sphere {
		return p.rc * dlam, p.a * (phi - p.phi0), nil
	}
	return p.rc * dlam, p.a * (mlfn(phi, p.F, p.en0) - p.m0), nil
}

// Inverse x, y(米)→ lon/lat(度)。eqc 全平面可逆,无定义域错误;
// 仅拒绝非有限输入(EnterDomain 不冒充,§2.5)。
func (p *eqcProj) Inverse(x, y float64) (lon, lat float64, err error) {
	if math.IsNaN(x) || math.IsInf(x, 0) || math.IsNaN(y) || math.IsInf(y, 0) {
		return math.NaN(), math.NaN(), fmt.Errorf("proj: eqc Inverse 输入非有限 (%v,%v)", x, y)
	}
	lam := x / p.rc
	lon = normLonDeg(p.lon0 + lam*rad2deg)
	if p.sphere {
		return lon, (y/p.a + p.phi0) * rad2deg, nil
	}
	// μ'(en-scaled 归一弧)→ φ
	mu := (y/p.a + p.m0) / p.en0
	return lon, mlfnInverse(mu, p.Finv) * rad2deg, nil
}

func init() {
	register("eqc", newEqc)
}
