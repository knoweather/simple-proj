package proj

import "fmt"

// CRS 投影定义。零值不可用,只能由 Parse 构造。不可变。
type CRS struct {
	Proj          string             // "geos" 等(sans "+"、lowercased)
	Params        map[string]float64 // 归一化后的数值参数(lon_0/lat_0/k_0/h/lat_1/lat_2/x_0/y_0/lat_ts...)
	Ellipsoid     Ellipsoid
	UnknownParams map[string]string // 未识别的参数原样保留(只字符串值)
	projString    string            // 原始串,往返用
}

// Parse 解析 proj 字符串(如 "+proj=geos +lon_0=140.7 +h=35785863 +ellps=WGS84
// +units=m +no_defs"),也识别 "EPSG:4326"/"EPSG:3857" 等常用码。
// 设计(DESIGN §1.1):
//   - 参数拼写容错: lat1/lat_1 等别名统一归一;
//   - 默认值填补: lat_0/lon_0/x_0/y_0=0、k_0=1(其余投影专属参数由各 impl 处理);
//   - 未知 +key 不报错,仅保留 UnknownParams(警告语义);
//   - 椭球键 (+a/+b/+f/+rf/+es/+R) 进 Params,由 ellipsoid.go 消费;+ellps 字符串
//     名与 +proj/mod	Name 等字符串参数单独处理,不落数值表。
func Parse(s string) (*CRS, error) {
	return nil, fmt.Errorf("proj: Parse 骨架未实现")
}

// EPSG 依 CRS 状态识别常用 EPSG 码(DESIGN §1.1:4326/3857/4269;识别不到返回 0,
// 不查 proj.db)。优先识别由 "EPSG:xxxx" 字面量进入的字符串。
func (c *CRS) EPSG() int {
	return 0
}
