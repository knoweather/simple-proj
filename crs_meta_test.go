package proj

// crs_meta_test.go — IsGeographic / Units / Equals(第二轮 goproj 评审的融合)。
//
// goproj 里那几个 PROJ 查询(proj_angular_input / proj_degree_input …)的用途是
// "让调用方知道该喂度还是米";本库的约定固定,但调用方仍需要机器可读的判断,
// 于是有 IsGeographic / Units。Equals 对应 pyproj 的 CRS.equals。

import "testing"

func TestIsGeographicAndUnits(t *testing.T) {
	cases := []struct {
		crs   string
		geo   bool
		units string
	}{
		{"EPSG:4326", true, "degree"},
		{"+proj=longlat +ellps=WGS84", true, "degree"},
		{"EPSG:3857", false, "m"},
		{"EPSG:32650", false, "m"},
		{"+proj=utm +zone=50 +units=km +ellps=WGS84", false, "km"},
		{"+proj=merc +a=6378137 +b=6378137 +units=us-ft", false, "us-ft"},
		{"+proj=merc +a=6378137 +b=6378137 +to_meter=1000", false, "to_meter=1000"},
		{"+proj=aeqd +lat_0=45 +lon_0=10 +ellps=WGS84", false, "m"},
	}
	for _, c := range cases {
		crs, err := Parse(c.crs)
		if err != nil {
			t.Fatalf("Parse(%q): %v", c.crs, err)
		}
		if got := crs.IsGeographic(); got != c.geo {
			t.Errorf("%q: IsGeographic = %v, want %v", c.crs, got, c.geo)
		}
		if got := crs.Units(); got != c.units {
			t.Errorf("%q: Units = %q, want %q", c.crs, got, c.units)
		}
	}
	var nilCRS *CRS
	if nilCRS.IsGeographic() {
		t.Error("nil CRS 不该是地理坐标系")
	}
	if nilCRS.Units() != "" {
		t.Error("nil CRS 的 Units 应为空")
	}
}

func TestCRSEquals(t *testing.T) {
	must := func(s string) *CRS {
		t.Helper()
		c, err := Parse(s)
		if err != nil {
			t.Fatalf("Parse(%q): %v", s, err)
		}
		return c
	}
	// 注意:本库的 Equals 比 pyproj 的 CRS.equals **宽松** —— 后者还比 datum 名
	// 与 CRS 名,所以 "EPSG:4326" vs "+proj=latlong +ellps=WGS84" 在 pyproj 里是
	// False。本库不建模 datum/名称,只关心"算出来的坐标是否一致",故判相等
	// (口径见 CRS.Equals 的注释)。
	same := [][2]string{
		// 写法不同、语义相同
		{"EPSG:4326", "+proj=latlong +ellps=WGS84"},
		{"+proj=eqc +lat_ts=0 +lon_0=180 +ellps=WGS84", "+proj=eqc +ellps=WGS84 +lon_0=180"},
		{"+proj=tmerc +k=0.9996 +lon_0=117 +ellps=WGS84", "+proj=tmerc +k_0=0.9996 +lat_0=0 +lon_0=117 +x_0=0 +y_0=0 +ellps=WGS84"},
		{"EPSG:3857", "+proj=merc +a=6378137 +b=6378137 +lat_ts=0 +lon_0=0 +x_0=0 +y_0=0 +units=m"},
		{"+proj=merc +a=6378137 +b=6378137 +units=m", "+proj=merc +a=6378137 +b=6378137"},
		{"+proj=utm +zone=50 +ellps=WGS84", "+proj=utm +zone=50 +ellps=WGS84 +units=m"},
	}
	for _, p := range same {
		if !must(p[0]).Equals(must(p[1])) {
			t.Errorf("%q 与 %q 应判相等", p[0], p[1])
		}
	}

	diff := [][2]string{
		{"EPSG:4326", "EPSG:3857"}, // 投影不同
		{"+proj=eqc +lon_0=180 +ellps=WGS84", "+proj=eqc +lon_0=0 +ellps=WGS84"}, // 参数不同
		{"+proj=geos +h=35785863 +sweep=x +ellps=WGS84", "+proj=geos +h=35785863 +sweep=y +ellps=WGS84"},
		{"+proj=merc +ellps=WGS84", "+proj=merc +ellps=GRS80"}, // 椭球不同
		{"+proj=utm +zone=50 +ellps=WGS84", "+proj=utm +zone=51 +ellps=WGS84"},
		{"+proj=merc +a=6378137 +b=6378137 +units=km", "+proj=merc +a=6378137 +b=6378137"},
		{"+proj=longlat +ellps=WGS84 +towgs84=1,2,3", "+proj=longlat +ellps=WGS84"},
		{"+proj=longlat +ellps=WGS84 +towgs84=1,2,3", "+proj=longlat +ellps=WGS84 +towgs84=1,2,4"},
	}
	for _, p := range diff {
		if must(p[0]).Equals(must(p[1])) {
			t.Errorf("%q 与 %q 应判不等", p[0], p[1])
		}
	}

	// nil 语义:两个 nil 相等,一个 nil 不等
	var a, b *CRS
	if !a.Equals(b) {
		t.Error("两个 nil 应判相等")
	}
	if a.Equals(must("EPSG:4326")) || must("EPSG:4326").Equals(a) {
		t.Error("nil 与非 nil 应判不等")
	}
}
