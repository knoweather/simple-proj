package proj

import "strings"

// String 往返输出 proj 串(COORDINATION §1:projstr.go 为 String 的归宿)。
// 原 proj 串进入的 CRS 原样返回(往返不变最稳);由 "EPSG:xxxx" 特例码进入的
// CRS 回放特例码对应的规范 proj 串(可被 Parse 再吸入,达成同义往返)。
func (c *CRS) String() string {
	if c == nil {
		return ""
	}
	if strings.HasPrefix(c.projString, "+") {
		return c.projString
	}
	if code, ok := parseEPSGCode(c.projString); ok {
		if base, found := specialCodes[code]; found {
			return base
		}
		// WGS84 UTM(326xx/327xx)同样展开成等价 proj 串
		if base, found := utmEPSGString(code); found {
			return base
		}
	}
	return c.projString
}
