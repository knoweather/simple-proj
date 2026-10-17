package proj

import "fmt"

// Ellipsoid(A 拥有,结构冻结 §2.2)。
type Ellipsoid struct {
	A, B   float64 // 长短半轴(米);正球体 A == B
	F      float64 // 扁率
	ES     float64 // e²(第一偏心率平方)
	E      float64 // e
	Sphere bool
}

// ellpsRef 内置椭球表行(PROJ proj_ellps.cpp 同值常量;只取数值,不抄代码)。
type ellpsRef struct {
	a    float64 // 长半轴(米)
	rf   float64 // 扁率倒数(0 = 正球)
	desc string
}

// ellpsRefTable PROJ 内置椭球(PROJ 惯用小写名)。键全小写(查找前归一),
// 值为公知测量常数。
var ellpsRefTable = map[string]ellpsRef{}

// EllipsoidFromParams 由参数组合推导椭球(结构冻结 §2.2 签名)。
// 组合与优先级(DESIGN §2.2,同 PROJ 约定):
//
//	+R > +ellps=名 > +a/+b 组合 > 默认 WGS84
//
// 接受的参数键(解析进 CRS.Params 的数值): +a/+b/+rf/+f/+es/+R。
// 仅给 +a 而无短轴类参数视作正球体(PROJ 语义);+rf=0 同义正球体。
func EllipsoidFromParams(params map[string]float64, ellpsName string) (Ellipsoid, error) {
	return Ellipsoid{}, fmt.Errorf("proj: EllipsoidFromParams 骨架未实现")
}
