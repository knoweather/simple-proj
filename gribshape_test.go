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

// TestGRIBDualAxisGap 双轴形状 3/7 → 冻结契约装不下(C#1,PROGRESS-A.md)。
func TestGRIBDualAxisGap(t *testing.T) {
	for _, shape := range []int{3, 7} {
		_, err := GRIBEllipsoid(shape, 0xc0, 1234)
		if err == nil {
			t.Fatalf("双轴形状 %d 当前应返回“契约缺口”错误", shape)
		}
	}
	if _, err := GRIBEllipsoid(42, 0, 0); err == nil {
		t.Fatalf("未知形状应报错")
	}
}
