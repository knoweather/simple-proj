package proj

// impl_utm_test.go — UTM 是 tmerc 的固定参数化(zone → lon_0/k_0/x_0/y_0),
// 所以这里只测「参数化是否正确 + 与 PROJ 的同义覆盖规则」,tmerc 本身的
// 精度由 tmerc_test.go / sweep 覆盖。
// 参照值来自 pyproj 3.8.0 / PROJ 9.8.1 实测(2026-10-22)。

import (
	"math"
	"strings"
	"testing"
)

const utmTol = 1e-6

// TestUtmZones 北/南半球与 zone→中央子午线的映射。
func TestUtmZones(t *testing.T) {
	cases := []struct {
		crs  string
		x, y float64
	}{
		// zone 50 → lon_0 = 117,(117,30) 恰在中央子午线上:x 必为假东 500000
		{"+proj=utm +zone=50 +ellps=WGS84", 500000.0, 3318785.3525812062},
		{"+proj=utm +zone=50 +south +ellps=WGS84", 500000.0, 13318785.352581207},
		{"+proj=utm +zone=50 +south +ellps=GRS80", 500000.0, 13318785.352485882},
		{"EPSG:32650", 500000.0, 3318785.3525812062},
		{"EPSG:32750", 500000.0, 13318785.352581207},
	}
	for _, c := range cases {
		crs, err := Parse(c.crs)
		if err != nil {
			t.Fatalf("Parse(%q): %v", c.crs, err)
		}
		p, err := crs.NewProjection()
		if err != nil {
			t.Fatalf("NewProjection(%q): %v", c.crs, err)
		}
		x, y, err := p.Forward(117, 30)
		if err != nil {
			t.Fatalf("%q Forward: %v", c.crs, err)
		}
		if math.Abs(x-c.x) > utmTol || math.Abs(y-c.y) > utmTol {
			t.Errorf("%q: (117,30) → (%.6f, %.6f), want (%.6f, %.6f)", c.crs, x, y, c.x, c.y)
		}
		lon, lat, err := p.Inverse(x, y)
		if err != nil || math.Abs(lon-117) > 1e-9 || math.Abs(lat-30) > 1e-9 {
			t.Errorf("%q 往返 = (%.12f, %.12f, %v)", c.crs, lon, lat, err)
		}
	}
}

// TestUtmOverrides zone/+south 覆盖用户给的 lon_0/x_0/y_0(PROJ utm.cpp 同款)。
func TestUtmOverrides(t *testing.T) {
	ref := mustProj(t, "+proj=utm +zone=50 +ellps=WGS84")
	rx, ry, _ := ref.Forward(117, 30)
	for _, s := range []string{
		"+proj=utm +zone=50 +lon_0=100 +ellps=WGS84",
		"+proj=utm +zone=50 +x_0=0 +y_0=0 +ellps=WGS84",
		"+proj=utm +zone=50 +lat_0=10 +k_0=1 +ellps=WGS84",
	} {
		p := mustProj(t, s)
		x, y, _ := p.Forward(117, 30)
		if x != rx || y != ry {
			t.Errorf("%q 应与基准一致(lon_0/x_0/y_0/lat_0/k_0 被 zone 覆盖), got (%.6f, %.6f) vs (%.6f, %.6f)", s, x, y, rx, ry)
		}
	}
}

// TestUtmZoneValidation zone 缺失/越界/非整数一律报错(PROJ: Invalid value for zone)。
func TestUtmZoneValidation(t *testing.T) {
	// 投影级参数校验落在 NewProjection(与 geos 的 +h、eqc 的 +lat_ts 同口径),
	// Parse 只管语法/数值域。
	for _, s := range []string{
		"+proj=utm +ellps=WGS84",
		"+proj=utm +zone=0 +ellps=WGS84",
		"+proj=utm +zone=61 +ellps=WGS84",
		"+proj=utm +zone=50.5 +ellps=WGS84",
	} {
		c, err := Parse(s)
		if err != nil {
			continue // Parse 就拦下也算过
		}
		if _, err := c.NewProjection(); err == nil {
			t.Errorf("NewProjection(%q) 应报错", s)
		}
	}
	// 边界值 1 / 60 合法
	for _, z := range []string{"1", "60"} {
		if _, err := mustProjOK("+proj=utm +zone=" + z + " +ellps=WGS84"); err != nil {
			t.Errorf("zone=%s 应合法: %v", z, err)
		}
	}
}

// TestUtmEPSGRecognise EPSG 码进 → 特征回识别出同一个码(326xx/327xx)。
func TestUtmEPSGRecognise(t *testing.T) {
	for _, code := range []int{32601, 32650, 32660, 32701, 32733, 32760} {
		s := "EPSG:" + itoa(code)
		crs, err := Parse(s)
		if err != nil {
			t.Fatalf("Parse(%q): %v", s, err)
		}
		if got := crs.EPSG(); got != code {
			t.Errorf("%s: EPSG() = %d", s, got)
		}
		if !strings.Contains(crs.String(), "+proj=utm") {
			t.Errorf("%s: String() 应为等价 proj 串, got %q", s, crs.String())
		}
	}
	// GRS80 的 UTM 不是 WGS84 UTM,不该识别成 326xx
	crs, err := Parse("+proj=utm +zone=50 +ellps=GRS80")
	if err != nil {
		t.Fatal(err)
	}
	if got := crs.EPSG(); got != 0 {
		t.Errorf("GRS80 UTM 的 EPSG() 应为 0, got %d", got)
	}
}

// mustProjOK Parse + NewProjection,只关心错误。
func mustProjOK(s string) (Projection, error) {
	c, err := Parse(s)
	if err != nil {
		return nil, err
	}
	return c.NewProjection()
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var b []byte
	for v > 0 {
		b = append([]byte{byte('0' + v%10)}, b...)
		v /= 10
	}
	return string(b)
}

// mustProj 解析并构造任意投影实例。
func mustProj(t *testing.T, s string) Projection {
	t.Helper()
	c, err := Parse(s)
	if err != nil {
		t.Fatalf("Parse(%q): %v", s, err)
	}
	p, err := c.NewProjection()
	if err != nil {
		t.Fatalf("NewProjection(%q): %v", s, err)
	}
	return p
}
