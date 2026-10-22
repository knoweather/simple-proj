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
	"strings"
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

// TestWKTPerParameterUnits WKT2 允许每个 PARAMETER 自带单位,且可以与 CS 不同。
// 只按 CS 单位换算会算错 —— 参照值由 pyproj 解析同一份 WKT 得到(2026-10-22):
// 两种写法都应给出 (500000, 3318785.3525812062)。
func TestWKTPerParameterUnits(t *testing.T) {
	const wantX, wantY = 500000.0, 3318785.3525812062
	cases := map[string]string{
		// false easting 用美国测量英尺(1640416.6666666667 ft = 500000 m),CS 是米
		"easting-in-us-ft": `PROJCRS["mixed-ft",BASEGEOGCRS["WGS 84",DATUM["WGS 1984",` +
			`ELLIPSOID["WGS 84",6378137,298.257223563,LENGTHUNIT["metre",1]]]],` +
			`CONVERSION["c",METHOD["Transverse Mercator"],` +
			`PARAMETER["Latitude of natural origin",0,ANGLEUNIT["degree",0.0174532925199433]],` +
			`PARAMETER["Longitude of natural origin",117,ANGLEUNIT["degree",0.0174532925199433]],` +
			`PARAMETER["Scale factor at natural origin",0.9996,SCALEUNIT["unity",1]],` +
			`PARAMETER["False easting",1640416.6666666667,LENGTHUNIT["US survey foot",0.304800609601219]],` +
			`PARAMETER["False northing",0,LENGTHUNIT["metre",1]]],` +
			`CS[Cartesian,2],AXIS["E",east,ORDER[1],LENGTHUNIT["metre",1]],` +
			`AXIS["N",north,ORDER[2],LENGTHUNIT["metre",1]]]`,
		// 中央经线用 gon(400 gon = 360 度;117 度 = 130 gon)
		"lon-in-gon": `PROJCRS["mixed-gon",BASEGEOGCRS["WGS 84",DATUM["WGS 1984",` +
			`ELLIPSOID["WGS 84",6378137,298.257223563,LENGTHUNIT["metre",1]]]],` +
			`CONVERSION["c",METHOD["Transverse Mercator"],` +
			`PARAMETER["Latitude of natural origin",0,ANGLEUNIT["degree",0.0174532925199433]],` +
			`PARAMETER["Longitude of natural origin",130,ANGLEUNIT["gon",0.015707963267948967]],` +
			`PARAMETER["Scale factor at natural origin",0.9996,SCALEUNIT["unity",1]],` +
			`PARAMETER["False easting",500000,LENGTHUNIT["metre",1]],` +
			`PARAMETER["False northing",0,LENGTHUNIT["metre",1]]],` +
			`CS[Cartesian,2],AXIS["E",east,ORDER[1],LENGTHUNIT["metre",1]],` +
			`AXIS["N",north,ORDER[2],LENGTHUNIT["metre",1]]]`,
	}
	for name, wkt := range cases {
		crs, err := Parse(wkt)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		p, err := crs.NewProjection()
		if err != nil {
			t.Fatalf("%s: NewProjection: %v", name, err)
		}
		x, y, err := p.Forward(117, 30)
		if err != nil {
			t.Fatalf("%s: Forward: %v", name, err)
		}
		if math.Abs(x-wantX) > 1e-6 || math.Abs(y-wantY) > 1e-6 {
			t.Errorf("%s: (117,30) → (%.6f, %.6f), want (%.6f, %.6f)", name, x, y, wantX, wantY)
		}
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

// TestWKTMalformed 词法/语法错误与各类"缺东西"的 WKT 都要给出可读报错,
// 不能 panic、也不能静默当成别的 CRS。
func TestWKTMalformed(t *testing.T) {
	base := `BASEGEOGCRS["WGS 84",DATUM["WGS 1984",ELLIPSOID["WGS 84",6378137,298.257223563]]]`
	cases := []struct{ name, wkt, wantSub string }{
		{"字符串未闭合", `PROJCRS["x`, "未闭合"},
		{"括号未闭合", `PROJCRS["x"`, "括号未闭合"},
		{"多余内容", `PROJCRS["x"] junk`, "多余内容"},
		{"空关键字", `PROJCRS[,]`, "缺关键字"},
		{"坏数字", `PROJCRS["x",1e]`, "数字"},
		{"缺椭球", `PROJCRS["x"]`, "找不到 ELLIPSOID"},
		{"缺方法", `PROJCRS["x",` + base + `]`, "找不到 METHOD"},
		{"方法没名字", `PROJCRS["x",` + base + `,CONVERSION["c",METHOD[]]]`, "METHOD 没有名字"},
		{"参数没数值", `PROJCRS["x",` + base + `,CONVERSION["c",METHOD["Transverse Mercator"],PARAMETER["False easting"]]]`, "没有数值"},
		{"参数单位缺系数", `PROJCRS["x",` + base + `,CONVERSION["c",METHOD["Transverse Mercator"],PARAMETER["False easting",500000,LENGTHUNIT["metre"]]]]`, "LENGTHUNIT 缺换算系数"},
		{"未知参数名", `PROJCRS["x",` + base + `,CONVERSION["c",METHOD["Transverse Mercator"],PARAMETER["Fictional",1]]]`, "不认识的投影参数"},
		{"未知方法", `PROJCRS["x",` + base + `,CONVERSION["c",METHOD["Krovak"]]]`, "不支持的投影方法"},
		{"地理 CRS 用 gon", `GEOGCRS["g",DATUM["d",ELLIPSOID["e",6378137,298.257223563]],ANGLEUNIT["gon",0.015707963267948967]]`, "latlong 输出固定为度"},
	}
	for _, c := range cases {
		_, err := Parse(c.wkt)
		if err == nil {
			t.Errorf("%s: 应报错", c.name)
			continue
		}
		if !strings.Contains(err.Error(), c.wantSub) {
			t.Errorf("%s: 报错应含 %q, got %v", c.name, c.wantSub, err)
		}
	}
	// 根节点不在白名单时 Parse 不会当 WKT 分派(按 proj 串处理),但直接调
	// ParseWKT 必须给出明确报错。
	if _, err := ParseWKT(`FOOCRS["x"]`); err == nil || !strings.Contains(err.Error(), "不认识的根节点") {
		t.Errorf("ParseWKT 未知根节点应报错, got %v", err)
	}
}

// TestWKTStructuralVariants 结构上的边角写法都要落到正确的 CRS:
// 括号式 WKT1、内嵌引号、非 EPSG 的 ID、球体椭球、南半球极射赤面。
func TestWKTStructuralVariants(t *testing.T) {
	// 1) WKT1 括号式(PROJCS(...)),且投影方法用下划线拼写
	paren := `PROJCS("t",GEOGCS("g",DATUM("d",SPHEROID("s",6378137,298.257223563)),` +
		`PRIMEM("Greenwich",0),UNIT("degree",0.0174532925199433)),` +
		`PROJECTION("Transverse_Mercator"),PARAMETER("latitude_of_origin",0),` +
		`PARAMETER("central_meridian",117),PARAMETER("scale_factor",0.9996),` +
		`PARAMETER("false_easting",500000),PARAMETER("false_northing",0),UNIT("metre",1))`
	crs, err := Parse(paren)
	if err != nil {
		t.Fatalf("括号式 WKT1: %v", err)
	}
	p, err := crs.NewProjection()
	if err != nil {
		t.Fatalf("括号式 WKT1 NewProjection: %v", err)
	}
	if x, y, _ := p.Forward(117, 30); math.Abs(x-500000) > 1e-6 || math.Abs(y-3318785.3525812062) > 1e-6 {
		t.Errorf("括号式 WKT1 的 (117,30) = (%.6f, %.6f)", x, y)
	}

	// 2) 字符串里内嵌双引号(WKT2 用 "" 转义)
	if _, err := Parse(`GEOGCRS["a""b",DATUM["d",ELLIPSOID["e",6378137,298.257223563]]]`); err != nil {
		t.Errorf("内嵌引号: %v", err)
	}

	// 3) ID 不是 EPSG(ESRI)→ 不走快通道,仍按内容翻译
	esri := `PROJCRS["x",` + `BASEGEOGCRS["WGS 84",DATUM["WGS 1984",ELLIPSOID["WGS 84",6378137,298.257223563]]],` +
		`CONVERSION["c",METHOD["Transverse Mercator"],PARAMETER["Longitude of natural origin",117,ANGLEUNIT["degree",0.0174532925199433]],` +
		`PARAMETER["Scale factor at natural origin",0.9996,SCALEUNIT["unity",1]],PARAMETER["False easting",500000,LENGTHUNIT["metre",1]]],` +
		`CS[Cartesian,2],ID["ESRI",3857]]`
	if crs, err := Parse(esri); err != nil {
		t.Errorf("非 EPSG 的 ID: %v", err)
	} else if crs.Proj != "tmerc" {
		t.Errorf("非 EPSG 的 ID 应按内容翻译成 tmerc, got %q", crs.Proj)
	}

	// 4) 球体椭球(rf=0)→ 正球体
	sphere := `PROJCRS["s",BASEGEOGCRS["S",DATUM["D",ELLIPSOID["Sphere",6371000,0]]],` +
		`CONVERSION["c",METHOD["Mercator (variant A)"],PARAMETER["Longitude of natural origin",0,ANGLEUNIT["degree",0.0174532925199433]],` +
		`PARAMETER["Scale factor at natural origin",1,SCALEUNIT["unity",1]]],` +
		`CS[Cartesian,2],AXIS["E",east,LENGTHUNIT["metre",1]],AXIS["N",north,LENGTHUNIT["metre",1]]]`
	crs2, err := Parse(sphere)
	if err != nil {
		t.Fatalf("球体 WKT: %v", err)
	}
	if !crs2.Ellipsoid.Sphere || math.Abs(crs2.Ellipsoid.A-6371000) > 1e-6 {
		t.Errorf("球体 WKT 的椭球不对: %+v", crs2.Ellipsoid)
	}

	// 5) 南半球极射赤面(变体 B):只给标准纬线 -70 → lat_0 = -90
	south := `PROJCRS["sp",BASEGEOGCRS["WGS 84",DATUM["WGS 1984",ELLIPSOID["WGS 84",6378137,298.257223563]]],` +
		`CONVERSION["c",METHOD["Polar Stereographic (variant B)"],` +
		`PARAMETER["Latitude of standard parallel",-70,ANGLEUNIT["degree",0.0174532925199433]],` +
		`PARAMETER["Longitude of origin",0,ANGLEUNIT["degree",0.0174532925199433]],` +
		`PARAMETER["False easting",0,LENGTHUNIT["metre",1]],PARAMETER["False northing",0,LENGTHUNIT["metre",1]]],` +
		`CS[Cartesian,2],AXIS["E",east,LENGTHUNIT["metre",1]],AXIS["N",north,LENGTHUNIT["metre",1]]]`
	crs3, err := Parse(south)
	if err != nil {
		t.Fatalf("南半球极射: %v", err)
	}
	p3, _ := crs3.NewProjection()
	ref, _ := Parse("+proj=stere +lat_0=-90 +lat_ts=-70 +lon_0=0 +ellps=WGS84")
	pr, _ := ref.NewProjection()
	x3, y3, _ := p3.Forward(20, -60)
	xr, yr, _ := pr.Forward(20, -60)
	if math.Abs(x3-xr) > 1e-6 || math.Abs(y3-yr) > 1e-6 {
		t.Errorf("南半球极射 = (%.6f,%.6f), 等价的 proj 串给 (%.6f,%.6f)", x3, y3, xr, yr)
	}
}

// TestWKTPseudoMercator 伪墨卡托(3857)在**没有 ID** 时必须按方法名强制球体:
// 底图 GEOGCS 是椭球,但投影按 a 当 R 用。若照椭球算,y 会差 ~1.5e4 m。
func TestWKTPseudoMercator(t *testing.T) {
	wkt := `PROJCRS["WGS 84 / Pseudo-Mercator",BASEGEOGCRS["WGS 84",DATUM["WGS 1984",` +
		`ELLIPSOID["WGS 84",6378137,298.257223563]]],CONVERSION["Pseudo-Mercator",` +
		`METHOD["Popular Visualisation Pseudo Mercator"],` +
		`PARAMETER["Latitude of natural origin",0,ANGLEUNIT["degree",0.0174532925199433]],` +
		`PARAMETER["Longitude of natural origin",0,ANGLEUNIT["degree",0.0174532925199433]],` +
		`PARAMETER["False easting",0,LENGTHUNIT["metre",1]],` +
		`PARAMETER["False northing",0,LENGTHUNIT["metre",1]]],` +
		`CS[Cartesian,2],AXIS["E",east,LENGTHUNIT["metre",1]],AXIS["N",north,LENGTHUNIT["metre",1]]]`
	crs, err := Parse(wkt)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !crs.Ellipsoid.Sphere {
		t.Errorf("伪墨卡托应强制球体, got %+v", crs.Ellipsoid)
	}
	p, err := crs.NewProjection()
	if err != nil {
		t.Fatalf("NewProjection: %v", err)
	}
	// 与 EPSG:3857 的官方值一致(椭球墨卡托在 lat=20 会给出 2258423.6)
	x, y, _ := p.Forward(10, 20)
	const wantX, wantY = 1113194.9079327357, 2273030.9269876895
	if math.Abs(x-wantX) > 1e-6 || math.Abs(y-wantY) > 1e-6 {
		t.Errorf("(10,20) → (%.6f, %.6f), want (%.6f, %.6f)", x, y, wantX, wantY)
	}
}

// TestWKTEPSGIDVariants ID 缺失/非 EPSG/码不认识的三种情况都要退回按内容翻译,
// 不能因为 ID 写坏了就把 CRS 丢掉或认错。
func TestWKTEPSGIDVariants(t *testing.T) {
	head := `PROJCRS["x",BASEGEOGCRS["WGS 84",DATUM["WGS 1984",ELLIPSOID["WGS 84",6378137,298.257223563]]],` +
		`CONVERSION["c",METHOD["Transverse Mercator"],` +
		`PARAMETER["Longitude of natural origin",117,ANGLEUNIT["degree",0.0174532925199433]],` +
		`PARAMETER["Scale factor at natural origin",0.9996,SCALEUNIT["unity",1]],` +
		`PARAMETER["False easting",500000,LENGTHUNIT["metre",1]]],CS[Cartesian,2]`
	for _, id := range []string{`ID[]`, `ID["EPSG","abc"]`, `ID["EPSG",1234]`, `ID["ESRI",3857]`} {
		crs, err := Parse(head + `,` + id + `]`)
		if err != nil {
			t.Errorf("%s: %v", id, err)
			continue
		}
		if crs.Proj != "tmerc" {
			t.Errorf("%s: 应翻译成 tmerc, got %q", id, crs.Proj)
		}
	}
	// 椭球节点数字不足 → 明确报错(不能拿半个椭球去算)
	if _, err := Parse(`PROJCRS["x",BASEGEOGCRS["g",DATUM["d",ELLIPSOID["e"]]]]`); err == nil {
		t.Error("椭球缺数字应报错")
	}
}
