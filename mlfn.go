package proj

import "math"

// mlfn.go — 子午线弧(meridional arc)与矩形纬度(rectifying latitude)。
// Owner: AI-A(B 的 geolib 管 等角/等积纬;矩形纬不在 geolib 清单内,在
// PROGRESS-A 决策 D6 声明过归属)。
//
// 口径与 pyproj 3.8 / PROJ 9.8 完全对齐:采用 Karney《On auxiliary latitudes》
// (Survey Review 56(395),2024;arXiv 2212.05818)的第三扁率 n 六阶级数。
// 公式参考下重写为 Go,idom 化;|f| ≤ 1/150 时达双精度级精度,覆盖 eqc 的
// 1e-6 m / 1e-9 度门槛 —— 且 PROJ 的远界是"无裁剪线性外推"(不迭代),
// golden 的 far outside 特例(如 lat=-80984°)必须复现同一形态,xcheck 才能对上。

// auxLatOrder Karney 系列阶(同 PROJ latitudes.cpp / mlfn.cpp)。
const auxLatOrder = 6

// mlPhiToMu C[phi,mu] 系数表(共 12 项,偶幂打包;分组见 mlCoeffs)。
var mlPhiToMu = [...]float64{
	3.0 / 2, -27.0 / 32, 269.0 / 512,
	21.0 / 16, -55.0 / 32, 6759.0 / 4096,
	151.0 / 96, -417.0 / 128,
	1097.0 / 512, -15543.0 / 2560,
	8011.0 / 2560,
	293393.0 / 61440,
}

// mlMuToPhi C[mu,phi] 系数表(同打包语义,供 μ→φ 逆转换)。
var mlMuToPhi = [...]float64{
	-3.0 / 2, 9.0 / 16, -3.0 / 32,
	15.0 / 16, -15.0 / 32, 135.0 / 2048,
	-35.0 / 48, 105.0 / 256,
	315.0 / 512, -189.0 / 512,
	-693.0 / 1280,
	1001.0 / 2048,
}

// mlCoeffs 预计算 φ→μ 的 Fourier 系数:F[l] 乘 sin(2(l+1)·φ),
// 各 F[l] 是 n 与 n² 的重 Pack 多项式(可见 PROJ latitudes.cpp 分组)。
func mlCoeffs(n float64) (F [auxLatOrder]float64) {
	n2 := n * n
	d := 1.0
	o := 0
	for l := 0; l < auxLatOrder; l++ {
		m := (auxLatOrder - l - 1) / 2
		F[l] = d * polyval(n2, mlPhiToMu[o:o+m+1])
		o += m + 1
		d *= n
	}
	return F
}

// mlInvCoeffs 预计算 μ→φ 的 Fourier 系数(同位模式)。
func mlInvCoeffs(n float64) (F [auxLatOrder]float64) {
	n2 := n * n
	d := 1.0
	o := 0
	for l := 0; l < auxLatOrder; l++ {
		m := (auxLatOrder - l - 1) / 2
		F[l] = d * polyval(n2, mlMuToPhi[o:o+m+1])
		o += m + 1
		d *= n
	}
	return F
}

// polyval Horner 多项式求值 Σ p[i]·x^i。
func polyval(x float64, p []float64) float64 {
	var y float64
	for i := len(p) - 1; i >= 0; i-- {
		y = y*x + p[i]
	}
	return y
}

// rectifyingRadius 矩形半径 = 四分之一子午线弧 / (π/2 · a)。
// 级数 (2k−3)!!/(2k)!!(系数 [1, 1/4, 1/64, 1/256]),除以 (1+n)。
func rectifyingRadius(n float64) float64 {
	return polyval(n*n, []float64{1, 1.0 / 4, 1.0 / 64, 1.0 / 256}) / (1 + n)
}

// clenshawDelta Clenshaw 求值:y = Σ F[k]·sin((2k+2)·zeta)(给 sin/cos 技巧)。
func clenshawDelta(szeta, czeta float64, F []float64) float64 {
	var u0, u1 float64
	X := 2 * (czeta - szeta) * (czeta + szeta) // = 2·cos(2ζ)
	for k := len(F) - 1; k >= 0; k-- {
		t := X*u0 - u1 + F[k]
		u1 = u0
		u0 = t
	}
	return 2 * szeta * czeta * u0
}

// mlfn 正向子午线弧(a 规一化,×a 由调用方负责):
// M/a = en0·(φ + Σ F[l]·sin(2(l+1)·φ)) = en0·auxlat_convert(φ→μ)。
func mlfn(phi float64, F [auxLatOrder]float64, en0 float64) float64 {
	sphi, cphi := math.Sincos(phi)
	return en0 * (phi + clenshawDelta(sphi, cphi, F[:]))
}

// mlfnInverse 逆(μ/a 域,含 en0 除权由调用方负责):φ = μ' + Σ F[l]·sin(2(l+1)μ')。
// 与 PROJ pj_inv_mlfn 一致:纯系列外推、不迭代、不裁剪 —— 远界狂值按键同复现。
func mlfnInverse(mu float64, F [auxLatOrder]float64) float64 {
	smu, cmu := math.Sincos(mu)
	return mu + clenshawDelta(smu, cmu, F[:])
}
