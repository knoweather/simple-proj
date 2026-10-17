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
	projString    string            // 原始串,往返用
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
}

// knownNumericKeys 识别的数值参数(收纳进 CRS.Params)。
// 椭球键(a/b/f/rf/es/R)同在其中,由 ellipsoid.go 消费。
var knownNumericKeys = map[string]bool{
	"a": true, "b": true, "es": true, "f": true, "rf": true, "R": true,
	"h": true, "k_0": true, "lat_0": true, "lat_1": true, "lat_2": true,
	"lat_ts": true, "lon_0": true, "x_0": true, "y_0": true,
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
}

// knownStringKeys 识别但非数值的参数。ellps/units/proj 由 Parse 直接消费;
// sweep/datum/towgs84 与布尔旗标(no_defs 等)存 UnknownParams(B 的适配层经
// paramString/paramFloat 取用,见 proj.go / PROGRESS-A 决策 D1)。
// datum/towgs84 语义(基准变换)在 v0 明确出界(DESIGN §1.3),只保留不生效。
var knownStringKeys = map[string]bool{
	"proj": true, "ellps": true, "units": true,
	"sweep": true, "datum": true, "towgs84": true, "no_defs": true,
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
//   - 未知 +key 不报错,原样进 UnknownParams;非数值的已识别键(sweep/datum/
//     towgs84/no_defs)同样存 UnknownParams(决策 D1);
//   - 椭球键收敛进数值表由 ellipsoid.go 组合推导,发现矛盾即报错;
//   - +units 只认 "m"(只认米,DESIGN §1.1);
//   - 未知 +proj 报错。
//
// 同名键后者胜;返回的 CRS 不可变(约定),零值不入。
func Parse(s string) (*CRS, error) {
	raw := strings.TrimSpace(s)
	if raw == "" {
		return nil, fmt.Errorf("proj: 空 proj 字符串")
	}

	// EPSG 特例码 → 递归走规范 proj 串;projString 保留原码便于识别/往返。
	if code, ok := parseEPSGCode(raw); ok {
		base, found := specialCodes[code]
		if !found {
			return nil, fmt.Errorf("proj: 不支持的 EPSG:%d(v0 只收录 4326/4269/3857)", code)
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

// parseProjString 解析纯 proj 串(不做 EPSG 特例)。projString 从此处定。
func parseProjString(s string) (*CRS, error) {
	crs := &CRS{
		Params:        map[string]float64{},
		UnknownParams: map[string]string{},
	}
	crs.projString = strings.TrimSpace(s)

	ellpsName := ""

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
			crs.UnknownParams[key] = "" // no_defs / 未知旗标
			continue
		}
		switch {
		case key == "proj":
			canonical, known := knownProj[strings.ToLower(strings.TrimSpace(val))]
			if !known {
				return nil, fmt.Errorf("proj: 未知 +proj=%q(v1 冻结清单见 PROGRESS-A 决策 D4)", val)
			}
			crs.Proj = canonical
		case key == "ellps":
			ellpsName = strings.TrimSpace(val)
		case key == "units":
			if val != "m" {
				return nil, fmt.Errorf("proj: 不支持的 +units=%q(v0 只认米)", val)
			}
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
			// sweep / datum / towgs84 / 未知:字符串原样保留(决策 D1)
			crs.UnknownParams[key] = val
		}
	}
	if crs.Proj == "" {
		return nil, fmt.Errorf("proj: 缺少 +proj 名(+proj=...)")
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
	for _, key := range []string{"lat_0", "lat_ts", "lat_1", "lat_2"} {
		if v, ok := params[key]; ok && math.Abs(v) > 90 {
			return fmt.Errorf("proj: +%s=%v 出界 [-90,90]", key, v)
		}
	}
	if v, ok := params["lon_0"]; ok && math.Abs(v) > 360 {
		return fmt.Errorf("proj: +lon_0=%v 出界 [-360,360]", v)
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
	}
	// 特征比对(容忍参数缺失但语义等同)
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
