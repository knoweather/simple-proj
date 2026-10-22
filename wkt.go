package proj

// wkt.go — WKT2 / WKT1 子集解析(把 CRS 描述翻译成等价的 proj 串再交给 Parse)。
//
// 为什么走「翻译成 proj 串」而不是直接填 CRS:proj 串那条路已经包含了全部
// 参数归一化、椭球组合校验、EPSG 特征识别与单位换算层,翻译后复用可以保证
// 「WKT 进来的 CRS」与「手写 proj 串进来的 CRS」行为逐位一致;String() 也
// 自然回落到 proj 串(本库不输出 WKT)。
//
// 支持范围(与 impl 清单对齐):
//
//	根节点  PROJCRS / PROJECTEDCRS / PROJCS(投影)、GEOGCRS / GEODCRS /
//	        GEOGCS(地理)、BOUNDCRS / COMPOUNDCRS 明确拒收(要基准变换/垂直分量)
//	椭球    ELLIPSOID(SPHEROID)/ DATUM / ENSEMBLE / BASEGEOGCRS 里递归找
//	方法    见 wktMethods(9 个 proj 全覆盖);其余方法报错并指出方法名
//	参数    见 wktParamKeys;角度按 ANGLEUNIT 归到度,长度按 LENGTHUNIT 归到米
//	单位    CS 的 LENGTHUNIT → +to_meter(非米时),ANGLEUNIT 只认度/弧度
//	ID      ID["EPSG",n] 命中 4326/4269/3857/900913/326xx/327xx 时直接走 EPSG 码,
//	        保证与手写 EPSG 码完全同一条路径
//
// 不做:WKT 输出、PROJJSON、ESRI WKT 的方言差异(如 ESRI 把 lat_0 写成
// Latitude_Of_Origin 的拼写已覆盖,但 ESRI 的 false origin 语义差异不保证)。

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ---- 词法 / 语法 -----------------------------------------------------------

// wktNode WKT 的一个节点:KEYWORD[arg, arg, ...];叶子可能是字符串、数字或裸标识符。
type wktNode struct {
	keyword string
	args    []wktNode
	isStr   bool
	str     string
	isNum   bool
	num     float64
}

// wktParser 极简递归下降:WKT 没有运算符,只有「关键字[列表]」与三种叶子。
type wktParser struct {
	s string
	i int
}

func wktParse(s string) (wktNode, error) {
	p := &wktParser{s: s}
	n, err := p.value()
	if err != nil {
		return wktNode{}, err
	}
	p.skipSpace()
	if p.i != len(p.s) {
		return wktNode{}, fmt.Errorf("proj(wkt): 第 %d 字节处有多余内容 %q", p.i, wktSnippet(p.s, p.i))
	}
	return n, nil
}

func wktSnippet(s string, i int) string {
	end := i + 20
	if end > len(s) {
		end = len(s)
	}
	return s[i:end]
}

func (p *wktParser) skipSpace() {
	for p.i < len(p.s) {
		switch p.s[p.i] {
		case ' ', '\t', '\r', '\n':
			p.i++
		default:
			return
		}
	}
}

// value 解析一个节点(关键字/字符串/数字)。
func (p *wktParser) value() (wktNode, error) {
	p.skipSpace()
	if p.i >= len(p.s) {
		return wktNode{}, fmt.Errorf("proj(wkt): 意外结束")
	}
	c := p.s[p.i]
	if c == '"' {
		return p.quoted()
	}
	if c == '+' || c == '-' || c == '.' || (c >= '0' && c <= '9') {
		return p.number()
	}
	return p.keyword()
}

// quoted 解析 "..."(WKT2 用 "" 转义内嵌引号)。
func (p *wktParser) quoted() (wktNode, error) {
	p.i++ // 跳过开引号
	var b strings.Builder
	for p.i < len(p.s) {
		if p.s[p.i] == '"' {
			if p.i+1 < len(p.s) && p.s[p.i+1] == '"' {
				b.WriteByte('"')
				p.i += 2
				continue
			}
			p.i++
			return wktNode{isStr: true, str: b.String()}, nil
		}
		b.WriteByte(p.s[p.i])
		p.i++
	}
	return wktNode{}, fmt.Errorf("proj(wkt): 字符串未闭合")
}

// number 解析数字(含指数形式)。
func (p *wktParser) number() (wktNode, error) {
	start := p.i
	for p.i < len(p.s) {
		c := p.s[p.i]
		if (c >= '0' && c <= '9') || c == '+' || c == '-' || c == '.' || c == 'e' || c == 'E' {
			p.i++
			continue
		}
		break
	}
	v, err := strconv.ParseFloat(p.s[start:p.i], 64)
	if err != nil {
		return wktNode{}, fmt.Errorf("proj(wkt): 数字 %q 非法", p.s[start:p.i])
	}
	return wktNode{isNum: true, num: v}, nil
}

// keyword 解析标识符,可能带 [ ... ] 子节点。
func (p *wktParser) keyword() (wktNode, error) {
	start := p.i
	for p.i < len(p.s) {
		c := p.s[p.i]
		if c == '[' || c == ']' || c == ',' || c == '(' || c == ')' || c == '"' ||
			c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			break
		}
		p.i++
	}
	word := p.s[start:p.i]
	if word == "" {
		return wktNode{}, fmt.Errorf("proj(wkt): 第 %d 字节处缺关键字", start)
	}
	p.skipSpace()
	// WKT1 也允许括号形式:PROJCS(...)
	if p.i < len(p.s) && (p.s[p.i] == '[' || p.s[p.i] == '(') {
		open := p.s[p.i]
		closeCh := byte(']')
		if open == '(' {
			closeCh = ')'
		}
		p.i++
		n := wktNode{keyword: word}
		for {
			p.skipSpace()
			if p.i >= len(p.s) {
				return wktNode{}, fmt.Errorf("proj(wkt): %s 的括号未闭合", word)
			}
			if p.s[p.i] == closeCh {
				p.i++
				return n, nil
			}
			child, err := p.value()
			if err != nil {
				return wktNode{}, err
			}
			n.args = append(n.args, child)
			p.skipSpace()
			if p.i < len(p.s) && p.s[p.i] == ',' {
				p.i++
			}
		}
	}
	return wktNode{keyword: word}, nil
}

// ---- 节点查询 -------------------------------------------------------------

// wktFind 深度优先找第一个 keyword 匹配的节点(大小写不敏感)。
func wktFind(n wktNode, keyword string) (wktNode, bool) {
	if strings.EqualFold(n.keyword, keyword) {
		return n, true
	}
	for _, a := range n.args {
		if got, ok := wktFind(a, keyword); ok {
			return got, true
		}
	}
	return wktNode{}, false
}

// wktDirect 只看直接子节点。
func wktDirect(n wktNode, keyword string) (wktNode, bool) {
	for _, a := range n.args {
		if strings.EqualFold(a.keyword, keyword) {
			return a, true
		}
	}
	return wktNode{}, false
}

// wktDirectAll 取出全部直接子节点里匹配的那些(如多个 PARAMETER)。
func wktDirectAll(n wktNode, keyword string) []wktNode {
	var out []wktNode
	for _, a := range n.args {
		if strings.EqualFold(a.keyword, keyword) {
			out = append(out, a)
		}
	}
	return out
}

// wktFirstString 取节点第一个字符串参数(如 METHOD["Transverse Mercator"])。
func wktFirstString(n wktNode) (string, bool) {
	for _, a := range n.args {
		if a.isStr {
			return a.str, true
		}
	}
	return "", false
}

// wktFirstNumber 取节点第一个数字参数(如 ID["EPSG",32650])。
func wktFirstNumber(n wktNode) (float64, bool) {
	for _, a := range n.args {
		if a.isNum {
			return a.num, true
		}
	}
	return 0, false
}

// ---- 方法 / 参数映射表 -----------------------------------------------------

// wktMethod 一种 WKT 投影方法到本库 proj 名的映射;sweep 仅 geos 用。
type wktMethod struct {
	proj  string
	sweep string
	// sphere 强制球体(Popular Visualisation Pseudo Mercator 用 a 当 R)
	sphere bool
	// wkt1Polar WKT1 的 "Polar_Stereographic" 把 latitude_of_origin 写成标准纬线
	// (WKT2 用 Latitude of standard parallel 表达同一件事),要按 lat_ts 收。
	wkt1Polar bool
}

// wktMethods 键为归一化方法名(小写、下划线/连字符→空格、压缩空格)。
var wktMethods = map[string]wktMethod{
	"transverse mercator":                   {proj: "tmerc"},
	"lambert conic conformal 2sp":           {proj: "lcc"},
	"lambert conic conformal 1sp":           {proj: "lcc"},
	"lambert conformal conic":               {proj: "lcc"},
	"lambert conformal conic 1sp":           {proj: "lcc"},
	"lambert conformal conic 2sp":           {proj: "lcc"},
	"mercator variant a":                    {proj: "merc"},
	"mercator variant b":                    {proj: "merc"},
	"mercator 1sp":                          {proj: "merc"},
	"mercator 2sp":                          {proj: "merc"},
	"mercator":                              {proj: "merc"},
	"popular visualisation pseudo mercator": {proj: "merc", sphere: true},
	"lambert azimuthal equal area":          {proj: "laea"},
	"polar stereographic variant a":         {proj: "stere"},
	"polar stereographic variant b":         {proj: "stere"},
	"polar stereographic":                   {proj: "stere", wkt1Polar: true},
	"stereographic":                         {proj: "stere"},
	"geostationary satellite sweep y":       {proj: "geos", sweep: "y"},
	"geostationary satellite sweep x":       {proj: "geos", sweep: "x"},
	"geostationary satellite view":          {proj: "geos", sweep: "y"},
	"geostationary satellite":               {proj: "geos", sweep: "y"},
	"equidistant cylindrical":               {proj: "eqc"},
	"equirectangular":                       {proj: "eqc"},
	"azimuthal equidistant":                 {proj: "aeqd"},
}

// wktParamKeys WKT 参数名(归一化)→ 本库参数键。
var wktParamKeys = map[string]string{
	"latitude of natural origin":           "lat_0",
	"longitude of natural origin":          "lon_0",
	"latitude of false origin":             "lat_0",
	"longitude of false origin":            "lon_0",
	"latitude of projection centre":        "lat_0",
	"longitude of projection centre":       "lon_0",
	"latitude of projection center":        "lat_0",
	"longitude of projection center":       "lon_0",
	"latitude of 1st standard parallel":    "lat_1",
	"latitude of first standard parallel":  "lat_1",
	"latitude of 2nd standard parallel":    "lat_2",
	"latitude of second standard parallel": "lat_2",
	"latitude of standard parallel":        "lat_ts",
	"standard parallel 1":                  "lat_1",
	"standard parallel 2":                  "lat_2",
	"central meridian":                     "lon_0",
	"latitude of origin":                   "lat_0",
	"latitude of center":                   "lat_0",
	"longitude of center":                  "lon_0",
	"longitude of origin":                  "lon_0",
	"scale factor":                         "k_0",
	"scale factor at natural origin":       "k_0",
	"scale factor on initial line":         "k_0",
	"false easting":                        "x_0",
	"false northing":                       "y_0",
	"easting at false origin":              "x_0",
	"northing at false origin":             "y_0",
	"satellite height":                     "h",
}

// wktParamOverride 同名参数在不同方法下归属不同的少数情况
// (EPSG 的「Latitude of 1st standard parallel」对 eqc 是 lat_ts;GDAL WKT1 的
// 「standard_parallel_1」对 stere 是 lat_ts)。键为 proj 名 + 归一化参数名。
var wktParamOverride = map[string]map[string]string{
	"eqc": {
		"latitude of 1st standard parallel":   "lat_ts",
		"latitude of first standard parallel": "lat_ts",
		"standard parallel 1":                 "lat_ts",
	},
	"stere": {
		"standard parallel 1":               "lat_ts",
		"latitude of 1st standard parallel": "lat_ts",
	},
}

// wktNorm 归一化名称:小写、下划线/连字符→空格、压缩多余空格。
func wktNorm(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	// 括号也当分隔符:"Polar Stereographic (variant B)" → "... variant b"
	s = strings.NewReplacer("_", " ", "-", " ", "(", " ", ")", " ").Replace(s)
	return strings.Join(strings.Fields(s), " ")
}

// wktIsAngularKey 该参数是否是角度(要按 ANGLEUNIT 归到度)。
func wktIsAngularKey(key string) bool {
	switch key {
	case "lat_0", "lat_1", "lat_2", "lat_ts", "lon_0":
		return true
	}
	return false
}

// ---- 主流程 ---------------------------------------------------------------

// ParseWKT 解析 WKT2(PROJCRS/GEOGCRS/GEODCRS)或 WKT1(PROJCS/GEOGCS)子集,
// 返回与等价 proj 串完全同构的 CRS。不支持的根节点/方法/参数会明确报错。
func ParseWKT(s string) (*CRS, error) {
	root, err := wktParse(s)
	if err != nil {
		return nil, err
	}
	switch strings.ToUpper(root.keyword) {
	case "BOUNDCRS", "COMPOUNDCRS", "VERTCRS", "VERTICALCRS", "ENGCRS",
		"ENGINEERINGCRS", "TIMECRS", "PARAMETRICCRS", "DERIVEDPROJCRS":
		return nil, fmt.Errorf("proj(wkt): 不支持 %s(需要基准变换/垂直分量/派生链,本库只做纯投影)", root.keyword)
	case "PROJCRS", "PROJECTEDCRS", "PROJCS":
		return crsFromWKTProj(root)
	case "GEOGCRS", "GEODCRS", "GEOGCS", "GEOGRAPHICCRS", "GEODETICCRS":
		return crsFromWKTGeog(root)
	}
	return nil, fmt.Errorf("proj(wkt): 不认识的根节点 %q(支持 PROJCRS/GEOGCRS/GEODCRS/PROJCS/GEOGCS)", root.keyword)
}

// wktEPSGFastPath ID["EPSG",n] 命中本库特例码时直接走 EPSG 路径,
// 保证 WKT 与手写码完全一致(尤其是 3857 的伪墨卡托与 UTM 的派生参数)。
func wktEPSGFastPath(root wktNode) (*CRS, bool) {
	id, ok := wktDirect(root, "ID")
	if !ok {
		id, ok = wktDirect(root, "AUTHORITY")
	}
	if !ok {
		return nil, false
	}
	auth, ok := wktFirstString(id)
	if !ok || !strings.EqualFold(auth, "EPSG") {
		return nil, false
	}
	code, ok := wktFirstNumber(id)
	if !ok {
		// WKT1 的 AUTHORITY["EPSG","3857"] 把码写成字符串
		var numStr string
		for _, a := range id.args {
			if a.isStr && !strings.EqualFold(a.str, "EPSG") {
				numStr = a.str
			}
		}
		n, err := strconv.Atoi(strings.TrimSpace(numStr))
		if err != nil {
			return nil, false
		}
		code = float64(n)
	}
	c := int(code)
	if _, found := specialCodes[c]; !found {
		if _, found := utmEPSGString(c); !found {
			return nil, false
		}
	}
	crs, err := Parse(fmt.Sprintf("EPSG:%d", c))
	if err != nil {
		return nil, false
	}
	return crs, true
}

// wktEllipsoid 在节点子树里找 ELLIPSOID/SPHEROID → (a, rf, 找到否)。
func wktEllipsoid(root wktNode) (a, rf float64, ok bool) {
	n, found := wktFind(root, "ELLIPSOID")
	if !found {
		n, found = wktFind(root, "SPHEROID")
	}
	if !found {
		return 0, 0, false
	}
	var nums []float64
	for _, arg := range n.args {
		if arg.isNum {
			nums = append(nums, arg.num)
		}
	}
	if len(nums) < 2 {
		return 0, 0, false
	}
	return nums[0], nums[1], true
}

// wktEllipsoidTokens 生成 proj 串里的椭球片段。rf=0 → 正球。
func wktEllipsoidTokens(a, rf float64) string {
	if rf == 0 {
		return "+a=" + wktNum(a) + " +b=" + wktNum(a)
	}
	return "+a=" + wktNum(a) + " +rf=" + wktNum(rf)
}

// wktNum 输出不丢精度的数字(proj 串里要能原样 Parse 回来)。
func wktNum(v float64) string {
	return strconv.FormatFloat(v, 'g', 17, 64)
}

// wktLinearUnit 找坐标系的长度单位,返回每单位多少米。
//
// 坑:WKT1 的 PROJCS 里有两个 UNIT —— 内层 GEOGCS 的角单位(度)和外层
// PROJCS 的线单位(米)。全树搜索会先撞上角单位(0.01745),所以 WKT1 分支
// 只认**直接子节点**的 UNIT。
func wktLinearUnit(root wktNode) (toMeter float64, name string) {
	// WKT2:长度单位挂在 AXIS 上(CS[Cartesian,2] 只声明维度)。
	// 不能在整棵树里搜 —— PARAMETER 自己也带 LENGTHUNIT["metre",1],
	// 深度优先会先撞上它,把 km/us-ft 的 CRS 当成米制。
	for _, ax := range wktDirectAll(root, "AXIS") {
		if u, ok := wktFind(ax, "LENGTHUNIT"); ok {
			if f, has := wktFirstNumber(u); has && f > 0 {
				nm, _ := wktFirstString(u)
				return f, nm
			}
		}
	}
	// 少数方言把单位直接放在 CS 下
	if cs, ok := wktFind(root, "CS"); ok {
		if u, ok := wktFind(cs, "LENGTHUNIT"); ok {
			if f, has := wktFirstNumber(u); has && f > 0 {
				nm, _ := wktFirstString(u)
				return f, nm
			}
		}
	}
	// WKT1:线单位是 PROJCS 的直接子节点
	if u, ok := wktDirect(root, "UNIT"); ok {
		if f, has := wktFirstNumber(u); has && f > 0 {
			nm, _ := wktFirstString(u)
			return f, nm
		}
	}
	return 1, "metre"
}

// wktAngularDegrees 角度参数换算到度:WKT2 看 ANGLEUNIT,WKT1 看 BASEGEOGCRS/
// GEOGCS 下的 UNIT;非度单位一律报错(本库输出固定为度)。
func wktAngularDegrees(root wktNode) (float64, error) {
	if u, ok := wktFind(root, "ANGLEUNIT"); ok {
		return wktAngularUnitToDeg(u)
	}
	for _, kw := range []string{"BASEGEOGCRS", "BASEGEODCRS", "GEOGCS"} {
		base, ok := wktDirect(root, kw)
		if !ok {
			continue
		}
		u, ok := wktDirect(base, "UNIT")
		if !ok {
			continue
		}
		if f, has := wktFirstNumber(u); has {
			deg := f * 180 / math.Pi
			if math.Abs(deg-1) > 1e-9 {
				return 0, fmt.Errorf("proj(wkt): 角度单位为 %g 度/单位,本库输出固定为度,不支持", deg)
			}
		}
	}
	return 1, nil
}

// wktAngularUnitToDeg 角度单位 → 每单位多少度。
//
// 度必须走名字短路:WKT 里的系数是 0.0174532925199433(π/180 的 17 位舍入),
// 乘 180/π 得 1.0000000000000002,会把 lon_0=180 变成 180.00000000000003,
// 于是 Δλ 越过 ±180 边界、eqc 的 x 直接翻号。
func wktAngularUnitToDeg(u wktNode) (float64, error) {
	if name, ok := wktFirstString(u); ok {
		n := strings.ToLower(strings.TrimSpace(name))
		if n == "degree" || n == "degrees" || n == "degree minute second" {
			return 1, nil
		}
	}
	f, ok := wktFirstNumber(u)
	if !ok || f <= 0 {
		return 0, fmt.Errorf("proj(wkt): ANGLEUNIT 缺换算系数")
	}
	// ANGLEUNIT 的第二参数是「每单位多少弧度」
	deg := f * 180 / math.Pi
	if math.Abs(deg-1) <= 1e-12 {
		return 1, nil
	}
	return 0, fmt.Errorf("proj(wkt): 角度单位换算为 %g 度/单位,本库输出固定为度,不支持", deg)
}

// crsFromWKTGeog GEOGCRS/GEOGCS → +proj=latlong。
func crsFromWKTGeog(root wktNode) (*CRS, error) {
	if crs, ok := wktEPSGFastPath(root); ok {
		return crs, nil
	}
	a, rf, ok := wktEllipsoid(root)
	if !ok {
		return nil, fmt.Errorf("proj(wkt): 地理 CRS 里找不到 ELLIPSOID/SPHEROID")
	}
	// 角度单位只支持度(本库 latlong 输出固定为度)
	if _, err := wktAngularDegrees(root); err != nil {
		return nil, err
	}
	return Parse("+proj=latlong " + wktEllipsoidTokens(a, rf))
}

// crsFromWKTProj PROJCRS/PROJCS → 等价 proj 串。
func crsFromWKTProj(root wktNode) (*CRS, error) {
	if crs, ok := wktEPSGFastPath(root); ok {
		return crs, nil
	}
	// 1) 椭球:优先从 BASEGEOGCRS 里取,退回全树搜索
	a, rf, ok := wktEllipsoid(root)
	if !ok {
		return nil, fmt.Errorf("proj(wkt): 投影 CRS 里找不到 ELLIPSOID/SPHEROID")
	}
	// 2) 方法:WKT2 在 CONVERSION/METHOD,WKT1 在 PROJECTION
	methodNode, found := wktFind(root, "METHOD")
	if !found {
		methodNode, found = wktFind(root, "PROJECTION")
	}
	if !found {
		return nil, fmt.Errorf("proj(wkt): 找不到 METHOD/PROJECTION(不是投影 CRS?)")
	}
	methodName, ok := wktFirstString(methodNode)
	if !ok {
		return nil, fmt.Errorf("proj(wkt): METHOD 没有名字")
	}
	m, known := wktMethods[wktNorm(methodName)]
	if !known {
		return nil, fmt.Errorf("proj(wkt): 不支持的投影方法 %q(本库 9 个 proj 的 WKT 映射见 wktMethods)", methodName)
	}

	// 3) 长度单位(WKT2 在 CS 里,WKT1 在 PROJCS 下)
	toMeter, _ := wktLinearUnit(root)

	// 4) 参数:角度归到度,长度归到米
	params := map[string]float64{}
	angularDeg, err := wktAngularDegrees(root)
	if err != nil {
		return nil, err
	}
	linearFactor := toMeter
	for _, pn := range wktFindAllParams(root) {
		pname, ok := wktFirstString(pn)
		if !ok {
			continue
		}
		norm := wktNorm(pname)
		if m.wkt1Polar && norm == "latitude of origin" {
			norm = "latitude of standard parallel" // → lat_ts
		}
		key, known := wktParamKeys[norm]
		if ov, hasOv := wktParamOverride[m.proj][norm]; hasOv {
			key, known = ov, true
		}
		if !known {
			return nil, fmt.Errorf("proj(wkt): 不认识的投影参数 %q(见 wktParamKeys)", pname)
		}
		val, ok := wktFirstNumber(pn)
		if !ok {
			return nil, fmt.Errorf("proj(wkt): 参数 %q 没有数值", pname)
		}
		if wktIsAngularKey(key) {
			params[key] = val * angularDeg
		} else if key == "k_0" {
			params[key] = val
		} else {
			// 长度参数(WKT 里用 CS 单位表达,本库内核按米)
			params[key] = val * linearFactor
		}
	}

	// 5) Polar Stereographic 变体 B:WKT 只给标准纬线,极点在 ±90(EPSG 语义)。
	//    没有这一步会退化成 lat_0=0 的斜轴 stere,坐标差一个量级。
	if m.proj == "stere" {
		if latTs, hasTs := params["lat_ts"]; hasTs {
			if _, hasLat0 := params["lat_0"]; !hasLat0 {
				if latTs < 0 {
					params["lat_0"] = -90
				} else {
					params["lat_0"] = 90
				}
			}
		}
	}

	// 6) LCC 1SP:只有 natural origin 没有标准纬线 → 切锥 lat_1 = lat_0
	if m.proj == "lcc" {
		if _, has1 := params["lat_1"]; !has1 {
			params["lat_1"] = params["lat_0"]
		}
	}

	// 7) 拼 proj 串
	var b strings.Builder
	b.WriteString("+proj=" + m.proj)
	for _, k := range []string{"lat_0", "lon_0", "lat_1", "lat_2", "lat_ts", "k_0", "x_0", "y_0", "h"} {
		if v, has := params[k]; has {
			b.WriteString(" +" + k + "=" + wktNum(v))
		}
	}
	if m.sweep != "" {
		b.WriteString(" +sweep=" + m.sweep)
	}
	if m.sphere {
		b.WriteString(" +a=" + wktNum(a) + " +b=" + wktNum(a))
	} else {
		b.WriteString(" " + wktEllipsoidTokens(a, rf))
	}
	if math.Abs(toMeter-1) > 1e-15 {
		b.WriteString(" +to_meter=" + wktNum(toMeter))
	} else {
		b.WriteString(" +units=m")
	}
	return Parse(b.String())
}

// wktFindAllParams 收集所有 PARAMETER 节点(深度优先,保持出现顺序)。
func wktFindAllParams(root wktNode) []wktNode {
	var out []wktNode
	var walk func(wktNode)
	walk = func(n wktNode) {
		if strings.EqualFold(n.keyword, "PARAMETER") {
			out = append(out, n)
			return
		}
		for _, a := range n.args {
			walk(a)
		}
	}
	walk(root)
	return out
}

// looksLikeWKT 判断一个输入串是否是 WKT(供 Parse 自动分派)。
func looksLikeWKT(s string) bool {
	t := strings.TrimSpace(s)
	i := strings.IndexByte(t, '[')
	j := strings.IndexByte(t, '(')
	switch {
	case i < 0 && j < 0:
		return false
	case i < 0:
		i = j
	case j >= 0 && j < i:
		i = j
	}
	head := strings.ToUpper(strings.TrimSpace(t[:i]))
	switch head {
	case "PROJCRS", "PROJECTEDCRS", "PROJCS", "GEOGCRS", "GEODCRS", "GEOGCS",
		"GEOGRAPHICCRS", "GEODETICCRS", "BOUNDCRS", "COMPOUNDCRS":
		return true
	}
	return false
}
