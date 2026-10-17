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
	case 3, 7: // 生产者双轴:major/minor 两组 (scale,raw) —— 冻结签名只装一组
		return Ellipsoid{}, fmt.Errorf(
			"proj: GRIBEllipsoid: shape %d 需要 major+minor 双轴,当前冻结契约签名装不下 —— 见 PROGRESS-A.md C#1(%s 流程)", shape, "§5")
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
