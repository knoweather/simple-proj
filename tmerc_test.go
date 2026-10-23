package proj

// tmerc_test.go — golden consumer for the Transverse Mercator fixture
// (+proj=tmerc +lat_0=0 +lon_0=105 +k=0.9996 +ellps=WGS84). Shared loader lives
// in golden_test.go.
//
// Tolerances: forward <= 1e-6 m absolute (relative floor of tol),
// inverse <= 1e-9 deg absolute, round-trip <= 1e-9 deg.
//
// Every fixture point sits at least 5 deg from the central meridian (the grid
// steps 11.25 deg around lon_0 = 105), so PROJ's auto dispatch (3 deg switch to
// the Evenden/Snyder series) never triggers: the whole fixture is the exact
// Poder/Engsager path. The two algorithms agree to ~1e-9 m at the switch, so
// running the exact path everywhere is safe even for inputs inside 3 deg.

import (
	"errors"
	"math"
	"testing"
)

func TestGoldenTmerc(t *testing.T) {
	g := loadGolden(t, "golden_tmerc.json")
	p := goldenParams(g.CRS)
	a, b := ellipsoidFor(p, g.CRS)
	k0 := p["k_0"]
	if k0 <= 0 { // the fixture writes +k=...: goldenParams has no alias, Parse does
		k0 = p["k"]
	}
	if k0 <= 0 { // truly absent: PROJ default
		k0 = 1
	}
	q := newTmercKernel(a, b,
		geoDegToRad(p["lat_0"]), geoDegToRad(p["lon_0"]), k0)

	maxFwd, maxInv := 0.0, 0.0
	for _, r := range g.Forward {
		x, y := q.forward(geoDegToRad(r.Lon), geoDegToRad(r.Lat))
		if math.IsNaN(x) || math.IsNaN(y) {
			t.Fatalf("forward NaN at (%v, %v): %s", r.Lon, r.Lat, r.Desc)
		}
		d := math.Max(math.Abs(x-r.X), math.Abs(y-r.Y))
		if d > tol(r.X) && d > tol(r.Y) {
			t.Fatalf("forward mismatch at (%v, %v): got (%.6f, %.6f) want (%.6f, %.6f)",
				r.Lon, r.Lat, x, y, r.X, r.Y)
		}
		if d > maxFwd {
			maxFwd = d
		}
	}

	for _, r := range g.Inverse {
		lon, lat, ok := q.inverse(r.X, r.Y)
		if !ok {
			t.Fatalf("inverse rejected at (%v, %v)", r.X, r.Y)
		}
		// Kernel-level inverse returns lam relative to lon0 (PROJ's pipeline
		// adds lam0 back); the fixture stores absolute longitudes.
		lon, lat = geoNormLon(geoRadToDeg(geoAdjLonPi(lon+geoDegToRad(p["lon_0"])))), geoRadToDeg(lat)
		d := math.Max(lonDiff(lon, r.Lon), math.Abs(lat-r.Lat))
		if d > 1e-9 {
			t.Fatalf("inverse mismatch at (%v, %v): got (%.10f, %.10f) want (%.10f, %.10f)",
				r.X, r.Y, lon, lat, r.Lon, r.Lat)
		}
		if d > maxInv {
			maxInv = d
		}
	}

	// Domain specials: the grid columns beyond ~81.7 deg of the central meridian
	// (|Ce| > 2.623395162778 on the complex sphere) and the far-outside inverse.
	for _, r := range g.Specials {
		if !r.ExpectNaN {
			continue
		}
		if r.X != 0 || r.Y != 0 {
			if lon, lat, ok := q.inverse(r.X, r.Y); ok || !math.IsNaN(lon) || !math.IsNaN(lat) {
				t.Fatalf("expected ErrDomain/NaN for %s, got (%v, %v)", r.Desc, lon, lat)
			}
			continue
		}
		if x, y := q.forward(geoDegToRad(r.Lon), geoDegToRad(r.Lat)); !math.IsNaN(x) || !math.IsNaN(y) {
			t.Fatalf("expected NaN for %s, got (%v, %v)", r.Desc, x, y)
		}
	}

	t.Logf("golden_tmerc: %d forward (max %.3e m), %d inverse (max %.3e deg), %d specials",
		len(g.Forward), maxFwd, len(g.Inverse), maxInv, len(g.Specials))
}

// TestTmercAdapter runs the fixture CRS through the full public path
// (Parse → NewProjection → Forward/Inverse), including the |Ce| domain edge
// (PROJ: PROJ_ERR_COORD_TRANSFM_OUTSIDE_PROJECTION_DOMAIN).
func TestTmercAdapter(t *testing.T) {
	g := loadGolden(t, "golden_tmerc.json")
	c, err := Parse(g.CRS)
	if err != nil {
		t.Fatalf("Parse(%q): %v", g.CRS, err)
	}
	p, err := c.NewProjection()
	if err != nil {
		t.Fatalf("NewProjection: %v", err)
	}
	if p.CRS() != c {
		t.Fatal("CRS() must return the CRS the projection was built from")
	}
	for _, r := range g.Forward[:25] {
		x, y, err := p.Forward(r.Lon, r.Lat)
		if err != nil {
			t.Fatalf("adapter Forward(%v, %v): %v", r.Lon, r.Lat, err)
		}
		if math.Abs(x-r.X) > tol(r.X) || math.Abs(y-r.Y) > tol(r.Y) {
			t.Fatalf("adapter forward at (%v, %v): got (%.6f, %.6f) want (%.6f, %.6f)",
				r.Lon, r.Lat, x, y, r.X, r.Y)
		}
		lon, lat, err := p.Inverse(x, y)
		if err != nil {
			t.Fatalf("adapter Inverse: %v", err)
		}
		if lonDiff(lon, r.Lon) > 1e-9 || math.Abs(lat-r.Lat) > 1e-9 {
			t.Fatalf("adapter round trip (%v, %v) -> (%.10f, %.10f)", r.Lon, r.Lat, lon, lat)
		}
	}
	// The exact-pole record (105, 90) is in the fixture with a finite y.
	if _, y, err := p.Forward(105, 90); err != nil || math.IsNaN(y) {
		t.Fatalf("pole should be finite, got (%v, %v)", y, err)
	}
	// |Ce| domain edge: 82.5 deg from the central meridian at the equator.
	if _, _, err := p.Forward(105+82.5, 0); !errorsIsDomain(err) {
		t.Fatalf("82.5 deg east of CM should be ErrDomain, got %v", err)
	}
	if _, x, err := p.Forward(105+75, 0); err != nil || math.IsNaN(x) {
		t.Fatalf("75 deg east of CM should be fine, got (%v, %v)", x, err)
	}
	if _, _, err := p.Inverse(9e9, -9e9); !errorsIsDomain(err) {
		t.Fatalf("far outside should be ErrDomain, got %v", err)
	}
	// Plain input errors, not ErrDomain.
	if _, _, err := p.Forward(0, 91); err == nil || errorsIsDomain(err) {
		t.Fatalf("lat=91 should be a plain error, got %v", err)
	}
	if _, _, err := p.Forward(400, 0); err == nil {
		t.Fatal("lon=400 should be a plain error")
	}
}

// TestTmercSphere covers the spherical branch (PROJ forces it when es == 0).
// Values are pyproj 3.8.0 / PROJ 9.8.1 on +a=6371000 +b=6371000.
func TestTmercSphere(t *testing.T) {
	q := newTmercKernel(6371000, 6371000, geoDegToRad(0), geoDegToRad(105), 0.9996)
	cases := []struct{ lon, lat, x, y float64 }{
		{105, 45, 0.0, 5001770.190325541},
		{120, 45, 1178787.823629704, 5112139.563051549},
		{105, 80, 0.0, 8892035.893912073},
		{-180, 0, 12912605.097406125, 0.0},
	}
	for _, c := range cases {
		x, y := q.forward(geoDegToRad(c.lon), geoDegToRad(c.lat))
		if math.Abs(x-c.x) > 1e-6 || math.Abs(y-c.y) > 1e-6 {
			t.Fatalf("sphere forward at (%v, %v): got (%.6f, %.6f) want (%.6f, %.6f)",
				c.lon, c.lat, x, y, c.x, c.y)
		}
	}
	for _, c := range []struct{ x, y, lon, lat float64 }{
		{0.0, 5000000.0, 105.00000000000001, 44.98407392550673},
		{500000.0, 1000000.0, 109.54953512288881, 8.968928518458004},
	} {
		lon, lat, ok := q.inverse(c.x, c.y)
		if !ok {
			t.Fatalf("sphere inverse rejected at (%v, %v)", c.x, c.y)
		}
		lon, lat = geoNormLon(geoRadToDeg(geoAdjLonPi(lon+geoDegToRad(105)))), geoRadToDeg(lat)
		if lonDiff(lon, c.lon) > 1e-9 || math.Abs(lat-c.lat) > 1e-9 {
			t.Fatalf("sphere inverse at (%v, %v): got (%.10f, %.10f) want (%.10f, %.10f)",
				c.x, c.y, lon, lat, c.lon, c.lat)
		}
	}
}

// TestTmercVariants pins +k_0, +x_0/+y_0 and lat_0 != 0 (which flips PROJ's auto
// dispatch to the exact path unconditionally) against pyproj 3.8.0.
func TestTmercVariants(t *testing.T) {
	proj := func(crs string) Projection {
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
	wgs84 := "+proj=tmerc +ellps=WGS84 "

	// k_0 = 1, false easting/northing, WGS84.
	px := proj(wgs84 + "+lat_0=0 +lon_0=105 +k_0=1 +x_0=100000 +y_0=-50000")
	x, y, err := px.Forward(105, 45)
	if err != nil || math.Abs(x-100000.0) > 1e-6 || math.Abs(y-4934944.377977743) > 1e-6 {
		t.Fatalf("k0=1 x0y0 forward: got (%v, %v, %v)", x, y, err)
	}
	x, y, _ = px.Forward(120, 30)
	if math.Abs(x-1555590.7388827328) > 1e-6 || math.Abs(y-3366780.562222126) > 1e-6 {
		t.Fatalf("k0=1 x0y0 forward 2: got (%v, %v)", x, y)
	}
	lon, lat, err := px.Inverse(105000, 4500000)
	if err != nil || lonDiff(lon, 105.05950455884772) > 1e-9 || math.Abs(lat-41.084868088415014) > 1e-9 {
		t.Fatalf("k0=1 x0y0 inverse: got (%v, %v, %v)", lon, lat, err)
	}

	// lat_0 = 30 (PROJ: phi0 != 0 forces the exact path).
	pp := proj(wgs84 + "+lat_0=30 +lon_0=105 +k_0=1")
	x, y, err = pp.Forward(105, 45)
	if err != nil || math.Abs(x) > 1e-9 || math.Abs(y-1664830.9800373602) > 1e-6 {
		t.Fatalf("phi0=30 forward: got (%v, %v, %v)", x, y, err)
	}
	x, y, _ = pp.Forward(130, 30)
	if math.Abs(x-2450218.798187598) > 1e-6 || math.Abs(y-278529.22994364344) > 1e-6 {
		t.Fatalf("phi0=30 forward 2: got (%v, %v)", x, y)
	}
	lon, lat, err = pp.Inverse(0, 1663791.464150959)
	if err != nil || lonDiff(lon, 105.00000000000001) > 1e-9 || math.Abs(lat-44.99064608912485) > 1e-9 {
		t.Fatalf("phi0=30 inverse: got (%v, %v, %v)", lon, lat, err)
	}
}

// TestTmercKernelSignature exercises the kernel entry points directly.
func TestTmercKernelSignature(t *testing.T) {
	g := loadGolden(t, "golden_tmerc.json")
	p := goldenParams(g.CRS)
	a, b := ellipsoidFor(p, g.CRS)
	lat0, lon0 := geoDegToRad(p["lat_0"]), geoDegToRad(p["lon_0"])
	k0 := p["k_0"]
	if k0 <= 0 { // +k=... alias
		k0 = p["k"]
	}
	if k0 <= 0 {
		k0 = 1
	}
	for _, r := range g.Forward[:10] {
		x, y := tmercForward(a, b, lat0, lon0, k0, geoDegToRad(r.Lon), geoDegToRad(r.Lat))
		if math.Abs(x-r.X) > tol(r.X) || math.Abs(y-r.Y) > tol(r.Y) {
			t.Fatalf("tmercForward at (%v, %v): got (%.6f, %.6f) want (%.6f, %.6f)",
				r.Lon, r.Lat, x, y, r.X, r.Y)
		}
		lon, lat, ok := tmercInverse(a, b, lat0, lon0, k0, r.X, r.Y)
		if !ok {
			t.Fatalf("tmercInverse rejected (%v, %v)", r.X, r.Y)
		}
		// The kernel returns lam relative to lon0 (PROJ's pipeline adds lam0 back).
		lon, lat = geoNormLon(geoRadToDeg(geoAdjLonPi(lon+lon0))), geoRadToDeg(lat)
		if lonDiff(lon, r.Lon) > 1e-9 || math.Abs(lat-r.Lat) > 1e-9 {
			t.Fatalf("tmercInverse at (%v, %v): got (%.10f, %.10f) want (%.10f, %.10f)",
				r.X, r.Y, lon, lat, r.Lon, r.Lat)
		}
	}
	if _, _, ok := tmercInverse(a, b, lat0, lon0, k0, 9e9, -9e9); ok {
		t.Fatal("tmercInverse must report the far-outside point as out of domain")
	}
}

// errorsIsDomain uses errors.Is (not ==) so DomainError-wrapped domain
// failures still match — proj.go's batch judge uses errors.Is for the same
// reason (ErrDomain stays the single sentinel).
func errorsIsDomain(err error) bool {
	return errors.Is(err, ErrDomain)
}
