package xcheck

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	proj "github.com/XC-Zero/simple-proj"
)

// xcheck_test.go —— 金标准消费端(PROGRESS-A 决策 D5):
// 消费 xcheck/golden_*.json(pyproj 3.8.0 / PROJ 9.8.1 生成,owner=B),对
// A 轨三投影(latlong / eqc / geos_x / geos_y)做前向/逆向/特例断言。
//
//   - 数值门槛(COORDINATION §4):forward ≤ 1e-6 m、inverse ≤ 1e-9 度;
//   - 经度断言用圆周距离(180 与 −180 同物理点;B 生成器对 latlong 在
//     lon=180 记录原始 180 而本实现归一 −180,D5 口径一致);
//   - golden 文件缺失 → t.Skip(不阻塞 A 轨 gate,B 生成即收紧,D5);
//   - 投影 impl 未注册(如 Day3 前的 geos)→ t.Skip,同 D5 哲学;
//   - expectNaN 定义域外特例:Forward 必须 (NaN,NaN,ErrDomain);
//   - JSON 里 pyproj 直写的 NaN 字面量(geos 远界等):用 nanF 容错解码。
//
// 本文件不复制生成器,不识别 B 轨投影(laea/lcc/merc/tmerc/stere)。

const (
	goldenDir = "."
	fwdTol    = 1e-6 // m
	invTol    = 1e-9 // 度
)

// nanF 容错 float:NA literal "NaN" → math.NaN()
type nanF float64

func (f *nanF) UnmarshalJSON(b []byte) error {
	s := string(b)
	if s == `"__NaN__"` || s == "NaN" || s == `"NaN"` {
		*f = nanF(math.NaN())
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fmt.Errorf("xcheck: 非法数值 %s: %w", s, err)
	}
	*f = nanF(v)
	return nil
}

// boolLike 宽容布尔:D5 —— B 生成器 quirk 下 expectNaN 既可为 true 也可为 0.0。
type boolLike bool

func (b *boolLike) UnmarshalJSON(raw []byte) error {
	switch s := string(raw); s {
	case "true":
		*b = true
	case "false", "0.0", "0":
		*b = false
	default:
		return fmt.Errorf("xcheck: expectNaN 期望 true/false/0.0, got %s", s)
	}
	return nil
}

type entry struct {
	Lon nanF `json:"lon"`
	Lat nanF `json:"lat"`
	X   nanF `json:"x"`
	Y   nanF `json:"y"`
}

type special struct {
	Desc      string `json:"desc"`
	Lon       *nanF  `json:"lon"` // {lon,lat} 形态 → 正向域外探针
	Lat       *nanF  `json:"lat"`
	X         *nanF  `json:"x"` // {x,y} 形态 → 逆向域外探针
	Y         *nanF  `json:"y"`
	ExpectNaN boolLike `json:"expectNaN"`
}

type golden struct {
	CRS      string    `json:"crs"`
	Forward  []entry   `json:"forward"`
	Inverse  []entry   `json:"inverse"`
	Specials []special `json:"specials"`
}

// loadGolden 读并容错解码一个 golden(NA 字面量 → math.NaN)。
func loadGolden(t *testing.T, file string) (*golden, error) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(goldenDir, file))
	if err != nil {
		return nil, err // D5:缺失 → 调用方 Skip
	}
	if i := strings.Index(string(raw), ": NaN"); i >= 0 {
		// 只替换值位置的裸 NaN(键名 expectNaN 含 "NaN" 子串,不能宽替换)
		raw = []byte(strings.ReplaceAll(string(raw), ": NaN", `: "__NaN__"`))
	}
	var g golden
	if err := json.Unmarshal(raw, &g); err != nil {
		return nil, err
	}
	return &g, nil
}

// circDist 经度圆周距离(度):min(|Δ|, 360−|Δ|)。
func circDist(a, b float64) float64 {
	d := math.Mod(a-b, 360)
	if d < 0 {
		d += 360
	}
	if d > 180 {
		d -= 360
	}
	return math.Abs(d)
}

// newXCheckProj 解析 golden 的 CRS 并取实例;未注册 → 记 skip。
func newXCheckProj(t *testing.T, crs string) proj.Projection {
	t.Helper()
	c, err := proj.Parse(crs)
	if err != nil {
		t.Fatalf("Parse(%q): %v", crs, err)
	}
	pr, err := c.NewProjection()
	if err != nil {
		t.Skipf("投影 %q 未注册(impl 移交前 xcheck 宽容跳过,D5): %v", c.Proj, err)
	}
	return pr
}

// xcheckForward 前向三段断言(golden forward 数组消费)。
// degrees 标记 x/逆回 lon 是"度"(latlong 系),比对时用圆周距离;米制投影用平距。
func xcheckForward(t *testing.T, name string, p proj.Projection, es []entry, degrees bool) {
	t.Helper()
	var maxE float64
	for i, e := range es {
		x, y, err := p.Forward(float64(e.Lon), float64(e.Lat))
		if math.IsNaN(float64(e.X)) || math.IsNaN(float64(e.Y)) {
			// 参照 NaN → 定义域外:必须 (NaN,NaN,ErrDomain)
			if !math.IsNaN(x) || !math.IsNaN(y) || !errors.Is(err, proj.ErrDomain) {
				t.Errorf("%s forward[%d](%v,%v)=NaN: got (%v,%v,%v)", name, i, e.Lon, e.Lat, x, y, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s forward[%d](%v,%v): %v", name, i, e.Lon, e.Lat, err)
			continue
		}
		d := math.Abs(x-float64(e.X)) + math.Abs(y-float64(e.Y))
		if degrees {
			d = circDist(x, float64(e.X)) + math.Abs(y-float64(e.Y))
		}
		if d > maxE {
			maxE = d
		}
		if d > fwdTol {
			t.Errorf("%s forward[%d](%v,%v) = (%v,%v) want (%v,%v) 偏差 %g", name, i, e.Lon, e.Lat, x, y, e.X, e.Y, d)
		}
	}
	t.Logf("%s forward: %d 点,最大偏差 %.3g m(门槛 %.0e)", name, len(es), maxE, fwdTol)
}

func xcheckInverse(t *testing.T, name string, p proj.Projection, es []entry, degrees bool) {
	t.Helper()
	var maxE float64
	for i, e := range es {
		lon, lat, err := p.Inverse(float64(e.X), float64(e.Y))
		if math.IsNaN(float64(e.Lon)) || math.IsNaN(float64(e.Lat)) {
			if !math.IsNaN(lon) || !math.IsNaN(lat) || !errors.Is(err, proj.ErrDomain) {
				t.Errorf("%s inverse[%d](%v,%v)=NaN: got (%v,%v,%v)", name, i, e.X, e.Y, lon, lat, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s inverse[%d](%v,%v): %v", name, i, e.X, e.Y, err)
			continue
		}
		// 经度一律圆周比对(纬度平距);米制侧 x/y 不直接比对(golden 的 x/y 输入即来)
		d := circDist(lon, float64(e.Lon)) + math.Abs(lat-float64(e.Lat))
		if d > maxE {
			maxE = d
		}
		if d > invTol {
			t.Errorf("%s inverse[%d](%v,%v) = (%v,%v) want (%v,%v) 偏差 %g", name, i, e.X, e.Y, lon, lat, e.Lon, e.Lat, d)
		}
	}
	t.Logf("%s inverse: %d 点,最大偏差 %.3g°(门槛 %.0e)", name, len(es), maxE, invTol)
}

func xcheckSpecials(t *testing.T, name string, p proj.Projection, cs []special) {
	t.Helper()
	for i, s := range cs {
		if !bool(s.ExpectNaN) {
			continue
		}
		switch {
		case s.X != nil: // {x,y} 逆向域外
			lon, lat, err := p.Inverse(float64(*s.X), float64(*s.Y))
			if !math.IsNaN(lon) || !math.IsNaN(lat) || !errors.Is(err, proj.ErrDomain) {
				t.Errorf("%s special[%d](%s): Inverse(%v,%v) 期望 (NaN,NaN,ErrDomain), got (%v,%v,%v)",
					name, i, s.Desc, *s.X, *s.Y, lon, lat, err)
			}
		case s.Lon != nil: // {lon,lat} 正向域外
			x, y, err := p.Forward(float64(*s.Lon), float64(*s.Lat))
			if !math.IsNaN(x) || !math.IsNaN(y) || !errors.Is(err, proj.ErrDomain) {
				t.Errorf("%s special[%d](%s): Forward(%v,%v) 期望 (NaN,NaN,ErrDomain), got (%v,%v,%v)",
					name, i, s.Desc, *s.Lon, *s.Lat, x, y, err)
			}
		default:
			t.Errorf("%s special[%d](%s): 既无 x/y 也无 lon/lat", name, i, s.Desc)
		}
	}
}

// runGolden 一个 golden 的完整三段断言(前向/逆向/特例)。
func runGolden(t *testing.T, name, file string) {
	t.Helper()
	g, err := loadGolden(t, file)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skipf("golden 缺失,宽容跳过(D5): %v", err)
		}
		t.Fatalf("loadGolden(%s): %v", file, err)
	}
	if g.CRS == "" {
		t.Fatalf("%s: golden 无 crs 字段", name)
	}
	p := newXCheckProj(t, g.CRS)
	degrees := name == "latlong" // 度制投影:比对口径用圆周距离(D5)
	xcheckForward(t, name, p, g.Forward, degrees)
	xcheckInverse(t, name, p, g.Inverse, degrees)
	xcheckSpecials(t, name, p, g.Specials)
}

// TestXCheck 消费 A 轨三投影的 golden(latlong/eqc;9 份中只消费 A 的，
// B 轨投影由 B 自己的 PROGRESS-B 流程覆盖,消费端不越权)。
func TestXCheck(t *testing.T) {
	runGolden(t, "latlong", "golden_latlong.json")
	runGolden(t, "eqc", "golden_eqc.json")
}

// TestXCheckGeos geos golden(sweep x/y)。impl 落地(Day3)前宽容跳过,
// 注册后自动收紧为真实断言(D5)。
func TestXCheckGeos(t *testing.T) {
	runGolden(t, "geos_x", "golden_geos_x.json")
	runGolden(t, "geos_y", "golden_geos_y.json")
}
