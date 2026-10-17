# simple-proj 双 AI 协作规程(AI-A × AI-B)

> 本文件是分工、契约、进度的**唯一事实源**。`D:\simple-proj\DESIGN.md` 是设计背景(只读,owner=A)。
> 两个会话同时开工;铁律:**谁拥有的文件谁才能写;接口冻结后不得单方面修改**。

---

## 0. 角色一句话

| 角色 | 代号 | 管什么 | 与对方的边界 |
|---|---|---|---|
| 地基 + P0 投影 | **AI-A** | git/CI、冻结契约落地、CRS 解析、椭球、注册表、`latlong`/`eqc`/`geos` 三投影、xcheck 消费端、批量 API、README | **不写任何 impl_{stere,laea,lcc,merc,tmerc}.go**;golden JSON 由 B 生成,A 只消费 |
| 公式 + fixture | **AI-B** | `xcheck/gen_golden.py` 全量 golden 生成、`stere`/`laea`/`lcc`/`merc`/`tmerc` 五投影、共享球面数学库 geolib、公式考据笔记 | **不写 crs.go/ellipsoid.go/proj.go**;遇到契约缺口走 §5 流程 |

## 1. 文件所有权表(防冲突的终极规则)

| 文件/路径 | Owner | 另一方权限 |
|---|---|---|
| `go.mod`、`.gitignore`、`README.md`、`LICENSE` | A | 只读 |
| `crs.go`、`crs_test.go` | A | 只读(B 只能 `import` 展开的 `proj.CRS`/`proj.Parse`) |
| `ellipsoid.go`、`ellipsoid_test.go`、`gribshape.go` | A | 只读 |
| `proj.go`(Projection 接口 + register + batch 批量)| A | 只读 |
| `projstr.go`(CRS.String 往返)| A | 只读 |
| `impl_latlong.go`、`impl_eqc.go`、`impl_geos.go` + 各自 `_test.go` | A | 只读 |
| `mlfn.go`(矩面子午线弧通用件:polyval/clenshawDelta/rectifyingRadius/mlfn 系)| A(C#10 补登)| 只读(B 复用助手,B 自己的投影不落新系数表) |
| `xcheck/xcheck_test.go`(消费 golden)| A | 只读 |
| `bench.go` | A | 只读 |
| `xcheck/gen_golden.py` + `xcheck/golden_*.json` | **B**(生成器与 fixture)| A 只读消费 |
| `impl_stere.go`、`impl_laea.go`、`impl_lcc.go`、`impl_merc.go`、`impl_tmerc.go` + 各自 `_test.go` | **B** | A 只读 |
| `geolib.go`(球面/椭球公共数学:等角纬、等积纬及其逆)| **B** | A 只读(若 geos/eqc 需要,找 B 商量:抽进 geolib 还是 A 在自家文件内自写小公式,见 §5)|
| `FORMULA-NOTES.md`(每个投影的公式出处考据)| **B** | A 只读 |
| `PROGRESS-A.md` | A | B 只读 |
| `PROGRESS-B.md` | B | A 只读 |
| `DESIGN.md` | A(记账用)| B 提议走 §5 |
| `COORDINATION.md` | **双方只读**,变更走 §5 流程(需对方在 PROGRESS 里回执同意)| — |

commit 前缀 `[A]` / `[B]`;动了对方的文件 = 违反所有权规则,发现即该行回滚(人工介入点,别走到那一步)。

## 2. 冻结契约(Frozen Contract v1)

**从第 0 分钟起双方都按以下代码编码,A 的第一个 commit 必须逐字落地这些声明,之后的 diff 只加函数体、不加新符号。**

### 2.1 A 拥有的公共类型与函数(crs.go / ellipsoid.go / proj.go)

```go
package proj

import "errors"

// ErrDomain 输入超出投影定义域(Inverse 定义域外时返回 NaN 坐标配此错误,
// 调用方用 errors.Is 判定,不当循环中断)。
var ErrDomain = errors.New("proj: outside projection domain")

// CRS 投影定义。零值不可用,只能由 Parse 构造。不可变。
type CRS struct {
    Proj          string             // "geos" 等(sans "+"、lowercased)
    Params        map[string]float64 // 归一化后的数值参数(lon_0/lat_0/k_0/h/lat_1/lat_2/x_0/y_0/lat_ts...)
    Ellipsoid     Ellipsoid
    UnknownParams map[string]string  // 未识别的参数原样保留(只字符串值)
    projString    string             // 原始串,往返用
}

func Parse(s string) (*CRS, error)            // 也识别 "EPSG:4326"/"EPSG:3857" 等常用码
func (c *CRS) String() string                  // 往返输出 proj 串
func (c *CRS) NewProjection() (Projection, error)

// Projection 是投影实例。实现必须是无状态的纯函数集;对象不可变、天然线程安全。
type Projection interface {
    CRS() *CRS
    // Forward 经纬度(度) → x, y(米)。latlong 投影返回度。
    Forward(lon, lat float64) (x, y float64, err error)
    // Inverse Forward 的逆: x, y(米) → 经纬度(度)。
    // 超出定义域 → (NaN, NaN, ErrDomain)。
    Inverse(x, y float64) (lon, lat float64, err error)
}

// —— 内部注册(各 impl 文件的 init 里调用;不是导出符号)——
// register 由 A 在 proj.go 中实现。B 的 impl 文件调用之:
func register(name string, factory func(*CRS) (Projection, error))
```

### 2.2 Ellipsoid(A 拥有,结构冻结)

```go
type Ellipsoid struct {
    A, B   float64 // 长短半轴(米);正球体 A == B
    F      float64 // 扁率
    ES     float64 // e²(第一偏心率平方)
    E      float64 // e
    Sphere bool
}
func EllipsoidFromParams(params map[string]float64, ellpsName string) (Ellipsoid, error)
// GRIB shape_of_the_earth → 椭球(逻辑从 simple-grib/grid/definition.go 平移)
func GRIBEllipsoid(shape int, scale uint8, raw uint32) (Ellipsoid, error)
```

### 2.3 B 拥有的内核签名(impl 调用、纯函数、**弧度**单位)

> 所有内核一律取 `a, b` 两轴半轴(不依赖 Ellipsoid 类型,使 B 的公式与 A 的地基完全解耦);
> lat0/lon0 等一律**弧度**;度/米的换算只发生在 impl_*.go 的**适配器**(也归 B)。
> Inverse 用返回值 `(lon, lat float64, ok bool)`,`ok==false` 表示定义域外(NaN 也要写明),适配层转成 ErrDomain。

```go
// impl_stere.go(B)
func stereForward(a, b, lat0, lon0, k float64, lon, lat float64) (x, y float64)
func stereInverse(a, b, lat0, lon0, k float64, x, y float64) (lon, lat float64, ok bool)

// impl_laea.go(B)
func laeaForward(a, b, lat0, lon0 float64, lon, lat float64) (x, y float64)
func laeaInverse(a, b, lat0, lon0 float64, x, y float64) (lon, lat float64, ok bool)

// impl_lcc.go(B) —— 双标准纬线版本;单纬线(只给 lat_ts)适配层里令 lat2=lat1 处理
func lccForward(a, b, lat0, lon0, lat1, lat2, k0 float64, lon, lat float64) (x, y float64)
func lccInverse(a, b, lat0, lon0, lat1, lat2, k0 float64, x, y float64) (lon, lat float64, ok bool)

// impl_merc.go(B) —— 椭球版 Mercator(+lat_ts 用作比例纬度;球体退化 a==b)
func mercForward(a, b, latTS, lon0 float64, lon, lat float64) (x, y float64)
func mercInverse(a, b, latTS, lon0 float64, x, y float64) (lon, lat float64, ok bool)

// impl_tmerc.go(B) —— Karney/Krüger 系列;先 6 阶(Krüger),精度不够再上 Karney 扩展
func tmercForward(a, b, lat0, lon0, k0 float64, lon, lat float64) (x, y float64)
func tmercInverse(a, b, lat0, lon0, k0 float64, x, y float64) (lon, lat float64, ok bool)
```

### 2.4 golden JSON schema(B 生成,双方消费)

```json
{
  "crs":       "+proj=stere +lat_0=90 +lat_ts=60 +lon_0=-100 +ellps=WGS84",
  "generator": "pyproj 3.7.x",
  "generated": "2026-10-17T12:00:00Z",
  "forward":   [{"lon": 2.5, "lat": 45.0, "x": 123456.789, "y": 654321.123}],
  "inverse":   [{"x": 123456.789, "y": 654321.123, "lon": 2.5, "lat": 45.0}],
  "specials":  [{"desc": "outside domain", "x": 9e9, "y": -9e9, "expectNaN": true}]
}
```

文件名(共 9 个):`golden_latlong.json`、`golden_eqc.json`、`golden_geos_x.json`、`golden_geos_y.json`、`golden_stere.json`、`golden_laea.json`、`golden_lcc.json`、`golden_merc.json`、`golden_tmerc.json`。

### 2.5 数值规范(有争议时的裁判标准)

| 项 | 标准 |
|---|---|
| API 单位 | 经纬:**度**;x/y:**米**(latlong 投影除外返回度)|
| 内核单位 | 一律**弧度**(B 内部),适配层换算 |
| 经度归一化输出 | `(lon+180) mod 360 - 180 → [-180, 180)`,极点输出恒 ±90 而非 ±90.0000001;|
| Forward 误差 | 黄金对拍 ≤ **1e-6 m**(绝对)|
| Inverse 误差 | 黄金对拍 ≤ **1e-9 度**(绝对)|
| 往返(Forward∘Inverse) | ≤ **1e-9 度** |
| 定义域外 | Inverse → (NaN, NaN, ok=false);Forward 输入非法(经度>360 或纬度>90)→ 返回 error 且坐标无意义(具体 error 类型自定,不冒充 ErrDomain)|
| 线程安全 | 所有 Projection 无可变字段 |
| panics | 禁止;一切失败走 error|

## 3. 日程(4 天推进,离散合流点双方对齐)

> 前提:A 第一步就是 git init + 建分支。每个 `→合流` 都是纪律点:**必须 `go build ./... && go test ./...` 绿才提交;两轨从 day0 起并行,各提交各的分支**。

### Day 0
- **A 上午(必须最先)**:`git init`,`main` 打空 commit;建 `ai-a` / `ai-b` 分支;**契约首提交**(把 §2 代码逐字嵌入真实文件:`proj.go`/`crs.go`/`ellipsoid.go` 骨架,函数体可暂 `return zero, nil, nil`);`.gitignore`;`PROGRESS-A.md` 建档。
- **B 上午**:读 DESIGN + COORDINATION;写 `xcheck/gen_golden.py`(案例清单见 DESIGN §3.1:9 个 CRS × 33×33 网格 + specials,含 geos sweep x/y、球体椭球、北/南极 stere、地平线外 NaN);跑通生成 9 个 JSON(D:\satpy 的 python 环境;缺 pyproj 就 `pip install pyproj`);fixture 提交;`FORMULA-NOTES.md` 建档(每投影列 Snyder 章节 + PROJ 源码文件)。
- **A 下午**:`ellipsoid.go` 完整实现(椭球表 + `+a/+b/+rf/+f/+es/+R` 组合推导 + `gribshape.go` 抄 simple-grib 的 earthDimension)+ 单测。
- **B 下午**:`geolib.go`(等角纬/等积纬及其逆)+ `stere` 内核 + `impl_stere.go` 适配器(register 样板)+ `stere_test.go` 直读 golden 断言(不等 A 的 xcheck 框架,自己写 100 行加载器)。**→合流**

### Day 1
- **A**:`crs.go` 的 `Parse` 全量(参数拼写、默认值、`+proj` 校验、EPSG 4326/3857/4269 特例)+ `projstr.go` 往返测试 + `impl_latlong`(注册 "latlong")。**→合流**
- **B**:`laea` 内核 + 适配 + 单测(green);`lcc` 开工。

### Day 2
- **A**:`impl_eqc`(注意 PROJ 的 eqc 遇椭球要转 **authalic 球半径** R2)→ 单测;`xcheck/xcheck_test.go` 消费端框架(读 golden_*.json,跑 A 的三投影 latlong/eqc/geos_x/geos_y 断言)。
- **B**:`lcc` 完成 + `merc` 内核 + 适配;各自 golden green;卫生检查 B 侧全部投影与 A 的 `register` 通过(临时 `go test ./impl_*` 小范围)。**→合流**
- B 交叉 review A 的 `impl_eqc.go`(authalic 半径公式是常见坑)。

### Day 3
- **A**:`impl_geos`(sweep x / y 双模式;+h 换算按 PROJ 语义:卫星到椭球面高度)→ 两个 golden 都 green;`batch.go` ForwardBatch/InverseBatch + `bench.go`(百万点 <100 ms 目标);README(含 simple-grib `grid.Definition → proj.CRS` 桥接文档)。
- **B**:`tmerc`(Krüger 6 阶)+ golden green;交叉 review A 的 `impl_geos.go`(sweep、地平线、NaN 场景)。**→合流**

### Day 4(合流与发布)
- 双负责人全跑:`go test ./... -race`、`go vet ./...`、四次交叉编译(GOOS×GOARCH: linux/windows/darwin × amd64/arm64 选 4 组)、coverage ≥85%(不含 xcheck/testdata);
- 双方各自 merge 回 `main`(文件零冲突是本规程的硬保证 → 契约没改 = merge 零冲突);
- A 打 tag `v0.1.0-projsubset`;两个 PROGRESS 各写 HANDOVER 节(下个会话可接续)。

## 4. 质量 Gate(merge 前双方各自自查)

- [ ] `go test ./... -race` 全绿(自己分支上)
- [ ] 自己文件的覆盖率(`go test -cover ./...` 查看)有意识达标 ≥ 85%
- [ ] 交叉编译 4 组:GOOS/GOARCH 任意 4 组通过(纯 Go 无 cgo)
- [ ] no vet warning
- [ ] 提交在每个 commit 都带上对应 [A]/[B] 前缀,信息可被对方进度文件索引
- [ ] golden 对拍:自己拥有的投影 9 个中自己负责的 N 个全绿(数值口径 §2.5)
- [ ] 禁 cgo、禁第三方依赖(除 stdlib;`go.mod` 归 A,若协作中误 `go get` 请立即告知 A 还原)

## 5. 跨轨问题处理(改契约的唯一通路)

发现问题(接口缺方法、Ellipsoid 数据缺字段等)→ **不许私下改对方文件**。流程:

1. 在自己的 `PROGRESS-A/B.md` 记一条 `C#序号:` 描述问题 + 希望 §2 契约怎么改;
2. 对方在其 PROGRESS 加 `C#x-ACK` 回执(同意/协商);
3. 双方同意后,契约修改由**契约 owner(A)**统一落地,受影响的双方 impl 文件各自同步改自家适配层(谁消费谁改);
4. 重大分歧(几何公式理解不一致)→ 双方各派自己的解释坐标进 golden 用例(往 `xcheck/gen_golden.py` 加 case),再对拍仲裁——**以 pyproj 输出为准**,谁的解释与 pyproj 偏差大谁改。

> C 前缀为本 repo 内独立编号,起始 C1(不复用 simple-grib 的 M 编号)。

## 6. 禁区(红线)

1. 禁 cgo、禁第三方依赖(go.mod 的 require 段只允许 stdlib;误 `go get` 手工还原并告知对方);
2. 禁 copy PROJ/satpy/pyresample **源码**(GPL 许可问题),只允许参考公式(Snyder 是 public domain,PROJ 源码思路可借鉴但代码须重写);
3. 禁改冻结契约符号(命名、签名、结构字段);禁改对方文件;
4. 禁在本 repo 里 import `simple-grib`/`simple-netcdf`(反向依赖易成环;GRIB 映射在 simple-sat 的 geom 层做);
5. 双方都不得直接动 `main`,合并只发生在 Day4(或人工批准的急件)。

---

## 7. 两个启动提示词(开新会话直接粘贴一侧)

**给 AI-A 的会话:**
> 工作目录 D:\simple-proj。先读 COORDINATION.md(role:AI-A)和 DESIGN.md。严格执行:只写 §1 表中 A 拥有的文件;第一个动作是 Day0 上午的 git init + 契约首提交(§2 逐字落地);之后按 §3 A 轨推进,每个完成点更新 PROGRESS-A.md。遇到跨轨问题走 §5。

**给 AI-B 的会话:**
> 工作目录 D:\simple-proj。先读 COORDINATION.md(role:AI-B)和 DESIGN.md。严格执行:只写 §1 表中 B 拥有的文件;第一个动作是 §3 Day0 的 `xcheck/gen_golden.py` + 9 个 fixture 提交;若 `proj.go` 契约尚未落地,先写内核纯函数 + 自带 golden 断言测试,契约一到即切 register 适配;按 §3 B 轨推进,每个完成点更新 PROGRESS-B.md。跨轨问题走 §5。

> 注:两会话若同一目录并发,工具层面可能有文件锁竞争——建议 B 在 A 完成 git init 之前只跑 `xcheck/` 的 Python 侧(fixture 生成不依赖 Go 编译),git 就绪后再提交 fixture。
