package proj

import "math"

// geodesic.go — Karney 大地线算法(GeographicLib geodesic.c 的 Go 移植子集)。
//
// 为什么需要它:PROJ 的 aeqd 在**椭球斜轴/赤道**方面不是解析公式,而是
// geod_inverse(s12, azi1) → x = s12·sin(azi1), y = s12·cos(azi1);反向走
// geod_direct。要与之对到 1e-6 m,就必须有同精度的大地线解(Karney 2013,
// 辅助球 + 6 阶第三扁率级数),Vincenty 的 0.5 mm 截断误差过不了门槛。
//
// 移植范围(只保留 aeqd 用得到的两条路径):
//   - inverse:两点 → 距离 s12 与两端方位 azi1/azi2;
//   - direct :(lat1, lon1, azi1, s12) → (lat2, lon2, azi2)。
// 面积(C4 级数)、约化长度以外的量、arcmode、polygon 一律不移植。
//
// 系数表与算法结构逐字对齐 GeographicLib 2.x src/geodesic.c(MIT 许可,
// 作者 Charles Karney),包括 eps 级数阶数 6 与全部辅助函数(误差自由求和、
// AngRound、sincosd 的象限归约)。改系数表前先看该文件的注释。
//
// 单位约定:经纬度/方位角用**度**(与 PROJ 一致);距离用 geod.a 的倍数
// (PROJ 的 aeqd 用 geod_init(&g, 1, f),故 s12 是 a-归一量,最后乘 a)。

const (
	geodOrder = 6                               // GEOGRAPHICLIB_GEODESIC_ORDER
	geodNC    = geodOrder + 1                   // C1/C1p/C2/C3 数组下标 1..6
	geodN3x   = geodOrder * (geodOrder - 1) / 2 // C3x 长度 = 15
)

var (
	geodTiny    = math.Sqrt(2.2250738585072014e-308) // sqrt(DBL_MIN)
	geodTol0    = 2.220446049250313e-16              // DBL_EPSILON
	geodTol1    = 200 * geodTol0
	geodTol2    = math.Sqrt(geodTol0)
	geodTolb    = geodTol0
	geodXthresh = 1000 * geodTol2
	geodMaxit1  = 20
	geodMaxit2  = 20 + 53 + 10
)

// geod 是一个椭球上的大地线求解器(初始化后不可变,线程安全)。
type geod struct {
	a, f, f1, e2, ep2, n, b float64
	etol2                   float64
	A3x                     [geodOrder]float64
	C3x                     [geodN3x]float64
}

// newGeod 对应 geod_init():f 为扁率,(a, f) 决定一切。
func newGeod(a, f float64) *geod {
	g := &geod{a: a, f: f}
	g.f1 = 1 - f
	g.e2 = f * (2 - f)
	g.ep2 = g.e2 / (g.f1 * g.f1)
	g.n = f / (2 - f)
	g.b = a * g.f1
	// etol2:「真的很短」的 sig12 阈值(见 geodesic.c 的推导)
	etol := 0.1 * geodTol2 / math.Sqrt(math.Max(0.001, math.Abs(f))*math.Min(1.0, 1-f/2)/2)
	g.etol2 = etol
	g.a3Coeff()
	g.c3Coeff()
	return g
}

// ---- 标量助手(全部对应 geodesic.c 的同名 static 函数) ---------------------

func geodSq(x float64) float64 { return x * x }

// geodSumx 误差自由求和:u+v = s+t(|t| ≤ ulp(s)/2)。
func geodSumx(u, v float64) (s, t float64) {
	s = u + v
	up := s - v
	vpp := s - up
	up -= u
	vpp -= v
	if s != 0 {
		t = 0 - (up + vpp)
	} else {
		t = s
	}
	return s, t
}

// geodPolyval 对应 polyvalx(N, p, x):p 含 N+1 个系数,最高次在前。
func geodPolyval(p []float64, x float64) float64 {
	y := p[0]
	for _, c := range p[1:] {
		y = y*x + c
	}
	return y
}

// geodNorm2 归一化 (sin, cos) 对。
func geodNorm2(s, c float64) (float64, float64) {
	r := math.Hypot(s, c)
	return s / r, c / r
}

// geodAngNormalize 归一到 (-180, 180],±180 取原符号。
func geodAngNormalize(x float64) float64 {
	y := math.Remainder(x, 360)
	if math.Abs(y) == 180 {
		return math.Copysign(180, x)
	}
	return y
}

// geodLatFix 纬度越界 → NaN。
func geodLatFix(x float64) float64 {
	if math.Abs(x) > 90 {
		return math.NaN()
	}
	return x
}

// geodAngDiff 精确计算 y-x 的经差(度),e 为其舍入误差。
func geodAngDiff(x, y float64) (d, e float64) {
	var t float64
	d, t = geodSumx(math.Remainder(-x, 360), math.Remainder(y, 360))
	d, t = geodSumx(math.Remainder(d, 360), t)
	if d == 0 || math.Abs(d) == 180 {
		if t == 0 {
			d = math.Copysign(d, y-x)
		} else {
			d = math.Copysign(d, -t)
		}
	}
	return d, t
}

// geodAngRound 把 |x| < 1/16 的数向上舍到 1/16 的网格(避免 tan 的抵消)。
func geodAngRound(x float64) float64 {
	const z = 1.0 / 16.0
	y := math.Abs(x)
	w := z - y
	if w > 0 {
		y = z - w
	}
	return math.Copysign(y, x)
}

// geodSincosd 度制 sin/cos,先把角度精确归约到 [-45,45] 再转弧度。
func geodSincosd(x float64) (s, c float64) {
	r := math.Remainder(x, 90)
	q := int(math.Round((x - r) / 90))
	r *= math.Pi / 180
	s, c = math.Sincos(r)
	switch uint(q) & 3 {
	case 1:
		s, c = c, -s
	case 2:
		s, c = -s, -c
	case 3:
		s, c = -c, s
	}
	if s == 0 {
		s = math.Copysign(s, x)
	}
	return s, c
}

// geodSincosde 同 geodSincosd,但把舍入误差 t 一起并进归约后的角度。
func geodSincosde(x, t float64) (s, c float64) {
	r := math.Remainder(x, 90)
	q := int(math.Round((x - r) / 90))
	r = geodAngRound(r + t)
	r *= math.Pi / 180
	s, c = math.Sincos(r)
	switch uint(q) & 3 {
	case 1:
		s, c = c, -s
	case 2:
		s, c = -s, -c
	case 3:
		s, c = -c, s
	}
	if s == 0 {
		s = math.Copysign(s, x)
	}
	return s, c
}

// geodAtan2d atan2 的度制版本(先折到 [-45,45] 再补象限)。
func geodAtan2d(y, x float64) float64 {
	q := 0
	if math.Abs(y) > math.Abs(x) {
		x, y = y, x
		q = 2
	}
	if math.Signbit(x) {
		x = -x
		q++
	}
	ang := math.Atan2(y, x) * 180 / math.Pi
	switch q {
	case 1:
		ang = math.Copysign(180, y) - ang
	case 2:
		ang = 90 - ang
	case 3:
		ang = -90 + ang
	}
	return ang
}

// geodSinCosSeries Clenshaw 求和:sinp 时 Σc[i]sin(2i·x)(i=1..n),
// 否则 Σc[i]cos((2i+1)·x)(i=0..n-1);c[0] 在 sin 级数中不用。
func geodSinCosSeries(sinp bool, sinx, cosx float64, c []float64, n int) float64 {
	idx := n
	if sinp {
		idx = n + 1
	}
	ar := 2 * (cosx - sinx) * (cosx + sinx) // 2·cos(2x)
	var y0, y1 float64
	if n&1 == 1 {
		idx--
		y0 = c[idx]
	}
	for n /= 2; n > 0; n-- {
		idx--
		y1 = ar*y0 - y1 + c[idx]
		idx--
		y0 = ar*y1 - y0 + c[idx]
	}
	if sinp {
		return 2 * sinx * cosx * y0
	}
	return cosx * (y0 - y1)
}

// ---- eps 级数(系数表逐字取自 geodesic.c) --------------------------------

// A1m1f (1-eps)·A1-1 的均值项。
func geodA1m1f(eps float64) float64 {
	coeff := [5]float64{1, 4, 64, 0, 256}
	m := geodOrder / 2
	t := geodPolyval(coeff[:m+1], geodSq(eps)) / coeff[m+1]
	return (t + eps) / (1 - eps)
}

// C1f B1 的 Fourier 系数 C1[1..n]。
func geodC1f(eps float64, c []float64) {
	coeff := [...]float64{
		-1, 6, -16, 32,
		-9, 64, -128, 2048,
		9, -16, 768,
		3, -5, 512,
		-7, 1280,
		-7, 2048,
	}
	eps2 := geodSq(eps)
	d := eps
	o := 0
	for l := 1; l <= geodOrder; l++ {
		m := (geodOrder - l) / 2
		c[l] = d * geodPolyval(coeff[o:o+m+1], eps2) / coeff[o+m+1]
		o += m + 2
		d *= eps
	}
}

// C1pf B1p(反解用)的 Fourier 系数。
func geodC1pf(eps float64, c []float64) {
	coeff := [...]float64{
		205, -432, 768, 1536,
		4005, -4736, 3840, 12288,
		-225, 116, 384,
		-7173, 2695, 7680,
		3467, 7680,
		38081, 61440,
	}
	eps2 := geodSq(eps)
	d := eps
	o := 0
	for l := 1; l <= geodOrder; l++ {
		m := (geodOrder - l) / 2
		c[l] = d * geodPolyval(coeff[o:o+m+1], eps2) / coeff[o+m+1]
		o += m + 2
		d *= eps
	}
}

// A2m1f (eps+1)·A2-1。
func geodA2m1f(eps float64) float64 {
	coeff := [5]float64{-11, -28, -192, 0, 256}
	m := geodOrder / 2
	t := geodPolyval(coeff[:m+1], geodSq(eps)) / coeff[m+1]
	return (t - eps) / (1 + eps)
}

// C2f B2 的 Fourier 系数。
func geodC2f(eps float64, c []float64) {
	coeff := [...]float64{
		1, 2, 16, 32,
		35, 64, 384, 2048,
		15, 80, 768,
		7, 35, 512,
		63, 1280,
		77, 2048,
	}
	eps2 := geodSq(eps)
	d := eps
	o := 0
	for l := 1; l <= geodOrder; l++ {
		m := (geodOrder - l) / 2
		c[l] = d * geodPolyval(coeff[o:o+m+1], eps2) / coeff[o+m+1]
		o += m + 2
		d *= eps
	}
}

// a3Coeff A3 的 n 多项式系数(6 项,eps^0..eps^5)。
func (g *geod) a3Coeff() {
	coeff := [...]float64{
		-3, 128,
		-2, -3, 64,
		-1, -3, -1, 16,
		3, -1, -2, 8,
		1, -1, 2,
		1, 1,
	}
	o, k := 0, 0
	for j := geodOrder - 1; j >= 0; j-- {
		m := geodOrder - j - 1
		if j < m {
			m = j
		}
		g.A3x[k] = geodPolyval(coeff[o:o+m+1], g.n) / coeff[o+m+1]
		k++
		o += m + 2
	}
}

// c3Coeff C3 的 n 多项式系数(15 项,l=1..5 分组)。
func (g *geod) c3Coeff() {
	coeff := [...]float64{
		3, 128,
		2, 5, 128,
		-1, 3, 3, 64,
		-1, 0, 1, 8,
		-1, 1, 4,
		5, 256,
		1, 3, 128,
		-3, -2, 3, 64,
		1, -3, 2, 32,
		7, 512,
		-10, 9, 384,
		5, -9, 5, 192,
		7, 512,
		-14, 7, 512,
		21, 2560,
	}
	o, k := 0, 0
	for l := 1; l < geodOrder; l++ {
		for j := geodOrder - 1; j >= l; j-- {
			m := geodOrder - j - 1
			if j < m {
				m = j
			}
			g.C3x[k] = geodPolyval(coeff[o:o+m+1], g.n) / coeff[o+m+1]
			k++
			o += m + 2
		}
	}
}

// A3f 求 A3(eps)。
func (g *geod) A3f(eps float64) float64 {
	return geodPolyval(g.A3x[:], eps)
}

// C3f 求 C3[1..nC3-1]。注意 mult 在 polyval 之前乘(与 C1f/C2f/C4f 的
// 「先取值后乘」相反):C3[l] 整体带 eps^l 因子,系数表注释里的 eps^5..eps^1
// 正是 eps·(eps^4..eps^0)。
func (g *geod) C3f(eps float64, c []float64) {
	mult := 1.0
	o := 0
	for l := 1; l < geodOrder; l++ {
		m := geodOrder - l - 1
		mult *= eps
		c[l] = mult * geodPolyval(g.C3x[o:o+m+1], eps)
		o += m + 1
	}
}

// ---- 长度积分 -------------------------------------------------------------

// lengths 对应 Lengths():返回 s12/b、m12/b 与 m0。M12/M21(测地线尺度)不移植。
// ca 为调用方提供的暂存区(长度 geodNC)。
func (g *geod) lengths(eps, sig12, ssig1, csig1, dn1, ssig2, csig2, dn2,
	cbet1, cbet2 float64, wantS12 bool, ca []float64,
) (s12b, m12b, m0 float64) {
	var Cb [geodNC]float64
	A1 := geodA1m1f(eps)
	geodC1f(eps, ca)
	A2 := geodA2m1f(eps)
	geodC2f(eps, Cb[:])
	m0 = A1 - A2
	A2 = 1 + A2
	A1 = 1 + A1

	var J12 float64
	if wantS12 {
		B1 := geodSinCosSeries(true, ssig2, csig2, ca, geodOrder) -
			geodSinCosSeries(true, ssig1, csig1, ca, geodOrder)
		s12b = A1 * (sig12 + B1) // 缺一个 b 因子
		B2 := geodSinCosSeries(true, ssig2, csig2, Cb[:], geodOrder) -
			geodSinCosSeries(true, ssig1, csig1, Cb[:], geodOrder)
		J12 = m0*sig12 + (A1*B1 - A2*B2)
	} else {
		for l := 1; l <= geodOrder; l++ {
			Cb[l] = A1*ca[l] - A2*Cb[l]
		}
		J12 = m0*sig12 + (geodSinCosSeries(true, ssig2, csig2, Cb[:], geodOrder) -
			geodSinCosSeries(true, ssig1, csig1, Cb[:], geodOrder))
	}
	m12b = dn2*(csig1*ssig2) - dn1*(ssig1*csig2) - csig1*csig2*J12
	return s12b, m12b, m0
}

// ---- 反解(两点 → 距离 + 方位) -------------------------------------------

// astroid 解 k^4+2k³-(x²+y²-1)k²-2y²k-y² = 0 的正根(Geocentric::Reverse 改编)。
func geodAstroid(x, y float64) float64 {
	p := geodSq(x)
	q := geodSq(y)
	r := (p + q - 1) / 6
	if q == 0 && r <= 0 {
		return 0
	}
	S := p * q / 4
	r2 := geodSq(r)
	r3 := r * r2
	disc := S * (S + 2*r3)
	u := r
	if disc >= 0 {
		T3 := S + r3
		if T3 < 0 {
			T3 -= math.Sqrt(disc)
		} else {
			T3 += math.Sqrt(disc)
		}
		T := math.Cbrt(T3)
		u += T
		if T != 0 {
			u += r2 / T
		}
	} else {
		ang := math.Atan2(math.Sqrt(-disc), -(S + r3))
		u += 2 * r * math.Cos(ang/3)
	}
	v := math.Sqrt(geodSq(u) + q)
	var uv float64
	if u < 0 {
		uv = q / (v - u)
	} else {
		uv = u + v
	}
	w := (uv - q) / (2 * v)
	return uv / (math.Sqrt(uv+geodSq(w)) + w)
}

// inverseStart 给 Newton 迭代一个起点;返回 -1 表示需要迭代,
// 否则返回短线的 sig12 并同时给出 salp2/calp2/dnm。
func (g *geod) inverseStart(sbet1, cbet1, dn1, sbet2, cbet2, dn2,
	lam12, slam12, clam12 float64, ca []float64,
) (sig12, salp1, calp1, salp2, calp2, dnm float64) {
	sig12 = -1
	sbet12 := sbet2*cbet1 - cbet2*sbet1
	cbet12 := cbet2*cbet1 + sbet2*sbet1
	sbet12a := sbet2*cbet1 + cbet2*sbet1
	shortline := cbet12 >= 0 && sbet12 < 0.5 && cbet2*lam12 < 0.5
	var somg12, comg12 float64
	if shortline {
		sbetm2 := geodSq(sbet1 + sbet2)
		sbetm2 /= sbetm2 + geodSq(cbet1+cbet2)
		dnm = math.Sqrt(1 + g.ep2*sbetm2)
		omg12 := lam12 / (g.f1 * dnm)
		somg12, comg12 = math.Sincos(omg12)
	} else {
		somg12, comg12 = slam12, clam12
	}

	salp1 = cbet2 * somg12
	if comg12 >= 0 {
		calp1 = sbet12 + cbet2*sbet1*geodSq(somg12)/(1+comg12)
	} else {
		calp1 = sbet12a - cbet2*sbet1*geodSq(somg12)/(1-comg12)
	}
	ssig12 := math.Hypot(salp1, calp1)
	csig12 := sbet1*sbet2 + cbet1*cbet2*comg12

	switch {
	case shortline && ssig12 < g.etol2:
		// 真的很短的线
		salp2 = cbet1 * somg12
		if comg12 >= 0 {
			calp2 = sbet12 - cbet1*sbet2*(geodSq(somg12)/(1+comg12))
		} else {
			calp2 = sbet12 - cbet1*sbet2*(1-comg12)
		}
		salp2, calp2 = geodNorm2(salp2, calp2)
		sig12 = math.Atan2(ssig12, csig12)
	case math.Abs(g.n) > 0.1 || csig12 >= 0 ||
		ssig12 >= 6*math.Abs(g.n)*math.Pi*geodSq(cbet1):
		// 零阶球面近似足够
	default:
		// 把 lam12/bet2 缩放到「对跖点在原点、奇异点在 (-1,0)」的坐标系
		lam12x := math.Atan2(-slam12, -clam12) // lam12 - π
		var x, y, lamscale, betscale float64
		if g.f >= 0 {
			k2 := geodSq(sbet1) * g.ep2
			eps := k2 / (2*(1+math.Sqrt(1+k2)) + k2)
			lamscale = g.f * cbet1 * g.A3f(eps) * math.Pi
			betscale = lamscale * cbet1
			x = lam12x / lamscale
			y = sbet12a / betscale
		} else {
			cbet12a := cbet2*cbet1 - sbet2*sbet1
			bet12a := math.Atan2(sbet12a, cbet12a)
			_, m12b, m0 := g.lengths(g.n, math.Pi+bet12a,
				sbet1, -cbet1, dn1, sbet2, cbet2, dn2, cbet1, cbet2, false, ca)
			x = -1 + m12b/(cbet1*cbet2*m0*math.Pi)
			if x < -0.01 {
				betscale = sbet12a / x
			} else {
				betscale = -g.f * geodSq(cbet1) * math.Pi
			}
			lamscale = betscale / cbet1
			y = lam12x / lamscale
		}
		if y > -geodTol1 && x > -1-geodXthresh {
			// 近切割线
			if g.f >= 0 {
				salp1 = math.Min(1.0, -x)
				calp1 = -math.Sqrt(1 - geodSq(salp1))
			} else {
				if x > -geodTol1 {
					calp1 = math.Max(0.0, x)
				} else {
					calp1 = math.Max(-1.0, x)
				}
				salp1 = math.Sqrt(1 - geodSq(calp1))
			}
		} else {
			k := geodAstroid(x, y)
			var omg12a float64
			if g.f >= 0 {
				omg12a = lamscale * (-x * k / (1 + k))
			} else {
				omg12a = lamscale * (-y * (1 + k) / k)
			}
			somg12, comg12 = math.Sincos(omg12a)
			comg12 = -comg12
			salp1 = cbet2 * somg12
			calp1 = sbet12a - cbet2*sbet1*geodSq(somg12)/(1-comg12)
		}
	}
	// 起点自检(NaN 放行)
	if salp1 <= 0 {
		salp1, calp1 = 1, 0
	} else {
		salp1, calp1 = geodNorm2(salp1, calp1)
	}
	return sig12, salp1, calp1, salp2, calp2, dnm
}

// lambda12 给定 alp1 求 lam12 及其导数(diffp 时)。
func (g *geod) lambda12(sbet1, cbet1, dn1, sbet2, cbet2, dn2,
	salp1, calp1, slam120, clam120 float64, diffp bool, ca []float64,
) (lam12, salp2, calp2, sig12, ssig1, csig1, ssig2, csig2, eps, domg12, dlam12 float64) {
	if sbet1 == 0 && calp1 == 0 {
		// 破坏赤道线的退化情形(该情形已在别处处理)
		calp1 = -geodTiny
	}
	salp0 := salp1 * cbet1
	calp0 := math.Hypot(calp1, salp1*sbet1) // calp0 > 0

	ssig1 = sbet1
	somg1 := salp0 * sbet1
	csig1 = calp1 * cbet1
	comg1 := csig1
	ssig1, csig1 = geodNorm2(ssig1, csig1)

	// 强制 |bet2| = -bet1 时的对称性
	if cbet2 != cbet1 {
		salp2 = salp0 / cbet2
	} else {
		salp2 = salp1
	}
	if cbet2 != cbet1 || math.Abs(sbet2) != -sbet1 {
		var t float64
		if cbet1 < -sbet1 {
			t = (cbet2 - cbet1) * (cbet1 + cbet2)
		} else {
			t = (sbet1 - sbet2) * (sbet1 + sbet2)
		}
		calp2 = math.Sqrt(geodSq(calp1*cbet1)+t) / cbet2
	} else {
		calp2 = math.Abs(calp1)
	}
	ssig2 = sbet2
	somg2 := salp0 * sbet2
	csig2 = calp2 * cbet2
	comg2 := csig2
	ssig2, csig2 = geodNorm2(ssig2, csig2)

	sig12 = math.Atan2(math.Max(0.0, csig1*ssig2-ssig1*csig2), csig1*csig2+ssig1*ssig2)
	somg12 := math.Max(0.0, comg1*somg2-somg1*comg2)
	comg12 := comg1*comg2 + somg1*somg2
	eta := math.Atan2(somg12*clam120-comg12*slam120, comg12*clam120+somg12*slam120)
	k2 := geodSq(calp0) * g.ep2
	eps = k2 / (2*(1+math.Sqrt(1+k2)) + k2)
	g.C3f(eps, ca)
	B312 := geodSinCosSeries(true, ssig2, csig2, ca, geodOrder-1) -
		geodSinCosSeries(true, ssig1, csig1, ca, geodOrder-1)
	domg12 = -g.f * g.A3f(eps) * salp0 * (sig12 + B312)
	lam12 = eta + domg12

	if diffp {
		if calp2 == 0 {
			dlam12 = -2 * g.f1 * dn1 / sbet1
		} else {
			_, m12b, _ := g.lengths(eps, sig12, ssig1, csig1, dn1, ssig2, csig2, dn2,
				cbet1, cbet2, false, ca)
			dlam12 = m12b * g.f1 / (calp2 * cbet2)
		}
	}
	return lam12, salp2, calp2, sig12, ssig1, csig1, ssig2, csig2, eps, domg12, dlam12
}

// inverse 两点(度)→ (s12, azi1, azi2);s12 以 geod.a 为单位。
// 任一点非法(NaN 或 |lat|>90)时 ok=false。
func (g *geod) inverse(lat1, lon1, lat2, lon2 float64) (s12, azi1, azi2 float64, ok bool) {
	var ca [geodNC]float64
	var lon12s float64
	lon12, lon12s := geodAngDiff(lon1, lon2)
	lonsign := 1.0
	if math.Signbit(lon12) {
		lonsign = -1
	}
	lon12 *= lonsign
	lon12s *= lonsign
	lam12 := lon12 * (math.Pi / 180)
	slam12, clam12 := geodSincosde(lon12, lon12s)
	lon12s = (180 - lon12) - lon12s // 补经差

	lat1 = geodAngRound(geodLatFix(lat1))
	lat2 = geodAngRound(geodLatFix(lat2))
	if math.IsNaN(lat1) || math.IsNaN(lat2) {
		return 0, 0, 0, false
	}

	swapp := 1.0
	if math.Abs(lat1) < math.Abs(lat2) || math.IsNaN(lat2) {
		swapp = -1
		lonsign *= -1
		lat1, lat2 = lat2, lat1
	}
	latsign := -1.0
	if math.Signbit(lat1) {
		latsign = 1
	}
	lat1 *= latsign
	lat2 *= latsign

	sbet1, cbet1 := geodSincosd(lat1)
	sbet1 *= g.f1
	sbet1, cbet1 = geodNorm2(sbet1, cbet1)
	cbet1 = math.Max(geodTiny, cbet1)

	sbet2, cbet2 := geodSincosd(lat2)
	sbet2 *= g.f1
	sbet2, cbet2 = geodNorm2(sbet2, cbet2)
	cbet2 = math.Max(geodTiny, cbet2)

	if cbet1 < -sbet1 {
		if cbet2 == cbet1 {
			sbet2 = math.Copysign(sbet1, sbet2)
		}
	} else {
		if math.Abs(sbet2) == -sbet1 {
			cbet2 = cbet1
		}
	}
	dn1 := math.Sqrt(1 + g.ep2*geodSq(sbet1))
	dn2 := math.Sqrt(1 + g.ep2*geodSq(sbet2))

	var (
		s12x, m12x   float64
		sig12        float64
		salp1, calp1 float64
		salp2, calp2 float64
	)
	meridian := lat1 == -90 || slam12 == 0

	if meridian {
		// 两端在同一子午线上:测地线可能沿子午线
		calp1 = clam12
		salp1 = slam12
		calp2 = 1
		salp2 = 0
		ssig1 := sbet1
		csig1 := calp1 * cbet1
		ssig2 := sbet2
		csig2 := calp2 * cbet2
		sig12 = math.Atan2(math.Max(0.0, csig1*ssig2-ssig1*csig2), csig1*csig2+ssig1*ssig2)
		s12x, m12x, _ = g.lengths(g.n, sig12, ssig1, csig1, dn1, ssig2, csig2, dn2,
			cbet1, cbet2, true, ca[:])
		if sig12 < geodTol2 || m12x >= 0 {
			if sig12 < 3*geodTiny || (sig12 < geodTol0 && (s12x < 0 || m12x < 0)) {
				sig12, m12x, s12x = 0, 0, 0
			}
			m12x *= g.b
			s12x *= g.b
		} else {
			// m12 < 0:长球体且太靠近对跖点
			meridian = false
		}
	}

	if !meridian && sbet1 == 0 && (g.f <= 0 || lon12s >= g.f*180) {
		// 沿赤道
		calp1, calp2 = 0, 0
		salp1, salp2 = 1, 1
		s12x = g.a * lam12
		sig12 = lam12 / g.f1
		m12x = g.b * math.Sin(sig12)
	} else if !meridian {
		var dnm float64
		sig12, salp1, calp1, salp2, calp2, dnm = g.inverseStart(
			sbet1, cbet1, dn1, sbet2, cbet2, dn2, lam12, slam12, clam12, ca[:])
		if sig12 >= 0 {
			// 短线(InverseStart 已给出 salp2/calp2/dnm)
			s12x = sig12 * g.b * dnm
			m12x = geodSq(dnm) * g.b * math.Sin(sig12/dnm)
		} else {
			// Newton 迭代:f(alp1) = lambda12(alp1) - lam12 = 0
			salp1a, calp1a := geodTiny, 1.0
			salp1b, calp1b := geodTiny, -1.0
			tripn, tripb := false, false
			var ssig1, csig1, ssig2, csig2, eps float64
			for numit := 0; ; numit++ {
				var dv float64
				var v float64
				v, salp2, calp2, sig12, ssig1, csig1, ssig2, csig2, eps, _, dv =
					g.lambda12(sbet1, cbet1, dn1, sbet2, cbet2, dn2, salp1, calp1,
						slam12, clam12, numit < geodMaxit1, ca[:])
				if tripb || !(math.Abs(v) >= boolToFloat(tripn, 8, 1)*geodTol0) || numit == geodMaxit2 {
					break
				}
				if v > 0 && (numit > geodMaxit1 || calp1/salp1 > calp1b/salp1b) {
					salp1b, calp1b = salp1, calp1
				} else if v < 0 && (numit > geodMaxit1 || calp1/salp1 < calp1a/salp1a) {
					salp1a, calp1a = salp1, calp1
				}
				if numit < geodMaxit1 && dv > 0 {
					dalp1 := -v / dv
					if math.Abs(dalp1) < math.Pi {
						sdalp1, cdalp1 := math.Sincos(dalp1)
						nsalp1 := salp1*cdalp1 + calp1*sdalp1
						if nsalp1 > 0 {
							calp1 = calp1*cdalp1 - salp1*sdalp1
							salp1 = nsalp1
							salp1, calp1 = geodNorm2(salp1, calp1)
							tripn = math.Abs(v) <= 16*geodTol0
							continue
						}
					}
				}
				// 退回二分法:取括号中点
				salp1 = (salp1a + salp1b) / 2
				calp1 = (calp1a + calp1b) / 2
				salp1, calp1 = geodNorm2(salp1, calp1)
				tripn = false
				tripb = math.Abs(salp1a-salp1)+(calp1a-calp1) < geodTolb ||
					math.Abs(salp1-salp1b)+(calp1-calp1b) < geodTolb
			}
			s12x, m12x, _ = g.lengths(eps, sig12, ssig1, csig1, dn1, ssig2, csig2, dn2,
				cbet1, cbet2, true, ca[:])
			m12x *= g.b
			s12x *= g.b
		}
	}

	// 把 salp/calp 还原回原坐标系
	if swapp < 0 {
		salp1, salp2 = salp2, salp1
		calp1, calp2 = calp2, calp1
	}
	salp1 *= swapp * lonsign
	calp1 *= swapp * latsign
	salp2 *= swapp * lonsign
	calp2 *= swapp * latsign

	return 0 + s12x, geodAtan2d(salp1, calp1), geodAtan2d(salp2, calp2), true
}

// boolToFloat 小工具(对应 C 里的三元表达式)。
func boolToFloat(b bool, t, f float64) float64 {
	if b {
		return t
	}
	return f
}

// ---- 正解((lat1, lon1, azi1, s12) → 终点) -------------------------------

// geodLine 对应 geod_geodesicline 的裁剪版(只保留距离模式需要的量)。
type geodLine struct {
	g              *geod
	lon1           float64
	f1, b          float64
	salp0, calp0   float64
	ssig1, csig1   float64
	somg1, comg1   float64
	dn1, k2        float64
	A1m1, B11      float64
	stau1, ctau1   float64
	A3c, B31       float64
	C1a, C1pa, C3a [geodNC]float64
}

// initLine 对应 geod_lineinit()(caps 固定为距离+经纬度+方位)。
// 写成「填调用方给的栈上结构」而不是 new 一个指针:direct() 在批量循环里
// 每点调用一次,返回指针会每点一次堆分配(实测 1e6 点 1e6 allocs)。
func (g *geod) initLine(l *geodLine, lat1, lon1, azi1 float64) {
	*l = geodLine{g: g, lon1: lon1, f1: g.f1, b: g.b}
	lat1 = geodLatFix(lat1)
	azi1 = geodAngNormalize(azi1)
	salp1, calp1 := geodSincosd(geodAngRound(azi1))

	var sbet1, cbet1 float64
	sbet1, cbet1 = geodSincosd(geodAngRound(lat1))
	sbet1 *= g.f1
	sbet1, cbet1 = geodNorm2(sbet1, cbet1)
	cbet1 = math.Max(geodTiny, cbet1)
	l.dn1 = math.Sqrt(1 + g.ep2*geodSq(sbet1))

	l.salp0 = salp1 * cbet1
	l.calp0 = math.Hypot(calp1, salp1*sbet1)

	l.ssig1 = sbet1
	l.somg1 = l.salp0 * sbet1
	if sbet1 != 0 || calp1 != 0 {
		l.csig1 = cbet1 * calp1
	} else {
		l.csig1 = 1
	}
	l.comg1 = l.csig1
	l.ssig1, l.csig1 = geodNorm2(l.ssig1, l.csig1)

	l.k2 = geodSq(l.calp0) * g.ep2
	eps := l.k2 / (2*(1+math.Sqrt(1+l.k2)) + l.k2)

	l.A1m1 = geodA1m1f(eps)
	geodC1f(eps, l.C1a[:])
	l.B11 = geodSinCosSeries(true, l.ssig1, l.csig1, l.C1a[:], geodOrder)
	s, c := math.Sincos(l.B11)
	l.stau1 = l.ssig1*c + l.csig1*s
	l.ctau1 = l.csig1*c - l.ssig1*s
	geodC1pf(eps, l.C1pa[:])

	g.C3f(eps, l.C3a[:])
	l.A3c = -g.f * l.salp0 * g.A3f(eps)
	l.B31 = geodSinCosSeries(true, l.ssig1, l.csig1, l.C3a[:], geodOrder-1)
}

// position 对应 geod_genposition() 的距离模式:返回终点经纬度与方位(度)。
func (l *geodLine) position(s12 float64) (lat2, lon2, azi2 float64) {
	g := l.g
	tau12 := s12 / (l.b * (1 + l.A1m1))
	s, c := math.Sincos(tau12)
	// tau2 = tau1 + tau12
	B12 := -geodSinCosSeries(true,
		l.stau1*c+l.ctau1*s, l.ctau1*c-l.stau1*s, l.C1pa[:], geodOrder)
	sig12 := tau12 - (B12 - l.B11)
	ssig12, csig12 := math.Sincos(sig12)
	if math.Abs(g.f) > 0.01 {
		// |f| > 1/100 时距离级数不准,用一次 Newton 修正 sig12
		ssig2 := l.ssig1*csig12 + l.csig1*ssig12
		csig2 := l.csig1*csig12 - l.ssig1*ssig12
		B12 = geodSinCosSeries(true, ssig2, csig2, l.C1a[:], geodOrder)
		serr := (1+l.A1m1)*(sig12+(B12-l.B11)) - s12/l.b
		sig12 -= serr / math.Sqrt(1+l.k2*geodSq(ssig2))
		ssig12, csig12 = math.Sincos(sig12)
	}
	ssig2 := l.ssig1*csig12 + l.csig1*ssig12
	csig2 := l.csig1*csig12 - l.ssig1*ssig12

	sbet2 := l.calp0 * ssig2
	cbet2 := math.Hypot(l.salp0, l.calp0*csig2)
	if cbet2 == 0 {
		// salp0 = 0, csig2 = 0:破坏退化
		cbet2 = geodTiny
		csig2 = geodTiny
	}
	salp2 := l.salp0
	calp2 := l.calp0 * csig2

	// 经度(非 LONG_UNROLL 模式:omg12 直接取差,不做展开)
	somg2 := l.salp0 * ssig2
	comg2 := csig2
	omg12 := math.Atan2(somg2*l.comg1-comg2*l.somg1, comg2*l.comg1+somg2*l.somg1)
	lam12 := omg12 + l.A3c*(sig12+(geodSinCosSeries(true, ssig2, csig2, l.C3a[:], geodOrder-1)-l.B31))
	lon12 := lam12 * (180 / math.Pi)
	lon2 = geodAngNormalize(geodAngNormalize(l.lon1) + geodAngNormalize(lon12))

	lat2 = geodAtan2d(sbet2, l.f1*cbet2)
	azi2 = geodAtan2d(salp2, calp2)
	return lat2, lon2, azi2
}

// direct (lat1, lon1, azi1, s12) → (lat2, lon2, azi2)。s12 以 geod.a 为单位。
func (g *geod) direct(lat1, lon1, azi1, s12 float64) (lat2, lon2, azi2 float64, ok bool) {
	if math.IsNaN(lat1) || math.IsNaN(lon1) || math.IsNaN(azi1) || math.IsNaN(s12) ||
		math.Abs(lat1) > 90 {
		return 0, 0, 0, false
	}
	var l geodLine
	g.initLine(&l, lat1, lon1, azi1)
	lat2, lon2, azi2 = l.position(s12)
	return lat2, lon2, azi2, true
}
