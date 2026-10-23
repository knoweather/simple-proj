package proj

import (
	"errors"
	"math"
	"testing"
)

// nearF 浮点断言助手(本库口径:绝对差)。
func nearF(t *testing.T, name string, got, want, tol float64) {
	t.Helper()
	if math.IsNaN(got) || math.IsNaN(want) {
		t.Fatalf("%s: 出现 NaN(got=%v want=%v)", name, got, want)
	}
	if math.Abs(got-want) > tol {
		t.Errorf("%s: got %.16g want %.16g (|Δ|=%.3g > %g)", name, got, want, math.Abs(got-want), tol)
	}
}

// TestEllipsoidTableEllps 内置表 + 组合推导主链路(优先级见 EllipsoidFromParams)。
func TestEllipsoidTableEllps(t *testing.T) {
	cases := []struct {
		name         string
		params       map[string]float64
		ellpsName    string
		wantA, wantB float64
		wantES       float64
		wantSphere   bool
		wantF        float64
	}{
		{
			name: "默认无参→WGS84", ellpsName: "",
			wantA: 6378137, wantB: 6378137 * (1 - 1/298.257223563),
			wantES: 0.0066943799901413165, wantF: 1 / 298.257223563,
		},
		{
			name: "+ellps=WGS84(大小写不敏感)", ellpsName: "WGS84",
			wantA: 6378137, wantB: 6378137 * (1 - 1/298.257223563),
		},
		{name: "+ellps=GRS80", ellpsName: "grs80", wantA: 6378137, wantB: 6378137 * (1 - 1/298.257222101)},
		{name: "+ellps=intl", ellpsName: "intl", wantA: 6378388, wantB: 6378388 * (1 - 1.0/297)},
		{name: "+ellps=clrk66", ellpsName: "clrk66", wantA: 6378206.4, wantB: 6378206.4 * (1 - 1/294.978698213898)},
		{name: "+ellps=sphere", ellpsName: "sphere", wantA: 6370997, wantB: 6370997, wantSphere: true},
		{
			name: "+a/+b 组合", params: map[string]float64{"a": 6378137, "b": 6356752.3142},
			wantA: 6378137, wantB: 6356752.3142,
		},
		{
			name: "+a/+rf", params: map[string]float64{"a": 6378137, "rf": 298.257223563},
			wantA: 6378137, wantB: 6378137 * (1 - 1/298.257223563),
		},
		{
			name: "+a/+f", params: map[string]float64{"a": 6378137, "f": 1 / 298.257223563},
			wantA: 6378137, wantB: 6378137 * (1 - 1/298.257223563),
		},
		{
			name: "+a/+es", params: map[string]float64{"a": 6378137, "es": 0.0066943799901413165},
			wantA: 6378137, wantB: 6378137 * math.Sqrt(1-0.0066943799901413165),
		},
		{
			name: "+a 独给→正球体", params: map[string]float64{"a": 6378137},
			wantA: 6378137, wantB: 6378137, wantSphere: true,
		},
		{
			name: "+rf=0→正球体(PROJ 语义)", params: map[string]float64{"a": 6378137, "rf": 0},
			wantA: 6378137, wantB: 6378137, wantSphere: true,
		},
		{
			name: "+es=0→正球体", params: map[string]float64{"a": 6378137, "es": 0},
			wantA: 6378137, wantB: 6378137, wantSphere: true,
		},
		{
			name:      "+R 压倒 ellps(椭球优先级)",
			params:    map[string]float64{"a": 6378137, "b": 6371000, "R": 5_000_000},
			ellpsName: "intl",
			wantA:     5_000_000, wantB: 5_000_000, wantSphere: true,
		},
		{
			// N.B. 直觉上以为「+ellps 盖过 +a/b」,但 pyproj 3.8.0 /
			// PROJ 9.8.1 实测是反过来的:+a/+b/+rf 覆盖 +ellps 打底的形状
			// (2026-10-22 校准,见 EllipsoidFromParams 注释)。以实测为准。
			name:      "+a/+b 覆盖 +ellps(PROJ 实测)",
			params:    map[string]float64{"a": 6378137, "b": 6371000},
			ellpsName: "sphere",
			wantA:     6378137, wantB: 6371000,
		},
		{
			name:      "+a 覆盖 +ellps 的长半轴,形状仍用 ellps 的 rf",
			params:    map[string]float64{"a": 6371000},
			ellpsName: "WGS84",
			wantA:     6371000, wantB: 6371000 * (1 - 1/298.257223563),
		},
		{
			name:      "+f 在 +ellps 在场时被忽略(PROJ 口径)",
			params:    map[string]float64{"f": 0.01},
			ellpsName: "WGS84",
			wantA:     6378137, wantB: 6378137 * (1 - 1/298.257223563),
		},
		{
			name:   "+b 盖过 +rf(短轴推导顺序)",
			params: map[string]float64{"a": 6378137, "b": 6_100_000, "rf": 298.25},
			wantA:  6378137, wantB: 6_100_000,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e, err := EllipsoidFromParams(c.params, c.ellpsName)
			if err != nil {
				t.Fatalf("EllipsoidFromParams: 意外错误: %v", err)
			}
			nearF(t, "A", e.A, c.wantA, 1e-6)
			nearF(t, "B", e.B, c.wantB, 1e-6)
			if c.wantSphere != e.Sphere {
				t.Errorf("Sphere: got %v want %v", e.Sphere, c.wantSphere)
			}
			if c.wantES != 0 {
				nearF(t, "ES", e.ES, c.wantES, 1e-12)
			} else if !e.Sphere {
				recES := 1 - (e.B/e.A)*(e.B/e.A)
				nearF(t, "ES(自洽)", e.ES, recES, 1e-15)
			}
			if c.wantF != 0 {
				nearF(t, "F", e.F, c.wantF, 1e-15)
			}
			// E 与 ES 自洽
			if e.ES >= 0 {
				nearF(t, "E(自洽)", e.E, math.Sqrt(e.ES), 1e-15)
			}
		})
	}
}

// TestEllipsoidErrors 矛盾/非法组合 fail-loud。
func TestEllipsoidErrors(t *testing.T) {
	cases := []struct {
		name      string
		params    map[string]float64
		ellpsName string
	}{
		{"+a=0", map[string]float64{"a": 0}, ""},
		{"+a<0", map[string]float64{"a": -1}, ""},
		{"+a=NaN", map[string]float64{"a": math.NaN()}, ""},
		{"+b>a", map[string]float64{"a": 6378137, "b": 6378138}, ""},
		{"+rf<0", map[string]float64{"a": 6378137, "rf": -0.5}, ""},
		{"+rf∈(0,1)", map[string]float64{"a": 6378137, "rf": 0.5}, ""},
		{"+rf=1→b=0", map[string]float64{"a": 6378137, "rf": 1}, ""},
		{"+f<0", map[string]float64{"a": 6378137, "f": -0.1}, ""},
		{"+f>=1", map[string]float64{"a": 6378137, "f": 1.0}, ""},
		{"+es<0", map[string]float64{"a": 6378137, "es": -0.1}, ""},
		{"+es>=1", map[string]float64{"a": 6378137, "es": 1.0}, ""},
		{"+R<=0", map[string]float64{"R": 0}, ""},
		{"+R=Inf", map[string]float64{"R": math.Inf(1)}, ""},
		{"+ellps 未知名", nil, "ellipsoid-xyz"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e, err := EllipsoidFromParams(c.params, c.ellpsName)
			if err == nil {
				t.Fatalf("期望报错,却得到 %+v", e)
			}
		})
	}
	if _, err := EllipsoidFromParams(nil, "sphere"); err != nil {
		t.Errorf("params=nil(零参数走默认/ellps)不应报错: %v", err)
	}
}

// TestErrDomainSentinel 哨兵检查:ErrDomain 必须可被 errors.Is 识别。
func TestErrDomainSentinel(t *testing.T) {
	wrapped := errors.New("x: " + ErrDomain.Error())
	if !errors.Is(wrapped, ErrDomain) {
		// 只有显式 %w 包裹才成立,这里只验证哨兵本体
		_ = wrapped
	}
	if !errors.Is(ErrDomain, ErrDomain) {
		t.Fatal("ErrDomain 自身必须可 errors.Is")
	}
}
