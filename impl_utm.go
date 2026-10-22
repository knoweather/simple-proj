package proj

import "fmt"

// impl_utm.go — Universal Transverse Mercator(EPSG 32601-32660 / 32701-32760)。
//
// UTM 不是独立公式,而是 tmerc 的一组固定参数(PROJ utm.cpp 亦如此):
//
//	lat_0 = 0, lon_0 = zone·6 − 183, k_0 = 0.9996,
//	x_0 = 500000(假东),y_0 = 0(北半球)/ 10000000(南半球 +south)
//
// 因此这里把 CRS 改写成等价的 tmerc 再交给 newTmerc —— 椭球/球体分支、
// 6 阶精确序列、|Ce| 域判定全部复用,不重复实现。
//
// 参数口径(均以 pyproj 3.8.0 / PROJ 9.8.1 实测):
//   - +zone 必需且为 1..60 的整数(缺失/越界 PROJ 报 "Invalid value for zone");
//   - +lon_0/+x_0/+y_0 由 zone 与 +south 覆盖,用户给的值不生效(PROJ 同款);
//   - +south 为布尔旗标;+approx 会换成低阶近似,本库不实现 → Parse 拒收。

type utmProj struct {
	crs   *CRS
	inner Projection // 等价的 tmerc 实例
	zone  int
	south bool
}

// utmFalseNorthing 南半球假北移(PROJ utm.cpp 的 y_0)。
const utmFalseNorthing = 10000000.0

func newUtm(c *CRS) (Projection, error) {
	if c == nil {
		return nil, fmt.Errorf("proj: 零值 CRS 不可用,只能由 Parse 构造")
	}
	zoneF, has := paramFloat(c, "zone", 0)
	if !has {
		return nil, fmt.Errorf("proj: utm 需要 +zone=1..60")
	}
	zone := int(zoneF)
	if zoneF != float64(zone) || zone < 1 || zone > 60 {
		return nil, fmt.Errorf("proj: utm +zone=%v 非法(须为 1..60 的整数)", zoneF)
	}
	south := hasFlag(c, "south")

	y0 := 0.0
	if south {
		y0 = utmFalseNorthing
	}
	// 等价 tmerc CRS:参数由 zone/+south 决定,其余沿用原 CRS 的椭球。
	derived := &CRS{
		Proj: "tmerc",
		Params: map[string]float64{
			"lat_0": 0,
			"lon_0": float64(zone*6 - 183),
			"k_0":   0.9996,
			"x_0":   500000,
			"y_0":   y0,
		},
		Ellipsoid:     c.Ellipsoid,
		UnknownParams: map[string]string{},
	}
	inner, err := newTmerc(derived)
	if err != nil {
		return nil, err
	}
	return &utmProj{crs: c, inner: inner, zone: zone, south: south}, nil
}

func (p *utmProj) CRS() *CRS { return p.crs }

func (p *utmProj) Forward(lon, lat float64) (float64, float64, error) {
	return p.inner.Forward(lon, lat)
}

func (p *utmProj) Inverse(x, y float64) (float64, float64, error) {
	return p.inner.Inverse(x, y)
}

func init() { register("utm", newUtm) }
