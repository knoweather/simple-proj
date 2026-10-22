package proj

import (
	"errors"
	"math"
	"testing"
)

// proj_batch_test.go — ForwardBatch / InverseBatch 语义(决策 D2):
//   - 单点 ErrDomain → 该点 (NaN,NaN),循环继续,批末 err = ErrDomain;
//   - 其他错误 → 整批立即失败返回 nil;
//   - 长度奇数 / nil Projection → 错误。

func TestForwardBatchErrors(t *testing.T) {
	p, _ := mustLatlongCRS(t).NewProjection()
	if _, err := ForwardBatch(p, []float64{0, 0, 10}); err == nil {
		t.Errorf("奇数长度应报错")
	}
	if _, err := ForwardBatch(nil, []float64{0, 0}); err == nil {
		t.Errorf("nil Projection 应报错")
	}
	// 非法输入(经度出界,非 ErrDomain)→ 整批失败
	if _, err := ForwardBatch(p, []float64{0, 0, 360.1, 0}); err == nil {
		t.Errorf("非域错误应整批失败")
	} else if errors.Is(err, ErrDomain) {
		t.Errorf("出界错误不应用 ErrDomain 冒充: %v", err)
	}
}

func mustLatlongCRS(t *testing.T) *CRS {
	t.Helper()
	c, err := Parse("+proj=latlong +ellps=WGS84")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return c
}

func TestForwardBatchDomainPause(t *testing.T) {
	// geos ellipsoid second point outside domain → NaN placeholder + ErrDomain at batch end, valid points processed normally
	p := newBatchGeos(t)
	xy, err := ForwardBatch(p, []float64{-30.5, -60, 105.2, 30, -60, -60})
	if !errors.Is(err, ErrDomain) {
		t.Errorf("batch domain-exit should return ErrDomain, got %v", err)
	}
	if !math.IsNaN(xy[2]) || !math.IsNaN(xy[3]) {
		t.Errorf("point outside domain should be written as NaN: %v,%v", xy[2], xy[3])
	}
	if math.IsNaN(xy[0]) || math.IsNaN(xy[4]) {
		t.Errorf("point inside domain should not be contaminated: %v", xy)
	}
	// 有效点数值抽查(参照 probe9;1ulp 级次序差容许,用 forward 门槛)
	if e := math.Abs(xy[0]-(-1472629.351973165)) + math.Abs(xy[1]-(-4958537.734914743)); e > 1e-6 {
		t.Errorf("域内点值漂移: %v,%v (e=%g)", xy[0], xy[1], e)
	}
}

// newBatchGeos geos ellipsoid lon_0=0 ([placed separately to make the t.Helper attribution clean])
func newBatchGeos(t *testing.T) Projection {
	t.Helper()
	c, err := Parse("+proj=geos +h=35785863 +ellps=WGS84 +lon_0=0")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	pr, err := c.NewProjection()
	if err != nil {
		t.Fatalf("NewProjection: %v", err)
	}
	return pr
}

func TestInverseBatchRoundtrip(t *testing.T) {
	c := mustLatlongCRS(t)
	p, err := c.NewProjection()
	if err != nil {
		t.Fatalf("NewProjection: %v", err)
	}
	ll := []float64{0, 0, 10.5, -33.25, 179.9, 85}
	xy, err := ForwardBatch(p, ll)
	if err != nil {
		t.Fatalf("ForwardBatch: %v", err)
	}
	// latlong 批往返:值应逐位一致(恒等映射 + 归一)
	back, err := InverseBatch(p, xy)
	if err != nil {
		t.Fatalf("InverseBatch: %v", err)
	}
	for i := 0; i < len(ll); i += 2 {
		if circDist(back[i], ll[i]) > 1e-9 || math.Abs(back[i+1]-ll[i+1]) > 1e-9 {
			t.Errorf("批往返漂移 @%d: got %v want %v", i, back[i:i+2], ll[i:i+2])
		}
	}
}

func TestInverseBatchDomain(t *testing.T) {
	c, err := Parse("+proj=geos +h=35785863 +ellps=WGS84 +lon_0=0")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	p, _ := c.NewProjection()
	// 第一/三点远值域外,第二点为域内 forward 结果
	inside := []float64{-1472629.351973165, -4958537.734914743}
	ll, err := InverseBatch(p, []float64{9e9, -9e9, inside[0], inside[1], 1e12, 1e12})
	if !errors.Is(err, ErrDomain) {
		t.Errorf("逆批量域外应返回 ErrDomain, got %v", err)
	}
	if math.IsNaN(ll[2]) || math.IsNaN(ll[3]) {
		t.Errorf("域内点被污染: %v,%v", ll[2], ll[3])
	}
	if e := math.Abs(ll[2]+30.5) + math.Abs(ll[3]+60.0); e > 1e-9 {
		t.Errorf("域内逆值漂移: %v,%v (e=%g)", ll[2], ll[3], e)
	}
}

// TestInverseBatchErrors 批量逆向的入参校验与域外语义(与 ForwardBatch 对称)。
func TestInverseBatchErrors(t *testing.T) {
	if _, err := InverseBatch(nil, []float64{1, 2}); err == nil {
		t.Error("nil Projection 应报错")
	}
	p := mustProj(t, "+proj=tmerc +lat_0=0 +lon_0=105 +k=0.9996 +ellps=WGS84")
	if _, err := InverseBatch(p, []float64{1, 2, 3}); err == nil {
		t.Error("奇数长度应报错")
	}
	// 域外点写 NaN,批末 ErrDomain,域内点结果仍可用
	out, err := InverseBatch(p, []float64{9e9, 9e9, 0, 0})
	if !errors.Is(err, ErrDomain) {
		t.Fatalf("批末应为 ErrDomain, got %v", err)
	}
	if !math.IsNaN(out[0]) || !math.IsNaN(out[1]) {
		t.Errorf("域外点应写 NaN, got (%v,%v)", out[0], out[1])
	}
	if math.Abs(out[2]-105) > 1e-9 || math.Abs(out[3]) > 1e-9 {
		t.Errorf("域内点应为 (105,0), got (%v,%v)", out[2], out[3])
	}
}
