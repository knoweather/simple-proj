package proj

// golden_test.go — shared golden-fixture helpers for the golden consumers.
//
// The package must not contain two definitions of these helpers: other test files
// should reuse them or keep their own prefixed (e.g. aGoldenRec).
//
// Tolerances:
//   forward    <= 1e-6 m absolute
//   inverse    <= 1e-9 deg absolute
//   round-trip <= 1e-9 deg
// plus a relative floor for extreme coordinates (see tol).

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

type goldenRec struct {
	Desc      string  `json:"desc"`
	Lon       float64 `json:"lon"`
	Lat       float64 `json:"lat"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	ExpectNaN bool    `json:"expectNaN"`
}

type goldenFile struct {
	CRS       string      `json:"crs"`
	Generator string      `json:"generator"`
	Generated string      `json:"generated"`
	Forward   []goldenRec `json:"forward"`
	Inverse   []goldenRec `json:"inverse"`
	Specials  []goldenRec `json:"specials"`
}

// Minimal ellipsoid table for the fixtures in use. Test-only, so the golden
// consumers do not depend on the ellipsoid derivation code. Keys are lowercase.
var testEllipsoids = map[string][2]float64{
	"wgs84":  {6378137, 6356752.314245179},
	"grs80":  {6378137, 6356752.314245179},
	"sphere": {6371000, 6371000},
}

func loadGolden(t *testing.T, name string) goldenFile {
	_, file, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(file), "xcheck", name)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	var g goldenFile
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatalf("parse golden: %v", err)
	}
	return g
}

func goldenParams(s string) map[string]float64 {
	p := map[string]float64{}
	for _, tok := range strings.Fields(s) {
		if !strings.HasPrefix(tok, "+") {
			continue
		}
		kv := strings.SplitN(strings.TrimPrefix(tok, "+"), "=", 2)
		if len(kv) != 2 {
			continue
		}
		v, err := strconv.ParseFloat(kv[1], 64)
		if err == nil {
			p[kv[0]] = v
		} else {
			p[kv[0]] = -1 // non-numeric (ellps name, sweep, units)
		}
	}
	return p
}

// ellipsoidFor resolves the fixture's ellipsoid the way EllipsoidFromParams does:
// explicit +a/+b win over the named ellipsoid.
func ellipsoidFor(p map[string]float64, crs string) (float64, float64) {
	name := "wgs84"
	for _, tok := range strings.Fields(crs) {
		if strings.HasPrefix(tok, "+ellps=") {
			name = strings.ToLower(strings.TrimPrefix(tok, "+ellps="))
		}
	}
	ell, ok := testEllipsoids[name]
	if !ok {
		ell = testEllipsoids["wgs84"]
	}
	if pa, ok := p["a"]; ok && pa > 0 {
		ell[0] = pa
	}
	if pb, ok := p["b"]; ok && pb > 0 {
		ell[1] = pb
	}
	return ell[0], ell[1]
}

// tol implements the "strict absolute, relative floor for extreme values" rule:
// strict 1e-6 m for ordinary coordinates; for the extreme grid points (polar
// stereographic rho reaches 1.3e10 m) double rounding alone is ~1e-13 relative,
// so a relative floor is allowed.
func tol(want float64) float64 {
	t := 1e-6
	if r := math.Abs(want) * 1e-12; r > t {
		t = r
	}
	return t
}

// lonDiff compares two longitudes modulo 360 so that -180 and 180 are the same point.
// pyproj returns 179.99999999999994 for the grid column lon = 180 while our kernel
// returns exactly -180; both are the same meridian.
func lonDiff(got, want float64) float64 {
	d := got - want
	if d > 180 {
		d -= 360
	} else if d < -180 {
		d += 360
	}
	return math.Abs(d)
}
