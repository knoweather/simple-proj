package proj

// impl_merc.go — Mercator projection (spherical + ellipsoidal). Owner: AI-B
// (COORDINATION §1).
//
// Formula source: Snyder PP1395 ch. 7; PROJ 9.8.1 src/projections/merc.cpp.
// y = a·factor·ψ(φ) with ψ the isometric latitude, so the kernel is a thin
// wrapper over geolib's isometric latitude — no new latitude algebra
// (FORMULA-NOTES.md §4).
//
// Kernel unit: radians in, metres out.
//
// Two PROJ 9.8.1 details that the day0 probing got wrong and pyproj settled
// (re-verified here, see FORMULA-NOTES.md §4):
//   1. +lat_ts present (any value, INCLUDING 0): factor = msfn(lat_ts) =
//      cos φs / sqrt(1 − e² sin²φs) (sphere: cos φs) and +k_0 is IGNORED —
//      PROJ *replaces* k0 with msfn(lat_ts), it does not multiply.
//   2. +lat_ts absent: factor = +k_0 (default 1). |lat_ts| = 90 is rejected.
// The frozen §2.3 kernel signature has no k0, so the adapter applies k0 as a
// post-factor exactly in the no-lat_ts branch.
//
// Numerics: PROJ evaluates the isometric latitude as asinh(tan φ) −
// e·atanh(e·sin φ), NOT ln tan(π/4+φ/2) + e/2·ln(...). The two are equal in
// exact arithmetic but differ at the pole: at φ = π/2 (double) the ln-tan form
// gives 37.3319 while asinh gives 38.0250 — and pyproj/the golden fixture say
// 38.0250 (golden_merc's (0, 90) → y = 242528680.94374272). Use the asinh form.
// The inverse reuses geolib's LatFromIsometric (Newton, ~1e-13), which matches
// PROJ's pj_phi2 well inside the 1e-9 deg gate.

import (
	"fmt"
	"math"
)

// mercEPS10 is PROJ's EPS10: the |lat_ts| = 90 rejection test.
const mercEPS10 = 1e-10

// mercKernel holds the precomputed constants. Immutable, thread-safe.
type mercKernel struct {
	a, e   float64
	lon0   float64 // radians
	factor float64 // msfn(lat_ts) when |lat_ts| > 0, else 1
}

// mercIsometricLat is PROJ's isometric-latitude formulation: asinh(tan φ) −
// e·atanh(e·sinφ). e == 0 degenerates to asinh(tan φ), the spherical limit.
// (Mathematically geolib's IsometricLat, but pole-stable — see the file header.)
func mercIsometricLat(phi, e float64) float64 {
	if e == 0 {
		return math.Asinh(math.Tan(phi))
	}
	return math.Asinh(math.Tan(phi)) - e*math.Atanh(e*math.Sin(phi))
}

// newMercKernel mirrors PROJ's merc setup. latTS is the scale latitude in
// radians (0 = no scale latitude: factor 1), lon0 radians, a/b metres.
func newMercKernel(a, b, latTS, lon0 float64) *mercKernel {
	e2 := (a*a - b*b) / (a * a)
	if e2 < 0 {
		e2 = 0
	}
	e := math.Sqrt(e2)
	factor := 1.0
	if latTS != 0 {
		// msfn(φs) = cos φs / sqrt(1 − e² sin²φs); the sphere (e² = 0) degenerates
		// to cos φs exactly. PROJ replaces k0 with this value whenever +lat_ts
		// is present.
		s, c := math.Sin(latTS), math.Cos(latTS)
		factor = c / math.Sqrt(1-e2*s*s)
	}
	return &mercKernel{a: a, e: e, lon0: lon0, factor: factor}
}

// forward: radians in, metres out.
func (q *mercKernel) forward(lon, lat float64) (x, y float64) {
	lam := geoAdjLonPi(lon - q.lon0)
	return q.a * q.factor * lam, q.a * q.factor * mercIsometricLat(lat, q.e)
}

// inverse: metres in, radians out. Every finite (x, y) maps back into the
// ellipsoid, so ok is false only when the latitude inversion fails.
func (q *mercKernel) inverse(x, y float64) (lon, lat float64, ok bool) {
	psi := y / (q.a * q.factor)
	phi := LatFromIsometric(psi, q.e)
	if math.IsNaN(phi) {
		return math.NaN(), math.NaN(), false
	}
	return q.lon0 + x/(q.a*q.factor), phi, true
}

// —— frozen contract entry points (COORDINATION §2.3) ——

func mercForward(a, b, latTS, lon0 float64, lon, lat float64) (x, y float64) {
	return newMercKernel(a, b, latTS, lon0).forward(lon, lat)
}

func mercInverse(a, b, latTS, lon0 float64, x, y float64) (lon, lat float64, ok bool) {
	return newMercKernel(a, b, latTS, lon0).inverse(x, y)
}

// —— adapter ——

type mercProj struct {
	crs    *CRS
	kernel *mercKernel
	scale  float64 // k0, applied only when +lat_ts is absent
	x0, y0 float64 // false easting / northing, metres
}

func newMerc(c *CRS) (Projection, error) {
	a, b := c.Ellipsoid.A, c.Ellipsoid.B
	if a <= 0 || b <= 0 || b > a {
		return nil, fmt.Errorf("proj(merc): 椭球半轴非法 a=%v b=%v", a, b)
	}
	latTs, hasLatTs := paramFloat(c, "lat_ts", 0)
	lon0, _ := paramFloat(c, "lon_0", 0)
	k0, _ := paramFloat(c, "k_0", 1)
	x0, _ := paramFloat(c, "x_0", 0)
	y0, _ := paramFloat(c, "y_0", 0)

	// PROJ 9.8.1 merc.cpp: lat_ts = ±90 makes msfn degenerate and is rejected.
	if hasLatTs && math.Abs(latTs) >= 90-mercEPS10 {
		return nil, fmt.Errorf("proj(merc): lat_ts=%v 非法(须 |lat_ts| < 90°,PROJ 口径)", latTs)
	}
	scale := k0
	if hasLatTs {
		// PROJ replaces k0 with msfn(lat_ts) whenever +lat_ts is present — even
		// lat_ts = 0 (verified: +lat_ts=0 +k_0=2 behaves like factor 1, not 2).
		scale = 1
	}
	return &mercProj{
		crs:    c,
		kernel: newMercKernel(a, b, geoDegToRad(latTs), geoDegToRad(lon0)),
		scale:  scale,
		x0:     x0,
		y0:     y0,
	}, nil
}

func (s *mercProj) CRS() *CRS { return s.crs }

func (s *mercProj) Forward(lon, lat float64) (float64, float64, error) {
	if math.IsNaN(lon) || math.IsNaN(lat) || math.Abs(lat) > 90 || math.Abs(lon) > 360 {
		return math.NaN(), math.NaN(), fmt.Errorf("proj(merc): 输入出界 lon=%v lat=%v", lon, lat)
	}
	x, y := s.kernel.forward(geoDegToRad(lon), geoDegToRad(lat))
	if math.IsNaN(x) || math.IsNaN(y) {
		return math.NaN(), math.NaN(), ErrDomain
	}
	return s.x0 + s.scale*x, s.y0 + s.scale*y, nil
}

func (s *mercProj) Inverse(x, y float64) (float64, float64, error) {
	lon, lat, ok := s.kernel.inverse((x-s.x0)/s.scale, (y-s.y0)/s.scale)
	if !ok {
		return math.NaN(), math.NaN(), ErrDomain
	}
	// Normalize in radians BEFORE converting to degrees: x/(a*factor) is
	// unbounded (|x| up to 1e12 m in the golden probe = 1.6e5 rad), and taking
	// 180/pi first amplifies the double noise past the 1e-9 deg gate. PROJ's
	// pipeline applies adjlon in radians too. geoNormLon then only folds the
	// +180 boundary to the frozen [-180, 180).
	return geoNormLon(geoRadToDeg(geoAdjLonPi(lon))), geoRadToDeg(lat), nil
}

func init() { register("merc", newMerc) }
