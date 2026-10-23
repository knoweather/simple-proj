# simple-proj

[English](README.md) | **简体中文**

[![CI](https://github.com/knoweather/simple-proj/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/knoweather/simple-proj/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/knoweather/simple-proj.svg)](https://pkg.go.dev/github.com/knoweather/simple-proj)
[![Go version](https://img.shields.io/github/go-mod/go-version/knoweather/simple-proj)](go.mod)
[![License](https://img.shields.io/github/license/knoweather/simple-proj)](LICENSE)
[![Latest tag](https://img.shields.io/github/v/tag/knoweather/simple-proj?sort=semver)](https://github.com/knoweather/simple-proj/tags)

面向气象卫星与 GRIB 网格的纯 Go 投影库：无 cgo、零第三方依赖，只用标准库。

`simple-proj` 实现了 GRIB 与卫星产品实际用到的那部分 PROJ —— 经纬、墨卡托、
兰勃特等角圆锥、极射赤面、静止卫星视图、UTM、旋转极点网格 —— 外加 WKT 输入与
CRS→CRS 转换。每个提交都用 **pyproj 3.8.0 / PROJ 9.8.1** 做数值核对。

**状态：** v0.2.0 · Go 1.27+ · 11 个投影 · 语句覆盖率 98.3%

## 为什么不用 pyproj / PROJ 的 C 库？

- **无 cgo。** 纯 Go 构建可以任意交叉编译、静态链接；不需要 PROJ 动态库、C 工具链
  或运行时数据目录。
- **到处能跑。** 无 cgo，交叉编译简单；库与完整测试套件也能以 **js/wasm**
  构建并运行（已在 Node 下验证）。
- **为网格而生。** 批量 API 直接吃 packed 坐标，域外点逐点标记，GRIB
  `shape_of_the_earth` → 椭球是内置能力。
- **对拍而非近似。** 每个投影都与 pyproj 在两套独立点集上逐点比较：33×33 的
  golden 网格 + 22 例边界/随机 sweep。参照数据随仓库提交，`go test` 不需要 Python。

## 安装

```bash
go get github.com/knoweather/simple-proj
```

```go
import proj "github.com/knoweather/simple-proj"
```

## 快速上手

```go
crs, err := proj.Parse("+proj=merc +a=6378137 +b=6378137 +units=km")
if err != nil {
    // 定义串有误
}
p, err := crs.NewProjection()   // 构建一次，之后反复使用
if err != nil {
    // 投影未实现或参数非法
}

x, y, err := p.Forward(10, 0)   // 度 -> 千米
lon, lat, err := p.Inverse(x, y)
```

批量接口使用 packed `[lon0,lat0,lon1,lat1,...]`：

```go
xy, err := proj.ForwardBatch(p, lonLat) // 域外点写成 NaN
if errors.Is(err, proj.ErrDomain) {
    // 整批已算完；(NaN,NaN) 标记越界的点
}
```

CRS→CRS 转换（含 `+towgs84` Helmert 基准变换）：

```go
from, _ := proj.Parse("EPSG:4326")
to, _ := proj.Parse("EPSG:3857")
tr, _ := proj.NewTransformer(from, to)
x, y, err := tr.Transform(0, 0)
```

示例代码都有可运行版本（[example_test.go](example_test.go)），随 `go test`
一起执行，文档不会与 API 漂移。

## WebAssembly（WASM）

`wasm/` 子包让同一套投影代码跑在浏览器和 Node.js 里；解析好的 CRS 句柄
留在 Go 侧，JS 按句柄调用：

```bash
pwsh wasm/build.ps1      # 或 ./wasm/build.sh
```

```js
const p = simpleProj.parse("+proj=geos +h=35785863 +lon_0=140.7 +ellps=WGS84");
const { x, y } = simpleProj.forward(p.id, 140.7, 30.5);
const { xy, domain } = simpleProj.forwardBatch(p.id, new Float64Array([0, 0, 140.7, 30.5]));
```

`simpleProj` 提供 `parse`、`forward`、`inverse`、`forwardBatch`、
`inverseBatch`、`transform`、`transformBatch`、`info`、`free`。完整 API、
批量语义与浏览器 demo 见 [wasm/README.md](wasm/README.md)。

## 支持的投影

| `+proj=` | 说明 | 球体/椭球 | 备注 |
|---|---|---|---|
| `latlong` / `longlat` | 经纬恒等投影 | — | 输出为度 |
| `eqc` | 等距圆柱（Plate Carrée） | ✓ | `+lat_ts`；忽略 `k_0` |
| `geos` | 静止卫星视图 | ✓ | 必须有 `+h`；`+sweep=x/y` |
| `stere` | 球面/椭球极射赤面 | ✓ | `+lat_ts` 折进有效比例 |
| `laea` | 兰勃特等积方位 | ✓ | 忽略 `k_0` |
| `lcc` | 兰勃特等角圆锥 | ✓ | 单/双标准纬线 |
| `merc` | 墨卡托 | ✓ | `+lat_ts`；球/椭球 |
| `tmerc` | 横轴墨卡托（Poder/Engsager 6 阶） | ✓ | `\|Ce\| ≤ 2.6234 rad` |
| `aeqd` | 方位等距 | ✓ | 椭球斜轴走 Karney 大地线 |
| `utm` | 通用横轴墨卡托 | ✓ | `+zone=1..60`、`+south` |
| `ob_tran` | 旋转极点 / 斜轴横轴 | ✓ | GRIB2 模板 3.1/3.3 |

此外还支持：

- `+units=` / `+to_meter=`（m、km、ft、us-ft、mi…），在 API 层换算；
- WKT2 / WKT1 子集解析（`Parse` 自动识别 WKT）；
- EPSG 快捷码：4326、4269、3857、900913，以及 WGS84 UTM 32601–32660 / 32701–32760；
- `+towgs84` 的 3 参 / 7 参 Helmert（经 `NewTransformer`，WGS84 中转）；
- GRIB `shape_of_the_earth` 0–11 与双轴形状 3/7（`GRIBEllipsoid`、
  `GRIBEllipsoidAxes`）；
- `Info()` 元数据（`Name`、`Domain`、`HasInverse`、`Accuracy`…）与带原因的
  `DomainError`（`errors.As` 可取 `far-pole`、`antipode`、`ce-limit`…），
  `errors.Is(err, ErrDomain)` 仍是主要判据。

## 正确性

参照数据由 **pyproj 3.8.0 / PROJ 9.8.1** 生成，提交在 [xcheck/](xcheck/README.md)：

- golden 网格：每个投影 33×33 点，正反双向；
- 22 例扩展 sweep：结构化边界 + 固定种子随机点，含定义域边界与单位换算；
- WKT 对拍：14 个 CRS × 2 种方言，解析后与 pyproj 输出比较；
- 大地线：86 组反解 + 71 组正解，含近对跖与 8 m 短线；
- 旋转极点：7 组配置，正反双向。

测试套件强制执行的门槛：

| 轴 | 门槛 |
|---|---|
| 正解 | ≤ 1e-6 m 绝对；坐标超过 ~1e6 m 时叠加相对下限 |
| 反解 | ≤ 1e-9 度绝对；极端位移叠加相对下限 |
| 往返 | ≤ 1e-9 度；已记录的病态边界点只校单向 |
| 经度 | 按圆周比较（−180° 与 +180° 是同一条经线） |

主包语句覆盖率 **98.3%**；剩下的是防御性分支（nil 接收者、构造器不可能状态），
可触达的部分都有专门测试。

## 并发

`CRS`、`Projection`、`Transformer` 构造后不可变，可被任意多个 goroutine 共享。
批量调用只分配输出切片：没有逐点分配、没有锁、没有全局缓存。

`TestThreadSafety` 用 8 个 goroutine 并发压每个投影，与串行结果逐位比对；
每次 push 都在 CI 上跑 `go test -race ./...`。

一条规则：把 `Parse` 返回的 `CRS` 当作只读。导出的 `Params` / `UnknownParams` /
`Towgs84` 与投影、元数据助手共享，并发改写属于 data race。

## 性能

每批 100 万点，packed 输入，包含输出分配。所有行都是 **1 alloc/op**（输出切片
16 MB）。测量环境：Windows 11、AMD Ryzen 7 7840HS、Go 1.27.3，2026-10-23；
数字随机器与负载波动。

| 投影 | ForwardBatch | InverseBatch |
|---|---|---|
| `latlong` | 18 ms | 18 ms |
| `eqc` | 48 ms | 43 ms |
| `geos` | 108 ms | 91 ms |
| `ob_tran` | 84 ms | 80 ms |
| `aeqd`（椭球斜轴） | 898 ms | 356 ms |

`aeqd` 是设计上的例外：椭球斜轴/赤道正解对每个点求一次大地线，与 PROJ 的算法
一致。等积、等角与墨卡托类工作负载的每点开销都在几十纳秒量级。

复现：

```bash
go test -run '^$' -bench . -benchmem
```

## API 一览

核心面很小：

- 类型：`CRS`、`Projection`、`Transformer`、`Ellipsoid`、`DomainError`；
- 构造：`Parse`、`ParseWKT`、`CRS.NewProjection`、`NewTransformer`；
- 转换：`Forward`、`Inverse`、`Transform` 及对应批量版本；
- 错误：`ErrDomain`，以及用 `errors.As` 取 `*DomainError`；
- 自省：`CRS.String/EPSG/IsGeographic/Units/Equals`、`Info`；
- GRIB：`GRIBEllipsoid`、`GRIBEllipsoidAxes`。

v1.0 之前，小版本仍可能调整边角行为；除非发布说明另有交代，导出名称与签名保持稳定。

## GRIB 映射

GRIB2 `grid_template_number` 的常见对应关系：

| 模板 | `+proj=` |
|---|---|
| 3.0 / 3.1 规则与旋转经纬 | `latlong`；旋转网格用 `ob_tran` |
| 3.10 墨卡托 | `merc` |
| 3.20 / 3.30 兰勃特等角 | `lcc` |
| 3.40 / 3.41 静止卫星 | `geos` |

旋转网格：GRIB 给的是南极点 `(LaSP, LoSP)`，用 `+o_lat_p = -LaSP`、
`+lon_0 = LoSP`（例如 ECMWF 的北极 39.25°N、162°W 对应
`+o_lat_p=39.25 +lon_0=18`）。

## 能力边界

明确不做：

- 网格基准（`+nadgrids` 的 NTv2/HARN）、动态/ITRF 基准；
- `+proj=pipeline` 语法 —— 两个 CRS 之间的转换请用 `NewTransformer`；
- 完整 EPSG 数据库（只有上面列的快捷码）；
- WKT **输出**、PROJJSON、ESRI 方言细节；
- 3D/4D 坐标、垂直基准、大地水准面网格；
- authority 轴序重排：坐标恒为 `lon,lat` / `x,y`；
- 框架层并行（批量循环本身已经足够紧、且无额外分配）。

解析是严格的：语义未实现的参数（`+pm`、`+axis`、`+nadgrids`、非 WGS84 的
`+datum`、`+R_A` 族、`+geoc`、`+over`…）在 `Parse` 阶段直接报错，而不是静默忽略。

## 相关项目

- [simple-grib](https://github.com/knoweather/simple-grib) —— 纯 Go 的
  GRIB1/GRIB2 读写库，本库消费它解析出的网格定义；
- simple-sat —— 使用 `simple-proj` 做重采样的气象卫星数据栈。

## 许可

MIT，见 [LICENSE](LICENSE)。
