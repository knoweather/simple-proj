package proj

import (
	"fmt"
	"math"
	"strings"
)

// Ellipsoid(A 拥有,结构冻结 §2.2)。
type Ellipsoid struct {
	A, B   float64 // 长短半轴(米);正球体 A == B
	F      float64 // 扁率
	ES     float64 // e²(第一偏心率平方)
	E      float64 // e
	Sphere bool
}

// ---- 内置椭球表(PROJ proj_ellps.cpp 的公知测量常量;只取数值,不抄代码) ----

// ellpsRef 椭球表行:a = 长半轴(米),rf = 扁率倒数(0 = 正球)。
type ellpsRef struct {
	a    float64
	rf   float64
	desc string
}

// ellpsRefTable PROJ 惯用名(查找时统一小写)。`clrk66`/`clrk80` 保持 PROJ
// 兼容拼写,`clarke1866`/`clarke1880` 为等值可读别名;`nad83` 与 GRS80 同轴。
var ellpsRefTable = map[string]ellpsRef{
	"wgs84":      {6378137.0, 298.257223563, "WGS 84"},
	"grs80":      {6378137.0, 298.257222101, "GRS 1980 (IUGG, 1980)"},
	"nad83":      {6378137.0, 298.257222101, "North American Datum 1983 (GRS80 轴)"},
	"grs67":      {6378160.0, 298.25, "GRS 1967 (IUGG 1967)"},
	"intl":       {6378388.0, 297.0, "International 1924 (Hayford)"},
	"int1924":    {6378388.0, 297.0, "International 1924 (别名 intl)"},
	"clrk66":     {6378206.4, 294.978698213898, "Clarke 1866"},
	"clarke1866": {6378206.4, 294.978698213898, "Clarke 1866 (别名 clrk66)"},
	"clrk80":     {6378249.145, 293.466307656, "Clarke 1880 (EN)"},
	"clarke1880": {6378249.145, 293.466307656, "Clarke 1880 (EN) (别名 clrk80)"},
	"airy":       {6377563.396, 299.3249646, "Airy 1830 (OSGB)"},
	"bessel":     {6377397.155, 299.1528128, "Bessel 1841"},
	"sphere":     {6370997.0, 0.0, "Normal Sphere (r=6370997)"},
}

// wgs84Ref 无任何椭球信息时的默认(DESIGN §2.2 优先级末位)。
var wgs84Ref = ellpsRef{6378137.0, 298.257223563, "默认 WGS 84"}

// EllipsoidFromParams 由参数组合推导椭球(结构冻结 §2.2 签名)。
// 组合与优先级(DESIGN §2.2,同 PROJ 约定):
//
//	+R > +ellps=名 > +a/+b 组合 > 默认 WGS84
//
// 参数键(已在 Parse 时归入 CRS.Params): +a/+b/+rf/+f/+es/+R;
// 短轴推导优先级 +b > +rf > +f > +es(PROJ 语义);仅给 +a 或 +rf=0 视作正球体。
// 零参数零名 → 默认 WGS84。发现矛盾组合(短轴>长轴、e²∉[0,1)、负半径等)
// 一律 fail-loud 返回 error。
func EllipsoidFromParams(params map[string]float64, ellpsName string) (Ellipsoid, error) {
	get := func(key string) (float64, bool) {
		v, ok := params[key]
		return v, ok
	}

	// 1) +R:正球体,压倒一切
	if r, ok := get("R"); ok {
		if !finitePositive(r) {
			return Ellipsoid{}, fmt.Errorf("proj: +R=%v 不是有效半径", r)
		}
		return newEllipsoid(r, r)
	}

	// 2) +ellps=名
	if name := strings.ToLower(strings.TrimSpace(ellpsName)); name != "" {
		ref, ok := ellpsRefTable[name]
		if !ok {
			return Ellipsoid{}, fmt.Errorf("proj: 未知椭球 +ellps=%q", ellpsName)
		}
		// DESIGN 优先级:+ellps 盖过 +a/b 组合(两者都给时按 ellps)
		return newEllipsoidFromRF(ref.a, ref.rf)
	}

	// 3) +a/+b 组合
	if aVal, ok := get("a"); ok {
		if !finitePositive(aVal) {
			return Ellipsoid{}, fmt.Errorf("proj: +a=%v 不是有效的长半轴", aVal)
		}
		a := aVal
		// 短轴推导顺序 +b > +rf > +f > +es;全无 → 正球体
		if bVal, ok := get("b"); ok {
			return newEllipsoid(a, bVal)
		}
		if rfVal, ok := get("rf"); ok {
			return rfFromFlat(a, rfVal)
		}
		if fVal, ok := get("f"); ok {
			return fFromFlat(a, fVal)
		}
		if esVal, ok := get("es"); ok {
			return esFromES(a, esVal)
		}
		return newEllipsoid(a, a) // 仅 +a:正球体(PROJ 语义)
	}

	// 4) 默认 WGS84
	return newEllipsoidFromRF(wgs84Ref.a, wgs84Ref.rf)
}

// newEllipsoidFromRF 由 (a, rf) 构造。
func newEllipsoidFromRF(a, rf float64) (Ellipsoid, error) {
	return rfFromFlat(a, rf)
}

// rfFromFlat 处理 +rf 语义:0 = 正球体;a×(1-1/rf) 反推短轴。
func rfFromFlat(a, rf float64) (Ellipsoid, error) {
	if rf == 0 {
		return newEllipsoid(a, a) // PROJ:+rf=0 即正球
	}
	if math.IsNaN(rf) || rf < 0 || rf < 1 && rf != 0 {
		return Ellipsoid{}, fmt.Errorf("proj: +rf=%v 不合理(需 0 或 >=1)", rf)
	}
	inv := 1 / rf
	if inv < 0 || inv >= 1 {
		return Ellipsoid{}, fmt.Errorf("proj: +rf=%v 导出扁率 %v 出界", rf, inv)
	}
	return newEllipsoid(a, a*(1-inv))
}

// fFromFlat 处理 +f 语义:0 = 正球体。
func fFromFlat(a, f float64) (Ellipsoid, error) {
	if f == 0 {
		return newEllipsoid(a, a)
	}
	if math.IsNaN(f) || f < 0 || f >= 1 {
		return Ellipsoid{}, fmt.Errorf("proj: +f=%v 出界 [0,1)", f)
	}
	return newEllipsoid(a, a*(1-f))
}

// esFromES 处理 +es 语义(第一偏心率平方):0 = 正球体。
func esFromES(a, es float64) (Ellipsoid, error) {
	if es == 0 {
		return newEllipsoid(a, a)
	}
	if math.IsNaN(es) || es < 0 || es >= 1 {
		return Ellipsoid{}, fmt.Errorf("proj: +es=%v 出界 [0,1)", es)
	}
	// b = a*√(1-es);同权 f = 1-√(1-es)
	return newEllipsoid(a, a*math.Sqrt(1-es))
}

// newEllipsoid 由 (a, b) 完格推导 F/ES/E/Sphere 并校验(fail-loud)。
func newEllipsoid(a, b float64) (Ellipsoid, error) {
	if math.IsNaN(a) || math.IsInf(a, 0) || a <= 0 {
		return Ellipsoid{}, fmt.Errorf("proj: 长半轴 a=%v 非法", a)
	}
	if math.IsNaN(b) || math.IsInf(b, 0) || b <= 0 {
		return Ellipsoid{}, fmt.Errorf("proj: 短半轴 b=%v 非法", b)
	}
	if b > a {
		return Ellipsoid{}, fmt.Errorf("proj: 短半轴 b=%v 大于长半轴 a=%v", b, a)
	}
	es := 1 - (b/a)*(b/a)
	return Ellipsoid{
		A:      a,
		B:      b,
		F:      (a - b) / a,
		ES:     es,
		E:      math.Sqrt(es),
		Sphere: b == a,
	}, nil
}

// finitePositive 数值有效且 > 0。
func finitePositive(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v > 0
}
