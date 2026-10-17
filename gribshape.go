package proj

import (
	"fmt"
	"math"
)

// GRIBEllipsoid 把 GRIB2 模板 3.x 的 shape_of_the_earth(WMO 表 3.2)映射到
// Ellipsoid(结构冻结 §2.2);解码语义与取值从 simple-grib grid/definition.go
// 的 earthDimension/Radius/oblate 分支平移(ecCodes grib2
// template.3.shape_of_the_earth.def 已在 CT133 上核实)。
// 维度编码:值 = raw × 10^(-scale);scale == 0xFF(unsigned[1] 缺失哨兵)或
// raw 全一(0xFFFFFFFF)视作"编码缺失"。
//
// 形状表(球面形状 = 0/1/6/8/11;其余为扁球,= ecCodes earthIsOblate 口径):
//
//	0  正球 R=6367470			1  正球 R=raw×10⁻ˢᶜᵃˡᵉ(米,生产者给定)
//	2  IAG 1967:a=6378160, f=1/298.25
//	3  生产者 double 双轴(km)→ 契约缺口,见 PROGRESS-A.md C#1
//	4  WGS84:a=6378137, f=1/298.257223563
//	5  WGS84:a=6378137, b=6356752.3142(显式短轴)
//	6  正球 R=6371229(NCEP)	7  生产者双轴(m)→ 契约缺口,C#1
//	8  正球 R=6371200			9  Airy 1830(OSGB):a=6377563.396
//	10 WGS84 地心修正:同半长轴	11 太阳:正球 R=695990000
//
// scale/raw 仅在生产者给定形状(1;双轴 3/7)时消费,其余形状忽略。
func GRIBEllipsoid(shape int, scale uint8, raw uint32) (Ellipsoid, error) {
	switch shape {
	case 0:
		return sphereGRIB(6367470.0)
	case 1:
		r, ok := gribDimension(raw, scale)
		if !ok {
			return Ellipsoid{}, fmt.Errorf("proj: GRIBEllipsoid: shape 1 生产者半径缺失(0xFF 尺度 / 全一值)")
		}
		if !finitePositive(r) {
			return Ellipsoid{}, fmt.Errorf("proj: GRIBEllipsoid: shape 1 解码半径 %v 非法", r)
		}
		return newEllipsoid(r, r)
	case 2: // IAG 1967(GRS 67 同轴): a=6378160, f=1/298.25
		return newEllipsoid(6378160.0, 6378160.0*(1-1/298.25))
	case 3, 7: // 生产者双轴:major/minor 两组 (scale,raw) —— 走 GRIBEllipsoidAxes(C#1-ACK)
		return Ellipsoid{}, fmt.Errorf(
			"proj: GRIBEllipsoid: shape %d 需要 major+minor 双轴,请用 GRIBEllipsoidAxes(%s)", shape, "PROGRESS-A C#1;原函数保留单维/固定形状")
	case 4: // WGS84(扁率形式)
		return newEllipsoid(6378137.0, 6378137.0*(1-1/298.257223563))
	case 5: // WGS84(显式短轴)
		return newEllipsoid(6378137.0, 6356752.3142)
	case 6:
		return sphereGRIB(6371229.0)
	case 8:
		return sphereGRIB(6371200.0)
	case 9: // Airy 1830(OSGB)
		return newEllipsoid(6377563.396, 6377563.396*(1-1/299.3249646))
	case 10: // WGS84 地心修正:与 WGS84 同半长轴
		return newEllipsoid(6378137.0, 6378137.0*(1-1/298.257223563))
	case 11:
		return sphereGRIB(695990000.0)
	default:
		return Ellipsoid{}, fmt.Errorf("proj: GRIBEllipsoid: 未知的 shape_of_the_earth %d", shape)
	}
}

// GRIBEllipsoidAxes 生产者双轴形状(shape 3 = km,shape 7 = m)的完整解码
// (C#1,owner=A;B 在 PROGRESS-B.md 回执 C#1-ACK 后落地):
//
//	GRIBEllipsoidAxes(shape, majorScale, majorRaw, minorScale, minorRaw)
//
// shape 3 维度单位为 km(解码值 ×1000 升为米);shape 7 单位为米。
// 单维/固定形状仍走 GRIBEllipsoid(本函数对其他 shape 报错,不重复解码逻辑)。
// 缺失哨兵(0xFF 尺度 / 全一 raw)→ 明确报错;minor > major 经 newEllipsoid
// fail-loud 校验。
func GRIBEllipsoidAxes(shape int, majorScale uint8, majorRaw uint32, minorScale uint8, minorRaw uint32) (Ellipsoid, error) {
	if shape != 3 && shape != 7 {
		return Ellipsoid{}, fmt.Errorf(
			"proj: GRIBEllipsoidAxes: 仅支持生产者双轴 shape 3(km)/7(m), shape %d 请走 GRIBEllipsoid", shape)
	}
	aKm, okA := gribDimension(majorRaw, majorScale)
	bKm, okB := gribDimension(minorRaw, minorScale)
	if !okA || !okB {
		which := "major"
		if !okB {
			which = "minor"
		}
		return Ellipsoid{}, fmt.Errorf(
			"proj: GRIBEllipsoidAxes: shape %d 的 %s 维度编码缺失(0xFF 尺度 / 全一值)", shape, which)
	}
	if !finitePositive(aKm) || !finitePositive(bKm) {
		return Ellipsoid{}, fmt.Errorf(
			"proj: GRIBEllipsoidAxes: shape %d 解码维度 major=%v minor=%v(finitePositive 校验)非法", shape, aKm, bKm)
	}
	const u = 1000.0 // shape 3 是 km,shape 7 该值恰为 1 的乘数
	if shape == 7 {
		return newEllipsoid(aKm, bKm)
	}
	return newEllipsoid(aKm*u, bKm*u)
}

// gribDimension 解码生产者给定维度:raw × 10^(-scale);缺失哨兵 → (NaN, false)。
// (语义与 simple-grib grid/definition.go earthDimension 完全一致,单位换算由
// 调用方承担;本契约口径下 shape 1 的维度单位是米。)
func gribDimension(raw uint32, scale uint8) (float64, bool) {
	const (
		scaleMissing uint8  = 0xFF
		rawMissing   uint32 = 0xFFFFFFFF
	)
	if scale == scaleMissing || raw == rawMissing {
		return math.NaN(), false
	}
	return float64(raw) * math.Pow(10, -float64(scale)), true
}

// sphereGRIB 正球体捷径(半径必须是有效正数,由 newEllipsoid 兜底校验)。
func sphereGRIB(r float64) (Ellipsoid, error) {
	return newEllipsoid(r, r)
}
