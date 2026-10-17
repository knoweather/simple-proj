package proj

import (
	"fmt"
	"math"
)

// impl_geos.go — Geostationary Satellite View(sweep x/y,球/椭球)。
// Owner: AI-A。公式口径 PROJ 9.8 projections/geos.cpp(参考实现,重写为 Go):
//
//	设置(归一到 a=1):radius_g_1 = h/a;radius_g = 1 + radius_g_1;
//	                    C = radius_g² − 1;
//	椭球额外:radius_p = √(1−es);radius_p2 = 1−es;radius_p_inv2 = 1/(1−es)。
//	Forward:卫星视点几何,椭球先转地心纬 φ' = atan(rp2·tanφ);
//	        可见性判据(仅椭球):(radius_g−Vx)·Vx − Vy² − Vz²/rp² < 0 → ErrDomain。
//	Inverse:射线-椭球相交的三次方程求根 k = (−b−√det)/(2a);det<0 → ErrDomain。
//	球体:forward 无可见性检查(全盘可见,PROJ 语义);inverse 同构(半径=1)。
//	尺度:投影内 a-归一,显式乘 a 完成(同 PROJ pj_fwd)。
//
// sweep 语义:缺省 "y"(PROJ ≥ 6,决策 D1 paramString);+sweep=x → flip_axis。
// k_0 被 geos 忽略(与 eqc 同为方位类,PROJ 语义)。

// geosProj geos 投影实例(不可变,线程安全)。
type geosProj struct {
	crs    *CRS
	flip   bool    // sweep=x(sweep=y 为 false)
	aMul   float64 // 尺度(椭球长半轴 a;球体 = a)
	rg1    float64 // h/a
	rg     float64 // 1 + rg1
	cC     float64 // rg² − 1
	ell    bool
	rp     float64 // √(1−es)
	rp2    float64 // 1−es
	rpInv2 float64 // 1/(1−es)
	lon0   float64 // lon_0(度)
}

func newGeos(c *CRS) (Projection, error) {
	if c == nil {
		return nil, fmt.Errorf("proj: 零值 CRS 不可用,只能由 Parse 构造")
	}
	h, _ := paramFloat(c, "h", 0)
	rg1 := h / c.Ellipsoid.A
	if rg1 <= 0 || rg1 > 1e10 {
		return nil, fmt.Errorf("proj: geos +h=%v 非法(需 0 < h/a ≤ 1e10,PROJ 语义)", h)
	}
	sweep := paramString(c, "sweep", "y")
	if sweep != "x" && sweep != "y" {
		return nil, fmt.Errorf("proj: geos +sweep=%q 非法(应为 x 或 y)", sweep)
	}
	lon0, _ := paramFloat(c, "lon_0", 0)
	p := &geosProj{
		crs:  c,
		flip: sweep == "x",
		aMul: c.Ellipsoid.A,
		rg1:  rg1,
		rg:   1 + rg1,
		lon0: lon0,
	}
	p.cC = p.rg*p.rg - 1
	el := c.Ellipsoid
	if !el.Sphere {
		p.ell = true
		oneEs := 1 - el.ES
		p.rp = math.Sqrt(oneEs)
		p.rp2 = oneEs
		p.rpInv2 = 1 / oneEs
	}
	return p, nil
}

// CRS 应回传绑定实例(契约 §2.3)。
func (p *geosProj) CRS() *CRS { return p.crs }

// errGeosDomain geos 定义域外(可见性/det),契约 (NaN,NaN,ErrDomain)。
func errGeosDomain() error {
	return ErrDomain
}

// Forward lon/lat(度)→ x, y(米)。定义域仅在椭球 forward 有可见性判据;
// 球体无检查(全盘可见,PROJ 语义)。
func (p *geosProj) Forward(lon, lat float64) (x, y float64, err error) {
	if !validLonLat(lon, lat) {
		return math.NaN(), math.NaN(), invalidLonLatErr(lon, lat)
	}
	lam := normLonDeg(lon-p.lon0) * deg2rad
	phi := lat * deg2rad

	var Vx, Vy, Vz, tmp float64
	if p.ell {
		// 地心纬
		phi = math.Atan(p.rp2 * math.Tan(phi))
		sphi, cphi := math.Sincos(phi)
		sl, cl := math.Sincos(lam)
		r := p.rp / math.Hypot(p.rp*cphi, sphi)
		Vx = r * cl * cphi
		Vy = r * sl * cphi
		Vz = r * sphi
		// 可见性(PROJ geos.cpp e_forward)
		if ((p.rg-Vx)*Vx - Vy*Vy - Vz*Vz*p.rpInv2) < 0 {
			return math.NaN(), math.NaN(), errGeosDomain()
		}
	} else {
		sphi, cphi := math.Sincos(phi)
		sl, cl := math.Sincos(lam)
		Vx = cl * cphi
		Vy = sl * cphi
		Vz = sphi
	}
	tmp = p.rg - Vx
	var xr, yr float64 // a-归一
	if p.flip {
		xr = p.rg1 * math.Atan(Vy/math.Hypot(Vz, tmp))
		yr = p.rg1 * math.Atan(Vz/tmp)
	} else {
		xr = p.rg1 * math.Atan(Vy/tmp)
		yr = p.rg1 * math.Atan(Vz/math.Hypot(Vy, tmp))
	}
	return p.aMul * xr, p.aMul * yr, nil
}

// Inverse x, y(米)→ lon, lat(度)。
// 定义域:x/y 使射线与椭球无交(三次方程 det<0)→ (NaN,NaN,ErrDomain)。
func (p *geosProj) Inverse(x, y float64) (lon, lat float64, err error) {
	if math.IsNaN(x) || math.IsInf(x, 0) || math.IsNaN(y) || math.IsInf(y, 0) {
		return math.NaN(), math.NaN(), fmt.Errorf("proj: geos Inverse 输入非有限 (%v,%v)", x, y)
	}
	xn := x / p.aMul // a-归一(同 PROJ)
	yn := y / p.aMul

	var Vx, Vy, Vz float64
	Vx = -1
	if p.flip {
		Vz = math.Tan(yn / p.rg1)
		Vy = math.Tan(xn/p.rg1) * math.Hypot(1, Vz)
	} else {
		Vy = math.Tan(xn / p.rg1)
		Vz = math.Tan(yn/p.rg1) * math.Hypot(1, Vy)
	}
	// 三次方程判别式;椭球半径栅格下 a 项带 1/rp(Vz/rp 平方)
	var aa float64
	if p.ell {
		t := Vz / p.rp
		aa = Vy*Vy + t*t + Vx*Vx
	} else {
		aa = Vy*Vy + Vz*Vz + Vx*Vx
	}
	b := 2 * p.rg * Vx
	det := b*b - 4*aa*p.cC
	if det < 0 {
		return math.NaN(), math.NaN(), errGeosDomain()
	}
	k := (-b - math.Sqrt(det)) / (2 * aa)
	Vx = p.rg + k*Vx
	Vy *= k
	Vz *= k

	lam := math.Atan2(Vy, Vx)
	var phi float64
	if p.ell {
		phi = math.Atan(p.rpInv2 * Vz / math.Hypot(Vx, Vy))
	} else {
		phi = math.Atan(Vz / math.Hypot(Vx, Vy))
	}
	return normLonDeg(p.lon0 + lam*rad2deg), phi * rad2deg, nil
}

func init() {
	register("geos", newGeos)
}
