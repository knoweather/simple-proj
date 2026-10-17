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
// register 由 A 在 proj.go 中实现。B 的 impl 文件调用之(idiomatic 样板):
//
//	func init() { register("stere", newStere) }
//
// 空名或重复注册属于装配错误,init 时立即暴露(不藏到运行期);Projection
// 库 API 本身从不 panic(见 COORDINATION §2.5)。
func register(name string, factory func(*CRS) (Projection, error)) {
	if name == "" {
		panic("proj: register: empty projection name")
	}
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
// 批量推荐: crs.NewProjection() 一次然后持句柄(DESIGN §2.1)。
// +proj 名合法但实现尚未注册(如 B 轨投影未移交)时报错,不 panic。
func (c *CRS) NewProjection() (Projection, error) {
	if c == nil || c.Proj == "" {
		return nil, errors.New("proj: 零值 CRS 不可用,只能由 Parse 构造")
	}
	factory, ok := factories[c.Proj]
	if !ok {
		return nil, fmt.Errorf("proj: 投影 %q 尚未注册(+proj 合法但缺实现)", c.Proj)
	}
	return factory(c)
}

// ForwardBatch 批量正向:lonlat 为行主序 packed [lon0,lat0,lon1,lat1,...](度),
// 返回同长 packed [x0,y0,...](米;latlong 投影为度)。
// 语义(DESIGN §1.1 重采样场景):单点 Inverse/Forward 一切错误即时返回;
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
// (冻结契约只开放 Params[float64]/UnknownParams[string] 两个口,见
// PROGRESS-A.md 决策 D1);def 为缺省值。
func paramString(c *CRS, key, def string) string {
	if c == nil || c.UnknownParams == nil {
		return def
	}
	if v, ok := c.UnknownParams[key]; ok {
		return v
	}
	return def
}
