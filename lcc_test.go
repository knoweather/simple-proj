package proj

// lcc_test.go — AI-B's golden consumer for the Lambert conformal conic fixture.
// Shared loader lives in golden_test.go (C#6 in PROGRESS-B.md).
//
// Tolerances: COORDINATION §2.5 — forward <= 1e-6 m absolute (with the relative
// floor of C#3), inverse <= 1e-9 deg absolute, round-trip <= 1e-9 deg.

import (
	"errors"
	"math"
	"testing"
)

func TestGoldenLcc(t *testing.T) {
	g := loadGolden(t, "golden_lcc.json")
	p := goldenParams(g.CRS)
	a, b := ellipsoidFor(p, g.CRS)
	k0 := p["k_0"]
	if k0 <= 0 { // absent from the fixture: PROJ default
		k0 = 1
	}
	q := newLccKernel(a, b,
		geoDegToRad(p["lat_0"]), geoDegToRad(p["lon_0"]),
		geoDegToRad(p["lat_1"]), geoDegToRad(p["lat_2"]), k0)

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
		lon, lat = geoNormLon(geoRadToDeg(lon)), geoRadToDeg(lat)
		d := math.Max(lonDiff(lon, r.Lon), math.Abs(lat-r.Lat))
		if d > 1e-9 {
			t.Fatalf("inverse mismatch at (%v, %v): got (%.10f, %.10f) want (%.10f, %.10f)",
				r.X, r.Y, lon, lat, r.Lon, r.Lat)
		}
		if d > maxInv {
			maxInv = d
		}
	}

	t.Logf("golden_lcc: %d forward (max %.3e m), %d inverse (max %.3e deg)",
		len(g.Forward), maxFwd, len(g.Inverse), maxInv)
}

// TestLccAdapter runs the golden points through the full contract path
// (Parse → NewProjection → Forward/Inverse) and checks PROJ's setup guards.
func TestLccAdapter(t *testing.T) {
	g := loadGolden(t, "golden_lcc.json")
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

	// The far pole (φ·n <= 0) is outside PROJ's lcc domain; pyproj surfaces the
	// PROJ error as (inf, inf) because errcheck defaults to False. Our contract
	// turns the kernel NaN into ErrDomain (COORDINATION §2.5).
	if _, _, err := p.Forward(0, -90); !errors.Is(err, ErrDomain) {
		t.Fatalf("far pole should give ErrDomain, got %v", err)
	}
	// The apex pole is inside the domain: (0, rho0), finite.
	x, y, err := p.Forward(0, 90)
	if err != nil || x != 0 || math.Abs(y-11417334.780967051) > 1e-6 {
		t.Fatalf("apex pole: got (%v, %v, %v)", x, y, err)
	}
	if _, _, err := p.Forward(0, 91); err == nil || errors.Is(err, ErrDomain) {
		t.Fatalf("lat=91 should be a plain error, got %v", err)
	}
	if _, _, err := p.Forward(400, 0); err == nil {
		t.Fatal("lon=400 should be a plain error")
	}

	// +k_0 scales rho (PROJ lcc honours it), +x_0/+y_0 shift after.
	ck, err := Parse(g.CRS + " +k_0=0.9999")
	if err != nil {
		t.Fatalf("Parse(+k_0=0.9999): %v", err)
	}
	pk, err := ck.NewProjection()
	if err != nil {
		t.Fatalf("NewProjection(+k_0): %v", err)
	}
	x1, _, _ := p.Forward(2.5, 45)
	x2, y2, _ := pk.Forward(2.5, 45)
	// pyproj 3.8.0 / PROJ 9.8.1, +proj=lcc ... +k_0=0.9999 at (2.5, 45):
	if math.Abs(x2+5720344.626745017) > 1e-6 || math.Abs(y2-9309014.180031694) > 1e-6 {
		t.Fatalf("k_0=0.9999 must match pyproj: got (%v, %v)", x2, y2)
	}
	if r := x1 / x2; math.Abs(r-1/0.9999) > 1e-9 {
		t.Fatalf("k_0 must scale rho linearly: ratio %v", r)
	}

	// PROJ's setup guards, mirrored verbatim.
	for _, bad := range []string{
		"+proj=lcc +lat_1=30 +lat_2=-30 +lon_0=10 +ellps=WGS84", // |lat_1+lat_2| = 0
		"+proj=lcc +lat_1=90 +lat_2=60 +lon_0=10 +ellps=WGS84",  // |lat_1| = 90
		"+proj=lcc +lat_1=30 +lat_2=90 +lon_0=10 +ellps=WGS84",  // |lat_2| = 90
	} {
		if c, err := Parse(bad); err == nil {
			if _, err := c.NewProjection(); err == nil {
				t.Fatalf("%q must be rejected", bad)
			}
		}
	}
}

// TestLccFrozenSignature exercises the frozen §2.3 entry points directly.
func TestLccFrozenSignature(t *testing.T) {
	g := loadGolden(t, "golden_lcc.json")
	p := goldenParams(g.CRS)
	a, b := ellipsoidFor(p, g.CRS)
	lat0, lon0 := geoDegToRad(p["lat_0"]), geoDegToRad(p["lon_0"])
	lat1, lat2 := geoDegToRad(p["lat_1"]), geoDegToRad(p["lat_2"])

	for _, r := range g.Forward[:10] {
		x, y := lccForward(a, b, lat0, lon0, lat1, lat2, 1, geoDegToRad(r.Lon), geoDegToRad(r.Lat))
		if math.Abs(x-r.X) > tol(r.X) || math.Abs(y-r.Y) > tol(r.Y) {
			t.Fatalf("lccForward at (%v, %v): got (%.6f, %.6f) want (%.6f, %.6f)",
				r.Lon, r.Lat, x, y, r.X, r.Y)
		}
		lon, lat, ok := lccInverse(a, b, lat0, lon0, lat1, lat2, 1, r.X, r.Y)
		if !ok {
			t.Fatalf("lccInverse rejected (%v, %v)", r.X, r.Y)
		}
		lon, lat = geoNormLon(geoRadToDeg(lon)), geoRadToDeg(lat)
		if lonDiff(lon, r.Lon) > 1e-9 || math.Abs(lat-r.Lat) > 1e-9 {
			t.Fatalf("lccInverse at (%v, %v): got (%.10f, %.10f) want (%.10f, %.10f)",
				r.X, r.Y, lon, lat, r.Lon, r.Lat)
		}
	}
	if _, _, ok := lccInverse(a, b, lat0, lon0, lat1, lat2, 1, 9e9, -9e9); !ok {
		t.Fatal("lcc has no hard inverse domain edge: a far point must still invert")
	}
}

// TestLccTangentAndSouthern covers the cone geometries the golden fixture does
// not exercise: the tangent (single-parallel) cone n = sin φ1, and the southern
// cone where n < 0 (PROJ negates (x, y, ρ) in the inverse). All values are
// pyproj 3.8.0 / PROJ 9.8.1 references with explicit lat_0=0 — recorded in
// FORMULA-NOTES.md §3.
func TestLccTangentAndSouthern(t *testing.T) {
	a, b := testEllipsoids["wgs84"][0], testEllipsoids["wgs84"][1]
	d := geoDegToRad

	// Tangent cone at 45°, lat_0=0 (pyproj, +lat_1=45 +lat_2=45 +lat_0=0 +lon_0=10).
	qt := newLccKernel(a, b, d(0), d(10), d(45), d(45), 1)
	for _, c := range []struct{ lon, lat, x, y float64 }{
		{2.5, 45, -590507.2378272155, 5513407.910526307},
		{-170, 60, -3738788.5505577056, 14720949.35175797},
		{179, 80, 1854377.1553107924, 12924098.786274323},
		{0, 0, -1461804.3485455296, 90317.79732417196},
	} {
		x, y := qt.forward(d(c.lon), d(c.lat))
		if math.Abs(x-c.x) > 1e-6 || math.Abs(y-c.y) > 1e-6 {
			t.Fatalf("tangent forward at (%v, %v): got (%.6f, %.6f) want (%.6f, %.6f)",
				c.lon, c.lat, x, y, c.x, c.y)
		}
		lon, lat, ok := qt.inverse(c.x, c.y)
		if !ok {
			t.Fatalf("tangent inverse rejected at (%v, %v)", c.x, c.y)
		}
		lon, lat = geoNormLon(geoRadToDeg(lon)), geoRadToDeg(lat)
		if lonDiff(lon, c.lon) > 1e-9 || math.Abs(lat-c.lat) > 1e-9 {
			t.Fatalf("tangent round trip (%v, %v) -> (%.10f, %.10f)", c.lon, c.lat, lon, lat)
		}
	}

	// Southern cone (n < 0), +lat_1=-30 +lat_2=-60 +lat_0=0 +lon_0=10.
	qs := newLccKernel(a, b, d(0), d(10), d(-30), d(-60), 1)
	if qs.n >= 0 {
		t.Fatalf("southern cone must give n < 0, got %v", qs.n)
	}
	for _, c := range []struct{ lon, lat, x, y float64 }{
		{2.5, 45, -2000092.1156337543, 9870172.114257164},
		{-170, 60, -22734486.22821187, -29709922.00892073},
		{11.25, -85, 19049.311226090376, -10197381.709217934},
	} {
		x, y := qs.forward(d(c.lon), d(c.lat))
		if math.Abs(x-c.x) > 1e-6 || math.Abs(y-c.y) > 1e-6 {
			t.Fatalf("south forward at (%v, %v): got (%.6f, %.6f) want (%.6f, %.6f)",
				c.lon, c.lat, x, y, c.x, c.y)
		}
		lon, lat, ok := qs.inverse(c.x, c.y)
		if !ok {
			t.Fatalf("south inverse rejected at (%v, %v)", c.x, c.y)
		}
		lon, lat = geoNormLon(geoRadToDeg(lon)), geoRadToDeg(lat)
		if lonDiff(lon, c.lon) > 1e-9 || math.Abs(lat-c.lat) > 1e-9 {
			t.Fatalf("south round trip (%v, %v) -> (%.10f, %.10f)", c.lon, c.lat, lon, lat)
		}
	}
	// The apex is the SOUTH pole for n < 0: (0, rho0) with rho0 < 0.
	if x, y := qs.forward(d(0), d(-90)); x != 0 || math.Abs(y+11417334.78096705) > 1e-6 {
		t.Fatalf("south apex: got (%v, %v)", x, y)
	}
	// The north pole is outside the domain (pyproj: PROJ error → inf/inf).
	if x, y := qs.forward(d(0), d(90)); !math.IsNaN(x) || !math.IsNaN(y) {
		t.Fatalf("north pole should be NaN/ErrDomain, got (%v, %v)", x, y)
	}
}

// TestLccSphere checks the e2 == 0 degeneration: with a == b the formulas
// collapse to Snyder's spherical conic (m = cosφ, ψ = ln tan(π/4+φ/2),
// n = sin φ1 for the tangent case). Values from pyproj 3.8.0 on
// +a=6371000 +b=6371000.
func TestLccSphere(t *testing.T) {
	a, b := testEllipsoids["sphere"][0], testEllipsoids["sphere"][1]
	d := geoDegToRad
	q := newLccKernel(a, b, d(0), d(10), d(30), d(60), 1)
	cases := []struct{ lon, lat float64 }{
		{2.5, 45}, {-170, 60}, {179, -80}, {10, 0}, {10, 45},
	}
	// pyproj 3.8.0 / PROJ 9.8.1, +proj=lcc +lat_1=30 +lat_2=60 +lat_0=0 +lon_0=10
	// +a=6371000 +b=6371000
	ref := []struct{ x, y float64 }{
		{-568651.4009794276, 5370172.770361219},
		{-3469291.916447767, 14212951.210818317},
		{56011689.62398164, 44986575.01779067},
		{0, 0},
		{0, 5343521.209450735},
	}
	for i, c := range cases {
		x, y := q.forward(d(c.lon), d(c.lat))
		if math.Abs(x-ref[i].x) > 1e-6 || math.Abs(y-ref[i].y) > 1e-6 {
			t.Fatalf("sphere forward at (%v, %v): got (%.6f, %.6f) want (%.6f, %.6f)",
				c.lon, c.lat, x, y, ref[i].x, ref[i].y)
		}
		lon, lat, ok := q.inverse(ref[i].x, ref[i].y)
		if !ok {
			t.Fatalf("sphere inverse rejected at (%v, %v)", ref[i].x, ref[i].y)
		}
		lon, lat = geoNormLon(geoRadToDeg(lon)), geoRadToDeg(lat)
		if lonDiff(lon, c.lon) > 1e-9 || math.Abs(lat-c.lat) > 1e-9 {
			t.Fatalf("sphere round trip (%v, %v) -> (%.10f, %.10f)", c.lon, c.lat, lon, lat)
		}
	}
}
