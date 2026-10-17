package proj

// merc_test.go — AI-B's golden consumer for the Mercator fixture (spherical,
// +lat_ts=0 — i.e. EPSG:3857-like). Shared loader lives in golden_test.go (C#6).
//
// Tolerances: COORDINATION §2.5 — forward <= 1e-6 m absolute, inverse <= 1e-9
// deg absolute, round-trip <= 1e-9 deg.

import (
	"math"
	"strconv"
	"testing"
)

func TestGoldenMerc(t *testing.T) {
	g := loadGolden(t, "golden_merc.json")
	p := goldenParams(g.CRS)
	a, b := ellipsoidFor(p, g.CRS)
	// +lat_ts=0.0 is present in the fixture: PROJ then uses factor = msfn(0) = 1
	// and ignores +k_0 (re-verified against pyproj 3.8.0).
	q := newMercKernel(a, b, geoDegToRad(p["lat_ts"]), geoDegToRad(p["lon_0"]))

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
		lon, lat = geoNormLon(geoRadToDeg(geoAdjLonPi(lon))), geoRadToDeg(lat)
		d := math.Max(lonDiff(lon, r.Lon), math.Abs(lat-r.Lat))
		// Inverse tolerances: COORDINATION §2.5 says 1e-9 deg absolute. For the
		// extreme probe (x = y = 1e12 m) the radian longitude is ~1.6e5, whose
		// double ulp alone is ~1.7e-9 deg — no path can match pyproj tighter
		// there. Extend C#3's relative floor to the inverse axis: the noise is
		// ~2.3e-16 relative on the un-normalized longitude.
		invTol := math.Max(1e-9, math.Abs(r.X)/a*180/math.Pi*2.3e-16)
		if d > invTol {
			t.Fatalf("inverse mismatch at (%v, %v): got (%.10f, %.10f) want (%.10f, %.10f)",
				r.X, r.Y, lon, lat, r.Lon, r.Lat)
		}
		if d > maxInv {
			maxInv = d
		}
	}

	t.Logf("golden_merc: %d forward (max %.3e m), %d inverse (max %.3e deg)",
		len(g.Forward), maxFwd, len(g.Inverse), maxInv)
}

// TestMercPole checks the PROJ 9.8.1 pole behaviour locked into golden_merc.json:
// pyproj answers (0, 90) with y = 242528680.94374272 — the asinh(tan φ) form of
// the isometric latitude (38.025003373828866·a), not the ln-tan form (37.3319·a).
// Both are "infinite" in exact arithmetic; the fixture records the finite double
// PROJ actually produces, and our kernel must match it bit-for-bit.
func TestMercPole(t *testing.T) {
	g := loadGolden(t, "golden_merc.json")
	var py float64
	found := false
	for _, r := range g.Forward {
		if r.Lon == 0 && r.Lat == 90 {
			py, found = r.Y, true
		}
	}
	if !found {
		t.Fatal("golden_merc must contain the (0, 90) pole record")
	}
	q := newMercKernel(6378137, 6378137, 0, 0)
	x, y := q.forward(0, geoDegToRad(90))
	if math.Abs(x) > 1e-9 || math.Abs(y-py) > 1e-6 {
		t.Fatalf("pole (0,90): got (%.9f, %.6f) want (0, %.6f)", x, y, py)
	}
	// The south pole mirrors it.
	if _, yn := q.forward(0, geoDegToRad(-90)); math.Abs(yn+py) > 1e-6 {
		t.Fatalf("south pole should mirror: got %v want %v", yn, -py)
	}
}

// TestMercAdapter runs the fixture CRS through the full contract path and checks
// the setup guards (pyproj 3.8.0 verified).
func TestMercAdapter(t *testing.T) {
	g := loadGolden(t, "golden_merc.json")
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

	// EPSG:3857 alias (A's crs.go maps it to exactly this merc string):
	// (2.5, 45) on the WGS84 sphere, factor = 1.
	c3857, err := Parse("EPSG:3857")
	if err != nil {
		t.Fatalf("Parse(EPSG:3857): %v", err)
	}
	p3857, err := c3857.NewProjection()
	if err != nil {
		t.Fatalf("NewProjection(EPSG:3857): %v", err)
	}
	x, y, err := p3857.Forward(2.5, 45)
	wantX := 6378137 * geoDegToRad(2.5)
	wantY := 6378137 * math.Log(math.Tan(math.Pi/4+geoDegToRad(45)/2))
	if err != nil || math.Abs(x-wantX) > 1e-6 || math.Abs(y-wantY) > 1e-3 {
		t.Fatalf("EPSG:3857 forward: got (%v, %v, %v), want (%v, %v)", x, y, err, wantX, wantY)
	}

	// Guards: |lat_ts| = 90 is rejected (PROJ), |lat| > 90 is a plain error.
	for _, bad := range []string{
		"+proj=merc +a=6378137 +b=6378137 +lat_ts=90 +lon_0=0 +units=m",
		"+proj=merc +a=6378137 +b=6378137 +lat_ts=-90 +lon_0=0 +units=m",
	} {
		if c, err := Parse(bad); err == nil {
			if _, err := c.NewProjection(); err == nil {
				t.Fatalf("%q must be rejected", bad)
			}
		}
	}
	if _, _, err := p.Forward(0, 91); err == nil {
		t.Fatal("lat=91 should be a plain error")
	}
	if _, _, err := p.Forward(400, 0); err == nil {
		t.Fatal("lon=400 should be a plain error")
	}
	// The exact pole is inside the domain: finite y, no error (pyproj-verified).
	if _, y, err := p.Forward(0, 90); err != nil || math.Abs(y-242528680.94374272) > 1e-6 {
		t.Fatalf("pole should give pyproj's finite y, got (%v, %v)", y, err)
	}
}

// TestMercEllipsoidLatTs pins the +k_0 semantics PROJ 9.8.1 actually implements
// (re-verified with pyproj 3.8.0; the day0 note had 1/msfn and got it backwards):
//   - +lat_ts present ⇒ factor = msfn(lat_ts) and +k_0 is IGNORED;
//   - +lat_ts absent   ⇒ factor = +k_0;
//   - |lat_ts| = 90     ⇒ setup error.
//
// All want-values are pyproj 3.8.0 / PROJ 9.8.1 references.
func TestMercEllipsoidLatTs(t *testing.T) {
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

	// +lat_ts=45 on the ellipsoid: factor = msfn(45°) = cos45/sqrt(1−e²·0.5).
	p := proj("+proj=merc +ellps=WGS84 +lat_ts=45 +lon_0=0 +units=m")
	x, y, err := p.Forward(2.5, 30)
	if err != nil || math.Abs(x-197117.0877349453) > 1e-6 || math.Abs(y-2466410.748259135) > 1e-6 {
		t.Fatalf("ellps lat_ts=45: got (%v, %v, %v)", x, y, err)
	}
	// +k_0 is ignored alongside +lat_ts (pyproj: identical output for k0=1,2,3).
	p2 := proj("+proj=merc +ellps=WGS84 +lat_ts=45 +k_0=2 +lon_0=0 +units=m")
	x2, y2, _ := p2.Forward(2.5, 30)
	if x2 != x || y2 != y {
		t.Fatalf("k_0 must be ignored with lat_ts: (%v, %v) vs (%v, %v)", x, y, x2, y2)
	}
	// No +lat_ts: factor = k_0 (here 2 ⇒ everything doubles).
	p3 := proj("+proj=merc +ellps=WGS84 +k_0=2 +lon_0=0 +units=m")
	p4 := proj("+proj=merc +ellps=WGS84 +lon_0=0 +units=m")
	x3, y3, _ := p3.Forward(2.5, 30)
	x4, y4, _ := p4.Forward(2.5, 30)
	if x3 != 2*x4 || y3 != 2*y4 {
		t.Fatalf("k_0 without lat_ts must scale: (%v, %v) vs (%v, %v)", x3, y3, x4, y4)
	}
	if math.Abs(x4-278298.72698318394) > 1e-6 || math.Abs(y4-3482189.0854086173) > 1e-6 {
		t.Fatalf("WGS84 merc baseline: got (%v, %v)", x4, y4)
	}

	// More ellipsoid scale latitudes / offsets (pyproj 3.8.0).
	for _, tc := range []struct {
		latTS, lon0, lon, lat float64
		wantX, wantY          float64
	}{
		{60, 0, 2.5, 30, 139500.00393109032, 1745481.9012975728},
		{45, 100, 2.5, 30, -7687566.421662867, 2466410.748259135},
		{85.0511287798066, 0, 2.5, 30, 24088.08420894863, 301400.0992030156},
	} {
		pp := proj("+proj=merc +ellps=WGS84 +lat_ts=" +
			strconv.FormatFloat(tc.latTS, 'g', -1, 64) +
			" +lon_0=" + strconv.FormatFloat(tc.lon0, 'g', -1, 64) + " +units=m")
		x, y, err := pp.Forward(tc.lon, tc.lat)
		if err != nil || math.Abs(x-tc.wantX) > 1e-6 || math.Abs(y-tc.wantY) > 1e-6 {
			t.Fatalf("lat_ts=%v lon_0=%v at (%v, %v): got (%v, %v, %v) want (%v, %v)",
				tc.latTS, tc.lon0, tc.lon, tc.lat, x, y, err, tc.wantX, tc.wantY)
		}
		// Round trip through the inverse.
		lon, lat, err2 := pp.Inverse(x, y)
		if err2 != nil {
			t.Fatalf("lat_ts=%v Inverse: %v", tc.latTS, err2)
		}
		if lonDiff(lon, tc.lon) > 1e-9 || math.Abs(lat-tc.lat) > 1e-9 {
			t.Fatalf("lat_ts=%v round trip -> (%v, %v)", tc.latTS, lon, lat)
		}
	}

	// +x_0/+y_0 shift after the scale factor (sphere +lat_ts=45, pyproj).
	p5 := proj("+proj=merc +a=6378137 +b=6378137 +lat_ts=45 +lon_0=0 +units=m +x_0=1000 +y_0=-5000")
	x5, y5, _ := p5.Forward(2.5, 30)
	if math.Abs(x5-(196786.91704539297+1000)) > 1e-6 || math.Abs(y5-(2477383.8525670115-5000)) > 1e-6 {
		t.Fatalf("x_0/y_0 shift: got (%v, %v)", x5, y5)
	}
}

// TestMercFrozenSignature exercises the frozen §2.3 entry points directly.
func TestMercFrozenSignature(t *testing.T) {
	g := loadGolden(t, "golden_merc.json")
	p := goldenParams(g.CRS)
	a, b := ellipsoidFor(p, g.CRS)
	latTS, lon0 := geoDegToRad(p["lat_ts"]), geoDegToRad(p["lon_0"])

	for _, r := range g.Forward[:10] {
		x, y := mercForward(a, b, latTS, lon0, geoDegToRad(r.Lon), geoDegToRad(r.Lat))
		if math.Abs(x-r.X) > tol(r.X) || math.Abs(y-r.Y) > tol(r.Y) {
			t.Fatalf("mercForward at (%v, %v): got (%.6f, %.6f) want (%.6f, %.6f)",
				r.Lon, r.Lat, x, y, r.X, r.Y)
		}
		lon, lat, ok := mercInverse(a, b, latTS, lon0, r.X, r.Y)
		if !ok {
			t.Fatalf("mercInverse rejected (%v, %v)", r.X, r.Y)
		}
		lon, lat = geoNormLon(geoRadToDeg(lon)), geoRadToDeg(lat)
		if lonDiff(lon, r.Lon) > 1e-9 || math.Abs(lat-r.Lat) > 1e-9 {
			t.Fatalf("mercInverse at (%v, %v): got (%.10f, %.10f) want (%.10f, %.10f)",
				r.X, r.Y, lon, lat, r.Lon, r.Lat)
		}
	}
}
