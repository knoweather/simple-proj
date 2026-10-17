package proj

import (
	"fmt"
	"testing"
)

// TestGBRIBFixedShapes 固定椭球/正球形状 → 半轴(ecCodes grib2
// template.3.shape_of_the_earth.def 口径;解码语义平移自 simple-grib
// grid/definition.go Radius()/earthDimension,CT133 已核实)。
func TestGRIBFixedShapes(t *testing.T) {
	cases := []struct {
		shape      int
		wantA      float64
		wantB      float64
		wantSphere bool
	}{
		{0, 6367470, 6367470, true},
		{2, 6378160, 6378160 * (1 - 1/298.25), false},              // IAG 1967
		{4, 6378137, 6378137 * (1 - 1/298.257223563), false},       // WGS84(f)
		{5, 6378137, 6356752.3142, false},                          // WGS84(显式短轴)
		{6, 6371229, 6371229, true},                                // NCEP 气象球
		{8, 6371200, 6371200, true},                                // BallSphere
		{9, 6377563.396, 6377563.396 * (1 - 1/299.3249646), false}, // Airy 1830 (OSGB)
		{10, 6378137, 6378137 * (1 - 1/298.257223563), false},      // WGS84 地心修正
		{11, 695990000, 695990000, true},                           // 太阳
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("shape=%d", c.shape), func(t *testing.T) {
			e, err := GRIBEllipsoid(c.shape, 0xFF, 0xFFFFFFFF) // 固定形状不消费维度八位组
			if err != nil {
				t.Fatalf("GRIBEllipsoid: 意外错误: %v", err)
			}
			nearF(t, "A", e.A, c.wantA, 1e-6)
			nearF(t, "B", e.B, c.wantB, 1e-6)
			if e.Sphere != c.wantSphere {
				t.Errorf("Sphere: got %v want %v", e.Sphere, c.wantSphere)
			}
		})
	}
}

// TestGRIBProducerRadius shape 1:radius = raw × 10^(-scale)(米);
// 缺失哨兵(scale 0xFF / raw 全一)报错。0xFF=255,raw 全一=0xFFFFFFFF。
func TestGRIBProducerRadius(t *testing.T) {
	// 6378137 m 直接编码:scale=0
	e, err := GRIBEllipsoid(1, 0, 6378137)
	if err != nil {
		t.Fatalf("意外错误: %v", err)
	}
	nearF(t, "A", e.A, 6378137, 1e-9)
	if !e.Sphere {
		t.Fatalf("shape 1 必为正球体")
	}
	// 缩放恢复:scale=3 → 6378137000/1000? 6378137 × 10^-3? 实际 r = 6378.137*1000
	// 取 scale=3、raw=6378137 → r=6378.137(米级下不合理但验证解码语义)
	e2, err := GRIBEllipsoid(1, 3, 6378137)
	if err != nil {
		t.Fatalf("意外错误: %v", err)
	}
	nearF(t, "A(scaled)", e2.A, 6378.137, 1e-9)
	// 缺失哨兵
	for _, c := range []struct {
		scale uint8
		raw   uint32
	}{
		{0xFF, 6378137},
		{0, 0xFFFFFFFF},
	} {
		if _, err := GRIBEllipsoid(1, c.scale, c.raw); err == nil {
			t.Errorf("scale=%d raw=%d 应报缺失错误", c.scale, c.raw)
		}
	}
	// 非法半径(r<=0):raw=0、scale=3 → 0
	if _, err := GRIBEllipsoid(1, 3, 0); err == nil {
		t.Errorf("raw=0 应报非正半径错误")
	}
}

// TestGRIBDualAxisGap GRIBEllipsoid 对双轴形状 3/7 保持 fail-loud(C#1-ACK 后
// 双轴解码改走 GRIBEllipsoidAxes,原函数只管单维/固定形状)。
func TestGRIBDualAxisGap(t *testing.T) {
	for _, shape := range []int{3, 7} {
		_, err := GRIBEllipsoid(shape, 0xc0, 1234)
		if err == nil {
			t.Fatalf("双轴形状 %d 应由 GRIBEllipsoidAxes 处理,GRIBEllipsoid 应报错", shape)
		}
	}
	if _, err := GRIBEllipsoid(42, 0, 0); err == nil {
		t.Fatalf("未知形状应报错")
	}
}

// TestGRIBEllipsoidAxes C#1 落地:生产者双轴(shape 3 单位 km,shape 7 单位 m)。
// 参照 WGS84:a=6378137 m,b=6356752 m(ecCodes 值 = raw × 10^-scale,km 项 ×1000)。
func TestGRIBEllipsoidAxes(t *testing.T) {
	// shape 7(米):scale=0 直接解码
	m7, err := GRIBEllipsoidAxes(7, 0, 6378137, 0, 6356752)
	if err != nil {
		t.Fatalf("shape 7 解码: %v", err)
	}
	nearF(t, "shape7 a", m7.A, 6378137, 1e-9)
	nearF(t, "shape7 b", m7.B, 6356752, 1e-9)
	if m7.Sphere {
		t.Errorf("双轴不等 → 不应为正球体")
	}

	// shape 3(km):scale=3 → 6378.137 km / 6356.752 km,×1000 升为米
	m3, err := GRIBEllipsoidAxes(3, 3, 6378137, 3, 6356752)
	if err != nil {
		t.Fatalf("shape 3 解码: %v", err)
	}
	nearF(t, "shape3 a", m3.A, 6378137, 1e-6)
	nearF(t, "shape3 b", m3.B, 6356752, 1e-6)

	// 两轴各自独立解码(scale 组合:major scale=0,minor scale=9)
	mm, err := GRIBEllipsoidAxes(3, 0, 6378, 9, 6356752)
	if err != nil {
		t.Fatalf("scale 组合: %v", err)
	}
	nearF(t, "scale 组合 a", mm.A, 6378*1000, 1e-6)
	nearF(t, "scale 组合 b", mm.B, 6.356752, 1e-6)

	// 缺失哨兵:任一轴缺失都报错
	for _, c := range []struct {
		name       string
		majorScale uint8
		minorScale uint8
		majorRaw   uint32
		minorRaw   uint32
	}{
		{"major 0xFF 尺度", 0xFF, 0, 6378137, 6356752},
		{"minor 0xFF 尺度", 0, 0xFF, 6378137, 6356752},
		{"major 全一", 0, 0, 0xFFFFFFFF, 1000},
		{"minor 全一", 0, 0, 1000, 0xFFFFFFFF},
	} {
		if _, err := GRIBEllipsoidAxes(7, c.majorScale, c.majorRaw, c.minorScale, c.minorRaw); err == nil {
			t.Errorf("%s: 应报缺失错误", c.name)
		}
	}

	// 非正轴 / minor > major(newEllipsoid fail-loud)
	if _, err := GRIBEllipsoidAxes(7, 0, 0, 0, 1000); err == nil {
		t.Errorf("major=0 应报错")
	}
	if _, err := GRIBEllipsoidAxes(7, 0, 1000, 0, 2000); err == nil {
		t.Errorf("minor > major 应报错")
	}

	// 非双轴 shape:拒收并提示走 GRIBEllipsoid
	if _, err := GRIBEllipsoidAxes(4, 0, 0, 0, 0); err == nil {
		t.Errorf("shape 4 应被 GRIBEllipsoidAxes 拒收")
	}
}
