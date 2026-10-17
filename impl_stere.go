package proj

// impl_stere.go — Stereographic projection. Owner: AI-B (COORDINATION §1).
//
// Formula source: Snyder, "Map Projections — A Working Manual" (USGS PP 1395),
// ch. 13 (general/polar aspect, ellipsoidal). Structure cross-checked against
// PROJ 9.8.1 src/projections/stere.cpp through the pyproj golden fixtures.
// Rewritten, not copied (COORDINATION §6 red line 2). See FORMULA-NOTES.md.
//
// Kernel unit: radians in, metres out (COORDINATION §2.5). Inverse returns
// ok == false for out-of-domain; the adapter turns that into ErrDomain.

import (
	"fmt"
	"math"
)

// Modes as PROJ numbers them (stere.cpp anonymous enum).
const (
	stereSPole = iota
	stereNPole
	stereOblq
	stereEquit
)

// stereEPS10 is PROJ's EPS10, used for mode selection and the lat_ts == 90 test.
const stereEPS10 = 1e-10

// stereKernel holds the precomputed constants. Immutable, so a Projection built on
// it is thread-safe (COORDINATION §2.5).
type stereKernel struct {
	a, e  float64
	lat0  float64 // radians
	lon0  float64 // radians
	k     float64 // effective scale factor (see stereEffectiveK)
	mode  int
	akm1  float64
	sinX1 float64
	cosX1 float64
}

// stereEffectiveK folds +lat_ts into the plain k0 so that the frozen kernel
// signature (a, b, lat0, lon0, k, lon, lat) stays untouched. The kernel always builds
// akm1 from k as "2k / base" (polar) or "2k cos(lat0)/sqrt(1-e2 sin^2 lat0)"
// (OBLIQ/EQUIT), so k must be the value that reproduces PROJ's akm1 under that rule.
//
// PROJ semantics (verified with pyproj 3.8.0):
//   - OBLIQ / EQUIT ignore +lat_ts entirely -> k = k0.
//   - polar aspect with |lat_ts| = 90 -> akm1 = 2 k0 / base -> k = k0.
//   - polar aspect with lat_ts != 90 -> akm1 is the raw value and PROJ refuses
//     k_0 != 1 there ("lat_ts != lat_0 and k != 1 not supported for Polar
//     Stereographic"), so k = akm1_raw * base / 2.
func stereEffectiveK(a, b, lat0, latTs, k0 float64) float64 {
	e := math.Sqrt((a*a - b*b) / (a * a))
	base := math.Sqrt(math.Pow(1+e, 1+e) * math.Pow(1-e, 1-e))
	if math.Abs(math.Abs(lat0)-math.Pi/2) >= stereEPS10 {
		return k0
	}
	if math.Abs(math.Abs(latTs)-math.Pi/2) < stereEPS10 {
		return k0
	}
	t := math.Sin(latTs)
	akm1 := math.Cos(latTs) / ExpNegIsometric(latTs, e)
	akm1 /= math.Sqrt(1 - e*e*t*t)
	return akm1 * base / 2
}

// newStereKernel mirrors PROJ's stere_setup. k is the effective scale factor
// (stereEffectiveK), lat0/lon0 are radians, a/b metres.
func newStereKernel(a, b, lat0, lon0, k float64) *stereKernel {
	e := math.Sqrt((a*a - b*b) / (a * a))
	q := &stereKernel{a: a, e: e, lat0: lat0, lon0: lon0, k: k}
	switch {
	case math.Abs(math.Abs(lat0)-math.Pi/2) < stereEPS10:
		if lat0 < 0 {
			q.mode = stereSPole
		} else {
			q.mode = stereNPole
		}
		// base == 1 when e == 0, so this also covers the spherical case.
		q.akm1 = 2 * k / math.Sqrt(math.Pow(1+e, 1+e)*math.Pow(1-e, 1-e))
	case math.Abs(lat0) > stereEPS10:
		q.mode = stereOblq
		t := math.Sin(lat0)
		q.akm1 = 2 * k * math.Cos(lat0) / math.Sqrt(1-e*e*t*t)
		X := ConformalLat(lat0, e)
		q.sinX1, q.cosX1 = math.Sin(X), math.Cos(X)
	default:
		q.mode = stereEquit
		t := math.Sin(lat0)
		q.akm1 = 2 * k * math.Cos(lat0) / math.Sqrt(1-e*e*t*t)
	}
	return q
}

// forward: radians in, metres out.
func (q *stereKernel) forward(lon, lat float64) (x, y float64) {
	lam := lon - q.lon0
	coslam, sinlam := math.Cos(lam), math.Sin(lam)
	switch q.mode {
	case stereOblq:
		X := ConformalLat(lat, q.e)
		sinX, cosX := math.Sin(X), math.Cos(X)
		denom := q.cosX1 * (1 + q.sinX1*sinX + q.cosX1*cosX*coslam)
		if denom == 0 {
			return math.NaN(), math.NaN()
		}
		A := q.akm1 * q.a / denom
		return A * cosX * sinlam, A * (q.cosX1*sinX - q.sinX1*cosX*coslam)
	case stereEquit:
		X := ConformalLat(lat, q.e)
		sinX, cosX := math.Sin(X), math.Cos(X)
		denom := 1 + cosX*coslam
		if denom == 0 {
			return math.NaN(), math.NaN()
		}
		A := q.akm1 * q.a / denom
		return A * cosX * sinlam, A * sinX
	default: // polar aspect; S_POLE is N_POLE with phi and coslam negated
		phi, cl := lat, coslam
		if q.mode == stereSPole {
			phi, cl = -phi, -cl
		}
		if math.Abs(phi-math.Pi/2) < 1e-15 {
			return 0, 0
		}
		rho := q.akm1 * q.a * ExpNegIsometric(phi, q.e)
		return rho * sinlam, -rho * cl
	}
}

// inverse: metres in, radians out. ok == false means outside the domain.
func (q *stereKernel) inverse(x, y float64) (lon, lat float64, ok bool) {
	rho := math.Hypot(x, y) / q.a
	switch q.mode {
	case stereOblq, stereEquit:
		tp := 2 * math.Atan2(rho*q.cosX1, q.akm1)
		cosphi, sinphi := math.Cos(tp), math.Sin(tp)
		phiL := 0.0
		if rho == 0 {
			phiL = math.Asin(cosphi * q.sinX1)
		} else {
			phiL = math.Asin(cosphi*q.sinX1 + y*sinphi*q.cosX1/rho)
		}
		if math.IsNaN(phiL) {
			return math.NaN(), math.NaN(), false
		}
		tp = math.Tan(math.Pi/4 + phiL/2)
		phi, ok := iterateGeodetic(q.e, tp, phiL, math.Pi/2, 0.5*q.e)
		if !ok {
			return math.NaN(), math.NaN(), false
		}
		lam := 0.0
		if q.mode == stereEquit {
			c := math.Cos(tp)
			if c != 0 || x != 0 {
				lam = math.Atan2(x*math.Sin(tp), c*rho)
			}
		} else {
			c := math.Cos(tp) - q.sinX1*math.Sin(phi)
			if c != 0 || x != 0 {
				lam = math.Atan2(x*math.Sin(tp)*q.cosX1, c*rho)
			}
		}
		return q.lon0 + lam, phi, true
	default: // polar aspect
		tp := -rho / q.akm1
		phiL := math.Pi/2 - 2*math.Atan(tp)
		phi, ok := iterateGeodetic(q.e, tp, phiL, -math.Pi/2, -0.5*q.e)
		if !ok {
			return math.NaN(), math.NaN(), false
		}
		if q.mode == stereSPole {
			phi = -phi
			return q.lon0 + atan2Safe(x, y), phi, true
		}
		// PROJ negates y before atan2 for the north pole.
		return q.lon0 + atan2Safe(x, -y), phi, true
	}
}

// iterateGeodetic is PROJ's fixed-point loop that turns the conformal latitude
// back into the geodetic one. Returns false when it does not converge, which PROJ
// reports as "outside projection domain".
func iterateGeodetic(e, tp, phiL, halfpi, halfe float64) (float64, bool) {
	if e == 0 {
		return phiL, true
	}
	phi := phiL
	for i := 0; i < 8; i++ {
		s := e * math.Sin(phi)
		next := 2*math.Atan(tp*math.Pow((1+s)/(1-s), halfe)) - halfpi
		if math.Abs(next-phi) < 1e-10 {
			return next, true
		}
		phi = next
	}
	return math.NaN(), false
}

func atan2Safe(x, y float64) float64 {
	if x == 0 && y == 0 {
		return 0
	}
	return math.Atan2(x, y)
}

// —— frozen contract entry points (COORDINATION §2.3) ——
//
// These are the exact signatures A's proj.go will call. They assume lat_ts is
// already folded into k (see stereEffectiveK); the adapter does that once.

func stereForward(a, b, lat0, lon0, k float64, lon, lat float64) (x, y float64) {
	return newStereKernel(a, b, lat0, lon0, k).forward(lon, lat)
}

func stereInverse(a, b, lat0, lon0, k float64, x, y float64) (lon, lat float64, ok bool) {
	return newStereKernel(a, b, lat0, lon0, k).inverse(x, y)
}

// —— adapter (contract landed 2026-10-17: proj.go / crs.go are A's) ——

type stereProj struct {
	crs    *CRS
	kernel *stereKernel
	x0, y0 float64 // false easting / northing, metres
}

func newStere(c *CRS) (Projection, error) {
	a, b := c.Ellipsoid.A, c.Ellipsoid.B
	if a <= 0 || b <= 0 || b > a {
		return nil, fmt.Errorf("proj(stere): 椭球半轴非法 a=%v b=%v", a, b)
	}
	lat0, _ := paramFloat(c, "lat_0", 0)
	lon0, _ := paramFloat(c, "lon_0", 0)
	latTs, _ := paramFloat(c, "lat_ts", 0)
	k0, _ := paramFloat(c, "k_0", 1)
	x0, _ := paramFloat(c, "x_0", 0)
	y0, _ := paramFloat(c, "y_0", 0)

	rLat0, rLon0, rLatTs := geoDegToRad(lat0), geoDegToRad(lon0), geoDegToRad(latTs)
	// PROJ refuses k_0 != 1 in the polar aspect when lat_ts is not the pole itself
	// ("lat_ts != lat_0 and k != 1 not supported for Polar Stereographic") — verified
	// with pyproj 3.8.0. Match that instead of silently folding a scale PROJ rejects.
	if math.Abs(math.Abs(rLat0)-math.Pi/2) < stereEPS10 &&
		math.Abs(math.Abs(rLatTs)-math.Pi/2) >= stereEPS10 && k0 != 1 {
		return nil, fmt.Errorf("proj(stere): 极冠模式 lat_ts=%v 与 k_0=%v 不能同时使用(PROJ 拒收)", latTs, k0)
	}
	k := stereEffectiveK(a, b, rLat0, rLatTs, k0)
	return &stereProj{
		crs:    c,
		kernel: newStereKernel(a, b, rLat0, rLon0, k),
		x0:     x0,
		y0:     y0,
	}, nil
}

func (s *stereProj) CRS() *CRS { return s.crs }

func (s *stereProj) Forward(lon, lat float64) (float64, float64, error) {
	if math.IsNaN(lon) || math.IsNaN(lat) || math.Abs(lat) > 90 || math.Abs(lon) > 360 {
		return math.NaN(), math.NaN(), fmt.Errorf("proj(stere): 输入出界 lon=%v lat=%v", lon, lat)
	}
	x, y := s.kernel.forward(geoDegToRad(lon), geoDegToRad(lat))
	if math.IsNaN(x) || math.IsNaN(y) {
		return math.NaN(), math.NaN(), ErrDomain
	}
	return s.x0 + x, s.y0 + y, nil
}

func (s *stereProj) Inverse(x, y float64) (float64, float64, error) {
	lon, lat, ok := s.kernel.inverse(x-s.x0, y-s.y0)
	if !ok {
		return math.NaN(), math.NaN(), ErrDomain
	}
	return geoNormLon(geoRadToDeg(lon)), geoRadToDeg(lat), nil
}

func init() { register("stere", newStere) }
