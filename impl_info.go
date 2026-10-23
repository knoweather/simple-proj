package proj

// impl_info.go — projection metadata introspection.
//
// PROJ exposes PJ_PROJ_INFO (id / description / definition / has_inverse /
// accuracy) through info(), so a caller can ask an instance "what are you, do
// you have an inverse, what is your domain" without reading the source. This
// file provides the equivalent surface: Info(p) plus the static table below.
//
// Why a package-level function and not a method on the Projection interface:
// the interface is deliberately minimal (CRS / Forward / Inverse), and adding
// Info would widen every implementation and every consumer at once. A
// package-level function adds surface without touching the interface; if it
// proves useful, promoting it to a method remains a later, compatible change.
//
// Dynamic fields come from the CRS itself (so they reflect what was actually
// parsed, including EPSG recognition); static fields come from the table below,
// cross-checked against the pyproj golden fixtures.

import "fmt"

// ProjectionInfo is static + dynamic metadata about a projection instance.
type ProjectionInfo struct {
	Name        string // proj 名(如 "tmerc")
	Description string // 一句话描述
	CRS         string // 规范 proj 串(crs.String())
	EPSG        int    // 特征回识别;0 = 未命中(4326/4269/3857/900913)
	Spherical   bool   // 正球体(a == b)
	Ellipsoidal bool   // 椭球(与 Spherical 互补,显式列出便于读)
	HasInverse  bool   // 是否有逆变换(本库所有已注册投影均为 true)
	Units       string // 输入→输出单位约定
	Domain      string // 域规则一句话
	Accuracy    string // 精度口径(golden/sweep 实测,PJ_PROJ_INFO accuracy 的对应物)
}

// projInfoEntry 静态元数据表。各投影的域/单位口径与 impl_*.go 的守卫逐条对应,
// accuracy 取 golden/sweep 实测最大值。
type projInfoEntry struct {
	description string
	hasInverse  bool
	units       string
	domain      string
	accuracy    string
}

var projInfoTable = map[string]projInfoEntry{
	"latlong": {
		description: "等经纬恒等投影(geodetic longitude/latitude)",
		hasInverse:  true,
		units:       "度 → 度",
		domain:      "全局(latlong 是恒等映射,无域界)",
		accuracy:    "恒等,往返逐位一致(golden_latlong 偏差 0)",
	},
	"eqc": {
		description: "Equidistant Cylindrical / Plate Carrée(等距圆柱)",
		hasInverse:  true,
		units:       "度 → 米",
		domain:      "|lat| ≤ 90;|lat_ts| = 90 拒收;k_0 忽略",
		accuracy:    "golden forward ≤ 5.4e-9 m,inverse ≤ 1.5e-11 度",
	},
	"geos": {
		description: "Geostationary Satellite View(静止卫星视图)",
		hasInverse:  true,
		units:       "度 → 米",
		domain:      "+h 必需;椭球视界外 → ErrDomain(球体无可见性检查);k_0 忽略",
		accuracy:    "golden forward ≤ 4.2e-9 m,inverse ≤ 5.4e-12 度",
	},
	"stere": {
		description: "Stereographic(球面/椭球方位投影,极冠/斜轴/赤道模式)",
		hasInverse:  true,
		units:       "度 → 米",
		domain:      "远极点(φ·n ≤ 0)→ ErrDomain;极冠 lat_ts≠90 且 k_0≠1 拒收",
		accuracy:    "golden forward 4.5e-4 m @ 1.3e10 m(相对 3.5e-14);inverse ≤ 1.6e-11 度",
	},
	"laea": {
		description: "Lambert Azimuthal Equal-Area(等积方位投影)",
		hasInverse:  true,
		units:       "度 → 米",
		domain:      "中心对跖点 → ErrDomain;不用 k_0(PROJ 口径)",
		accuracy:    "golden forward ≤ 2.97e-7 m,inverse ≤ 1.38e-12 度",
	},
	"lcc": {
		description: "Lambert Conformal Conic(双标准纬等角圆锥;单 lat_ts 为切锥)",
		hasInverse:  true,
		units:       "度 → 米",
		domain:      "|lat_1+lat_2| 须 > 0、|lat_i| < 90°;远极点(φ·n ≤ 0)→ ErrDomain;k_0 缩放 ρ",
		accuracy:    "golden forward 4.84e-5 m @ 9e7 m(相对 5.4e-13);inverse ≤ 2.6e-13 度",
	},
	"merc": {
		description: "Mercator(墨卡托;+lat_ts 作比例纬度)",
		hasInverse:  true,
		units:       "度 → 米",
		domain:      "|lat_ts| = 90 拒收;lat_ts 一旦给出 k_0 即被忽略(factor = msfn)",
		accuracy:    "golden forward ≤ 5.96e-8 m;inverse 1.98e-9 度(1e12 m 极端探针,按相对容差)",
	},
	"tmerc": {
		description: "Transverse Mercator(Poder/Engsager 6 阶精确序列,PROJ etmerc 同款算法)",
		hasInverse:  true,
		units:       "度 → 米",
		domain:      "|Ce| ≤ 2.623395162778 rad(赤道约 ±81.7° 中央子午线圈外)",
		accuracy:    "golden forward ≤ 7.45e-9 m,inverse ≤ 3.13e-13 度",
	},
	"aeqd": {
		description: "Azimuthal Equidistant(方位等距;极轴/赤道/斜轴,球+椭球)",
		hasInverse:  true,
		units:       "度 → 米",
		domain:      "球体:中心对跖点/极轴对侧极点 → ErrDomain;椭球:全域可解;k_0 忽略",
		accuracy:    "sweep forward ≤ 3.3e-8 m,inverse ≤ 1.6e-12 度;椭球斜轴走 Karney 大地线(geodesic.go)",
	},
	"utm": {
		description: "Universal Transverse Mercator(= tmerc + zone 派生的固定参数)",
		hasInverse:  true,
		units:       "度 → 米",
		domain:      "+zone 须为 1..60;域规则同 tmerc(±81.7° 中央子午线);+south 加 1e7 假北移",
		accuracy:    "sweep forward ≤ 3.7e-9 m,inverse ≤ 1.7e-13 度(等价 tmerc)",
	},
	"ob_tran": {
		description: "Oblique Transverse / 旋转极点(GRIB2 模板 3.1 的旋转经纬网格)",
		hasInverse:  true,
		units:       "度 → 度(旋转系经纬 → 未旋转系经纬)",
		domain:      "+o_lat_p 必需且 |o_lat_p| ≤ 90;+o_proj 只支持 longlat;全域可解(旋转是双射)",
		accuracy:    "xcheck/obtran_ref.json 7 组 × ~190 点:正反解均 ≤ 1.3e-13 度(pyproj 3.8.0)",
	},
}

// Info 返回投影实例的元数据(PJ_PROJ_INFO 的对应物)。
// p 为 nil 或 CRS 的 proj 名不在表内(理论上不会:未注册的 proj 过不了
// NewProjection)时报错。
func Info(p Projection) (*ProjectionInfo, error) {
	if p == nil {
		return nil, fmt.Errorf("proj: Info: nil Projection")
	}
	c := p.CRS()
	if c == nil {
		return nil, fmt.Errorf("proj: Info: Projection 的 CRS() 为 nil")
	}
	e, ok := projInfoTable[c.Proj]
	if !ok {
		return nil, fmt.Errorf("proj: Info: 未知投影 %q(表内应有尽有,此为防御分支)", c.Proj)
	}
	// 单位字段跟着 CRS 实际配置走(+units/+to_meter 生效时不能还报「米」)。
	// 输出是角度的 CRS(latlong、ob_tran+longlat)不受 +units 影响,保持表里的写法。
	units := e.units
	if !c.angularOutput() {
		if f, has := paramFloat(c, "to_meter", 1); has && f != 1 {
			if name := paramString(c, "units", ""); name != "" {
				units = "度 → " + name
			} else {
				units = fmt.Sprintf("度 → 米/%g(自定义 +to_meter)", f)
			}
		}
	}
	return &ProjectionInfo{
		Name:        c.Proj,
		Description: e.description,
		CRS:         c.String(),
		EPSG:        c.EPSG(),
		Spherical:   c.Ellipsoid.A == c.Ellipsoid.B,
		Ellipsoidal: c.Ellipsoid.A != c.Ellipsoid.B,
		HasInverse:  e.hasInverse,
		Units:       units,
		Domain:      e.domain,
		Accuracy:    e.accuracy,
	}, nil
}
