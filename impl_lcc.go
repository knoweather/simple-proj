package proj

// impl_lcc.go — Lambert conformal conic projection (ellipsoidal, two standard
// parallels).
//
// Formula source: Snyder, "Map Projections — A Working Manual" (USGS PP 1395),
// ch. 14. Structure cross-checked against PROJ 9.8.1 src/projections/lcc.cpp
// (code rewritten, not copied).
//
// Derivation note: writing the map as (φ, λ) → (ρ(φ), n·(λ−λ0)) in polar form,
// conformality requires dρ/dφ = −n·ρ·M/(N·cosφ), and M/(N·cosφ) integrates to
// the isometric latitude ψ. Hence ρ(φ) = C·exp(−n·ψ(φ)) with ψ = geolib's
// IsometricLat. Scale 1 along both standard parallels pins
//   n = ln(m1/m2) / ln(t1/t2),  C = a·k0·m1·exp(n·ψ1)/n
// (m = msfn, t = pj_tsfn = exp(−ψ), both in radians); the single-parallel
// limit is n = sin φ1. The inverse reuses geolib's LatFromIsometric — no new
// latitude algebra.
//
// PROJ 9.8.1 lcc.cpp semantics mirrored exactly (pyproj 3.8.0 verified):
//   - +k_0 scales ρ; |lat_1 + lat_2| < EPS10 is rejected at setup
//     ("|lat_1 + lat_2| should be > 0" — kills lat_1 = −lat_2, n = 0);
//     |lat_1|, |lat_2| must be < 90°.
//   - n > 0 (northern cone, lat_1 < lat_2): north pole is the apex → (0, ρ0);
//     south pole is outside the domain. n < 0 (southern cone) is the mirror:
//     ρ0 < 0 and the south pole is the apex. The sign of n comes out of the
//     ln(m1/m2)/ln(t1/t2) formula naturally — no branch needed.
//   - The far pole (φ·n <= 0) is "outside projection domain" in PROJ
//     (pyproj shows (inf, inf) there only because errcheck defaults to False).
//     Our kernel returns NaN, the adapter maps that to ErrDomain.

import (
	"fmt"
	"math"
)

// lccEPS10 is PROJ's EPS10: the pole test and the secant-vs-tangent decision.
const lccEPS10 = 1e-10

// lccKernel holds the precomputed constants. Immutable, so a Projection built
// on it is thread-safe.
type lccKernel struct {
	a, e, es float64
	lat0     float64 // radians
	lon0     float64 // radians
	n        float64 // cone constant (positive: northern, negative: southern)
	c        float64 // ρ = c · exp(−n·ψ(φ)); carries the sign of n
	rho0     float64 // ρ(φ0); carries the sign of n
}

// lccMSfn is PROJ's pj_msfn: m(φ) = cosφ / sqrt(1 − e² sin²φ).
func lccMSfn(sinphi, cosphi, es float64) float64 {
	return cosphi / math.Sqrt(1-es*sinphi*sinphi)
}

// newLccKernel mirrors PROJ's lcc setup. lat0/lon0/lat1/lat2 are radians, a/b
// metres, k0 the scale factor.
func newLccKernel(a, b, lat0, lon0, lat1, lat2, k0 float64) *lccKernel {
	e2 := (a*a - b*b) / (a * a)
	if e2 < 0 {
		e2 = 0
	}
	e := math.Sqrt(e2)
	q := &lccKernel{a: a, e: e, es: e2, lat0: lat0, lon0: lon0}

	m1 := lccMSfn(math.Sin(lat1), math.Cos(lat1), e2)
	t1 := ExpNegIsometric(lat1, e) // pj_tsfn = exp(−ψ)

	n := math.Sin(lat1) // tangent default
	if math.Abs(lat1-lat2) >= lccEPS10 {
		// Secant cone. n = ln(m1/m2) / ln(t1/t2); with ln(t1/t2) = ψ2 − ψ1
		// this is positive for lat_1 < lat_2 (northern) and negative for a
		// southern pair — exactly PROJ's sign convention (ρ0 carries it too).
		m2 := lccMSfn(math.Sin(lat2), math.Cos(lat2), e2)
		t2 := ExpNegIsometric(lat2, e)
		n = math.Log(m1/m2) / math.Log(t1/t2)
	}
	q.n = n
	q.c = a * k0 * m1 * math.Exp(n*IsometricLat(lat1, e)) / n
	q.rho0 = q.c * math.Exp(-n*IsometricLat(lat0, e))
	return q
}

// forward: radians in, metres out.
func (q *lccKernel) forward(lon, lat float64) (x, y float64) {
	if math.Abs(math.Abs(lat)-math.Pi/2) < lccEPS10 {
		// PROJ: at a pole, ρ = 0 on the apex side (φ·n > 0); the other pole is
		// outside the projection domain (PROJ_ERR_COORD_TRANSFM_OUTSIDE_...).
		if lat*q.n <= 0 {
			return math.NaN(), math.NaN()
		}
		return 0, q.rho0
	}
	// PROJ's pipeline reduces lon - lon0 to (-pi, pi] before fwd; for a conic
	// theta = n*dlon is not 2*pi-periodic (n != 1), so the normalization is
	// load-bearing, not cosmetic.
	lam := geoAdjLonPi(lon-q.lon0) * q.n
	rho := q.c * math.Exp(-q.n*IsometricLat(lat, q.e))
	return rho * math.Sin(lam), q.rho0 - rho*math.Cos(lam)
}

// inverse: metres in, radians out. ok == false means outside the domain.
func (q *lccKernel) inverse(x, y float64) (lon, lat float64, ok bool) {
	dy := q.rho0 - y
	rho := math.Hypot(x, dy)
	if rho == 0 { // the apex: every meridian meets here
		return q.lon0, math.Copysign(math.Pi/2, q.n), true
	}
	// PROJ negates all three for n < 0 so that ρ/c stays positive (c < 0 there)
	// and the longitude branch is picked on the mirrored quadrant.
	if q.n < 0 {
		x, dy, rho = -x, -dy, -rho
	}
	psi := math.Log(q.c/rho) / q.n // = (ln c − ln ρ)/n with same-signed operands
	phi := LatFromIsometric(psi, q.e)
	if math.IsNaN(phi) {
		return math.NaN(), math.NaN(), false
	}
	return q.lon0 + math.Atan2(x, dy)/q.n, phi, true
}

// —— adapter entry points ——

func lccForward(a, b, lat0, lon0, lat1, lat2, k0 float64, lon, lat float64) (x, y float64) {
	return newLccKernel(a, b, lat0, lon0, lat1, lat2, k0).forward(lon, lat)
}

func lccInverse(a, b, lat0, lon0, lat1, lat2, k0 float64, x, y float64) (lon, lat float64, ok bool) {
	return newLccKernel(a, b, lat0, lon0, lat1, lat2, k0).inverse(x, y)
}

// —— adapter ——

type lccProj struct {
	crs    *CRS
	kernel *lccKernel
	x0, y0 float64 // false easting / northing, metres
}

func newLcc(c *CRS) (Projection, error) {
	a, b := c.Ellipsoid.A, c.Ellipsoid.B
	if a <= 0 || b <= 0 || b > a {
		return nil, fmt.Errorf("proj(lcc): 椭球半轴非法 a=%v b=%v", a, b)
	}
	lat0, _ := paramFloat(c, "lat_0", 0)
	lon0, _ := paramFloat(c, "lon_0", 0)
	lat1, hasLat1 := paramFloat(c, "lat_1", 0)
	if !hasLat1 {
		// A single-parallel lcc given only +lat_ts is the tangent cone
		// lat1 = lat2 = lat_ts. (PROJ 9.8.1 itself rejects that input — lat_1
		// defaults to 0 and the |lat_1+lat_2| guard fires — but the tangent
		// reading is what the caller means, so we accept it.)
		lat1, _ = paramFloat(c, "lat_ts", 0)
	}
	lat2, hasLat2 := paramFloat(c, "lat_2", 0)
	if !hasLat2 {
		lat2 = lat1 // default: tangent cone (matches PROJ's lcc.cpp default)
	}
	k0, _ := paramFloat(c, "k_0", 1)
	x0, _ := paramFloat(c, "x_0", 0)
	y0, _ := paramFloat(c, "y_0", 0)

	if math.Abs(lat0) > 90 || math.Abs(lat1) > 90 || math.Abs(lat2) > 90 {
		return nil, fmt.Errorf("proj(lcc): lat_0/lat_1/lat_2 非法 |lat| > 90 (%v %v %v)", lat0, lat1, lat2)
	}
	// PROJ 9.8.1 lcc.cpp verbatim guards:
	//   "Invalid value for lat_1 and lat_2: |lat_1 + lat_2| should be > 0"
	//   "Invalid value for lat_1: |lat_1| should be < 90°" (and lat_2).
	if math.Abs(lat1+lat2) < lccEPS10 {
		return nil, fmt.Errorf("proj(lcc): lat_1=%v 与 lat_2=%v 组合被拒(|lat_1+lat_2| 须 > 0,PROJ 口径)", lat1, lat2)
	}
	if math.Abs(math.Cos(geoDegToRad(lat1))) < lccEPS10 || math.Abs(lat1) >= 90 {
		return nil, fmt.Errorf("proj(lcc): lat_1=%v 非法(须 |lat_1| < 90°,PROJ 口径)", lat1)
	}
	if math.Abs(math.Cos(geoDegToRad(lat2))) < lccEPS10 || math.Abs(lat2) >= 90 {
		return nil, fmt.Errorf("proj(lcc): lat_2=%v 非法(须 |lat_2| < 90°,PROJ 口径)", lat2)
	}

	rLat0, rLon0 := geoDegToRad(lat0), geoDegToRad(lon0)
	rLat1, rLat2 := geoDegToRad(lat1), geoDegToRad(lat2)
	return &lccProj{
		crs:    c,
		kernel: newLccKernel(a, b, rLat0, rLon0, rLat1, rLat2, k0),
		x0:     x0,
		y0:     y0,
	}, nil
}

func (s *lccProj) CRS() *CRS { return s.crs }

func (s *lccProj) Forward(lon, lat float64) (float64, float64, error) {
	if math.IsNaN(lon) || math.IsNaN(lat) || math.Abs(lat) > 90 || math.Abs(lon) > 360 {
		return math.NaN(), math.NaN(), fmt.Errorf("proj(lcc): 输入出界 lon=%v lat=%v", lon, lat)
	}
	x, y := s.kernel.forward(geoDegToRad(lon), geoDegToRad(lat))
	if math.IsNaN(x) || math.IsNaN(y) {
		return math.NaN(), math.NaN(), domainErr(lccDomainReason(lat, s.kernel.n), fmt.Sprintf("lon=%v lat=%v", lon, lat))
	}
	return s.x0 + x, s.y0 + y, nil
}

func (s *lccProj) Inverse(x, y float64) (float64, float64, error) {
	lon, lat, ok := s.kernel.inverse(x-s.x0, y-s.y0)
	if !ok {
		return math.NaN(), math.NaN(), domainErr("outside", fmt.Sprintf("x=%v y=%v", x, y))
	}
	return geoNormLon(geoRadToDeg(lon)), geoRadToDeg(lat), nil
}

func init() { register("lcc", newLcc) }
