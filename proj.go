package proj

import (
	"errors"
	"fmt"
	"math"
)

// ErrDomain 输入超出投影定义域(Inverse 定义域外时返回 NaN 坐标配此错误,
// 调用方用 errors.Is 判定,不当循环中断)。
var ErrDomain = errors.New("proj: outside projection domain")

// factories 投影工厂注册表:proj 名 → 工厂函数。各 impl_*.go 在自己的 init 里
// 通过 register 登记;NewProjection 按名分发。
var factories = map[string]func(*CRS) (Projection, error){}

// register —— 内部注册(各 impl 文件的 init 里调用;不是导出符号)——
// idiomatic 样板:
//
//	func init() { register("stere", newStere) }
//
// 空名或重复注册属于装配错误,init 时立即暴露(不藏到运行期);Projection
// 库 API 本身从不 panic。
func register(name string, factory func(*CRS) (Projection, error)) {
	if name == "" {
		panic("proj: register: empty projection name")
	}
	// 先查重、再写入。测试会在运行期调用本函数验证上面两条 panic 分支
	// (proj_internal_test.go);一旦把写入挪到查重之前,重复注册就会污染
	// 全局注册表,并与并发的 NewProjection 读构成 data race。顺序不能颠倒。
	if _, dup := factories[name]; dup {
		panic("proj: register: duplicate projection " + name)
	}
	factories[name] = factory
}

// Projection 是投影实例。实现必须是无状态的纯函数集;对象不可变、天然线程安全。
type Projection interface {
	CRS() *CRS
	// Forward 经纬度(度) → x, y(米)。latlong 投影返回度。
	Forward(lon, lat float64) (x, y float64, err error)
	// Inverse Forward 的逆: x, y(米) → 经纬度(度)。
	// 超出定义域 → (NaN, NaN, ErrDomain)。
	Inverse(x, y float64) (lon, lat float64, err error)
}

// NewProjection 依 CRS.Proj 在工厂表里查找实现,返回实例。
// 批量推荐: crs.NewProjection() 一次然后持句柄。
// +proj 名合法但实现尚未注册时报错,不 panic。
//
// 非米单位(+units/+to_meter)在这里套一层换算:各 impl 一律按**米**计算
// (含 x_0/y_0),对外坐标 = 米 / to_meter —— 与 PROJ 的 pj_fwd 收尾一致
// (pyproj 实测:+units=km +x_0=500000 的输出是 1113.19+500,即 x_0 也按米
// 参与后再整体换算)。latlong 输出是度,不做换算(PROJ 同样忽略)。
//
// 带 +towgs84 的 CRS 在这里**拒收**:单个投影只在自己的椭球上映射,基准平移
// 根本没有落脚点,静默忽略就是悄悄算错。要做基准变换请走 NewTransformer
// (transform.go),它才会把 towgs84 当 Helmert 用起来。
func (c *CRS) NewProjection() (Projection, error) { return c.newProjection(false) }

// newProjection allowDatum=true 时放行 +towgs84(仅供 NewTransformer 内部使用)。
func (c *CRS) newProjection(allowDatum bool) (Projection, error) {
	if c == nil || c.Proj == "" {
		return nil, errors.New("proj: 零值 CRS 不可用,只能由 Parse 构造")
	}
	if !allowDatum && hasTowgs84(c) {
		return nil, fmt.Errorf("proj: %s 带 +towgs84 基准变换,单投影无法表达(投影只在自己的椭球上映射);CRS→CRS 请用 NewTransformer", c.Proj)
	}
	factory, ok := factories[c.Proj]
	if !ok {
		return nil, fmt.Errorf("proj: 投影 %q 尚未注册(+proj 合法但缺实现)", c.Proj)
	}
	p, err := factory(c)
	if err != nil {
		return nil, err
	}
	if f, has := paramFloat(c, "to_meter", 1); has && f != 1 && !c.angularOutput() {
		return &unitScaled{Projection: p, toMeter: f}, nil
	}
	return p, nil
}

// unitScaled 单位换算装饰器:+units/+to_meter 时把米制坐标换成目标单位。
// 只包一层接口转发,内核实现与 bench 路径不变。
type unitScaled struct {
	Projection
	toMeter float64 // 1 单位 = toMeter 米
}

func (u *unitScaled) Forward(lon, lat float64) (float64, float64, error) {
	x, y, err := u.Projection.Forward(lon, lat)
	if err != nil {
		return x, y, err // NaN/ErrDomain 原样透出
	}
	return x / u.toMeter, y / u.toMeter, nil
}

func (u *unitScaled) Inverse(x, y float64) (float64, float64, error) {
	return u.Projection.Inverse(x*u.toMeter, y*u.toMeter)
}

// ForwardBatch 批量正向:lonlat 为行主序 packed [lon0,lat0,lon1,lat1,...](度),
// 返回同长 packed [x0,y0,...](米;latlong 投影为度)。
// 语义(批量重采样场景):单点 Inverse/Forward 一切错误即时返回;
// ErrDomain(定义域外)时该点写入 (NaN,NaN) 且循环继续,批末 err 返回 ErrDomain
// 供 errors.Is 判定,有效点结果仍可用。
func ForwardBatch(p Projection, lonlat []float64) (xy []float64, err error) {
	if p == nil {
		return nil, errors.New("proj: nil Projection")
	}
	if len(lonlat)%2 != 0 {
		return nil, fmt.Errorf("proj: ForwardBatch: lonlat 长度 %d 不是偶数(需 lon/lat 成对)", len(lonlat))
	}
	xy = make([]float64, len(lonlat))
	domainErr := false
	for i := 0; i < len(lonlat); i += 2 {
		x, y, err := p.Forward(lonlat[i], lonlat[i+1])
		if err != nil && !errors.Is(err, ErrDomain) {
			return nil, fmt.Errorf("proj: ForwardBatch 第 %d 点: %w", i/2, err)
		}
		if errors.Is(err, ErrDomain) {
			x, y = math.NaN(), math.NaN()
			domainErr = true
		}
		xy[i], xy[i+1] = x, y
	}
	if domainErr {
		return xy, ErrDomain
	}
	return xy, nil
}

// InverseBatch 批量逆向:xy 为 packed [x0,y0,...](米)→ [lon0,lat0,...](度)。
// 错误语义同 ForwardBatch。
func InverseBatch(p Projection, xy []float64) (lonlat []float64, err error) {
	if p == nil {
		return nil, errors.New("proj: nil Projection")
	}
	if len(xy)%2 != 0 {
		return nil, fmt.Errorf("proj: InverseBatch: xy 长度 %d 不是偶数(需 x/y 成对)", len(xy))
	}
	lonlat = make([]float64, len(xy))
	domainErr := false
	for i := 0; i < len(xy); i += 2 {
		lon, lat, err := p.Inverse(xy[i], xy[i+1])
		if err != nil && !errors.Is(err, ErrDomain) {
			return nil, fmt.Errorf("proj: InverseBatch 第 %d 点: %w", i/2, err)
		}
		if errors.Is(err, ErrDomain) {
			lon, lat = math.NaN(), math.NaN()
			domainErr = true
		}
		lonlat[i], lonlat[i+1] = lon, lat
	}
	if domainErr {
		return lonlat, ErrDomain
	}
	return lonlat, nil
}

// paramFloat 取归一化数值参数;缺省时返回 (def, false)。
// key 一律规范化拼写(latlong 侧 Parse 已把 lat1→lat_1 等别名归一)。
func paramFloat(c *CRS, key string, def float64) (float64, bool) {
	if c == nil || c.Params == nil {
		return def, false
	}
	v, ok := c.Params[key]
	if !ok {
		return def, false
	}
	return v, true
}

// paramString 取非数值参数的字符串值。Parse 把识别到的非数值参数(sweep、
// datum、towgs84、no_defs 等布尔旗标)与真正的未知参数同存 UnknownParams
// (CRS 只开放 Params[float64]/UnknownParams[string] 两个口);def 为缺省值。
func paramString(c *CRS, key, def string) string {
	if c == nil || c.UnknownParams == nil {
		return def
	}
	if v, ok := c.UnknownParams[key]; ok {
		return v
	}
	return def
}

// hasFlag 判断布尔旗标是否出现(+south/+no_defs 这类无值参数在
// UnknownParams 里存空串,与「不存在」靠 key 在不在区分)。
func hasFlag(c *CRS, key string) bool {
	if c == nil || c.UnknownParams == nil {
		return false
	}
	_, ok := c.UnknownParams[key]
	return ok
}
