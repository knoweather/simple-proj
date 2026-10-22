package proj

import (
	"fmt"
	"math"
)

// impl_aeqd.go — Azimuthal Equidistant(方位等距)。
//
// 公式口径 PROJ 9.8 src/projections/aeqd.cpp(参考实现,重写为 Go),四种方面:
//
//	球体 极轴   x = rho·sinλ, y = ∓rho·cosλ,rho = π/2 − |φ|
//	球体 赤道/斜轴 c = 中心到点的球面角距,k = c/sin c(PROJ 用 hypot+atan2 的稳定形)
//	椭球 极轴   rho = |M(±π/2) − M(φ)|(子午线弧差,解析式)
//	椭球 赤道/斜轴 s12 = 大地线距离,azi1 = 大地线起始方位
//	              x = s12·sin(azi1), y = s12·cos(azi1) —— 靠 geodesic.go 的
//	              Karney 解,这也是 PROJ 的做法(不是解析公式)
//
// 域:球体斜轴的中心对跖点、球体极轴的对侧极点 → ErrDomain;椭球两模式全域可解。
// k_0 被 aeqd **忽略**(pyproj 实测:+k_0=0.9/2 与缺省逐位相同,与 eqc/geos/laea
// 同为「方位类不吃 k_0」);x_0/y_0 仍是末端仿射偏移(米)。

const (
	aeqdNPole = iota
	aeqdSPole
	aeqdEquit
	aeqdObliq
)

// aeqdEPS10 / aeqdTOL 与 aeqd.cpp 的同名宏一致。
const (
	aeqdEPS10 = 1e-10
	aeqdTOL   = 1e-14
)

type aeqdProj struct {
	crs            *CRS
	a              float64
	x0, y0         float64
	lon0, phi0     float64 // rad
	mode           int
	sinph0, cosph0 float64
	sphere         bool
	// 椭球极轴:子午线弧(M/a)与逆级数
	Mp   float64
	en0  float64
	F    [auxLatOrder]float64
	Finv [auxLatOrder]float64
	// 椭球斜轴:大地线(a=1 归一,同 PROJ 的 geod_init(&g, 1, f))
	g *geod
}

func newAeqd(c *CRS) (Projection, error) {
	if c == nil {
		return nil, fmt.Errorf("proj: 零值 CRS 不可用,只能由 Parse 构造")
	}
	lat0, _ := paramFloat(c, "lat_0", 0)
	lon0, _ := paramFloat(c, "lon_0", 0)
	x0, _ := paramFloat(c, "x_0", 0)
	y0, _ := paramFloat(c, "y_0", 0)
	if math.Abs(lat0) > 90 {
		return nil, fmt.Errorf("proj: aeqd +lat_0=%v 出界 [-90,90]", lat0)
	}
	el := c.Ellipsoid
	p := &aeqdProj{
		crs:    c,
		a:      el.A,
		x0:     x0,
		y0:     y0,
		lon0:   lon0 * deg2rad,
		phi0:   lat0 * deg2rad,
		sphere: el.Sphere,
	}
	// 方面判定(同 aeqd.cpp 的 setup)
	switch {
	case math.Abs(math.Abs(lat0)-90) < aeqdEPS10:
		if lat0 < 0 {
			p.mode = aeqdSPole
			p.sinph0 = -1
		} else {
			p.mode = aeqdNPole
			p.sinph0 = 1
		}
		p.cosph0 = 0
	case math.Abs(lat0) < aeqdEPS10:
		p.mode = aeqdEquit
		p.sinph0 = 0
		p.cosph0 = 1
	default:
		p.mode = aeqdObliq
		p.sinph0, p.cosph0 = math.Sincos(p.phi0)
	}

	if p.sphere {
		return p, nil
	}
	n := el.F / (2 - el.F)
	p.en0 = rectifyingRadius(n)
	p.F = mlCoeffs(n)
	p.Finv = mlInvCoeffs(n)
	switch p.mode {
	case aeqdNPole:
		p.Mp = mlfn(math.Pi/2, p.F, p.en0)
	case aeqdSPole:
		p.Mp = mlfn(-math.Pi/2, p.F, p.en0)
	default:
		p.g = newGeod(1, el.F)
	}
	return p, nil
}

func (p *aeqdProj) CRS() *CRS { return p.crs }

// scale 末端乘 a 并加偏移(对应 PROJ pj_fwd 的 ×a + x_0/y_0;k_0 不参与)。
func (p *aeqdProj) scale(x, y float64) (float64, float64) {
	return p.a*x + p.x0, p.a*y + p.y0
}

func (p *aeqdProj) Forward(lon, lat float64) (x, y float64, err error) {
	if !validLonLat(lon, lat) {
		return math.NaN(), math.NaN(), invalidLonLatErr(lon, lat)
	}
	lam := normLonDeg(lon-p.lon0*rad2deg) * deg2rad
	phi := lat * deg2rad

	var xr, yr float64
	if p.sphere {
		xr, yr, err = p.fwdSphere(lam, phi)
	} else {
		xr, yr, err = p.fwdEllipsoid(lam, phi)
	}
	if err != nil {
		return math.NaN(), math.NaN(), err
	}
	x, y = p.scale(xr, yr)
	return x, y, nil
}

func (p *aeqdProj) fwdSphere(lam, phi float64) (float64, float64, error) {
	coslam := math.Cos(lam)
	switch p.mode {
	case aeqdNPole, aeqdSPole:
		if p.mode == aeqdNPole {
			phi = -phi
			coslam = -coslam
		}
		if math.Abs(phi-math.Pi/2) < aeqdEPS10 {
			return 0, 0, domainErr("degenerate", "球体极轴投影的对侧极点无定义")
		}
		y := math.Pi/2 + phi
		return y * math.Sin(lam), y * coslam, nil
	default:
		sinphi, cosphi := math.Sincos(phi)
		sinlam := math.Sin(lam)
		cosphiCoslam := cosphi * coslam
		// c = 中心到点的球面角距;用 hypot+atan2 而不是 acos,中心与对跖点附近都稳
		cosc := p.sinph0*sinphi + p.cosph0*cosphiCoslam
		t := p.cosph0*sinphi - p.sinph0*cosphiCoslam
		sinc := math.Hypot(cosphi*sinlam, t)
		if cosc < aeqdTOL-1 {
			return 0, 0, domainErr("antipode", "投影中心的对跖点半径无穷")
		}
		k := 1.0
		if sinc != 0 {
			k = math.Atan2(sinc, cosc) / sinc
		}
		return k * cosphi * sinlam, k * t, nil
	}
}

func (p *aeqdProj) fwdEllipsoid(lam, phi float64) (float64, float64, error) {
	switch p.mode {
	case aeqdNPole, aeqdSPole:
		coslam := math.Cos(lam)
		if p.mode == aeqdNPole {
			coslam = -coslam
		}
		rho := math.Abs(p.Mp - mlfn(phi, p.F, p.en0))
		return rho * math.Sin(lam), rho * coslam, nil
	default:
		// 中心点本身:PROJ 直接给 (0,0),避免大地线在零长度上的方位不定
		if math.Abs(lam) < aeqdEPS10 && math.Abs(phi-p.phi0) < aeqdEPS10 {
			return 0, 0, nil
		}
		s12, azi1, _, ok := p.g.inverse(p.phi0*rad2deg, 0, phi*rad2deg, lam*rad2deg)
		if !ok {
			return 0, 0, domainErr("outside", "大地线反解失败")
		}
		az := azi1 * deg2rad
		return s12 * math.Sin(az), s12 * math.Cos(az), nil
	}
}

func (p *aeqdProj) Inverse(x, y float64) (lon, lat float64, err error) {
	if math.IsNaN(x) || math.IsInf(x, 0) || math.IsNaN(y) || math.IsInf(y, 0) {
		return math.NaN(), math.NaN(), fmt.Errorf("proj: aeqd Inverse 输入非有限 (%v,%v)", x, y)
	}
	// 先撤偏移与缩放,回到 a 归一的投影平面
	xr := (x - p.x0) / p.a
	yr := (y - p.y0) / p.a

	var lam, phi float64
	if p.sphere {
		lam, phi, err = p.invSphere(xr, yr)
	} else {
		lam, phi, err = p.invEllipsoid(xr, yr)
	}
	if err != nil {
		return math.NaN(), math.NaN(), err
	}
	return normLonDeg(p.lon0*rad2deg + lam*rad2deg), phi * rad2deg, nil
}

func (p *aeqdProj) invSphere(x, y float64) (lam, phi float64, err error) {
	crh := math.Hypot(x, y)
	if crh > math.Pi {
		if crh-aeqdEPS10 > math.Pi {
			return 0, 0, domainErr("outside", "超出球面 aeqd 的半径范围(>π)")
		}
		crh = math.Pi
	} else if crh < aeqdEPS10 {
		return 0, p.phi0, nil
	}
	switch p.mode {
	case aeqdNPole:
		return math.Atan2(x, -y), math.Pi/2 - crh, nil
	case aeqdSPole:
		return math.Atan2(x, y), crh - math.Pi/2, nil
	}
	sinc, cosc := math.Sincos(crh)
	if p.mode == aeqdEquit {
		phi = math.Asin(clampUnit(y * sinc / crh))
		x *= sinc
		y = cosc * crh
	} else {
		phi = math.Asin(clampUnit(cosc*p.sinph0 + y*sinc*p.cosph0/crh))
		y = (cosc - p.sinph0*math.Sin(phi)) * crh
		x *= sinc * p.cosph0
	}
	// 注意:PROJ 此处写的是 `lp.lam = xy.y == 0. ? 0. : atan2(xy.x, xy.y)`。
	// 那个 ==0 短路在刀刃点上会出事:(lat_0=45, lon_0=10) 投 (100,45) 时
	// y' 只是 ~1e-16 的舍入残差,C 侧恰好舍成 0 就把经度直接判成中心经度,
	// 一次往返丢掉 90°。中心点已由上面 crh < EPS10 提前返回,这里直接用
	// atan2(x, 0) = ±90° 才是对的 —— 这是与 PROJ 唯一有意保留的差异。
	lam = math.Atan2(x, y)
	return lam, phi, nil
}

func (p *aeqdProj) invEllipsoid(x, y float64) (lam, phi float64, err error) {
	s12 := math.Hypot(x, y)
	if s12 < aeqdEPS10 {
		return 0, p.phi0, nil
	}
	switch p.mode {
	case aeqdNPole:
		m := p.Mp - s12
		return math.Atan2(x, -y), mlfnInverse(m/p.en0, p.Finv), nil
	case aeqdSPole:
		m := p.Mp + s12
		return math.Atan2(x, y), mlfnInverse(m/p.en0, p.Finv), nil
	default:
		azi1 := math.Atan2(x, y) * rad2deg
		lat2, lon2, _, ok := p.g.direct(p.phi0*rad2deg, 0, azi1, s12)
		if !ok {
			return 0, 0, domainErr("outside", "大地线正解失败")
		}
		return lon2 * deg2rad, lat2 * deg2rad, nil
	}
}

// clampUnit 把 |v| > 1 的 asin 参数夹回定义域(PROJ aasin 的同义)。
func clampUnit(v float64) float64 {
	if v > 1 {
		return 1
	}
	if v < -1 {
		return -1
	}
	return v
}

func init() { register("aeqd", newAeqd) }
