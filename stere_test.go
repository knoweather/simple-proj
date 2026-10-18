package proj

// stere_test.go — AI-B's golden consumer for the stereographic fixture. Shared loader
// lives in golden_test.go (see C#6 in PROGRESS-B.md).
//
// Tolerances: COORDINATION §2.5 — forward <= 1e-6 m absolute (with the relative floor
// of C#3), inverse <= 1e-9 deg absolute, round-trip <= 1e-9 deg.

import (
	"errors"
	"math"
	"testing"
)

func TestGoldenStere(t *testing.T) {
	g := loadGolden(t, "golden_stere.json")
	p := goldenParams(g.CRS)
	a, b := ellipsoidFor(p, g.CRS)

	lat0 := geoDegToRad(p["lat_0"])
	lon0 := geoDegToRad(p["lon_0"])
	latTs := geoDegToRad(p["lat_ts"])
	k0 := p["k"]
	if k0 == 0 {
		k0 = 1
	}
	k := stereEffectiveK(a, b, lat0, latTs, k0)
	q := newStereKernel(a, b, lat0, lon0, k)

	for _, r := range g.Forward {
		x, y := q.forward(geoDegToRad(r.Lon), geoDegToRad(r.Lat))
		if math.IsNaN(x) || math.IsNaN(y) {
			t.Fatalf("forward NaN at (%v, %v): %s", r.Lon, r.Lat, r.Desc)
		}
		if math.Abs(x-r.X) > tol(r.X) || math.Abs(y-r.Y) > tol(r.Y) {
			t.Fatalf("forward mismatch at (%v, %v): got (%.6f, %.6f) want (%.6f, %.6f)",
				r.Lon, r.Lat, x, y, r.X, r.Y)
		}
	}

	for _, r := range g.Inverse {
		lon, lat, ok := q.inverse(r.X, r.Y)
		if !ok {
			t.Fatalf("inverse rejected at (%v, %v)", r.X, r.Y)
		}
		lon, lat = geoNormLon(geoRadToDeg(lon)), geoRadToDeg(lat)
		if lonDiff(lon, r.Lon) > 1e-9 || math.Abs(lat-r.Lat) > 1e-9 {
			t.Fatalf("inverse mismatch at (%v, %v): got (%.10f, %.10f) want (%.10f, %.10f)",
				r.X, r.Y, lon, lat, r.Lon, r.Lat)
		}
	}

	for _, r := range g.Specials {
		if !r.ExpectNaN {
			continue
		}
		if r.X != 0 || r.Y != 0 {
			lon, lat, ok := q.inverse(r.X, r.Y)
			if ok || !math.IsNaN(lon) || !math.IsNaN(lat) {
				t.Fatalf("expected ErrDomain/NaN for %s, got (%v, %v)", r.Desc, lon, lat)
			}
			continue
		}
		x, y := q.forward(geoDegToRad(r.Lon), geoDegToRad(r.Lat))
		if !math.IsNaN(x) || !math.IsNaN(y) {
			t.Fatalf("expected NaN for %s, got (%v, %v)", r.Desc, x, y)
		}
	}
}

// TestStereEffectiveK pins the +lat_ts / +k_0 semantics against pyproj 3.8.0:
// OBLIQ ignores +lat_ts, the polar aspect uses the raw akm1, and PROJ refuses
// k_0 != 1 in the polar aspect when lat_ts is not the pole.
func TestStereEffectiveK(t *testing.T) {
	cases := []struct {
		crs            string
		lon, lat, x, y float64
	}{
		{"+proj=stere +lat_0=45 +lon_0=0 +lat_ts=30 +ellps=WGS84", 10, 40, 854524.2786795973, -505314.70313599025},
		{"+proj=stere +lat_0=45 +lon_0=0 +ellps=WGS84", 10, 40, 854524.2786795973, -505314.70313599025},
		{"+proj=stere +lat_0=90 +lat_ts=60 +lon_0=-100 +ellps=WGS84", -70, 40, 2777764.454821, -4811229.167209},
	}
	for _, c := range cases {
		cr, err := Parse(c.crs)
		if err != nil {
			t.Fatalf("Parse(%q): %v", c.crs, err)
		}
		p, err := cr.NewProjection()
		if err != nil {
			t.Fatalf("NewProjection(%q): %v", c.crs, err)
		}
		x, y, err := p.Forward(c.lon, c.lat)
		if err != nil {
			t.Fatalf("Forward: %v", err)
		}
		if math.Abs(x-c.x) > 1e-6 || math.Abs(y-c.y) > 1e-6 {
			t.Fatalf("%s at (%v, %v): got (%.6f, %.6f) want (%.6f, %.6f)",
				c.crs, c.lon, c.lat, x, y, c.x, c.y)
		}
	}

	// PROJ refuses this combination; our adapter must report the same, not fold it.
	cr, err := Parse("+proj=stere +lat_0=90 +lat_ts=60 +lon_0=-100 +ellps=WGS84 +k_0=2")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if _, err := cr.NewProjection(); err == nil {
		t.Fatal("polar aspect with lat_ts != 90 and k_0 != 1 must be rejected")
	}
	// The same string with k_0 = 1 is fine.
	cr, err = Parse("+proj=stere +lat_0=90 +lat_ts=60 +lon_0=-100 +ellps=WGS84 +k_0=1")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if _, err := cr.NewProjection(); err != nil {
		t.Fatalf("k_0 = 1 must be accepted: %v", err)
	}
}

// TestStereFrozenSignature exercises the frozen §2.3 entry points directly
// (stereForward/stereInverse), which the adapter tests do not reach.
func TestStereFrozenSignature(t *testing.T) {
	g := loadGolden(t, "golden_stere.json")
	p := goldenParams(g.CRS)
	a, b := ellipsoidFor(p, g.CRS)
	lat0, lon0, latTs := geoDegToRad(p["lat_0"]), geoDegToRad(p["lon_0"]), geoDegToRad(p["lat_ts"])
	k0 := p["k"]
	if k0 == 0 {
		k0 = 1
	}
	k := stereEffectiveK(a, b, lat0, latTs, k0)

	for _, r := range g.Forward[:10] {
		x, y := stereForward(a, b, lat0, lon0, k, geoDegToRad(r.Lon), geoDegToRad(r.Lat))
		if math.Abs(x-r.X) > tol(r.X) || math.Abs(y-r.Y) > tol(r.Y) {
			t.Fatalf("stereForward at (%v, %v): got (%.6f, %.6f) want (%.6f, %.6f)",
				r.Lon, r.Lat, x, y, r.X, r.Y)
		}
		lon, lat, ok := stereInverse(a, b, lat0, lon0, k, r.X, r.Y)
		if !ok {
			t.Fatalf("stereInverse rejected (%v, %v)", r.X, r.Y)
		}
		lon, lat = geoNormLon(geoRadToDeg(lon)), geoRadToDeg(lat)
		if lonDiff(lon, r.Lon) > 1e-9 || math.Abs(lat-r.Lat) > 1e-9 {
			t.Fatalf("stereInverse at (%v, %v): got (%.10f, %.10f) want (%.10f, %.10f)",
				r.X, r.Y, lon, lat, r.Lon, r.Lat)
		}
	}
}

// TestStereAdapter runs the golden points through the full contract path
// (Parse → NewProjection → Forward/Inverse), which is what PROGRESS-A.md C#2 asked
// for: the kernel-only tests leave the adapter and the register path uncovered.
func TestStereAdapter(t *testing.T) {
	g := loadGolden(t, "golden_stere.json")
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

	// Illegal input must be a plain error, not ErrDomain (§2.5).
	if _, _, err := p.Forward(0, 91); err == nil || errors.Is(err, ErrDomain) {
		t.Fatalf("lat=91 should be a plain error, got %v", err)
	}
	if _, _, err := p.Forward(math.NaN(), 0); err == nil {
		t.Fatal("NaN input must be rejected")
	}
}

// TestStereAspects covers the three projection aspects (equatorial lat_0=0,
// oblique lat_0=45, polar) through the full contract path. The golden fixture
// only exercises the polar aspect (lat_0=90), and TestStereVariants only pins a
// couple of oblique forward points — so the equatorial/oblique FORWARD and the
// equatorial INVERSE branches were unverified. Values are pyproj 3.8.0 /
// PROJ 9.8.1 ground truth.
func TestStereAspects(t *testing.T) {
	// pyproj reference for equatorial (lat_0=0) and oblique (lat_0=45).
	forwardRef := []struct {
		crs            string
		lon, lat, x, y float64
	}{
		{"+proj=stere +lat_0=0 +lon_0=0 +ellps=WGS84", 10, 40, 968726.947836, 4649742.655420},
		{"+proj=stere +lat_0=0 +lon_0=0 +ellps=WGS84", -170, 60, -2203716.057171, 21833902.609421},
		{"+proj=stere +lat_0=0 +lon_0=0 +ellps=WGS84", 179, 80, 47151.116933, 15219624.952877},
		{"+proj=stere +lat_0=0 +lon_0=0 +ellps=WGS84", 10, -30, 1036266.482543, -3422349.026030},
		{"+proj=stere +lat_0=45 +lon_0=0 +ellps=WGS84", 10, 40, 854524.278680, -505314.703136},
		{"+proj=stere +lat_0=45 +lon_0=0 +ellps=WGS84", -170, 60, -883242.645320, 9738671.542770},
		{"+proj=stere +lat_0=45 +lon_0=0 +ellps=WGS84", 179, 80, 24744.512702, 6665713.357645},
		{"+proj=stere +lat_0=45 +lon_0=0 +ellps=WGS84", 10, -30, 1527936.033253, -9686836.601875},
	}
	for _, c := range forwardRef {
		cr, err := Parse(c.crs)
		if err != nil {
			t.Fatalf("Parse(%q): %v", c.crs, err)
		}
		p, err := cr.NewProjection()
		if err != nil {
			t.Fatalf("NewProjection(%q): %v", c.crs, err)
		}
		x, y, err := p.Forward(c.lon, c.lat)
		if err != nil {
			t.Fatalf("Forward(%q, %v, %v): %v", c.crs, c.lon, c.lat, err)
		}
		if math.Abs(x-c.x) > 5e-4 || math.Abs(y-c.y) > 5e-4 {
			t.Fatalf("%s at (%v, %v): got (%.6f, %.6f) want (%.6f, %.6f)",
				c.crs, c.lon, c.lat, x, y, c.x, c.y)
		}
		// Inverse of our own forward must round trip (catches the equatorial
		// inverse branch, which the golden fixture never reaches).
		rl, ra, err := p.Inverse(x, y)
		if err != nil {
			t.Fatalf("Inverse(%q): %v", c.crs, err)
		}
		if lonDiff(rl, c.lon) > 1e-9 || math.Abs(ra-c.lat) > 1e-9 {
			t.Fatalf("%s round trip (%v, %v) -> (%.10f, %.10f)", c.crs, c.lon, c.lat, rl, ra)
		}
	}
}

// TestStereRoundTrip is the "at least 3 sanity checks" gate from DESIGN §2.3,
// run through our own kernel rather than through pyproj.
func TestStereRoundTrip(t *testing.T) {
	g := loadGolden(t, "golden_stere.json")
	p := goldenParams(g.CRS)
	a, b := ellipsoidFor(p, g.CRS)
	lat0, lon0, latTs := geoDegToRad(p["lat_0"]), geoDegToRad(p["lon_0"]), geoDegToRad(p["lat_ts"])
	k0 := p["k"]
	if k0 == 0 {
		k0 = 1
	}
	k := stereEffectiveK(a, b, lat0, latTs, k0)
	q := newStereKernel(a, b, lat0, lon0, k)

	for _, r := range []struct{ lon, lat float64 }{
		{0, 0}, {-100, 90}, {-100, 60}, {170, -85}, {-100, 89.9}, {0, -89.9},
	} {
		x, y := q.forward(geoDegToRad(r.lon), geoDegToRad(r.lat))
		lon, lat, ok := q.inverse(x, y)
		if !ok {
			t.Fatalf("round trip lost (%v, %v)", r.lon, r.lat)
		}
		lon, lat = geoNormLon(geoRadToDeg(lon)), geoRadToDeg(lat)
		if lonDiff(lon, r.lon) > 1e-9 || math.Abs(lat-r.lat) > 1e-9 {
			t.Fatalf("round trip (%v, %v) -> (%.12f, %.12f)", r.lon, r.lat, lon, lat)
		}
	}
}
