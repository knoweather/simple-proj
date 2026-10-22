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
	"errors"
	"fmt"
	"math"
	"strings"
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

// TestTransformConcurrent Transformer 不可变,多个 goroutine 共享同一实例
// 必须与串行结果逐位一致(与 TestThreadSafety 同口径;-race 由 CI 的
// ubuntu job 覆盖,见 .github/workflows/ci.yml)。
func TestTransformConcurrent(t *testing.T) {
	tr := mustTransformer(t, "EPSG:4326", dhdn7)
	pts := [][2]float64{{13.4, 52.5}, {0, 0}, {-60, -45}, {140, 10}}
	type ref struct{ x, y float64 }
	refs := make([]ref, len(pts))
	for i, p := range pts {
		x, y, err := tr.Transform(p[0], p[1])
		if err != nil {
			t.Fatalf("Transform: %v", err)
		}
		refs[i] = ref{x, y}
	}
	const workers = 8
	errs := make(chan error, workers)
	for w := 0; w < workers; w++ {
		go func() {
			for i := 0; i < 200; i++ {
				for j, p := range pts {
					x, y, err := tr.Transform(p[0], p[1])
					if err != nil {
						errs <- err
						return
					}
					if x != refs[j].x || y != refs[j].y {
						errs <- fmt.Errorf("并发结果漂移 @%d", j)
						return
					}
				}
			}
			errs <- nil
		}()
	}
	for w := 0; w < workers; w++ {
		if err := <-errs; err != nil {
			t.Fatalf("并发: %v", err)
		}
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

// TestTransformerAccessorsAndErrors From/To、nil 入参、构造期与执行期的报错面。
func TestTransformerAccessorsAndErrors(t *testing.T) {
	from, err := Parse("EPSG:4326")
	if err != nil {
		t.Fatal(err)
	}
	to, err := Parse("EPSG:3857")
	if err != nil {
		t.Fatal(err)
	}
	tr, err := NewTransformer(from, to)
	if err != nil {
		t.Fatalf("NewTransformer: %v", err)
	}
	if tr.From() != from || tr.To() != to {
		t.Error("From()/To() 应回传构造时的 CRS")
	}

	if _, err := NewTransformer(nil, to); err == nil {
		t.Error("nil 源 CRS 应报错")
	}
	if _, err := NewTransformer(from, nil); err == nil {
		t.Error("nil 目标 CRS 应报错")
	}

	// 目标 CRS 本身构造不出来(utm 缺 zone)→ 构造期报错并指出是哪一侧
	bad, err := Parse("+proj=utm +ellps=WGS84")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewTransformer(from, bad); err == nil {
		t.Error("目标 CRS 无法建投影时应报错")
	} else if !strings.Contains(err.Error(), "目标 CRS") {
		t.Errorf("报错应指出是目标 CRS 一侧: %v", err)
	}

	// 执行期:源是 latlong,输入经度 400 → 普通 error(非 ErrDomain)
	if _, _, err := tr.Transform(400, 0); err == nil {
		t.Error("经度 400 应报错")
	}
}

// TestTransformBatchDomain 批量里域外点写 NaN、批末返回 ErrDomain。
func TestTransformBatchDomain(t *testing.T) {
	// tmerc 的 |Ce| 限界:9e9 米在反变换时必然出界
	tm, err := Parse("+proj=tmerc +lat_0=0 +lon_0=105 +k=0.9996 +ellps=WGS84")
	if err != nil {
		t.Fatal(err)
	}
	wgs, err := Parse("EPSG:4326")
	if err != nil {
		t.Fatal(err)
	}
	tr, err := NewTransformer(tm, wgs)
	if err != nil {
		t.Fatal(err)
	}
	out, err := tr.TransformBatch([]float64{9e9, 9e9, 0, 0})
	if !errors.Is(err, ErrDomain) {
		t.Fatalf("批末应为 ErrDomain, got %v", err)
	}
	if !math.IsNaN(out[0]) || !math.IsNaN(out[1]) {
		t.Errorf("域外点应写 NaN, got (%v, %v)", out[0], out[1])
	}
	// (0,0) 在 tmerc 平面里是中央子午线上的点 → (105, 0)
	if math.Abs(out[2]-105) > 1e-9 || math.Abs(out[3]) > 1e-9 {
		t.Errorf("域内点应正常输出 (105, 0), got (%v, %v)", out[2], out[3])
	}
	// 非域外错误(坏输入)整批失败
	if _, err := tr.TransformBatch([]float64{math.Inf(1), 0}); err == nil {
		t.Error("Inf 输入应整批失败")
	}
}

// TestGeocentricPole 地心↔大地在极点上的退化处理(经度丢失,纬度 ±90)。
func TestGeocentricPole(t *testing.T) {
	for _, c := range []struct{ z, wantLat float64 }{
		{wgs84Ellipsoid.B, 90}, {-wgs84Ellipsoid.B, -90},
	} {
		lon, lat := geocentricToGeodetic(0, 0, c.z, wgs84Ellipsoid)
		if lon != 0 || math.Abs(lat-c.wantLat) > 1e-12 {
			t.Errorf("(0,0,%v) → (%.3f, %.12f), want (0, %v)", c.z, lon, lat, c.wantLat)
		}
	}
	// 极点回地心:XY 归零
	X, Y, Z := geodeticToGeocentric(123, 90, wgs84Ellipsoid)
	if math.Hypot(X, Y) > 1e-9 || math.Abs(Z-wgs84Ellipsoid.B) > 1e-6 {
		t.Errorf("北极 → (%.6f, %.6f, %.6f)", X, Y, Z)
	}
}

// TestHelmertDegenerateScale 尺度因子退化成 0(s = 1+ppm·1e-6 = 0)时,
// 矩阵不可逆,走兜底分支而不是 panic 或 NaN。
func TestHelmertDegenerateScale(t *testing.T) {
	c, err := Parse("+proj=longlat +ellps=bessel +towgs84=0,0,0,0,0,0,-1000000")
	if err != nil {
		t.Fatal(err)
	}
	wgs, err := Parse("EPSG:4326")
	if err != nil {
		t.Fatal(err)
	}
	tr, err := NewTransformer(wgs, c)
	if err != nil {
		t.Fatalf("NewTransformer: %v", err)
	}
	lon, lat, err := tr.Transform(13.4, 52.5)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if math.IsNaN(lon) || math.IsNaN(lat) || math.IsInf(lon, 0) || math.IsInf(lat, 0) {
		t.Errorf("退化尺度应给有限值, got (%.6f, %.6f)", lon, lat)
	}
}
