package proj

// bench_b_test.go — batch benchmarks for stere / laea / lcc / merc / tmerc,
// complementing bench_test.go (latlong / eqc / geos). Same methodology:
// 1e6 deterministic random points, ForwardBatch/InverseBatch, ReportAllocs.
//
// Point bands stay inside each projection's valid region so the inverse
// benchmarks get self-consistent inputs (an out-of-domain point pauses the
// batch with ErrDomain):
//   - stere / laea / lcc: global grid (no hard domain edge in the batch band);
//   - merc (sphere, +lat_ts=0): |lat| < 89.5 (the poles are degenerate);
//   - tmerc (+lon_0=105 +k=0.9996): lon within +-75 deg of the central
//     meridian (WGS84 etmerc's |Ce| limit corresponds to ~81.7 deg at the
//     equator; 75 keeps the whole band comfortably inside).

import (
	"math/rand"
	"testing"
)

// benchBLonLatBand 生成 1e6 级带限制随机经纬(packed);deterministic seed。
func benchBLonLatBand(n int, lonLo, lonHi, latLo, latHi float64) []float64 {
	rng := rand.New(rand.NewSource(43))
	out := make([]float64, 2*n)
	for i := 0; i < n; i++ {
		out[2*i] = rng.Float64()*(lonHi-lonLo) + lonLo
		out[2*i+1] = rng.Float64()*(latHi-latLo) + latLo
	}
	return out
}

func benchBForwardBatch(b *testing.B, crs string, lonLo, lonHi, latLo, latHi float64) {
	const n = 1_000_000
	p := mustProjBench(b, crs)
	lonLat := benchBLonLatBand(n, lonLo, lonHi, latLo, latHi)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ForwardBatch(p, lonLat); err != nil {
			b.Fatal(err)
		}
	}
}

func benchBInverseBatch(b *testing.B, crs string, lonLo, lonHi, latLo, latHi float64) {
	const n = 1_000_000
	p := mustProjBench(b, crs)
	src := benchBLonLatBand(n, lonLo, lonHi, latLo, latHi)
	forward, err := ForwardBatch(p, src) // 一次性 forward 造合法输入
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

const (
	benchGlobalLonLo = -180.0
	benchGlobalLonHi = 180.0
	benchGlobalLatLo = -89.0
	benchGlobalLatHi = 89.0
	benchMercLat     = 89.5 // 球面 merc:极点退化,带内收缩到 ±89.5
	benchTmerLonLo   = 30.0 // 105-75
	benchTmerLonHi   = 180.0
)

func BenchmarkForwardBatchStere(b *testing.B) {
	benchBForwardBatch(b, "+proj=stere +lat_0=90 +lat_ts=60 +lon_0=-100 +ellps=WGS84",
		benchGlobalLonLo, benchGlobalLonHi, benchGlobalLatLo, benchGlobalLatHi)
}
func BenchmarkForwardBatchLaea(b *testing.B) {
	benchBForwardBatch(b, "+proj=laea +lat_0=45 +lon_0=10 +ellps=WGS84",
		benchGlobalLonLo, benchGlobalLonHi, benchGlobalLatLo, benchGlobalLatHi)
}
func BenchmarkForwardBatchLcc(b *testing.B) {
	benchBForwardBatch(b, "+proj=lcc +lat_1=30 +lat_2=60 +lat_0=0 +lon_0=100 +ellps=WGS84",
		benchGlobalLonLo, benchGlobalLonHi, benchGlobalLatLo, benchGlobalLatHi)
}
func BenchmarkForwardBatchMerc(b *testing.B) {
	benchBForwardBatch(b, "+proj=merc +a=6378137 +b=6378137 +lat_ts=0 +lon_0=0 +units=m",
		benchGlobalLonLo, benchGlobalLonHi, -benchMercLat, benchMercLat)
}
func BenchmarkForwardBatchTmerc(b *testing.B) {
	benchBForwardBatch(b, "+proj=tmerc +lat_0=0 +lon_0=105 +k=0.9996 +ellps=WGS84",
		benchTmerLonLo, benchTmerLonHi, benchGlobalLatLo, benchGlobalLatHi)
}

func BenchmarkInverseBatchStere(b *testing.B) {
	benchBInverseBatch(b, "+proj=stere +lat_0=90 +lat_ts=60 +lon_0=-100 +ellps=WGS84",
		benchGlobalLonLo, benchGlobalLonHi, benchGlobalLatLo, benchGlobalLatHi)
}
func BenchmarkInverseBatchLaea(b *testing.B) {
	benchBInverseBatch(b, "+proj=laea +lat_0=45 +lon_0=10 +ellps=WGS84",
		benchGlobalLonLo, benchGlobalLonHi, benchGlobalLatLo, benchGlobalLatHi)
}
func BenchmarkInverseBatchLcc(b *testing.B) {
	benchBInverseBatch(b, "+proj=lcc +lat_1=30 +lat_2=60 +lat_0=0 +lon_0=100 +ellps=WGS84",
		benchGlobalLonLo, benchGlobalLonHi, benchGlobalLatLo, benchGlobalLatHi)
}
func BenchmarkInverseBatchMerc(b *testing.B) {
	benchBInverseBatch(b, "+proj=merc +a=6378137 +b=6378137 +lat_ts=0 +lon_0=0 +units=m",
		benchGlobalLonLo, benchGlobalLonHi, -benchMercLat, benchMercLat)
}
func BenchmarkInverseBatchTmerc(b *testing.B) {
	benchBInverseBatch(b, "+proj=tmerc +lat_0=0 +lon_0=105 +k=0.9996 +ellps=WGS84",
		benchTmerLonLo, benchTmerLonHi, benchGlobalLatLo, benchGlobalLatHi)
}
