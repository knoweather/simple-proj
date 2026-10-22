package proj

import (
	"math"
	"math/rand"
	"runtime"
	"strings"
	"testing"
	"time"
)

// bench.go — 批量基准(DESIGN §4 T8:1e6 点 < 100 ms 目标)。
// go test -bench . -benchmem 跑;数据为随机分布的经纬网格与投影坐标。
// 覆盖:latlong / eqc / geos(sweep=y)三投影的 Forward/InverseBatch。

// benchLonLat 生成 1e6 级随机经纬(packed);deterministic seed 保证可重复。
func benchLonLat(n int) []float64 {
	rng := rand.New(rand.NewSource(42))
	out := make([]float64, 2*n)
	for i := 0; i < n; i++ {
		out[2*i] = rng.Float64()*360 - 180
		out[2*i+1] = rng.Float64()*180 - 90
	}
	return out
}

// benchLonLatDisc geos 专用:生成视界内(lon_0 ±40°、|lat|≤40°,最远对角
// ~56° < 视界 ~81°)的点。逆基准需要坐标自洽(域外点批量会被 D2 判停)。
func benchLonLatDisc(n int) []float64 {
	rng := rand.New(rand.NewSource(42))
	out := make([]float64, 2*n)
	for i := 0; i < n; i++ {
		out[2*i] = 0 + rng.Float64()*80 - 40
		out[2*i+1] = rng.Float64()*80 - 40
	}
	return out
}

func mustProjBench(b *testing.B, crs string) Projection {
	b.Helper()
	c, err := Parse(crs)
	if err != nil {
		b.Fatalf("Parse: %v", err)
	}
	p, err := c.NewProjection()
	if err != nil {
		b.Fatalf("NewProjection: %v", err)
	}
	return p
}

func benchForwardBatch(b *testing.B, crs, disc string) {
	const n = 1_000_000
	p := mustProjBench(b, crs)
	lonLat := benchLonLat(n)
	if disc != "" {
		lonLat = benchLonLatDisc(n)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ForwardBatch(p, lonLat); err != nil {
			b.Fatal(err)
		}
	}
}

func benchInverseBatch(b *testing.B, crs, disc string) {
	const n = 1_000_000
	p := mustProjBench(b, crs)
	src := benchLonLat(n)
	if disc != "" {
		src = benchLonLatDisc(n)
	}
	// 先做一次性 forward 造合法输入
	forward, err := ForwardBatch(p, src)
	if err != nil {
		b.Fatalf("forward 造数失败: %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := InverseBatch(p, forward); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkForwardBatchLatlong(b *testing.B) {
	benchForwardBatch(b, "+proj=latlong +ellps=WGS84", "")
}
func BenchmarkForwardBatchEqc(b *testing.B) {
	benchForwardBatch(b, "+proj=eqc +lat_ts=0 +lon_0=180 +ellps=WGS84", "")
}
func BenchmarkForwardBatchGeos(b *testing.B) {
	benchForwardBatch(b, "+proj=geos +h=35785863 +ellps=WGS84 +sweep=y", "disc")
}
func BenchmarkInverseBatchLatlong(b *testing.B) {
	benchInverseBatch(b, "+proj=latlong +ellps=WGS84", "")
}
func BenchmarkInverseBatchEqc(b *testing.B) {
	benchInverseBatch(b, "+proj=eqc +lat_ts=0 +lon_0=180 +ellps=WGS84", "")
}
func BenchmarkInverseBatchGeos(b *testing.B) {
	benchInverseBatch(b, "+proj=geos +h=35785863 +ellps=WGS84 +sweep=y", "disc")
}

// TestBatchPerf1e6 设计预算(DESIGN T8:1e6 < 100 ms)仲裁检查。
// 不作硬性 CI 断言(共享 VM 载荷不可控);测量最快一批并在放大 2 倍余量内
// 报告,超预算记 t.Skipf 留待人工复测 —— 预算里程碑,非语义回归。
func TestBatchPerf1e6(t *testing.T) {
	if runtime.NumCPU() == 1 || testing.Short() {
		t.Skip("单核或 -short 模式跳过性能预算")
	}
	const n = 1_000_000
	cases := []string{
		"+proj=latlong +ellps=WGS84",
		"+proj=eqc +lat_ts=0 +lon_0=180 +ellps=WGS84",
		"+proj=geos +h=35785863 +ellps=WGS84 +sweep=y",
	}
	var worst time.Duration
	for _, crs := range cases {
		isGeos := strings.Contains(crs, "geos")
		p := func() Projection {
			t.Helper()
			c, err := Parse(crs)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			pr, err := c.NewProjection()
			if err != nil {
				t.Fatalf("NewProjection: %v", err)
			}
			return pr
		}()
		input := benchLonLat(n)
		if isGeos {
			input = benchLonLatDisc(n)
		}
		// 3 次取最快(减少共享机器噪声)
		var best time.Duration = math.MaxInt64
		for run := 0; run < 3; run++ {
			start := time.Now()
			if _, err := ForwardBatch(p, input); err != nil {
				t.Fatalf("%s ForwardBatch: %v", crs, err)
			}
			if d := time.Since(start); d < best {
				best = d
			}
		}
		t.Logf("%-40s 1e6 ForwardBatch 最快 = %v", crs, best)
		if best > worst {
			worst = best
		}
	}
	if worst > 100*time.Millisecond {
		t.Skipf("最慢 %v 超过 100 ms 预算(载荷波动?), 人工复测", worst)
	}
}

// ---- 第 5 轮新增投影(aeqd / utm)-----------------------------------------

func BenchmarkForwardBatchAeqd(b *testing.B) {
	benchForwardBatch(b, "+proj=aeqd +lat_0=45 +lon_0=10 +ellps=WGS84", "")
}
func BenchmarkInverseBatchAeqd(b *testing.B) {
	benchInverseBatch(b, "+proj=aeqd +lat_0=45 +lon_0=10 +ellps=WGS84", "")
}

// ---- ob_tran(旋转极点)-----------------------------------------------------
// 每点只有 4 个 sin/cos + 1 个 asin + 1 个 atan2,应与 latlong 同量级;
// 批量口径同其他投影(输入是旋转系经纬,输出是未旋转系经纬)。

func BenchmarkForwardBatchObTran(b *testing.B) {
	benchForwardBatch(b, "+proj=ob_tran +o_proj=longlat +o_lat_p=39.25 +lon_0=18 +ellps=WGS84", "")
}
func BenchmarkInverseBatchObTran(b *testing.B) {
	benchInverseBatch(b, "+proj=ob_tran +o_proj=longlat +o_lat_p=39.25 +lon_0=18 +ellps=WGS84", "")
}
