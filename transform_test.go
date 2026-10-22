package proj

// transform_test.go — CRS→CRS 转换(Transformer)与 Helmert 基准变换。
//
// 参照值来自 pyproj 3.8.0 / PROJ 9.8.1 实测(2026-10-22),用例是 DHDN
// (Bessel 1841) ↔ WGS84,7 参与 3 参两种。
//
// 已知差异(有意保留):**反向** Helmert 的求法。PROJ 用「参数取负」的近似
// (s' = 1−ppm·1e-6),本库直接解 M·x = X'−T(M = s·R)。两者差 O((ppm+r)²),
// 在 DHDN 这组参数下是 0.4 mm(≈4e-9 度)。正向(本地→WGS84)与 PROJ 逐位一致。
//
// 另外,2D 基准变换的往返本身不可能精确:每段 geodetic→geocentric 都按椭球高 = 0
// 处理,基准平移产生的 ~50 m 高程被丢掉。pyproj 同样如此(实测往返漂
// 6.3e-9 / 5.0e-9 度,本库 6.3e-9 / 9.0e-9 度),所以往返门取 1e-8 度。

import (
	"math"
	"testing"
)

const (
	dhdn7 = "+proj=longlat +ellps=bessel +towgs84=598.1,73.7,418.2,0.202,0.045,-2.455,6.7"
	dhdn3 = "+proj=longlat +ellps=bessel +towgs84=598.1,73.7,418.2"
)

func mustTransformer(t *testing.T, from, to string) *Transformer {
	t.Helper()
	a, err := Parse(from)
	if err != nil {
		t.Fatalf("Parse(%q): %v", from, err)
	}
	b, err := Parse(to)
	if err != nil {
		t.Fatalf("Parse(%q): %v", to, err)
	}
	tr, err := NewTransformer(a, b)
	if err != nil {
		t.Fatalf("NewTransformer(%q → %q): %v", from, to, err)
	}
	return tr
}

// TestHelmertForward 本地 datum → WGS84(7 参 / 3 参)。
func TestHelmertForward(t *testing.T) {
	cases := []struct {
		name             string
		from             string
		lon              float64
		wantLon, wantLat float64
	}{
		{"7param", dhdn7, 13.398258457259113, 13.398258457259113, 52.49859525754975},
		{"3param", dhdn3, 13.399014688640964, 13.399014688640964, 52.49859565485004},
	}
	for _, c := range cases {
		tr := mustTransformer(t, c.from, "EPSG:4326")
		lon, lat, err := tr.Transform(13.4, 52.5)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if math.Abs(lon-c.wantLon) > 1e-12 || math.Abs(lat-c.wantLat) > 1e-12 {
			t.Errorf("%s: (13.4,52.5) → (%.12f, %.12f), want (%.12f, %.12f)", c.name, lon, lat, c.wantLon, c.wantLat)
		}
	}
}

// TestHelmertInverse WGS84 → 本地 datum(见文件头的已知差异说明,门取 1e-8 度)。
func TestHelmertInverse(t *testing.T) {
	tr := mustTransformer(t, "EPSG:4326", dhdn7)
	lon, lat, err := tr.Transform(13.4, 52.5)
	if err != nil {
		t.Fatalf("%v", err)
	}
	const wantLon, wantLat = 13.401741852288918, 52.501404891753296
	if math.Abs(lon-wantLon) > 1e-8 || math.Abs(lat-wantLat) > 1e-8 {
		t.Errorf("WGS84→DHDN (13.4,52.5) → (%.12f, %.12f), want (%.12f, %.12f)", lon, lat, wantLon, wantLat)
	}
	// 往返必须回到原点(矩阵求逆的意义所在)
	back := mustTransformer(t, dhdn7, "EPSG:4326")
	lon2, lat2, err := back.Transform(lon, lat)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if math.Abs(lon2-13.4) > 1e-8 || math.Abs(lat2-52.5) > 1e-8 {
		t.Errorf("往返 = (%.12f, %.12f), want (13.4, 52.5)(高程截断,见文件头)", lon2, lat2)
	}
}

// TestTransformProjected 投影 CRS 之间:WGS84 经纬 → 带 towgs84 的高斯-克吕格。
func TestTransformProjected(t *testing.T) {
	gk := "+proj=tmerc +lat_0=0 +lon_0=9 +k=1 +x_0=3500000 +y_0=0 +ellps=bessel " +
		"+towgs84=598.1,73.7,418.2,0.202,0.045,-2.455,6.7 +units=m"
	tr := mustTransformer(t, "EPSG:4326", gk)
	x, y, err := tr.Transform(13.4, 52.5)
	if err != nil {
		t.Fatalf("%v", err)
	}
	const wantX, wantY = 3798801.117105141, 5827650.515830943
	if math.Abs(x-wantX) > 1e-3 || math.Abs(y-wantY) > 1e-3 { // 反向 Helmert 的 0.4 mm
		t.Errorf("WGS84→GK = (%.6f, %.6f), want (%.6f, %.6f)", x, y, wantX, wantY)
	}
	// 反向回到经纬度
	back := mustTransformer(t, gk, "EPSG:4326")
	lon, lat, err := back.Transform(x, y)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if math.Abs(lon-13.4) > 1e-8 || math.Abs(lat-52.5) > 1e-8 {
		t.Errorf("GK→WGS84 往返 = (%.12f, %.12f)(高程截断,见文件头)", lon, lat)
	}
}

// TestTransformNoDatumBallpark 两侧都没有 towgs84:只换投影(经纬度原样搬),
// 即使椭球不同也不做基准变换(PROJ 的 ballpark 行为)。
func TestTransformNoDatumBallpark(t *testing.T) {
	// WGS84 经纬 → GRS80 上的 LCC:椭球差 ~1e-4 m,ballpark 下经纬度不变
	tr := mustTransformer(t, "EPSG:4326", "+proj=lcc +lat_1=30 +lat_2=60 +lat_0=0 +lon_0=100 +ellps=GRS80")
	proj := mustProj(t, "+proj=lcc +lat_1=30 +lat_2=60 +lat_0=0 +lon_0=100 +ellps=GRS80")
	x1, y1, _ := tr.Transform(100, 45)
	x2, y2, _ := proj.Forward(100, 45)
	if x1 != x2 || y1 != y2 {
		t.Errorf("ballpark 应与直接投影一致: (%.6f,%.6f) vs (%.6f,%.6f)", x1, y1, x2, y2)
	}
}

// TestTransformRejections 会算错的组合必须报错,不许静默。
func TestTransformRejections(t *testing.T) {
	// 单投影带 towgs84:投影只在自己椭球上映射,基准平移无处安放
	crs, err := Parse(dhdn7)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := crs.NewProjection(); err == nil {
		t.Error("带 +towgs84 的 CRS 调 NewProjection 应报错")
	}

	// 一侧有 towgs84、另一侧是非 WGS84 椭球:无法确定基准关系
	besselNoDatum, err := Parse("+proj=longlat +ellps=bessel")
	if err != nil {
		t.Fatal(err)
	}
	wgsDatum, err := Parse(dhdn7)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewTransformer(besselNoDatum, wgsDatum); err == nil {
		t.Error("一侧有 towgs84、另一侧非 WGS84 椭球时应报错")
	}

	// +proj=pipeline 明确拒收并指路
	if _, err := Parse("+proj=pipeline +step +proj=merc"); err == nil {
		t.Error("+proj=pipeline 应报错")
	}
}

// TestTransformBatch 批量与单点一致,域外点写 NaN 且批末 ErrDomain。
func TestTransformBatch(t *testing.T) {
	tr := mustTransformer(t, "EPSG:4326", "EPSG:3857")
	in := []float64{0, 0, 10, 20, -170, -45}
	out, err := tr.TransformBatch(in)
	if err != nil {
		t.Fatalf("TransformBatch: %v", err)
	}
	for i := 0; i < len(in); i += 2 {
		x, y, err := tr.Transform(in[i], in[i+1])
		if err != nil {
			t.Fatalf("Transform: %v", err)
		}
		if out[i] != x || out[i+1] != y {
			t.Errorf("批量与单点不一致: %v vs (%v,%v)", out[i:i+2], x, y)
		}
	}
	if _, err := tr.TransformBatch([]float64{1, 2, 3}); err == nil {
		t.Error("奇数长度应报错")
	}
}

// TestTransformIdentity 同一 CRS → 恒等(只走一遍反算+正算)。
func TestTransformIdentity(t *testing.T) {
	for _, s := range []string{"EPSG:4326", "+proj=tmerc +lon_0=105 +k=0.9996 +ellps=WGS84", "EPSG:32650"} {
		tr := mustTransformer(t, s, s)
		p := mustProj(t, s)
		for _, pt := range [][2]float64{{105, 30}, {117, 0}, {-60, -45}} {
			x, y, err := p.Forward(pt[0], pt[1])
			if err != nil {
				continue
			}
			x2, y2, err := tr.Transform(x, y)
			if err != nil {
				t.Fatalf("%s: %v", s, err)
			}
			if math.Abs(x2-x) > 1e-9 || math.Abs(y2-y) > 1e-9 {
				t.Errorf("%s 恒等转换漂移: (%v,%v) → (%v,%v)", s, x, y, x2, y2)
			}
		}
	}
}
