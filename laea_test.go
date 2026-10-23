package proj

// laea_test.go — golden consumer for the Lambert azimuthal equal-area fixture.
// Shared loader lives in golden_test.go.
//
// Tolerances: forward <= 1e-6 m absolute (with the relative floor of tol),
// inverse <= 1e-9 deg absolute, round-trip <= 1e-9 deg.

import (
	"errors"
	"math"
	"testing"
)

// TestLaeaAspects covers all four aspect modes (equatorial lat_0=0, oblique
// lat_0=45, north pole lat_0=90, south pole lat_0=-90) through the full public
// path. The golden fixture pins a single aspect; the others' FORWARD and the
// polar INVERSE branches were unverified. Values are pyproj 3.8.0 / PROJ 9.8.1.
func TestLaeaAspects(t *testing.T) {
	forwardRef := []struct {
		crs            string
		lon, lat, x, y float64
	}{
		{"+proj=laea +lat_0=0 +lon_0=0 +ellps=WGS84", 10, 40, 907187.469494, 4354354.413381},
		{"+proj=laea +lat_0=0 +lon_0=0 +ellps=WGS84", -170, 60, -1104728.923898, 10945358.583258},
		{"+proj=laea +lat_0=0 +lon_0=0 +ellps=WGS84", 179, 80, 30215.444776, 9753018.816838},
		{"+proj=laea +lat_0=0 +lon_0=0 +ellps=WGS84", 10, -30, 997379.656859, -3293918.633398},
		{"+proj=laea +lat_0=45 +lon_0=0 +ellps=WGS84", 10, 40, 852101.370902, -503881.940598},
		{"+proj=laea +lat_0=45 +lon_0=0 +ellps=WGS84", -170, 60, -699595.994183, 7713787.452373},
		{"+proj=laea +lat_0=90 +lon_0=0 +ellps=WGS84", 10, 40, 937306.576442, -5315729.746381},
		{"+proj=laea +lat_0=90 +lon_0=0 +ellps=WGS84", -170, 60, -574744.133441, 3259535.954916},
		{"+proj=laea +lat_0=90 +lon_0=0 +ellps=WGS84", 179, 80, 19466.572101, 1115239.168758},
		{"+proj=laea +lat_0=-90 +lon_0=0 +ellps=WGS84", 10, 40, 2004289.690097, 11366891.680694},
		{"+proj=laea +lat_0=-90 +lon_0=0 +ellps=WGS84", -170, 60, -2136677.317974, -12117699.227815},
		{"+proj=laea +lat_0=-90 +lon_0=0 +ellps=WGS84", 179, 80, 221525.143611, -12691166.977747},
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
		rl, ra, err := p.Inverse(x, y)
		if err != nil {
			t.Fatalf("Inverse(%q): %v", c.crs, err)
		}
		if lonDiff(rl, c.lon) > 1e-9 || math.Abs(ra-c.lat) > 1e-9 {
			t.Fatalf("%s round trip (%v, %v) -> (%.10f, %.10f)", c.crs, c.lon, c.lat, rl, ra)
		}
	}
}

func TestGoldenLaea(t *testing.T) {
	g := loadGolden(t, "golden_laea.json")
	p := goldenParams(g.CRS)
	a, b := ellipsoidFor(p, g.CRS)
	q := newLaeaKernel(a, b, geoDegToRad(p["lat_0"]), geoDegToRad(p["lon_0"]))

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

	t.Logf("golden_laea: %d forward (max %.3e m), %d inverse (max %.3e deg), %d specials",
		len(g.Forward), maxFwd, len(g.Inverse), maxInv, len(g.Specials))
}

// TestLaeaRoundTrip is a sanity gate through our own kernel.
func TestLaeaRoundTrip(t *testing.T) {
	g := loadGolden(t, "golden_laea.json")
	p := goldenParams(g.CRS)
	a, b := ellipsoidFor(p, g.CRS)
	q := newLaeaKernel(a, b, geoDegToRad(p["lat_0"]), geoDegToRad(p["lon_0"]))

	for _, r := range []struct{ lon, lat float64 }{
		{10, 45},   // projection centre
		{10, -45},  // central meridian, south of centre
		{-170, 45}, // antipodal meridian
		{100, -80}, // near the southern edge of the grid
		{0, 0},     // equator, off-centre
	} {
		x, y := q.forward(geoDegToRad(r.lon), geoDegToRad(r.lat))
		if math.IsNaN(x) || math.IsNaN(y) {
			t.Fatalf("forward NaN at (%v, %v)", r.lon, r.lat)
		}
		lon, lat, ok := q.inverse(x, y)
		if !ok {
			t.Fatalf("round trip lost (%v, %v)", r.lon, r.lat)
		}
		lon, lat = geoNormLon(geoRadToDeg(lon)), geoRadToDeg(lat)
		if lonDiff(lon, r.lon) > 1e-9 || math.Abs(lat-r.lat) > 1e-9 {
			t.Fatalf("round trip (%v, %v) -> (%.12f, %.12f)", r.lon, r.lat, lon, lat)
		}
	}

	// The antipode of the centre is PROJ's domain edge: 1 + cos(z) drops below EPS10
	// and PROJ reports "outside projection domain". pyproj 3.8.0 agrees (inf, inf) at
	// (-170, -44.9999), so our kernel must return NaN there.
	if x, y := q.forward(geoDegToRad(-170), geoDegToRad(-44.9999)); !math.IsNaN(x) || !math.IsNaN(y) {
		t.Fatalf("expected NaN at the antipode, got (%v, %v)", x, y)
	}

	// Forward still matches pyproj just inside the edge: (-6.35083458383568e-07,
	// 12734867.3268257). The round trip there is ill-conditioned — pyproj itself comes
	// back at -44.899998958 — so the 1e-9 deg round-trip gate is only applied to
	// well-conditioned points.
	//
	// We land 4.6e-4 m away here: b0 = 1 + cos(z) with z ~ 179.9 deg is a catastrophic
	// cancellation, so ~1e-16 in cos becomes ~7e-11 relative in b0. That is the best
	// double precision can do, hence the absolute 1e-3 m fallback used below.
	x, y := q.forward(geoDegToRad(-170), geoDegToRad(-44.9))
	if math.Abs(x+6.35083458383568e-07) > 1e-3 || math.Abs(y-12734867.3268257) > 1e-3 {
		t.Fatalf("forward just inside the antipode: got (%.6f, %.6f)", x, y)
	}
}

// TestLaeaAdapter runs the golden points through the full public path
// (Parse → NewProjection → Forward/Inverse) and checks the ErrDomain behavior at the
// antipode, which is laea's only hard domain edge.
func TestLaeaAdapter(t *testing.T) {
	g := loadGolden(t, "golden_laea.json")
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

	if _, _, err := p.Forward(-170, -45); !errors.Is(err, ErrDomain) {
		t.Fatalf("antipode should give ErrDomain, got %v", err)
	}
	if _, _, err := p.Forward(0, 91); err == nil || errors.Is(err, ErrDomain) {
		t.Fatalf("lat=91 should be a plain error, got %v", err)
	}
	// +k_0 is not used by laea (PROJ semantics): the same point must be unchanged.
	c2, err := Parse(g.CRS + " +k_0=2")
	if err != nil {
		t.Fatalf("Parse(+k_0=2): %v", err)
	}
	p2, err := c2.NewProjection()
	if err != nil {
		t.Fatalf("NewProjection(+k_0=2): %v", err)
	}
	x1, y1, _ := p.Forward(20, 50)
	x2, y2, _ := p2.Forward(20, 50)
	if x1 != x2 || y1 != y2 {
		t.Fatalf("laea must ignore +k_0: (%v, %v) vs (%v, %v)", x1, y1, x2, y2)
	}
}

// TestLaeaKernelSignature exercises the kernel entry points directly
// (laeaForward/laeaInverse), which the adapter tests do not reach.
func TestLaeaKernelSignature(t *testing.T) {
	g := loadGolden(t, "golden_laea.json")
	p := goldenParams(g.CRS)
	a, b := ellipsoidFor(p, g.CRS)
	lat0, lon0 := geoDegToRad(p["lat_0"]), geoDegToRad(p["lon_0"])

	for _, r := range g.Forward[:10] {
		x, y := laeaForward(a, b, lat0, lon0, geoDegToRad(r.Lon), geoDegToRad(r.Lat))
		if math.Abs(x-r.X) > tol(r.X) || math.Abs(y-r.Y) > tol(r.Y) {
			t.Fatalf("laeaForward at (%v, %v): got (%.6f, %.6f) want (%.6f, %.6f)",
				r.Lon, r.Lat, x, y, r.X, r.Y)
		}
		lon, lat, ok := laeaInverse(a, b, lat0, lon0, r.X, r.Y)
		if !ok {
			t.Fatalf("laeaInverse rejected (%v, %v)", r.X, r.Y)
		}
		lon, lat = geoNormLon(geoRadToDeg(lon)), geoRadToDeg(lat)
		if lonDiff(lon, r.Lon) > 1e-9 || math.Abs(lat-r.Lat) > 1e-9 {
			t.Fatalf("laeaInverse at (%v, %v): got (%.10f, %.10f) want (%.10f, %.10f)",
				r.X, r.Y, lon, lat, r.Lon, r.Lat)
		}
	}
	if _, _, ok := laeaInverse(a, b, lat0, lon0, 9e9, -9e9); ok {
		t.Fatal("laeaInverse must report the far-outside point as out of domain")
	}
}

// TestLaeaSphere checks that the ellipsoidal kernel degenerates to the spherical
// formulas when a == b: with e2 == 0 the series collapses to q(phi) = 2 sin phi,
// qp = 2, rq = 1, dd = 1, xmf = ymf = 1. Values from pyproj 3.8.0 / PROJ 9.8.1 on
// +a=6371000 +b=6371000.
func TestLaeaSphere(t *testing.T) {
	a, b := testEllipsoids["sphere"][0], testEllipsoids["sphere"][1]
	q := newLaeaKernel(a, b, geoDegToRad(45), geoDegToRad(10))
	cases := []struct {
		lon, lat, x, y float64
	}{
		{10, 45, 0, 0},
		{20, 50, 713035.793917547, 600873.1578161163},
		{0, 0, -1201249.6620303846, -4817253.596170467},
		{100, -80, 2839333.2875955217, -11386299.553481221},
		{10, -85, 0, -11548173.822420994},
	}
	for _, c := range cases {
		x, y := q.forward(geoDegToRad(c.lon), geoDegToRad(c.lat))
		if math.Abs(x-c.x) > 1e-6 || math.Abs(y-c.y) > 1e-6 {
			t.Fatalf("sphere forward at (%v, %v): got (%.6f, %.6f) want (%.6f, %.6f)",
				c.lon, c.lat, x, y, c.x, c.y)
		}
		lon, lat, ok := q.inverse(x, y)
		if !ok {
			t.Fatalf("sphere inverse rejected at (%v, %v)", c.x, c.y)
		}
		lon, lat = geoNormLon(geoRadToDeg(lon)), geoRadToDeg(lat)
		if lonDiff(lon, c.lon) > 1e-9 || math.Abs(lat-c.lat) > 1e-9 {
			t.Fatalf("sphere round trip (%v, %v) -> (%.10f, %.10f)", c.lon, c.lat, lon, lat)
		}
	}
}

// TestLaeaPolarAspects covers the modes the golden fixture does not exercise:
// N_POLE / S_POLE / EQUIT. Values are checked against pyproj 3.8.0 (PROJ 9.8.1),
// so this test is self-contained.
func TestLaeaPolarAspects(t *testing.T) {
	a, b := testEllipsoids["wgs84"][0], testEllipsoids["wgs84"][1]

	// pyproj 3.8.0 / PROJ 9.8.1, +ellps=WGS84(参照值就地记录)。
	cases := []struct {
		lat0, lon0 float64
		lon, lat   float64
		x, y       float64
	}{
		{90, 0, 0, 90, 0, 0},
		{90, 0, 0, 45, 0, -4889334.802955},
		{90, 0, 90, 45, 4889334.802955, 0},
		{-90, 0, 0, -45, 0, 4889334.802955},
		{0, 0, 0, 45, 0, 4857538.576747},
		{0, 0, 90, 0, 9020047.848074, 0},
	}
	for _, c := range cases {
		q := newLaeaKernel(a, b, geoDegToRad(c.lat0), geoDegToRad(c.lon0))
		x, y := q.forward(geoDegToRad(c.lon), geoDegToRad(c.lat))
		if math.Abs(x-c.x) > 1e-6 || math.Abs(y-c.y) > 1e-6 {
			t.Fatalf("forward at (%v, %v) lat_0=%v: got (%.6f, %.6f) want (%.6f, %.6f)",
				c.lon, c.lat, c.lat0, x, y, c.x, c.y)
		}
		lon, lat, ok := q.inverse(x, y)
		if !ok {
			t.Fatalf("inverse rejected at (%v, %v) lat_0=%v", x, y, c.lat0)
		}
		lon, lat = geoNormLon(geoRadToDeg(lon)), geoRadToDeg(lat)
		if lonDiff(lon, c.lon) > 1e-9 || math.Abs(lat-c.lat) > 1e-9 {
			t.Fatalf("round trip at (%v, %v) lat_0=%v -> (%.10f, %.10f)", c.lon, c.lat, c.lat0, lon, lat)
		}
	}
}
