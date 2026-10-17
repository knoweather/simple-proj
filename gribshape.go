package proj

import "fmt"

// GRIBEllipsoid 把 GRIB2 模板 3.x 的 shape_of_the_earth(WMO 表 3.2)映射到
// Ellipsoid(结构冻结 §2.2);解码语义从 simple-grib grid/definition.go 的
// earthDimension/Radius 平移(ecCodes template.3.shape_of_the_earth.def 已在
// CT133 上核实):维度值 = raw × 10^(-scale);scale == 0xFF(unsigned[1] 缺失
// 哨兵)或 raw 全一视作"编码缺失"。
//
// 当前签名只装载一组 (scale, raw),对应编码维度在 octets 17-20(radius
// scale octet 16)/single-dim 形状;双轴形状 3(km 双轴)/7(m 双轴)需要
// major+minor 两组维度,冻结契约装不下 —— shape 3/7 暂报明确错误,缺口与
// 提案见 PROGRESS-A.md C#1(§5 流程,B 回执后由 owner 落地扩展签名)。
func GRIBEllipsoid(shape int, scale uint8, raw uint32) (Ellipsoid, error) {
	return Ellipsoid{}, fmt.Errorf("proj: GRIBEllipsoid 骨架未实现")
}
