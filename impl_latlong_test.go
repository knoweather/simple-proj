package proj

import (
	"errors"
	"math"
	"testing"
)

func newLatlongProj(t *testing.T) Projection {
	t.Helper()
	c, err := Parse("+proj=latlong +ellps=WGS84")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	p, err := c.NewProjection()
	if err != nil {
		t.Fatalf("NewProjection: %v", err)
	}
	return p
}

func TestLatlongIdentity(t *testing.T) {
	p := newLatlongProj(t)
	for _, c := range []struct{ lon, lat float64 }{
		{0, 0}, {10.5, -33.25}, {-180, 85}, {179.9, -85}, {0, 90}, {0, -90}, {150, 45},
	} {
		x, y, err := p.Forward(c.lon, c.lat)
		if err != nil {
			t.Fatalf("Forward(%v,%v): %v", c.lon, c.lat, err)
		}
		if x != c.lon || y != c.lat {
			t.Errorf("Forward(%v,%v) = (%v,%v)", c.lon, c.lat, x, y)
		}
		lon, lat, err := p.Inverse(x, y)
		if err != nil {
			t.Fatalf("Inverse: %v", err)
		}
		if lon != x || lat != y {
			t.Errorf("Inverse 往返失真: (%v,%v)", lon, lat)
		}
	}
}

func TestLatlongLonNormalization(t *testing.T) {
	p := newLatlongProj(t)
	for _, c := range []struct {
		in, want float64
	}{
		{180, -180},      // §2.5: [-180,180),180 归一 -180(同物理点)
		{-180, -180},     // 边界保留
		{190, -170},      // 出 [-180,180] 带内
		{-190, 170},      //
		{360, 0},         // 360 → 0
		{-360, 0},        //
		{279, -81},       // 内圈:279 → -81
	} {
		x, y, err := p.Forward(c.in, 0)
		if err != nil {
			t.Fatalf("Forward(%v,0): %v", c.in, err)
		}
		if y != 0 {
			t.Fatalf("Forward y 非零: %v", y)
		}
		if x != c.want {
			t.Errorf("Forward(%v) 归一 got %v want %v (圆周距离 %v)", c.in, x, c.want, circDist(x, c.want))
		}
	}
}

// circDist 经度圆周距离(度):min(|Δ|, 360-|Δ|)。
func circDist(a, b float64) float64 {
	d := math.Mod(a-b, 360)
	if d < 0 {
		d += 360
	}
	if d > 180 {
		d -= 360
	}
	return math.Abs(d)
}

func TestLatlongInvalidInputs(t *testing.T) {
	p := newLatlongProj(t)
	for _, c := range []struct{ lon, lat float64 }{
		{361, 0}, {-361, 0}, {0, 90.1}, {0, -90.1}, {math.NaN(), 0}, {0, math.NaN()},
		{math.Inf(1), 0}, {math.Inf(-1), -90},
	} {
		_, _, err := p.Forward(c.lon, c.lat)
		if err == nil || errors.Is(err, ErrDomain) {
			t.Errorf("Forward(%v,%v) 应报非 ErrDomain 错误, got %v", c.lon, c.lat, err)
		}
		_, _, err = p.Inverse(c.lon, c.lat)
		if err == nil || errors.Is(err, ErrDomain) {
			t.Errorf("Inverse(%v,%v) 应报非 ErrDomain 错误, got %v", c.lon, c.lat, err)
		}
	}
}
