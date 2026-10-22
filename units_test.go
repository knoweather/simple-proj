package proj

// units_test.go — +units / +to_meter 的单位换算层。
//
// 语义(全部按 pyproj 3.8.0 / PROJ 9.8.1 实测锁定):
//   - 各 impl 内部一律按米算(含 x_0/y_0),对外输出 = 米 / to_meter;
//   - +units 查表(m/km/dm/cm/mm/kmi/in/ft/yd/mi/fath/ch/link/us-*/ind-*);
//   - +to_meter=N 表示 1 单位 = N 米;
//   - 两者同给时 +units 胜(PROJ pj_units 口径);
//   - latlong 输出是度,不受影响(PROJ 同样忽略);
//   - 逆变换先乘回 to_meter 再进内核。

import (
	"errors"
	"math"
	"testing"
)

// TestUnitsScaling 参照值 = pyproj(merc 球版,点 (10,20))。
func TestUnitsScaling(t *testing.T) {
	cases := []struct {
		crs  string
		x, y float64
	}{
		{"+proj=merc +a=6378137 +b=6378137 +units=m", 1113194.9079327357, 2273030.9269876895},
		{"+proj=merc +a=6378137 +b=6378137 +units=km", 1113.1949079327358, 2273.0309269876893},
		{"+proj=merc +a=6378137 +b=6378137 +to_meter=1000", 1113.1949079327358, 2273.0309269876893},
		{"+proj=merc +a=6378137 +b=6378137 +units=us-ft", 3652206.960442651, 7457435.632958779},
		// x_0 按米参与运算后再整体换算:1113.1949… + 500
		{"+proj=merc +a=6378137 +b=6378137 +units=km +x_0=500000", 1613.1949079327358, 2273.0309269876893},
		// +units 胜过 +to_meter
		{"+proj=merc +a=6378137 +b=6378137 +units=km +to_meter=2000", 1113.1949079327358, 2273.0309269876893},
		// +units=m 与 +to_meter=1000 同给:+units 胜,输出仍是米(pyproj 实测)
		{"+proj=merc +a=6378137 +b=6378137 +units=m +to_meter=1000", 1113194.9079327357, 2273030.9269876895},
		// 椭球投影同样适用
		{"+proj=eqc +ellps=WGS84 +units=ft", 3652214.26487118, 7258419.469067037},
	}
	for _, c := range cases {
		p := mustProj(t, c.crs)
		x, y, err := p.Forward(10, 20)
		if err != nil {
			t.Fatalf("%q Forward: %v", c.crs, err)
		}
		if math.Abs(x-c.x) > 1e-6 || math.Abs(y-c.y) > 1e-6 {
			t.Errorf("%q: (10,20) → (%.9f, %.9f), want (%.9f, %.9f)", c.crs, x, y, c.x, c.y)
		}
		lon, lat, err := p.Inverse(x, y)
		if err != nil || math.Abs(lon-10) > 1e-9 || math.Abs(lat-20) > 1e-9 {
			t.Errorf("%q 往返 = (%.12f, %.12f, %v)", c.crs, lon, lat, err)
		}
	}
}

// TestUnitsLatlongUnaffected latlong 的输出是度,单位层不介入。
func TestUnitsLatlongUnaffected(t *testing.T) {
	for _, s := range []string{
		"+proj=latlong +ellps=WGS84",
		"+proj=latlong +ellps=WGS84 +units=km",
		"+proj=latlong +ellps=WGS84 +to_meter=1000",
	} {
		p := mustProj(t, s)
		x, y, err := p.Forward(10, 20)
		if err != nil || x != 10 || y != 20 {
			t.Errorf("%q: (10,20) → (%.12g, %.12g, %v)", s, x, y, err)
		}
	}
}

// TestUnitsParseErrors 未知单位/非法 to_meter 在 Parse 期报错。
func TestUnitsParseErrors(t *testing.T) {
	for _, s := range []string{
		"+proj=merc +ellps=WGS84 +units=furlong",
		"+proj=merc +ellps=WGS84 +units=KMX",
		"+proj=merc +ellps=WGS84 +to_meter=0",
		"+proj=merc +ellps=WGS84 +to_meter=-1000",
		"+proj=merc +ellps=WGS84 +to_meter=abc",
	} {
		if _, err := Parse(s); err == nil {
			t.Errorf("Parse(%q) 应报错", s)
		}
	}
}

// TestUnitsEPSGRecognition 非米单位不能凭几何特征认成米制 EPSG 码;
// latlong 例外(单位不影响它的数值)。
func TestUnitsEPSGRecognition(t *testing.T) {
	cases := []struct {
		crs  string
		want int
	}{
		{"+proj=merc +a=6378137 +b=6378137 +units=m", 3857},
		{"+proj=merc +a=6378137 +b=6378137 +units=km", 0},
		{"+proj=merc +a=6378137 +b=6378137 +units=us-ft", 0},
		{"+proj=latlong +ellps=WGS84 +units=km", 4326},
	}
	for _, c := range cases {
		crs, err := Parse(c.crs)
		if err != nil {
			t.Fatalf("Parse(%q): %v", c.crs, err)
		}
		if got := crs.EPSG(); got != c.want {
			t.Errorf("%q: EPSG() = %d, want %d", c.crs, got, c.want)
		}
	}
}

// TestUnitsBatchConsistency 批量路径走的是同一个装饰器,结果须与单点逐位一致。
func TestUnitsBatchConsistency(t *testing.T) {
	p := mustProj(t, "+proj=merc +a=6378137 +b=6378137 +units=km")
	in := []float64{10, 20, -30, 45}
	xy, err := ForwardBatch(p, in)
	if err != nil {
		t.Fatalf("ForwardBatch: %v", err)
	}
	for i := 0; i < len(in); i += 2 {
		x, y, err := p.Forward(in[i], in[i+1])
		if err != nil {
			t.Fatal(err)
		}
		if xy[i] != x || xy[i+1] != y {
			t.Errorf("批量与单点不一致: %v vs (%v,%v)", xy[i:i+2], x, y)
		}
	}
	back, err := InverseBatch(p, xy)
	if err != nil {
		t.Fatalf("InverseBatch: %v", err)
	}
	for i := 0; i < len(in); i++ {
		if math.Abs(back[i]-in[i]) > 1e-9 {
			t.Errorf("批量往返第 %d 项: %v vs %v", i, back[i], in[i])
		}
	}
}

// TestUnitsDomainErrorPassthrough 单位装饰器不能吞掉域外语义:
// 错误要原样透出,坐标要留 NaN(不能被 to_meter 除成 -0 或 0)。
func TestUnitsDomainErrorPassthrough(t *testing.T) {
	p := mustProj(t, "+proj=geos +h=35785863 +lon_0=140.7 +ellps=WGS84 +units=km")
	x, y, err := p.Forward(0, 0) // 地球背面,视界外
	if !errors.Is(err, ErrDomain) {
		t.Fatalf("应 ErrDomain, got %v", err)
	}
	if !math.IsNaN(x) || !math.IsNaN(y) {
		t.Errorf("域外点应为 (NaN,NaN), got (%v,%v)", x, y)
	}
	// 批量:域外点 NaN、批末 ErrDomain,域内点仍按 km 输出
	out, err := ForwardBatch(p, []float64{0, 0, 140, 20})
	if !errors.Is(err, ErrDomain) {
		t.Fatalf("批量应 ErrDomain, got %v", err)
	}
	if !math.IsNaN(out[0]) || !math.IsNaN(out[1]) {
		t.Errorf("批量域外点应为 NaN, got (%v,%v)", out[0], out[1])
	}
	// pyproj: +proj=geos +h=35785863 +lon_0=140.7 +units=km 在 (140,20) 给
	// (-72.4758714859, 2142.1906756109) km
	if math.Abs(out[2]-(-72.4758714859)) > 1e-6 || math.Abs(out[3]-2142.1906756109) > 1e-6 {
		t.Errorf("批量域内点应为 km 单位 (-72.4759, 2142.1907), got (%.6f,%.6f)", out[2], out[3])
	}
}
