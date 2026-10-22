package proj

// transform.go — CRS→CRS 转换(对标 pyproj 的 Transformer.from_crs)。
//
// 这是本库对 PROJ「操作链」的最小可用替代:不做 +proj=pipeline 的文本语法,
// 而是直接给一个两段式 API —— 源 CRS 反算到经纬度 → (可选)基准变换 →
// 目标 CRS 正算。PROJ 内部把这件事展开成 pipeline;对使用者来说两者等价,
// 而 Go 侧少一套字符串 DSL 要维护。
//
// 基准变换只支持 +towgs84 的 3 参/7 参(EPSG 9603/9606,position vector 约定),
// 以 WGS84 为中转:源 datum → WGS84 → 目标 datum。网格文件(NTv2/HARN)与
// 动态基准(ITRF 历元)不在范围内,Parse 阶段就会拒收 +nadgrids。
//
// 语义约定:
//   - Transform 的入参/出参都是**各自 CRS 的单位**:地理 CRS 是 (经度, 纬度) 度,
//     投影 CRS 是 (x, y) 按 +units/+to_meter 换算后的值;轴序固定 lon/lat、
//     x/y(不做 authority 轴序重排,同本库其他 API);
//   - 两侧都没有 +towgs84 时不做基准变换(相当于 PROJ 的 ballpark):
//     经纬度原样搬到目标椭球上,只换投影;
//   - 一侧有 +towgs84、另一侧没有时,没有的那侧必须是 WGS84 系椭球
//     (WGS84/GRS80 轴),否则报错 —— 否则就成了「把别的基准当 WGS84」的静默错误。

import (
	"errors"
	"fmt"
	"math"
)

// helmert 7 参基准变换(位置矢量约定 EPSG 9606;3 参时旋转与尺度为 0)。
type helmert struct {
	dx, dy, dz float64 // 平移,米
	rx, ry, rz float64 // 旋转,弧度(输入是角秒)
	s          float64 // 尺度因子 = 1 + ppm·1e-6
}

// newHelmert 由 +towgs84 的 3/7 个数构造。
func newHelmert(p []float64) *helmert {
	h := &helmert{dx: p[0], dy: p[1], dz: p[2], s: 1}
	if len(p) == 7 {
		const as2rad = math.Pi / (180 * 3600)
		h.rx = p[3] * as2rad
		h.ry = p[4] * as2rad
		h.rz = p[5] * as2rad
		h.s = 1 + p[6]*1e-6
	}
	return h
}

// apply 正向:本地 datum 的地心坐标 → WGS84 地心坐标。
// X' = T + s·R·X,R 为位置矢量约定的旋转矩阵。
func (h *helmert) apply(X, Y, Z float64) (float64, float64, float64) {
	return h.dx + h.s*(X-h.rz*Y+h.ry*Z),
		h.dy + h.s*(h.rz*X+Y-h.rx*Z),
		h.dz + h.s*(-h.ry*X+h.rx*Y+Z)
}

// inverse 反向:WGS84 地心坐标 → 本地 datum 地心坐标。
// 解 M·x = X'−T(M = s·R)而不是简单取负参数 —— 小角度下两者差 O(r²),
// 在 6000 km 量级能差到 0.1 mm,足以顶破 1e-9 度的门槛。
func (h *helmert) inverse(X, Y, Z float64) (float64, float64, float64) {
	m := [3][3]float64{
		{h.s, -h.s * h.rz, h.s * h.ry},
		{h.s * h.rz, h.s, -h.s * h.rx},
		{-h.s * h.ry, h.s * h.rx, h.s},
	}
	inv, ok := mat3Inverse(m)
	if !ok { // 行列式为 0 只可能出现在退化参数上,退回转置近似
		return (X - h.dx) / h.s, (Y - h.dy) / h.s, (Z - h.dz) / h.s
	}
	bx, by, bz := X-h.dx, Y-h.dy, Z-h.dz
	return inv[0][0]*bx + inv[0][1]*by + inv[0][2]*bz,
		inv[1][0]*bx + inv[1][1]*by + inv[1][2]*bz,
		inv[2][0]*bx + inv[2][1]*by + inv[2][2]*bz
}

// mat3Inverse 3×3 求逆(伴随矩阵/行列式)。
func mat3Inverse(m [3][3]float64) ([3][3]float64, bool) {
	det := m[0][0]*(m[1][1]*m[2][2]-m[1][2]*m[2][1]) -
		m[0][1]*(m[1][0]*m[2][2]-m[1][2]*m[2][0]) +
		m[0][2]*(m[1][0]*m[2][1]-m[1][1]*m[2][0])
	if det == 0 || math.IsNaN(det) {
		return [3][3]float64{}, false
	}
	var inv [3][3]float64
	inv[0][0] = (m[1][1]*m[2][2] - m[1][2]*m[2][1]) / det
	inv[0][1] = (m[0][2]*m[2][1] - m[0][1]*m[2][2]) / det
	inv[0][2] = (m[0][1]*m[1][2] - m[0][2]*m[1][1]) / det
	inv[1][0] = (m[1][2]*m[2][0] - m[1][0]*m[2][2]) / det
	inv[1][1] = (m[0][0]*m[2][2] - m[0][2]*m[2][0]) / det
	inv[1][2] = (m[0][2]*m[1][0] - m[0][0]*m[1][2]) / det
	inv[2][0] = (m[1][0]*m[2][1] - m[1][1]*m[2][0]) / det
	inv[2][1] = (m[0][1]*m[2][0] - m[0][0]*m[2][1]) / det
	inv[2][2] = (m[0][0]*m[1][1] - m[0][1]*m[1][0]) / det
	return inv, true
}

// geodeticToGeocentric 大地坐标(度,椭球高按 0)→ 地心直角坐标(米)。
func geodeticToGeocentric(lonDeg, latDeg float64, el Ellipsoid) (X, Y, Z float64) {
	phi := latDeg * deg2rad
	lam := lonDeg * deg2rad
	sp, cp := math.Sincos(phi)
	sl, cl := math.Sincos(lam)
	n := el.A / math.Sqrt(1-el.ES*sp*sp)
	return n * cp * cl, n * cp * sl, n * (1 - el.ES) * sp
}

// geocentricToGeodetic 地心直角坐标 → 大地坐标(Bowring 闭式解,
// 地球量级误差 < 1e-11 rad,远小于 1e-9 度的门槛)。
func geocentricToGeodetic(X, Y, Z float64, el Ellipsoid) (lonDeg, latDeg float64) {
	p := math.Hypot(X, Y)
	if p == 0 {
		if Z >= 0 {
			return 0, 90
		}
		return 0, -90
	}
	lon := math.Atan2(Y, X)
	b := el.B
	ep2 := (el.A*el.A - b*b) / (b * b)
	th := math.Atan2(Z*el.A, p*b)
	st, ct := math.Sincos(th)
	lat := math.Atan2(Z+ep2*b*st*st*st, p-el.ES*el.A*ct*ct*ct)
	return lon * rad2deg, lat * rad2deg
}

// Transformer 两个 CRS 之间的坐标转换器(不可变、线程安全)。
type Transformer struct {
	from, to       *CRS
	fromP, toP     Projection
	fromEl, toEl   Ellipsoid
	toWGS, fromWGS *helmert // 源→WGS84 / WGS84→目标
}

// NewTransformer 构造 from → to 的转换器。
func NewTransformer(from, to *CRS) (*Transformer, error) {
	if from == nil || to == nil {
		return nil, fmt.Errorf("proj: NewTransformer: 两侧 CRS 都不能为 nil")
	}
	fromP, err := from.newProjection(true)
	if err != nil {
		return nil, fmt.Errorf("proj: NewTransformer 源 CRS: %w", err)
	}
	toP, err := to.newProjection(true)
	if err != nil {
		return nil, fmt.Errorf("proj: NewTransformer 目标 CRS: %w", err)
	}
	t := &Transformer{from: from, to: to, fromP: fromP, toP: toP, fromEl: from.Ellipsoid, toEl: to.Ellipsoid}

	fromHas, toHas := hasTowgs84(from), hasTowgs84(to)
	// 一侧有基准参数、另一侧没有:没有的那侧必须本来就是 WGS84 系。
	if fromHas != toHas {
		plain := from
		if !fromHas {
			plain = from
		} else {
			plain = to
		}
		if !isWGS84Aligned(plain.Ellipsoid) {
			return nil, fmt.Errorf("proj: NewTransformer: 只有一侧带 +towgs84,另一侧椭球(a=%g)不是 WGS84 系,无法确定基准关系", plain.Ellipsoid.A)
		}
	}
	if fromHas {
		t.toWGS = newHelmert(from.Towgs84)
	}
	if toHas {
		t.fromWGS = newHelmert(to.Towgs84)
	}
	return t, nil
}

// isWGS84Aligned 椭球是否是 WGS84/GRS80 轴(两者差 < 0.1 mm,基准上视为同系)。
func isWGS84Aligned(el Ellipsoid) bool {
	return near(el.A, 6378137.0) &&
		(near(el.B, 6378137.0*(1-1/298.257223563)) || near(el.B, 6378137.0*(1-1/298.257222101)))
}

// From / To 返回两端 CRS。
func (t *Transformer) From() *CRS { return t.from }
func (t *Transformer) To() *CRS   { return t.to }

// wgs84Ellipsoid 基准变换的中转椭球。
var wgs84Ellipsoid = Ellipsoid{
	A: 6378137.0,
	B: 6378137.0 * (1 - 1/298.257223563),
	F: 1 / 298.257223563,
}

func init() {
	wgs84Ellipsoid.ES = 1 - (wgs84Ellipsoid.B/wgs84Ellipsoid.A)*(wgs84Ellipsoid.B/wgs84Ellipsoid.A)
	wgs84Ellipsoid.E = math.Sqrt(wgs84Ellipsoid.ES)
}

// Transform 源坐标 → 目标坐标(单位与轴序见文件头)。
func (t *Transformer) Transform(x, y float64) (float64, float64, error) {
	lon, lat, err := t.fromP.Inverse(x, y)
	if err != nil {
		return math.NaN(), math.NaN(), err
	}
	if t.toWGS != nil {
		X, Y, Z := geodeticToGeocentric(lon, lat, t.fromEl)
		X, Y, Z = t.toWGS.apply(X, Y, Z)
		lon, lat = geocentricToGeodetic(X, Y, Z, wgs84Ellipsoid)
	}
	if t.fromWGS != nil {
		X, Y, Z := geodeticToGeocentric(lon, lat, wgs84Ellipsoid)
		X, Y, Z = t.fromWGS.inverse(X, Y, Z)
		lon, lat = geocentricToGeodetic(X, Y, Z, t.toEl)
	}
	return t.toP.Forward(lon, lat)
}

// TransformBatch 批量版:xy 为 packed [x0,y0,...],语义同 ForwardBatch
// (域外点写 (NaN,NaN),批末返回 ErrDomain)。
func (t *Transformer) TransformBatch(xy []float64) ([]float64, error) {
	if len(xy)%2 != 0 {
		return nil, fmt.Errorf("proj: TransformBatch: 长度 %d 不是偶数(需 x/y 成对)", len(xy))
	}
	out := make([]float64, len(xy))
	domainErr := false
	for i := 0; i < len(xy); i += 2 {
		x, y, err := t.Transform(xy[i], xy[i+1])
		if err != nil {
			if !errors.Is(err, ErrDomain) {
				return nil, fmt.Errorf("proj: TransformBatch 第 %d 点: %w", i/2, err)
			}
			x, y = math.NaN(), math.NaN()
			domainErr = true
		}
		out[i], out[i+1] = x, y
	}
	if domainErr {
		return out, ErrDomain
	}
	return out, nil
}
