package proj

// impl_laea.go — Lambert azimuthal equal-area projection.
//
// Formula source: Snyder, "Map Projections — A Working Manual" (USGS PP 1395), ch. 14
// (ellipsoidal azimuthal equal-area). Structure cross-checked against PROJ 9.8.1
// src/projections/laea.cpp through the pyproj golden fixtures. Rewritten, not copied.
//
// Kernel unit: radians in, metres out. Inverse returns ok == false for out-of-domain;
// the adapter turns that into ErrDomain.
//
// Note on +k_0: PROJ's laea does not use k_0 at all (verified with pyproj: the same
// point gives identical coordinates for +k_0=1 and +k_0=2). The kernel signature has
// no k0 argument for laea, which is consistent. The adapter therefore does not
// multiply by k_0 either.

import (
	"fmt"
	"math"
)

// Modes as PROJ numbers them (laea.cpp anonymous enum).
const (
	laeaSPole = iota
	laeaNPole
	laeaEquit
	laeaOblq
)

// laeaEPS10 is PROJ's EPS10: mode selection and the "point maps to infinity" test.
const laeaEPS10 = 1e-10

// laeaMinRho is PROJ's 1e-15 threshold in the polar forward branch: below it the
// projection returns exactly (0, 0) rather than sqrt(q).
const laeaMinRho = 1e-15

// laeaKernel holds the precomputed constants. Immutable, so a Projection built on it
// is thread-safe.
type laeaKernel struct {
	a, e, es float64
	lat0     float64 // radians
	lon0     float64 // radians
	mode     int
	ser      *AuthalicSeries
	qp, rq   float64
	dd, xmf  float64
	ymf      float64
	sinb1    float64
	cosb1    float64
}

// newLaeaKernel mirrors PROJ's laea_setup. lat0/lon0 are radians, a/b metres.
func newLaeaKernel(a, b, lat0, lon0 float64) *laeaKernel {
	e2 := (a*a - b*b) / (a * a)
	if e2 < 0 {
		e2 = 0
	}
	q := &laeaKernel{
		a: a, e: math.Sqrt(e2), es: e2,
		lat0: lat0, lon0: lon0,
		ser: NewAuthalicSeries(e2),
	}
	t := math.Abs(lat0)
	switch {
	case t > math.Pi/2+laeaEPS10:
		q.mode = -1 // |lat_0| > 90 deg: PROJ rejects this at setup; the adapter does too
	case math.Abs(t-math.Pi/2) < laeaEPS10:
		if lat0 < 0 {
			q.mode = laeaSPole
		} else {
			q.mode = laeaNPole
		}
	case t < laeaEPS10:
		q.mode = laeaEquit
	default:
		q.mode = laeaOblq
	}

	q.qp = q.ser.qp
	switch q.mode {
	case laeaSPole, laeaNPole:
		q.dd = 1
		q.xmf, q.ymf = 1, 1 // unused in the polar branch
	case laeaEquit:
		q.rq = math.Sqrt(0.5 * q.qp)
		q.dd = 1 / q.rq
		q.xmf = 1
		q.ymf = 0.5 * q.qp
	case laeaOblq:
		q.rq = math.Sqrt(0.5 * q.qp)
		sinphi, cosphi := math.Sin(lat0), math.Cos(lat0)
		b1 := q.ser.AuthalicLat(lat0)
		q.sinb1, q.cosb1 = math.Sin(b1), math.Cos(b1)
		q.dd = cosphi / (math.Sqrt(1-e2*sinphi*sinphi) * q.rq * q.cosb1)
		q.xmf = q.rq * q.dd
		q.ymf = q.rq / q.dd
	}
	return q
}

// forward: radians in, metres out.
func (q *laeaKernel) forward(lon, lat float64) (x, y float64) {
	lam := lon - q.lon0
	coslam, sinlam := math.Cos(lam), math.Sin(lam)
	sinphi := math.Sin(lat)

	switch q.mode {
	case laeaOblq, laeaEquit:
		b0 := q.ser.AuthalicLat(lat)
		sb, cb := math.Sin(b0), math.Cos(b0)
		var denom, yterm float64
		if q.mode == laeaOblq {
			denom = 1 + q.sinb1*sb + q.cosb1*cb*coslam
			yterm = q.cosb1*sb - q.sinb1*cb*coslam
		} else {
			denom = 1 + cb*coslam
			yterm = sb
		}
		if denom <= laeaEPS10 { // antipode of the projection centre: infinite radius
			return math.NaN(), math.NaN()
		}
		denom = math.Sqrt(2 / denom)
		return q.xmf * denom * cb * sinlam * q.a, q.ymf * denom * yterm * q.a
	default: // polar aspect
		var bb, qq float64
		if q.mode == laeaNPole {
			bb = math.Pi/2 + lat
			qq = q.qp - q.ser.Q(sinphi)
		} else {
			bb = lat - math.Pi/2
			qq = q.qp + q.ser.Q(sinphi)
		}
		if math.Abs(bb) < laeaEPS10 { // the far pole maps to infinity
			return math.NaN(), math.NaN()
		}
		if qq >= laeaMinRho {
			bb = math.Sqrt(qq)
			if q.mode == laeaSPole {
				return bb * sinlam * q.a, bb * coslam * q.a
			}
			return bb * sinlam * q.a, -bb * coslam * q.a
		}
		return 0, 0
	}
}

// inverse: metres in, radians out. ok == false means outside the domain.
func (q *laeaKernel) inverse(x, y float64) (lon, lat float64, ok bool) {
	xx, yy := x/q.a, y/q.a

	switch q.mode {
	case laeaEquit, laeaOblq:
		xx /= q.dd
		yy *= q.dd
		rho := math.Hypot(xx, yy)
		if rho < laeaEPS10 {
			return q.lon0, q.lat0, true
		}
		arg := 0.5 * rho / q.rq
		if arg > 1 {
			return math.NaN(), math.NaN(), false
		}
		sCe := 2 * math.Asin(arg)
		cCe := math.Cos(sCe)
		sCe = math.Sin(sCe)
		xx *= sCe
		var ab, y2 float64
		if q.mode == laeaOblq {
			ab = cCe*q.sinb1 + yy*sCe*q.cosb1/rho
			// PROJ recomputes y before atan2; without this line the longitude is wrong.
			y2 = rho*q.cosb1*cCe - yy*q.sinb1*sCe
		} else {
			ab = yy * sCe / rho
			y2 = rho * cCe
		}
		if math.Abs(ab) > 1 {
			return math.NaN(), math.NaN(), false
		}
		phi := q.ser.LatFromAuthalic(math.Asin(ab))
		if math.IsNaN(phi) {
			return math.NaN(), math.NaN(), false
		}
		return q.lon0 + math.Atan2(xx, y2), phi, true
	default: // polar aspect
		if q.mode == laeaNPole {
			yy = -yy
		}
		qq := xx*xx + yy*yy
		if qq == 0 {
			return q.lon0, q.lat0, true
		}
		ab := 1 - qq/q.qp
		if q.mode == laeaSPole {
			ab = -ab
		}
		if math.Abs(ab) > 1 {
			return math.NaN(), math.NaN(), false
		}
		phi := q.ser.LatFromAuthalic(math.Asin(ab))
		if math.IsNaN(phi) {
			return math.NaN(), math.NaN(), false
		}
		return q.lon0 + math.Atan2(xx, yy), phi, true
	}
}

// —— adapter entry points ——

func laeaForward(a, b, lat0, lon0 float64, lon, lat float64) (x, y float64) {
	return newLaeaKernel(a, b, lat0, lon0).forward(lon, lat)
}

func laeaInverse(a, b, lat0, lon0 float64, x, y float64) (lon, lat float64, ok bool) {
	return newLaeaKernel(a, b, lat0, lon0).inverse(x, y)
}

// —— adapter ——

type laeaProj struct {
	crs    *CRS
	kernel *laeaKernel
	x0, y0 float64 // false easting / northing, metres
}

func newLaea(c *CRS) (Projection, error) {
	a, b := c.Ellipsoid.A, c.Ellipsoid.B
	if a <= 0 || b <= 0 || b > a {
		return nil, fmt.Errorf("proj(laea): 椭球半轴非法 a=%v b=%v", a, b)
	}
	lat0, _ := paramFloat(c, "lat_0", 0)
	lon0, _ := paramFloat(c, "lon_0", 0)
	x0, _ := paramFloat(c, "x_0", 0)
	y0, _ := paramFloat(c, "y_0", 0)
	if math.Abs(lat0) > 90 {
		return nil, fmt.Errorf("proj(laea): lat_0 非法 |lat_0| > 90 (%v)", lat0)
	}

	rLat0, rLon0 := geoDegToRad(lat0), geoDegToRad(lon0)
	return &laeaProj{
		crs:    c,
		kernel: newLaeaKernel(a, b, rLat0, rLon0),
		x0:     x0,
		y0:     y0,
	}, nil
}

func (s *laeaProj) CRS() *CRS { return s.crs }

func (s *laeaProj) Forward(lon, lat float64) (float64, float64, error) {
	if math.IsNaN(lon) || math.IsNaN(lat) || math.Abs(lat) > 90 || math.Abs(lon) > 360 {
		return math.NaN(), math.NaN(), fmt.Errorf("proj(laea): 输入出界 lon=%v lat=%v", lon, lat)
	}
	x, y := s.kernel.forward(geoDegToRad(lon), geoDegToRad(lat))
	if math.IsNaN(x) || math.IsNaN(y) {
		return math.NaN(), math.NaN(), domainErr(laeaDomainReason(s.kernel, lat), fmt.Sprintf("lon=%v lat=%v", lon, lat))
	}
	return s.x0 + x, s.y0 + y, nil
}

func (s *laeaProj) Inverse(x, y float64) (float64, float64, error) {
	lon, lat, ok := s.kernel.inverse(x-s.x0, y-s.y0)
	if !ok {
		return math.NaN(), math.NaN(), domainErr("outside", fmt.Sprintf("x=%v y=%v", x, y))
	}
	return geoNormLon(geoRadToDeg(lon)), geoRadToDeg(lat), nil
}

func init() { register("laea", newLaea) }
