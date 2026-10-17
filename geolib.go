package proj

// geolib.go — shared spherical/ellipsoidal math helpers. Owner: AI-B (COORDINATION §1).
//
// Everything here is in radians, stateless and allocation-free. Only the adapter
// (impl_*.go) converts degrees/metres; these helpers never do.
//
// Sources: Snyder, "Map Projections — A Working Manual" (USGS PP 1395), ch. 1
// (isometric/conformal latitude, Gudermannian); cross-checked against PROJ 9.8.1
// through the pyproj golden fixtures in xcheck/.

import "math"

// Unit helpers used by the impl_*.go adapters. Prefixed with geo* so they cannot
// collide with whatever A lands in proj.go (COORDINATION §1: these files are A's).
func geoDegToRad(d float64) float64 { return d * math.Pi / 180 }
func geoRadToDeg(r float64) float64 { return r * 180 / math.Pi }

// geoNormLon implements the frozen output rule: [-180, 180).
// Go's math.Mod keeps the sign of the dividend, so the negative branch is folded
// explicitly — otherwise lon = -190 stays -190 instead of becoming 170.
func geoNormLon(lon float64) float64 {
	m := math.Mod(lon+180, 360)
	if m < 0 {
		m += 360
	}
	return m - 180
}

// Gudermannian: gd(x) = arcsin(tanh x) = 2 atan(tanh(x/2)).
func Gudermannian(x float64) float64 {
	return 2 * math.Atan(math.Tanh(x/2))
}

// InvGudermannian: gd^-1(phi) = ln tan(pi/4 + phi/2) = asinh(tan phi).
func InvGudermannian(phi float64) float64 {
	return math.Log(math.Tan(math.Pi/4 + phi/2))
}

// IsometricLat is the isometric latitude psi of geodetic latitude phi for first
// eccentricity e:
//
//	psi = ln tan(pi/4 + phi/2) + (e/2) ln((1 - e sin phi)/(1 + e sin phi))
//
// e == 0 reduces to InvGudermannian.
func IsometricLat(phi, e float64) float64 {
	if e == 0 {
		return InvGudermannian(phi)
	}
	s := e * math.Sin(phi)
	return InvGudermannian(phi) + 0.5*e*math.Log((1-s)/(1+s))
}

// ExpNegIsometric is exp(-psi). PROJ calls this value pj_tsfn and uses it as the
// polar-aspect radius factor of the ellipsoidal stereographic projection; it equals
// tan(pi/4 - X/2) where X is the conformal latitude. Verified against pyproj 3.8.0
// (PROJ 9.8.1) on the golden grid — note it is NOT tan(pi/4 - phi/2) once e > 0.
func ExpNegIsometric(phi, e float64) float64 {
	return math.Exp(-IsometricLat(phi, e))
}

// ConformalLat is the conformal latitude X of geodetic latitude phi: X = gd(psi).
func ConformalLat(phi, e float64) float64 {
	if e == 0 {
		return phi
	}
	return Gudermannian(IsometricLat(phi, e))
}

// —— authalic (equal-area) latitude ——
//
// Needed by impl_laea.go (B) and, for the equal-area branch, by A's impl_eqc.go.
// Raised as C#4 in PROGRESS-B.md so the ownership of this block is explicit.

// AuthalicMaxTerms is the number of series terms kept. PROJ 9 evaluates P->n
// coefficients; the series is already converged by term 8 for every ellipsoid in
// the fixtures, so 12 is safe and reproduces pyproj 3.8.0 / PROJ 9.8.1 to the last
// double on the 33x33 grid (max forward diff 6.1e-7 m on golden_laea.json).
const AuthalicMaxTerms = 12

// AuthalicSeries holds the zone-area series coefficients for one eccentricity:
//
//	C_i = (i + 1) / (2i + 1) * e2^i
//	q(phi) = 2 (1 - e2) * sum_i C_i sin^(2i+1) phi
//	sin(authalic phi) = q(phi) / q(pi/2)
//
// This is the definition of the authalic latitude: the zone area between the equator
// and geodetic latitude phi on the ellipsoid equals the zone area between the equator
// and authalic latitude mu on the authalic sphere. The integrand is
// (1 - e2 sin^2 t)^-2, which is where the (i+1) comes from.
//
// Caution (this cost me a whole debugging session): the frequently transcribed
// C_i = ((2i-1)!!/(2i)!!) / (2i+1) is the series for the *meridian arc* integrand
// (1 - e2 sin^2 t)^-3/2, not for the zone area. Using it gives authalic(45 deg) =
// 44.9812 deg instead of 44.8717 deg — a 6.4e6 m error in laea forward. Snyder's
// direct series mu = phi - (e2/3) sin^3 phi - ... is a third, different value
// (44.9548 deg); only the (i+1)/(2i+1) form matches PROJ/pyproj.
type AuthalicSeries struct {
	e2    float64
	coef  [AuthalicMaxTerms]float64 // C_i, for q(phi)
	dcoef [AuthalicMaxTerms]float64 // (i+1) e2^i, for dq/dphi
	qp    float64                   // q(pi/2)
}

// NewAuthalicSeries builds the coefficient table for e2 (first eccentricity squared).
func NewAuthalicSeries(e2 float64) *AuthalicSeries {
	s := &AuthalicSeries{e2: e2}
	p := 1.0
	sum := 0.0
	for i := 0; i < AuthalicMaxTerms; i++ {
		s.coef[i] = float64(i+1) / float64(2*i+1) * p
		s.dcoef[i] = float64(i+1) * p
		sum += s.coef[i]
		p *= e2
	}
	s.qp = 2 * (1 - e2) * sum
	return s
}

// Q returns q(phi) evaluated from sin(phi). e2 == 0 gives 2 sin phi, which is the
// spherical limit, so the laea kernel degenerates correctly for a sphere.
func (s *AuthalicSeries) Q(sinphi float64) float64 {
	tot, sp := 0.0, sinphi
	for i := 0; i < AuthalicMaxTerms; i++ {
		tot += s.coef[i] * sp
		sp *= sinphi * sinphi
	}
	return 2 * (1 - s.e2) * tot
}

// AuthalicLat is the authalic latitude of geodetic latitude phi (radians in and out).
func (s *AuthalicSeries) AuthalicLat(phi float64) float64 {
	if s.e2 == 0 {
		return phi
	}
	return math.Asin(s.Q(math.Sin(phi)) / s.qp)
}

// LatFromAuthalic inverts AuthalicLat by Newton on f(phi) = q(phi) - qp sin(mu),
// f'(phi) = 2 (1 - e2) cos phi * sum_i (i + 1) e2^i sin^(2i) phi.
// Note the two sums use different coefficient tables: q() needs C_i, the derivative
// needs (i+1) e2^i. Returns NaN when the iteration cannot converge.
func (s *AuthalicSeries) LatFromAuthalic(mu float64) float64 {
	if s.e2 == 0 {
		return mu
	}
	target := s.qp * math.Sin(mu)
	phi := mu
	for i := 0; i < 8; i++ {
		sp := math.Sin(phi)
		sumC, sumD, p := 0.0, 0.0, 1.0
		for j := 0; j < AuthalicMaxTerms; j++ {
			sumC += s.coef[j] * p
			sumD += s.dcoef[j] * p
			p *= sp * sp
		}
		f := 2*(1-s.e2)*sumC*sp - target
		d := 2 * (1 - s.e2) * sumD * math.Cos(phi)
		if d == 0 {
			return math.NaN()
		}
		next := phi - f/d
		if math.Abs(next-phi) < 1e-13 {
			return next
		}
		phi = next
	}
	return phi
}

// LatFromIsometric inverts IsometricLat. Newton on psi; dpsi/dphi =
// (1 - e^2) / (cos phi * (1 - e^2 sin^2 phi)). Starts from the conformal latitude,
// which is already within ~1e-2 rad, so 4 iterations are plenty.
// Returns NaN when psi cannot correspond to a geodetic latitude.
func LatFromIsometric(psi, e float64) float64 {
	if e == 0 {
		return Gudermannian(psi)
	}
	phi := Gudermannian(psi)
	e2 := e * e
	for i := 0; i < 8; i++ {
		s := math.Sin(phi)
		c := math.Cos(phi)
		dpsi := (1 - e2) / (c * (1 - e2*s*s))
		if dpsi == 0 {
			return math.NaN()
		}
		next := phi - (IsometricLat(phi, e)-psi)/dpsi
		if math.Abs(next-phi) < 1e-13 {
			return next
		}
		phi = next
	}
	return phi
}
