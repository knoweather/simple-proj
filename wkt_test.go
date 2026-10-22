package proj

// wkt_test.go — WKT2 / WKT1 解析的端到端对拍。
//
// 口径:wkt.go 的做法是「WKT → 等价 proj 串 → Parse」,所以这里不比字符串,
// 而是比**坐标**:pyproj 用同一份 WKT 算出的 forward 结果(xcheck/wkt_ref.json,
// 由 xcheck/gen_wkt.py 生成),Go 侧从 WKT 解析出的投影必须复现同样的坐标。
// 这样任何「参数映射错键 / 单位换算两次 / 伪墨卡托当椭球」的失误都会露头。
//
// 容差:绝对 1e-6 + 相对 1e-12(km / us-ft 这类 CRS 的坐标量级到 1e6,
// 单位换算本身的 1 ulp 就在 1e-9 量级)。

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

type wktPoint struct {
	Lon, Lat, X, Y float64
}

// wktVariant 一种方言(WKT2 / WKT1)及其参照坐标。每个方言的参照由 pyproj
// 解析**该方言文本**得到 —— WKT1 有损(如 GDAL 的 EPSG:3857 WKT1 丢掉伪墨卡托
// 的球面性质),拿源 CRS 的坐标当基准会误判。
type wktVariant struct {
	Kind   string     `json:"kind"`
	WKT    string     `json:"wkt"`
	Points []wktPoint `json:"points"`
}

type wktCase struct {
	Name     string       `json:"name"`
	Source   string       `json:"source"`
	Variants []wktVariant `json:"variants"`
}

type wktRef struct {
	Generator string    `json:"generator"`
	Generated string    `json:"generated"`
	Cases     []wktCase `json:"cases"`
}

func loadWKTRef(t *testing.T) wktRef {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(file), "xcheck", "wkt_ref.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read wkt_ref: %v (run: python xcheck/gen_wkt.py)", err)
	}
	var r wktRef
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatalf("parse wkt_ref: %v", err)
	}
	return r
}

func wktTol(ref float64) float64 {
	return math.Max(1e-6, math.Abs(ref)*1e-12)
}

// TestWKTParity WKT2 与 WKT1 两种方言都要复现 pyproj 的坐标。
func TestWKTParity(t *testing.T) {
	r := loadWKTRef(t)
	t.Logf("reference: %s (%s), %d cases", r.Generator, r.Generated, len(r.Cases))

	for _, c := range r.Cases {
		for _, v := range c.Variants {
			crs, err := Parse(v.WKT)
			if err != nil {
				t.Errorf("%s/%s: Parse: %v", c.Name, v.Kind, err)
				continue
			}
			p, err := crs.NewProjection()
			if err != nil {
				t.Errorf("%s/%s: NewProjection: %v", c.Name, v.Kind, err)
				continue
			}
			for _, pt := range v.Points {
				x, y, err := p.Forward(pt.Lon, pt.Lat)
				if err != nil {
					t.Errorf("%s/%s: Forward(%v,%v): %v", c.Name, v.Kind, pt.Lon, pt.Lat, err)
					continue
				}
				if math.Abs(x-pt.X) > wktTol(pt.X) || math.Abs(y-pt.Y) > wktTol(pt.Y) {
					t.Errorf("%s/%s: (%v,%v) → (%.9f, %.9f), 参照 (%.9f, %.9f)",
						c.Name, v.Kind, pt.Lon, pt.Lat, x, y, pt.X, pt.Y)
				}
			}
			// 往返:解析结果 → proj 串 → 再解析,行为必须一致(String() 只回 proj 串)
			again, err := Parse(crs.String())
			if err != nil {
				t.Errorf("%s/%s: String() 不可再解析: %q → %v", c.Name, v.Kind, crs.String(), err)
				continue
			}
			p2, err := again.NewProjection()
			if err != nil {
				t.Errorf("%s/%s: 二次 NewProjection: %v", c.Name, v.Kind, err)
				continue
			}
			for _, pt := range v.Points {
				x1, y1, _ := p.Forward(pt.Lon, pt.Lat)
				x2, y2, _ := p2.Forward(pt.Lon, pt.Lat)
				if x1 != x2 || y1 != y2 {
					t.Errorf("%s/%s: 往返不一致 (%v,%v) vs (%v,%v) [%s]",
						c.Name, v.Kind, x1, y1, x2, y2, crs.String())
					break
				}
			}
		}
	}
}

// TestWKTRejections 明确拒收的根节点/方法/参数要有可读报错,不能静默给错结果。
func TestWKTRejections(t *testing.T) {
	cases := []struct{ name, wkt string }{
		{"compound", `COMPOUNDCRS["x",GEOGCRS["a",DATUM["d",ELLIPSOID["e",6378137,298.257223563]],CS[ellipsoidal,2]],VERTCRS["v",VDATUM["g"]]]`},
		{"bound", `BOUNDCRS[SOURCECRS[GEOGCRS["a"]],TARGETCRS[GEOGCRS["b"]]]`},
		{"omerc", `PROJCRS["o",BASEGEOGCRS["W",DATUM["d",ELLIPSOID["e",6378137,298.257223563]]],CONVERSION["c",METHOD["Hotine Oblique Mercator (variant B)"],PARAMETER["Latitude of projection centre",4,ANGLEUNIT["degree",0.0174532925199433]]],CS[Cartesian,2]]`},
		{"unknown-param", `PROJCRS["o",BASEGEOGCRS["W",DATUM["d",ELLIPSOID["e",6378137,298.257223563]]],CONVERSION["c",METHOD["Transverse Mercator"],PARAMETER["Fictional Parameter",1,ANGLEUNIT["degree",0.0174532925199433]]],CS[Cartesian,2]]`},
		{"garbage", `PROJCRS["o"`},
		{"unknown-root", `FOOCRS["o"]`},
	}
	for _, c := range cases {
		if _, err := Parse(c.wkt); err == nil {
			t.Errorf("%s: 应报错", c.name)
		}
	}
	// 前后带空白/换行的 WKT 也要认(文件里常这么贴)
	wrapped := "\n  " + `PROJCRS["x",BASEGEOGCRS["WGS 84",DATUM["WGS 1984",ELLIPSOID["WGS 84",6378137,298.257223563]]],CONVERSION["c",METHOD["Transverse Mercator"],PARAMETER["Latitude of natural origin",0,ANGLEUNIT["degree",0.0174532925199433]],PARAMETER["Longitude of natural origin",117,ANGLEUNIT["degree",0.0174532925199433]],PARAMETER["Scale factor at natural origin",0.9996,SCALEUNIT["unity",1]],PARAMETER["False easting",500000,LENGTHUNIT["metre",1]],PARAMETER["False northing",0,LENGTHUNIT["metre",1]]],CS[Cartesian,2],AXIS["E",east,ORDER[1],LENGTHUNIT["metre",1]],AXIS["N",north,ORDER[2],LENGTHUNIT["metre",1]]]` + "  \n"
	crs, err := Parse(wrapped)
	if err != nil {
		t.Fatalf("带空白的 WKT: %v", err)
	}
	pp, err := crs.NewProjection()
	if err != nil {
		t.Fatalf("带空白的 WKT NewProjection: %v", err)
	}
	x, y, _ := pp.Forward(117, 30)
	if math.Abs(x-500000) > 1e-6 || math.Abs(y-3318785.3525812062) > 1e-6 {
		t.Errorf("带空白 WKT 的 (117,30) = (%.6f, %.6f)", x, y)
	}

	// 非 WKT 的串仍走 proj 串路径
	if _, err := Parse("+proj=latlong +ellps=WGS84"); err != nil {
		t.Errorf("proj 串不该被误判成 WKT: %v", err)
	}
}

// TestWKTGeographic 地理 CRS:WKT → latlong(度),EPSG:4326 走 ID 快通道。
func TestWKTGeographic(t *testing.T) {
	crs, err := Parse(`GEOGCRS["WGS 84",DATUM["World Geodetic System 1984",ELLIPSOID["WGS 84",6378137,298.257223563]],CS[ellipsoidal,2],AXIS["latitude",north],AXIS["longitude",east],ANGLEUNIT["degree",0.0174532925199433]]`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if crs.Proj != "latlong" {
		t.Errorf("地理 CRS 应落到 latlong, got %q", crs.Proj)
	}
	if math.Abs(crs.Ellipsoid.A-6378137) > 1e-6 || crs.Ellipsoid.Sphere {
		t.Errorf("椭球不对: %+v", crs.Ellipsoid)
	}
	if got := crs.EPSG(); got != 4326 {
		t.Errorf("EPSG() = %d, want 4326(椭球特征识别)", got)
	}
}
