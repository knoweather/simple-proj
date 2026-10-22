package proj

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// CRS 投影定义。零值不可用,只能由 Parse 构造。不可变。
type CRS struct {
	Proj          string             // "geos" 等(sans "+"、lowercased)
	Params        map[string]float64 // 归一化后的数值参数(lon_0/lat_0/k_0/h/lat_1/lat_2/x_0/y_0/lat_ts...)
	Ellipsoid     Ellipsoid
	UnknownParams map[string]string // 未识别的参数原样保留(只字符串值)
	// Towgs84 +towgs84 的 3 参或 7 参(EPSG 9603/9606,position vector 约定),
	// 空 = 无基准变换信息。单投影(NewProjection)不消费它,只有
	// NewTransformer 的 CRS→CRS 转换会用到(见 transform.go)。
	Towgs84    []float64
	projString string // 原始串,往返用
}

// ---- 参数命名空间(归一化口径,PROJ 兼容拼写) --------------------------------

// knownProj 投影名白名单(冻结 v1,DESIGN §1.2;值 = 常规名)。
// longlat 为 latlong 的 PROJ 惯用别名。
var knownProj = map[string]string{
	"latlong": "latlong",
	"longlat": "latlong",
	"eqc":     "eqc",
	"geos":    "geos",
	"stere":   "stere",
	"laea":    "laea",
	"lcc":     "lcc",
	"merc":    "merc",
	"tmerc":   "tmerc",
	"aeqd":    "aeqd",
	"utm":     "utm",
	"ob_tran": "ob_tran",
}

// knownNumericKeys 识别的数值参数(收纳进 CRS.Params)。
// 椭球键(a/b/f/rf/es/R)同在其中,由 ellipsoid.go 消费。
var knownNumericKeys = map[string]bool{
	"a": true, "b": true, "es": true, "f": true, "rf": true, "R": true,
	"h": true, "k_0": true, "lat_0": true, "lat_1": true, "lat_2": true,
	"lat_ts": true, "lon_0": true, "x_0": true, "y_0": true,
	// ob_tran 的旋转参数(见 impl_obtran.go);o_proj 是字符串,走 UnknownParams
	"o_lat_p": true, "o_lon_p": true,
	"zone":     true, // utm 带号 1..60
	"to_meter": true, // 自定义单位:每单位多少米
}

// unitToMeter PROJ proj_units.cpp 的惯用单位表(1 单位 = 多少米)。
// 与 pyproj 3.8.0 实测对齐:输出坐标 = 米 / to_meter(即换成该单位)。
var unitToMeter = map[string]float64{
	"km":     1000.0,
	"m":      1.0,
	"dm":     0.1,
	"cm":     0.01,
	"mm":     0.001,
	"kmi":    1852.0, // 国际海里
	"in":     0.0254,
	"ft":     0.3048,
	"yd":     0.9144,
	"mi":     1609.344,
	"fath":   1.8288,
	"ch":     20.1168,
	"link":   0.201168,
	"us-in":  1.0 / 39.37,
	"us-ft":  1200.0 / 3937.0,
	"us-yd":  3600.0 / 3937.0,
	"us-ch":  79200.0 / 3937.0,
	"us-mi":  6336000.0 / 3937.0,
	"ind-yd": 0.91439523,
	"ind-ft": 0.30479841,
	"ind-ch": 20.11669506,
}

// paramAliases 拼写容错(PROJ 新旧写法):规范化时重命名为带下划线/大写形式。
// 键大小写不敏感(+R 与 +r 同义,存为 "R")。
var paramAliases = map[string]string{
	"lat0": "lat_0", "lat1": "lat_1", "lat2": "lat_2", "lon0": "lon_0",
	"k": "k_0", "k0": "k_0", "x0": "x_0", "y0": "y_0",
	"latts": "lat_ts", "r": "R",
}

// angularKeys 度数参数(支持 PROJ 的 "1.570796r" 后缀 → 弧度值)。
var angularKeys = map[string]bool{
	"lat_0": true, "lat_1": true, "lat_2": true, "lat_ts": true, "lon_0": true,
	// ob_tran 的旋转参数也是角度(见 impl_obtran.go)
	"o_lat_p": true, "o_lon_p": true,
}

// unsupportedParamReason 本库「认识语义但没实现」的参数 → Parse 直接报错。
//
// 判据不是「我们认不认识这个键」,而是「忽略它会不会让结果算错」:
//   - +pm/+axis/+nadgrids/+datum(非 WGS84)/+R_* 族/+geoc/
//     +over/+geoidgrids 在 PROJ 里都会改变输出,静默丢弃 = 悄悄算错,故拒收;
//     (+units/+to_meter 已支持,见 unitToMeter 与 NewProjection 的换算层;
//     +towgs84 收进 CRS.Towgs84,只供 NewTransformer 做基准变换)
//   - +sweep 只在 geos 有意义,其他投影下 PROJ 同样忽略,不会改变输出,故不在此表
//     (仍由 geos 的适配层消费)。
//
// 键一律小写规范形(Parse 已把键统一小写,故 +R_A → r_a)。
var unsupportedParamReason = map[string]string{
	"pm":         "本初子午线偏移",
	"axis":       "轴序重排(axis order)",
	"nadgrids":   "NTv2/HARN 网格基准变换",
	"geoidgrids": "大地水准面网格",
	"geoc":       "地心纬度",
	"over":       "经度不归约(over-range)",
	"vunits":     "垂直单位",
	"vto_meter":  "垂直单位换算",
	"r_a":        "authalic 半径",
	"r_v":        "等体积半径",
	"r_g":        "几何平均半径",
	"r_h":        "调和平均半径",
	"r_lat_a":    "authalic 纬度半径",
	"r_lat_g":    "几何纬度半径",
	"datum":      "基准(datum)定义",
	"init":       "+init= 语法(PROJ 6 起已移除)",
	"defs":       "proj 定义文件",
	"approx":     "utm 的低阶近似路径(本库恒定走精确 etmerc)",
	"guam":       "aeqd 的 Guam 变体(旧式区域投影)",
	// ob_tran 的另一套参数化(中心点 + 方位)。本库只实现"极点 + 绕轴转角"
	// 那一套(GRIB 用的就是它),这套会静默给出不同结果,故拒收。
	"o_alpha": "ob_tran 的方位角参数化(本库只支持 +o_lat_p/+o_lon_p/+lon_0)",
	"o_lat_c": "ob_tran 的中心点参数化(本库只支持 +o_lat_p/+o_lon_p/+lon_0)",
	"o_lon_c": "ob_tran 的中心点参数化(本库只支持 +o_lat_p/+o_lon_p/+lon_0)",
	"o_lon_w": "ob_tran 的西边界经度参数化(本库只支持 +o_lat_p/+o_lon_p/+lon_0)",
	"o_lat_1": "ob_tran 的两点参数化(本库只支持 +o_lat_p/+o_lon_p/+lon_0)",
	"o_lon_1": "ob_tran 的两点参数化(本库只支持 +o_lat_p/+o_lon_p/+lon_0)",
	"o_lat_2": "ob_tran 的两点参数化(本库只支持 +o_lat_p/+o_lon_p/+lon_0)",
	"o_lon_2": "ob_tran 的两点参数化(本库只支持 +o_lat_p/+o_lon_p/+lon_0)",
}

// isNoopParamValue 判断「有语义的参数」取到的是否恰好是恒等值:
// +nadgrids=@null(空网格)、+datum=WGS84(即本库默认)。这两种在 PROJ 里
// 等于不做变换,可以安全放行。(+towgs84 有自己的 case,全零时由 allZero 处理;
// +no_defs/+type/+wktext 之类本来就无操作,走「未知键」的宽容分支。)
func isNoopParamValue(key, val string) bool {
	v := strings.TrimSpace(val)
	switch key {
	case "nadgrids":
		return v == "@null"
	case "datum":
		return strings.EqualFold(v, "WGS84") || strings.EqualFold(v, "WGS_1984")
	}
	return false
}

// parseTowgs84 解析 +towgs84=dx,dy,dz[,rx,ry,rz,s](3 或 7 个数)。
// 单位:平移米、旋转角秒、尺度 ppm(EPSG 9603/9606)。
func parseTowgs84(val string) ([]float64, error) {
	fields := strings.Split(val, ",")
	if len(fields) != 3 && len(fields) != 7 {
		return nil, fmt.Errorf("proj: +towgs84=%q 需 3 或 7 个数值", val)
	}
	out := make([]float64, len(fields))
	for i, f := range fields {
		v, err := strconv.ParseFloat(strings.TrimSpace(f), 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, fmt.Errorf("proj: +towgs84 第 %d 项 %q 不是有效数字", i+1, f)
		}
		out[i] = v
	}
	return out, nil
}

// allZero 全零 towgs84 等价于无变换(PROJ 同义)。
func allZero(v []float64) bool {
	for _, x := range v {
		if x != 0 {
			return false
		}
	}
	return true
}

// hasTowgs84 是否带非零基准参数。
func hasTowgs84(c *CRS) bool { return c != nil && len(c.Towgs84) > 0 }

// rejectUnsupportedParam 对「未实现语义」的键给出可执行的报错文案。
func rejectUnsupportedParam(key, val string) error {
	reason := unsupportedParamReason[key]
	if key == "init" {
		return fmt.Errorf("proj: 不支持 +init=%s:%s;请改用 \"EPSG:<码>\" 或等价的 +proj 串", val, reason)
	}
	if val == "" {
		return fmt.Errorf("proj: 不支持旗标 +%s(%s);静默忽略会让结果算错,本库选择报错", key, reason)
	}
	return fmt.Errorf("proj: 不支持参数 +%s=%s(%s);静默忽略会让结果算错,本库选择报错", key, val, reason)
}

// specialCodes 特例码 → 规范 proj 串(DESIGN §1.1:EPSG 识别只做少量特例)。
// 4326/4269 走 latlong(基准位移 v0 不做,NAD83 与 WGS84 同系无平移);
// 3857 是球面 Web 墨卡토(a==b)。
var specialCodes = map[int]string{
	4326:   "+proj=latlong +ellps=WGS84",
	4269:   "+proj=latlong +ellps=GRS80",
	3857:   "+proj=merc +a=6378137 +b=6378137 +lat_ts=0 +lon_0=0 +x_0=0 +y_0=0 +units=m +no_defs",
	900913: "+proj=merc +a=6378137 +b=6378137 +lat_ts=0 +lon_0=0 +x_0=0 +y_0=0 +units=m +no_defs",
}

// specialCodeOrder String() 再生 / EPSG() 识别的稳定序。
var specialCodeOrder = []int{4326, 4269, 3857, 900913}

// Parse 解析 proj 字符串(如 "+proj=geos +lon_0=140.7 +h=35785863 +ellps=WGS84
// +units=m +no_defs"),也识别 "EPSG:4326"/"EPSG:3857" 等常用码。
// 归一化(DESIGN §1.1):
//   - 键拼写容错(lat1→lat_1、k/k0→k_0、latts→lat_ts、r→R;键统一小写);
//   - 度数参数支持 "+lat_0=1.57r" 弧度后缀;
//   - 默认值填补: lat_0/lon_0/x_0/y_0 = 0、k_0 = 1(其余投影专属参数由 impl 处理);
//   - 未知 +key 不报错,原样进 UnknownParams;**已知语义但未实现**的键
//     (pm/axis/nadgrids/towgs84≠0/datum≠WGS84/R_* 族/geoc/over/
//     geoidgrids/init 等)直接报错——静默忽略会让结果算错(见
//     unsupportedParamReason);恒等值(nadgrids=@null、towgs84=0,0,0、
//     datum=WGS84)与 no_defs/type/wktext 放行;
//   - 椭球键收敛进数值表由 ellipsoid.go 组合推导,发现矛盾即报错;
//   - +units 查 unitToMeter 表(m/km/ft/us-ft/mi/...),+to_meter 自定义;
//     两者同给时 +units 胜;坐标按「米 / to_meter」输出(见 proj.go 的换算层);
//   - 未知 +proj 报错。
//
// 同名键后者胜;返回的 CRS 不可变(约定),零值不入。
func Parse(s string) (*CRS, error) {
	raw := strings.TrimSpace(s)
	if raw == "" {
		return nil, fmt.Errorf("proj: 空 proj 字符串")
	}
	// WKT2/WKT1 自动分派(见 wkt.go)
	if looksLikeWKT(raw) {
		return ParseWKT(raw)
	}

	// EPSG 特例码 → 递归走规范 proj 串;projString 保留原码便于识别/往返。
	if code, ok := parseEPSGCode(raw); ok {
		base, found := specialCodes[code]
		if !found {
			base, found = utmEPSGString(code)
		}
		if !found {
			return nil, fmt.Errorf("proj: 不支持的 EPSG:%d(v0 只收录 4326/4269/3857 与 WGS84 UTM 32601-32660/32701-32760)", code)
		}
		crs, err := parseProjString(base)
		if err != nil {
			return nil, err
		}
		crs.projString = raw
		return crs, nil
	}
	return parseProjString(raw)
}

// utmEPSGString WGS84 UTM 的 EPSG 区间 → proj 串:
// 32601-32660 为北半球 zone 1-60,32701-32760 为南半球。
func utmEPSGString(code int) (string, bool) {
	switch {
	case code >= 32601 && code <= 32660:
		return fmt.Sprintf("+proj=utm +zone=%d +ellps=WGS84", code-32600), true
	case code >= 32701 && code <= 32760:
		return fmt.Sprintf("+proj=utm +zone=%d +south +ellps=WGS84", code-32700), true
	}
	return "", false
}

// parseProjString 解析纯 proj 串(不做 EPSG 特例)。projString 从此处定。
func parseProjString(s string) (*CRS, error) {
	crs := &CRS{
		Params:        map[string]float64{},
		UnknownParams: map[string]string{},
	}
	crs.projString = strings.TrimSpace(s)

	ellpsName := ""
	unitsName := ""

	tokens := parseTokens(crs.projString)
	for _, tok := range tokens {
		key, val, ok := strings.Cut(tok, "=")
		key = strings.TrimSpace(strings.ToLower(key))
		if key == "" {
			return nil, fmt.Errorf("proj: 空参数名:+%s", tok)
		}
		if alias, has := paramAliases[key]; has {
			key = alias
		}
		if !ok { // 旗标(+no_defs 等)
			if key == "proj" {
				return nil, fmt.Errorf("proj: +proj 必须带投影名")
			}
			if knownNumericKeys[key] {
				return nil, fmt.Errorf("proj: 数值参数 +%s 缺值", key)
			}
			// 这几个键裸着写等于没说,PROJ 也报错(pyproj 实测),不能静默放行
			switch key {
			case "ellps":
				return nil, fmt.Errorf("proj: +ellps 必须带椭球名(+ellps=WGS84)")
			case "units":
				return nil, fmt.Errorf("proj: +units 必须带单位名(+units=m)")
			case "towgs84":
				return nil, fmt.Errorf("proj: +towgs84 必须带 3 或 7 个数值(+towgs84=dx,dy,dz[,rx,ry,rz,s])")
			}
			if _, bad := unsupportedParamReason[key]; bad {
				return nil, rejectUnsupportedParam(key, "")
			}
			crs.UnknownParams[key] = "" // no_defs / 未知旗标
			continue
		}
		switch {
		case key == "proj":
			name := strings.ToLower(strings.TrimSpace(val))
			if name == "pipeline" || name == "concatenated_operation" {
				return nil, fmt.Errorf("proj: 不支持 +proj=%s 操作链;两个 CRS 之间的转换请用 NewTransformer(transform.go)", name)
			}
			canonical, known := knownProj[name]
			if !known {
				return nil, fmt.Errorf("proj: 未知 +proj=%q(v1 冻结清单见 PROGRESS-A 决策 D4)", val)
			}
			crs.Proj = canonical
		case key == "ellps":
			if strings.TrimSpace(val) == "" {
				return nil, fmt.Errorf("proj: +ellps= 缺椭球名(PROJ 同样报错)")
			}
			ellpsName = strings.TrimSpace(val)
		case key == "units":
			name := strings.ToLower(strings.TrimSpace(val))
			if _, known := unitToMeter[name]; !known {
				return nil, fmt.Errorf("proj: 未知 +units=%q(支持 km/m/dm/cm/mm/kmi/in/ft/yd/mi/fath/ch/link/us-*/ind-*)", val)
			}
			unitsName = name
			crs.UnknownParams["units"] = name
		case key == "towgs84":
			tp, err := parseTowgs84(val)
			if err != nil {
				return nil, err
			}
			if !allZero(tp) {
				crs.Towgs84 = tp
			}
			crs.UnknownParams[key] = val
		case knownNumericKeys[key]:
			num, err := parseParamNumber(key, val)
			if err != nil {
				return nil, err
			}
			if math.IsNaN(num) || math.IsInf(num, 0) {
				return nil, fmt.Errorf("proj: 参数 +%s=%s 非有限", key, val)
			}
			crs.Params[key] = num
		default:
			// 未实现语义的键 fail-loud;白名单/未知键原样保留(决策 D1)。
			if _, bad := unsupportedParamReason[key]; bad {
				if isNoopParamValue(key, val) {
					crs.UnknownParams[key] = val
					continue
				}
				return nil, rejectUnsupportedParam(key, val)
			}
			crs.UnknownParams[key] = val
		}
	}
	if crs.Proj == "" {
		return nil, fmt.Errorf("proj: 缺少 +proj 名(+proj=...)")
	}

	// +units 与 +to_meter 同时出现时 +units 胜(PROJ pj_units 口径,pyproj 实测)。
	if unitsName != "" {
		crs.Params["to_meter"] = unitToMeter[unitsName]
	}
	applyDefaults(crs.Params)
	if err := validateNumericParams(crs.Params); err != nil {
		return nil, err
	}
	ellipsoid, err := EllipsoidFromParams(crs.Params, ellpsName)
	if err != nil {
		return nil, fmt.Errorf("proj(ls): %w", err)
	}
	crs.Ellipsoid = ellipsoid
	return crs, nil
}

// parseTokens 切分 proj 串 token:以 '+' 分隔(容忍首前导 '+',空 token 忽略)。
func parseTokens(s string) []string {
	parts := strings.Split(s, "+")
	tokens := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			tokens = append(tokens, p)
		}
	}
	return tokens
}

// parseParamNumber 数值解析(度数参数支持 'r' 弧度后缀)。
func parseParamNumber(key, val string) (float64, error) {
	if strings.HasSuffix(val, "r") {
		if !angularKeys[key] {
			return 0, fmt.Errorf("proj: 参数 +%s=%s 不支持 'r' 弧度后缀", key, val)
		}
		rad, err := strconv.ParseFloat(strings.TrimSuffix(val, "r"), 64)
		if err != nil {
			return 0, fmt.Errorf("proj: 参数 +%s=%s 不是有效数字", key, val)
		}
		return rad * 180 / math.Pi, nil
	}
	num, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return 0, fmt.Errorf("proj: 参数 +%s=%s 不是有效数字", key, val)
	}
	return num, nil
}

// parseEPSGCode 识别 "EPSG:xxxx"/"epsg:4326"(前缀大小写不敏感)。
func parseEPSGCode(s string) (int, bool) {
	up := strings.ToUpper(strings.TrimSpace(s))
	if !strings.HasPrefix(up, "EPSG:") {
		return 0, false
	}
	codeStr := strings.TrimSpace(strings.TrimPrefix(up, "EPSG:"))
	code, err := strconv.Atoi(codeStr)
	if err != nil {
		return 0, false
	}
	return code, true
}

// applyDefaults 填补常用默认值(PROJ 口径:lat_0/lon_0/x_0/y_0=0、k_0=1)。
func applyDefaults(params map[string]float64) {
	for _, key := range []string{"lat_0", "lon_0", "x_0", "y_0"} {
		if _, ok := params[key]; !ok {
			params[key] = 0
		}
	}
	if _, ok := params["k_0"]; !ok {
		params["k_0"] = 1
	}
}

// validateNumericParams 数值域校验(fail-loud;投影级细节由各 impl 再查)。
func validateNumericParams(params map[string]float64) error {
	if k := params["k_0"]; k <= 0 {
		return fmt.Errorf("proj: +k_0=%v 必须 > 0", k)
	}
	if _, ok := params["h"]; ok {
		if h := params["h"]; h <= 0 {
			return fmt.Errorf("proj: +h=%v 必须 > 0(geos 卫星高度)", h)
		}
	}
	// o_lat_p 是 ob_tran 的旋转极点纬度,与 lat_0 同域
	for _, key := range []string{"lat_0", "lat_ts", "lat_1", "lat_2", "o_lat_p"} {
		if v, ok := params[key]; ok && math.Abs(v) > 90 {
			return fmt.Errorf("proj: +%s=%v 出界 [-90,90]", key, v)
		}
	}
	if v, ok := params["lon_0"]; ok && math.Abs(v) > 360 {
		return fmt.Errorf("proj: +lon_0=%v 出界 [-360,360]", v)
	}
	if v, ok := params["to_meter"]; ok && !(v > 0) {
		return fmt.Errorf("proj: +to_meter=%v 必须 > 0(每单位多少米)", v)
	}
	return nil
}

// EPSG 依 CRS 状态识别常用 EPSG 码(DESIGN §1.1:4326/3857/4269;识别不到返回 0,
// 不查 proj.db)。优先识别由 "EPSG:xxxx" 字面量进入的字符串;其余做特征比对
// (不含基准语义,4269 与 4326 同构差异只在椭球)。
func (c *CRS) EPSG() int {
	if c == nil {
		return 0
	}
	if code, ok := parseEPSGCode(c.projString); ok {
		if _, found := specialCodes[code]; found {
			if code == 900913 {
				return 3857 // 常规化为标准码
			}
			return code
		}
		if _, found := utmEPSGString(code); found {
			return code
		}
	}
	// 带 +towgs84 的 CRS 与同名 WGS84 系 CRS 不是同一个东西(差一个基准),
	// 特征比对必须让路,否则 "+proj=longlat +ellps=WGS84 +towgs84=1,2,3" 会被
	// 认成 EPSG:4326。
	if hasTowgs84(c) {
		return 0
	}
	// 特征比对(容忍参数缺失但语义等同)。非米单位下坐标数值不同,
	// 不能凭几何特征认成 4326/3857 之类的米制码(latlong 输出是度,不受影响)。
	if f, ok := paramFloat(c, "to_meter", 1); ok && f != 1 && !c.angularOutput() {
		return 0
	}
	if _, isLatlong := knownProj[c.Proj]; isLatlong && c.Proj == "latlong" {
		if near(c.Ellipsoid.A, 6378137.0) && near(c.Ellipsoid.B, 6378137.0*(1-1/298.257223563)) {
			return 4326
		}
		if near(c.Ellipsoid.A, 6378137.0) && near(c.Ellipsoid.B, 6378137.0*(1-1/298.257222101)) {
			return 4269
		}
	}
	if c.Proj == "merc" && c.Ellipsoid.Sphere && near(c.Ellipsoid.A, 6378137.0) {
		latTs, _ := paramFloat(c, "lat_ts", 0)
		lon0, _ := paramFloat(c, "lon_0", 0)
		x0, _ := paramFloat(c, "x_0", 0)
		y0, _ := paramFloat(c, "y_0", 0)
		if latTs == 0 && lon0 == 0 && x0 == 0 && y0 == 0 {
			return 3857
		}
	}
	// UTM:WGS84 椭球 + 合法 zone → 326xx/327xx
	if c.Proj == "utm" && near(c.Ellipsoid.A, 6378137.0) && near(c.Ellipsoid.B, 6378137.0*(1-1/298.257223563)) {
		if zone, ok := paramFloat(c, "zone", 0); ok {
			z := int(zone)
			if float64(z) == zone && z >= 1 && z <= 60 {
				if hasFlag(c, "south") {
					return 32700 + z
				}
				return 32600 + z
			}
		}
	}
	return 0
}

// nearEPSG 宽容比较(用于 EPSG() 特征比对)。
// eps 取 1e-12(量级 ≈ 1.3e-5 m):必须紧到能区分 GRS80(4269)与 WGS84
// (4326)的 b 差 ~1.05e-4 m(1e-9 口径会误判 4269→4326);同时宽容正常运行
// 期确定性公式的双舍入噪声(~1e-9 m)。
func near(a, b float64) bool {
	const eps = 1e-12
	return math.Abs(a-b) <= eps*(math.Abs(a)+math.Abs(b)+1)
}

// ---- 元数据自省(第二轮 goproj 评审的融合,见 PROGRESS-B)----------------------
//
// goproj(cgo 包装)里有一组 PROJ 的"输入/输出是什么量"的查询:
// proj_angular_input / proj_degree_input / proj_angular_output /
// proj_degree_output。本库的约定是固定的(地理 CRS 收发度、投影 CRS 收发米),
// 所以不需要那四个函数;但**调用方**仍然需要机器可读地知道"这个 CRS 该喂度
// 还是米、单位是什么"—— 那是 IsGeographic / Units 要做的事。
// 另外 pyproj 有 CRS.equals(),本库一直没有,这里补上(结构比较,不查数据库)。

// IsGeographic 是否地理坐标系(本库只有 latlong 一种,收发都是度)。
// ob_tran 的输出虽然也是度,但那是**旋转系**经纬,不算地理坐标系 ——
// 判断"输出是不是角度"请用 Units()=="degree"。
func (c *CRS) IsGeographic() bool { return c != nil && c.Proj == "latlong" }

// angularOutput 该 CRS 的输出是否为**角度(度)**而不是米。
//
// latlong 恒为度;ob_tran 在 o_proj=longlat 时输出也是旋转系经纬(度)——
// PROJ 同样不给它套 +units(pyproj 实测 +units=km 对 ob_tran+longlat 无影响)。
func (c *CRS) angularOutput() bool {
	if c == nil {
		return false
	}
	switch c.Proj {
	case "latlong":
		return true
	case "ob_tran":
		inner := strings.ToLower(strings.TrimSpace(paramString(c, "o_proj", "")))
		return inner == "longlat" || inner == "latlong"
	}
	return false
}

// Units 返回坐标单位(机器可读):
//   - 地理 CRS 恒为 "degree"(输出度);
//   - 投影 CRS 为 +units 的名字(m/km/ft/us-ft…),未给时 "m";
//   - 用 +to_meter 自定义时返回 "to_meter=<值>"。
//
// 与 Info().Units 的区别:后者是给人看的("度 → 米"),这个是给代码判断的。
func (c *CRS) Units() string {
	if c == nil {
		return ""
	}
	// 输出是角度的 CRS:latlong,以及 ob_tran + o_proj=longlat(旋转经纬)。
	if c.angularOutput() {
		return "degree"
	}
	if name := paramString(c, "units", ""); name != "" {
		return name
	}
	if f, ok := paramFloat(c, "to_meter", 1); ok && f != 1 {
		return fmt.Sprintf("to_meter=%g", f)
	}
	return "m"
}

// Equals 判断两个 CRS 是否**算出同一套坐标**(结构比较)。
//
// 比较的字段:proj 名、归一化参数(含 to_meter)、椭球、+towgs84,以及会影响
// 结果的字符串参数(目前只有 geos 的 +sweep)。字符串写法差异(参数顺序、别名、
// 缺省值省略)不算差异。
//
// 与 pyproj 的 CRS.equals 的关系:**本库更宽松**。pyproj 还比 datum 名、CRS 名、
// ID 等元数据,所以 `EPSG:4326` 与 `+proj=latlong +ellps=WGS84` 在 pyproj 里是
// False(后者没有 datum 名);本库不建模 datum/名称,两者算出来的坐标完全一致,
// 因此判 True。这个口径对"要不要跳过这次变换"这类判断更合用;需要元数据级
// 相等时,请比较 `String()` 或自己带 datum 信息。
//
// 不做的事:不查 proj.db、不做基准识别。
func (c *CRS) Equals(other *CRS) bool {
	if c == nil || other == nil {
		return c == other
	}
	if c.Proj != other.Proj {
		return false
	}
	if !near(c.Ellipsoid.A, other.Ellipsoid.A) || !near(c.Ellipsoid.B, other.Ellipsoid.B) {
		return false
	}
	// 参数:缺省与显式 0 视为同一件事(各 impl 的 paramFloat 缺省就是 0)
	for k := range c.Params {
		if !nearParam(c.Params[k], paramOr(other, k)) {
			return false
		}
	}
	for k := range other.Params {
		if !nearParam(other.Params[k], paramOr(c, k)) {
			return false
		}
	}
	// geos 的 sweep 存在 UnknownParams 里,但它改变结果
	if paramString(c, "sweep", "y") != paramString(other, "sweep", "y") {
		return false
	}
	// 基准变换参数
	if len(c.Towgs84) != len(other.Towgs84) {
		return false
	}
	for i := range c.Towgs84 {
		if math.Abs(c.Towgs84[i]-other.Towgs84[i]) > 1e-12 {
			return false
		}
	}
	return true
}

// paramOr 取参数值,缺失时按该参数的**缺省值**处理(与各 impl 的 paramFloat
// 缺省一致:多数是 0,但 to_meter 的缺省是 1 —— 否则 "+units=m" 与不写单位
// 会被判成不等)。
func paramOr(c *CRS, key string) float64 {
	def := 0.0
	if key == "to_meter" {
		def = 1
	}
	if v, ok := paramFloat(c, key, def); ok {
		return v
	}
	return def
}

// nearParam 参数比较:绝对 0 附近用绝对门,其余用相对门(容双精度舍入)。
func nearParam(a, b float64) bool {
	d := math.Abs(a - b)
	if d == 0 {
		return true
	}
	scale := math.Abs(a) + math.Abs(b)
	return d <= 1e-12*scale || d <= 1e-9
}
