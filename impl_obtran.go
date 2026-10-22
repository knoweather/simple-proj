package proj

import (
	"fmt"
	"math"
	"strings"
)

// impl_obtran.go — Oblique Transverse / 旋转极点（PROJ `ob_tran`）。
//
// 为什么需要它：GRIB2 模板 **3.1（旋转经纬）** 与 **3.3（旋转高斯）** 是区域
// 模式与 ECMWF/ICON 产品的常见网格。把旋转网格当普通经纬网格处理（DESIGN §5
// 早期把 3.1 映射成 `+proj=eqc`）会让整幅数据静默错位 —— 不报错、坐标看着也
// 正常，只是每个点都落在错的地方。正确的 PROJ 表达就是 `+proj=ob_tran
// +o_proj=longlat +o_lat_p=<北极纬度> +lon_0=<北极经度+180>`。
//
// # 参数与语义（口径：PROJ 9.8 ob_tran.cpp，pyproj 3.8.0 逐点实测）
//
//   - `+o_proj`：内层投影。**本库只支持 longlat/latlong**。PROJ 允许套米制投影
//     （如 `+o_proj=merc`），但那时 `+lon_0` 归旋转、`+x_0/+lat_0/+k_0` 归内层
//     的参数归属需要逐项核对（实测 `+o_proj=merc +lon_0=100` 时内层的中央经线
//     是 0，即 lon_0 被旋转吃掉），本库不猜，遇到其它内层投影直接报错。
//   - `+o_lat_p`：旋转极点在**未旋转系**里的纬度（度），**必需**（PROJ 缺省同样
//     报错 Error 1026）。|o_lat_p| > 90 本库拒收 —— PROJ 会照算并给出无意义的
//     结果（实测 `+o_lat_p=95` 得到 lat=-5），那属于"静默算错"。
//   - `+lon_0`：旋转极点在未旋转系里的经度（度），缺省 0。
//   - `+o_lon_p`：绕旋转极轴的附加转角（度），缺省 0。实测等价于先把旋转经度
//     平移 −o_lon_p 再做极点旋转。
//   - `+o_alpha` / `+o_lat_c` / `+o_lon_c` / `+o_lon_w` / `+o_lat_1..2` /
//     `+o_lon_1..2`：PROJ 的另一套"中心点+方位"参数化，本库未实现，Parse 阶段
//     即报错（见 crs.go 的 unsupportedParamReason）。
//
// # 数学
//
// 记极点 φp = o_lat_p、λp = lon_0，旋转系坐标 (λr, φr)。旋转是绕 Y 轴的正交
// 变换（下式即该矩阵），因此逆变换是其转置，往返精确：
//
//	λ' = λr − o_lon_p
//	sin φ  = sin φp·sin φ' + cos φp·cos φ'·cos λ'
//	λ      = λp + atan2(cos φ'·sin λ', sin φp·cos φ'·cos λ' − cos φp·sin φ')
//
//	sin φ' = sin φp·sin φ − cos φp·cos φ·cos(λ−λp)
//	λ'     = atan2(cos φ·sin(λ−λp), sin φp·cos φ·cos(λ−λp) + cos φp·sin φ)
//	λr     = λ' + o_lon_p
//
// 注意 atan2 的分母符号：PROJ 的约定下旋转北极（φr=90）落到
// (λp+180, φp)，与"极点定义"直觉相反，照抄 CF/教科书写法会把经度差 180°。
//
// 输入/输出都是**度**（与 latlong 同口径）：Forward 吃旋转系经纬、吐未旋转系
// 经纬；Inverse 反之。因此 +units/+to_meter 对它不生效（见 proj.go）。
//
// 域：全域可解（旋转是双射）；只有输入非有限/越界（|lon|>360、|lat|>90）报错。

// obTranProj ob_tran 实例（不可变，线程安全）。
type obTranProj struct {
	crs      *CRS
	sinP     float64 // sin(o_lat_p)
	cosP     float64 // cos(o_lat_p)
	lonP     float64 // lon_0（弧度）
	lonShift float64 // o_lon_p（弧度）
}

func init() { register("ob_tran", newObTran) }

// newObTran 工厂。校验放在这里而不是 Parse：Parse 只管语法与数值域，
// "这个投影的参数是否自洽"是投影级的判断（与 utm 的 +zone 同口径）。
func newObTran(c *CRS) (Projection, error) {
	if c == nil {
		return nil, fmt.Errorf("proj: 零值 CRS 不可用,只能由 Parse 构造")
	}
	inner := strings.ToLower(strings.TrimSpace(paramString(c, "o_proj", "")))
	switch inner {
	case "":
		return nil, fmt.Errorf("proj: ob_tran 需要 +o_proj(GRIB 旋转网格写 +o_proj=longlat)")
	case "longlat", "latlong":
		// 唯一支持的内层投影
	default:
		return nil, fmt.Errorf("proj: ob_tran 只支持 +o_proj=longlat(本库不猜内层投影的参数归属);"+
			"旋转后再套米制投影请用 NewTransformer 组合,内层取 %q", inner)
	}
	latP, hasLatP := paramFloat(c, "o_lat_p", 0)
	if !hasLatP {
		return nil, fmt.Errorf("proj: ob_tran 需要 +o_lat_p(旋转极点纬度,度);PROJ 同样拒收缺省")
	}
	if math.Abs(latP) > 90 {
		return nil, fmt.Errorf("proj: ob_tran +o_lat_p=%v 出界 [-90,90](PROJ 会照算并给出无意义结果,本库选择报错)", latP)
	}
	lonP, _ := paramFloat(c, "lon_0", 0)
	lonShift, _ := paramFloat(c, "o_lon_p", 0)

	rLatP := latP * deg2rad
	return &obTranProj{
		crs:      c,
		sinP:     math.Sin(rLatP),
		cosP:     math.Cos(rLatP),
		lonP:     lonP * deg2rad,
		lonShift: lonShift * deg2rad,
	}, nil
}

func (p *obTranProj) CRS() *CRS { return p.crs }

// Forward 旋转系经纬（度）→ 未旋转系经纬（度）。
func (p *obTranProj) Forward(lon, lat float64) (x, y float64, err error) {
	if !validLonLat(lon, lat) {
		return math.NaN(), math.NaN(), invalidLonLatErr(lon, lat)
	}
	lam := lon*deg2rad - p.lonShift
	phi := lat * deg2rad
	sinLam, cosLam := math.Sincos(lam)
	sinPhi, cosPhi := math.Sincos(phi)

	sinPhiOut := p.sinP*sinPhi + p.cosP*cosPhi*cosLam
	phiOut := math.Asin(clampUnit(sinPhiOut))
	lamOut := p.lonP + math.Atan2(cosPhi*sinLam, p.sinP*cosPhi*cosLam-p.cosP*sinPhi)
	return normLonDeg(lamOut * rad2deg), phiOut * rad2deg, nil
}

// Inverse 未旋转系经纬（度）→ 旋转系经纬（度）。
func (p *obTranProj) Inverse(x, y float64) (lon, lat float64, err error) {
	if !validLonLat(x, y) {
		return math.NaN(), math.NaN(), invalidLonLatErr(x, y)
	}
	dLam := x*deg2rad - p.lonP
	phi := y * deg2rad
	sinDLam, cosDLam := math.Sincos(dLam)
	sinPhi, cosPhi := math.Sincos(phi)

	sinPhiRot := p.sinP*sinPhi - p.cosP*cosPhi*cosDLam
	phiRot := math.Asin(clampUnit(sinPhiRot))
	lamRot := math.Atan2(cosPhi*sinDLam, p.sinP*cosPhi*cosDLam+p.cosP*sinPhi)
	return normLonDeg((lamRot + p.lonShift) * rad2deg), phiRot * rad2deg, nil
}
