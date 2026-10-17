package proj

import (
	"fmt"
	"math"
)

// impl_latlong.go — latlong/longlat(恒等投影,度)。
// GRIB 经纬网格与 CF 1D 坐标的直接输入(DESIGN §1.2 #1)。
// 输入口径(§2.5):经度输入放宽 [−360,360]、纬度 [−90,90],越界或 NaN/Inf
// 报错(具体 error 不冒充 ErrDomain);输出经度归一化到 [−180,180)。

// latlongProj 恒等投影实例(无状态,线程安全)。
type latlongProj struct {
	crs *CRS
}

func newLatlong(c *CRS) (Projection, error) {
	if c == nil {
		return nil, fmt.Errorf("proj: 零值 CRS 不可用,只能由 Parse 构造")
	}
	return &latlongProj{crs: c}, nil
}

func (p *latlongProj) CRS() *CRS { return p.crs }

// Forward lon/lat(度)→ x, y(度)。
func (p *latlongProj) Forward(lon, lat float64) (x, y float64, err error) {
	if !validLonLat(lon, lat) {
		return math.NaN(), math.NaN(), invalidLonLatErr(lon, lat)
	}
	return normLonDeg(lon), lat, nil
}

// Inverse x, y(度)→ lon, lat(度)。latlong 的 x/y 与 lon/lat 同单位(度),
// 定义域与 Forward 相同。
func (p *latlongProj) Inverse(x, y float64) (lon, lat float64, err error) {
	if !validLonLat(x, y) {
		return math.NaN(), math.NaN(), invalidLonLatErr(x, y)
	}
	return normLonDeg(x), y, nil
}

// ---- 共享的输入域助手(同包内供 eqc/geos 等 impl 复用) -------------------

// validLonLat 合法地理输入(§2.5:经度 >±360 或纬度 >±90 非法;NaN/Inf 非法)。
func validLonLat(lon, lat float64) bool {
	return validX(lon) == nil && validY(lat) == nil
}

// validX 校验经度(度):有限且 |lon| <= 360。
func validX(lon float64) error {
	if math.IsNaN(lon) || math.IsInf(lon, 0) {
		return fmt.Errorf("proj: lon=%v 非有限", lon)
	}
	if math.Abs(lon) > 360 {
		return fmt.Errorf("proj: lon=%v 出界(需 |lon|<=360 度)", lon)
	}
	return nil
}

// validY 校验纬度(度):有限且 |lat| <= 90。
func validY(lat float64) error {
	if math.IsNaN(lat) || math.IsInf(lat, 0) {
		return fmt.Errorf("proj: lat=%v 非有限", lat)
	}
	if math.Abs(lat) > 90 {
		return fmt.Errorf("proj: lat=%v 出界 [-90,90]", lat)
	}
	return nil
}

// invalidLonLatErr 输入域错误(非 ErrDomain,§2.5)。
func invalidLonLatErr(lon, lat float64) error {
	if err := validX(lon); err != nil {
		return err
	}
	return validY(lat)
}

// normLonDeg 输出经度归一化到 [-180, 180)(§2.5)。
// 边界约定:−180 允许,180 归一为 −180(180° 与 −180° 是同一物理点;
// xcheck 断言按经度圆周距离比对,x_0/经度往返不受影响)。
func normLonDeg(lon float64) float64 {
	v := math.Mod(lon, 360)
	if v >= 180 {
		v -= 360
	}
	if v < -180 {
		v += 360
	}
	return v
}

func init() {
	register("latlong", newLatlong)
}
