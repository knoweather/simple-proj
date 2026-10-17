package proj

// domain_reason.go —域外原因的携带者。Owner: AI-B。
//
// 这是 goproj 融合评审(C#14)的第二项:goproj 背后的 C PROJ 有
// proj_context_errno_string,域外时调用方能拿到"为什么出界"(地平线外/远极点/
// |Ce| 超限…);simple-proj 的 ErrDomain 是个裸 sentinel(§2.1 冻结),调用方只
// 知道出界、不知道原因。批量重采样时( DESIGN §1.1)这决定了该跳过、记簿还是报警。
//
// DomainError 携带机器可读的 Reason + 人读 Detail,并 Unwrap 到 ErrDomain:
//   - errors.Is(err, ErrDomain) 仍为 true —— proj.go 的批量 D2 语义
//     (单点域外写 NaN 继续、批末返回 ErrDomain)零影响;
//   - errors.As(err, &*DomainError) 可取原因;
//   - A 轨投影(latlong/eqc/geos)仍返回裸 sentinel(A 的文件,B 不碰),
//     Reason 对这些投影为空——故调用方永远应 errors.Is 判定,
//     Reason 只作增强,不当契约(记入 PROGRESS-B.md C#14)。
//
// Reason 取值(机器可读短码,集合冻结如下):
//   far-pole   —— 圆锥/方位投影的远端极点(φ·n ≤ 0)
//   antipode   —— 方位投影中心的对跖点(径向无穷)
//   horizon    —— (geos 视界外用例预留;当前 A 轨实现返回裸 sentinel)
//   ce-limit   —— tmerc 复球面东移 |Ce| > 2.623395162778
//   degenerate —— 球面 tmerc 的 |cosφ·sinΔλ| = 1 边
//   outside    —— 兜底(含逆变换纬度反解不收敛)

import "fmt"

// DomainError 是带 ErrDomain 语义的域外错误。构造用 domainErr。
type DomainError struct {
	Reason string // 机器可读短码(见文件头)
	Detail string // 人读细节(可含具体数值)
}

// domainErr 构造 DomainError。detail 可为空。
func domainErr(reason, detail string) *DomainError {
	return &DomainError{Reason: reason, Detail: detail}
}

// Error 实现 error;保持 ErrDomain 文案前缀,调用方 log 时语义不变。
func (e *DomainError) Error() string {
	if e == nil {
		return ErrDomain.Error()
	}
	if e.Detail == "" {
		return fmt.Sprintf("%s [%s]", ErrDomain.Error(), e.Reason)
	}
	return fmt.Sprintf("%s [%s]: %s", ErrDomain.Error(), e.Reason, e.Detail)
}

// Unwrap 让 errors.Is(err, ErrDomain) 为 true(库存 sentinel 仍是唯一判据)。
func (e *DomainError) Unwrap() error { return ErrDomain }

// —— 各投影的域外原因判别(B 轨适配层用)——
//
// 判别只能基于适配层可见的信息(度制入参 + 内核预计算常量):内核签名冻结为
// (x, y)/(lon, lat, ok),不带原因通道(§2.3),所以原因在适配层重建。

// laeaDomainReason:laea 前向 NaN 只源于两处——极冠分支的远极点
// (|bb| < EPS10)或斜轴/赤道分支中心对跖点(1+... ≤ EPS10)。
func laeaDomainReason(k *laeaKernel, lat float64) string {
	if k.mode == laeaNPole && lat < 0 {
		return "far-pole" // 北冠状的远极点是南极
	}
	if k.mode == laeaSPole && lat > 0 {
		return "far-pole" // 南冠状的远极点是北极
	}
	return "antipode"
}

// lccDomainReason:lcc 前向 NaN 只可能来自远极点(φ·n ≤ 0)。
func lccDomainReason(lat, n float64) string {
	if lat*n <= 0 {
		return "far-pole"
	}
	return "outside"
}

// tmercDomainReason:椭球路径的 NaN 只可能来自 |Ce| > 2.623395162778;
// 球面路径是 |cosφ·sinΔλ| = 1 的退化边。
func tmercDomainReason(k *tmercKernel) string {
	if k.spherical {
		return "degenerate"
	}
	return "ce-limit"
}
