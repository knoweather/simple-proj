package proj

// geolib_test.go — direct tests for geolib.go. The golden consumers only exercise
// the kernels, so these helpers would otherwise stay uncovered. Reference values
// are computed independently in Python from the definitions and cross-checked
// against pyproj 3.8.0 / PROJ 9.8.1.

import (
	"math"
	"testing"
)

const geolibTol = 1e-11

func TestGudermannianRoundTrip(t *testing.T) {
	for _, x := range []float64{0, 0.5, 1.0, 2.0, -1.0} {
		if got := InvGudermannian(Gudermannian(x)); math.Abs(got-x) > geolibTol {
			t.Fatalf("gd/gd^-1 round trip at %v: got %.12f", x, got)
		}
	}
	// gd(1.0) = 49.604937420855 deg
	if got := geoRadToDeg(Gudermannian(1.0)); math.Abs(got-49.604937420855) > geolibTol {
		t.Fatalf("Gudermannian(1.0) = %.12f deg", got)
	}
	// gd^-1(45 deg) = 0.881373587020
	if got := InvGudermannian(math.Pi / 4); math.Abs(got-0.881373587020) > geolibTol {
		t.Fatalf("InvGudermannian(45 deg) = %.12f", got)
	}
}

func TestIsometricLat(t *testing.T) {
	e := math.Sqrt((6378137*6378137 - 6356752.314245179*6356752.314245179) / (6378137 * 6378137))
	cases := []struct {
		latDeg, want float64
	}{
		{45, 0.876634653435},
		{80, 2.429639052865},
		{-30, -0.545957085182},
	}
	for _, c := range cases {
		got := IsometricLat(geoDegToRad(c.latDeg), e)
		if math.Abs(got-c.want) > geolibTol {
			t.Fatalf("IsometricLat(%v) = %.12f want %.12f", c.latDeg, got, c.want)
		}
		// exp(-psi) is the value PROJ uses in the polar stere branch.
		if got2 := ExpNegIsometric(geoDegToRad(c.latDeg), e); math.Abs(got2-math.Exp(-c.want)) > geolibTol {
			t.Fatalf("ExpNegIsometric(%v) = %.12f", c.latDeg, got2)
		}
		// Conformal latitude = gd(psi).
		if got3 := geoRadToDeg(ConformalLat(geoDegToRad(c.latDeg), e)); math.Abs(got3-conformalDeg(c.latDeg, e)) > geolibTol {
			t.Fatalf("ConformalLat(%v) = %.10f deg", c.latDeg, got3)
		}
		// Inverse: LatFromIsometric(psi) must come back to the geodetic latitude.
		phi := geoRadToDeg(LatFromIsometric(c.want, e))
		if math.Abs(phi-c.latDeg) > 1e-9 {
			t.Fatalf("LatFromIsometric round trip at %v: got %.10f deg", c.latDeg, phi)
		}
	}
	// e == 0 must reduce to the spherical Gudermannian.
	if got := ConformalLat(geoDegToRad(45), 0); math.Abs(geoRadToDeg(got)-45) > 1e-12 {
		t.Fatalf("ConformalLat with e=0: got %.10f deg", geoRadToDeg(got))
	}
}

func conformalDeg(latDeg, e float64) float64 {
	psi := IsometricLat(geoDegToRad(latDeg), e)
	return geoRadToDeg(math.Asin(math.Tanh(psi)))
}

func TestAuthalicLat(t *testing.T) {
	e2 := (6378137*6378137 - 6356752.314245179*6356752.314245179) / (6378137 * 6378137)
	s := NewAuthalicSeries(e2)

	// Reference values from the zone-area definition (independent Python Simpson
	// integral), matching pyproj 3.8.0 / PROJ 9.8.1.
	cases := []struct{ latDeg, wantDeg float64 }{
		{45, 44.8717028734},
		{80, 79.9560411435},
		{-30, -29.8889970345},
		{0, 0},
		{90, 90},
	}
	for _, c := range cases {
		got := geoRadToDeg(s.AuthalicLat(geoDegToRad(c.latDeg)))
		if math.Abs(got-c.wantDeg) > 1e-9 {
			t.Fatalf("AuthalicLat(%v) = %.10f deg want %.10f", c.latDeg, got, c.wantDeg)
		}
		phi := geoRadToDeg(s.LatFromAuthalic(geoDegToRad(c.wantDeg)))
		if math.Abs(phi-c.latDeg) > 1e-9 {
			t.Fatalf("LatFromAuthalic round trip at %v: got %.10f deg", c.latDeg, phi)
		}
	}

	// The wrong coefficient table ((2i-1)!!/(2i)!! / (2i+1)) gives 44.9812 deg at 45;
	// guard against a future regression back to it.
	if got := geoRadToDeg(s.AuthalicLat(math.Pi / 4)); math.Abs(got-44.9812037) < 1e-4 {
		t.Fatalf("authalic series regressed to the meridian-arc coefficients: %.10f deg", got)
	}

	// Spherical limit: q(phi) = 2 sin phi, qp = 2, authalic == geodetic.
	sp := NewAuthalicSeries(0)
	if math.Abs(sp.Q(math.Sin(0.7))-2*math.Sin(0.7)) > 1e-12 {
		t.Fatalf("spherical Q: got %.12f", sp.Q(math.Sin(0.7)))
	}
	if math.Abs(sp.qp-2) > 1e-12 {
		t.Fatalf("spherical qp: got %.12f", sp.qp)
	}
	if got := geoRadToDeg(sp.AuthalicLat(geoDegToRad(37.5))); math.Abs(got-37.5) > 1e-12 {
		t.Fatalf("spherical AuthalicLat: got %.10f deg", got)
	}
}

// TestGeoNormLon pins the output rule [-180, 180).
func TestGeoNormLon(t *testing.T) {
	for _, c := range []struct{ in, want float64 }{
		{170, 170}, {-190, 170}, {190, -170}, {-180, -180}, {180, -180}, {360, 0}, {-360, 0},
	} {
		if got := geoNormLon(c.in); math.Abs(got-c.want) > 1e-12 {
			t.Fatalf("geoNormLon(%v) = %.12f want %.12f", c.in, got, c.want)
		}
	}
}
