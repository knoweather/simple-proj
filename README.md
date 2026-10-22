# simple-proj

轻量 Go 投影库（无 cgo、零第三方依赖，仅标准库），面向气象卫星/GRIB 场景的
等经纬、等距圆柱、Geostationary 视图投影的正反变换。与 pyproj/PROJ 9.8 对拍
（xcheck/golden_*.json），数值门槛：Forward ≤ 1e-6 m，Inverse ≤ 1e-9 度。

> 状态：v0.2.0。10 个投影（+ aeqd/utm）+ 单位换算 + WKT2/WKT1 解析 +
> CRS→CRS 转换（`Transformer`，含 `+towgs84` 基准变换）+ Karney 大地线解。
> 测试/对拍/交叉编译全绿，覆盖率 92%+。后继 simple-sat 按本文「公共 API 契约」接入。

## 项目背景

simple-proj 是 simple-grib / simple-sat 气象卫星数据栈里的**投影底座**：GRIB
模板（3.0/3.1 经纬、3.10 墨卡托、3.20/3.30 LCC、Geostationary 等）描述的数据
网格，需要一个纯 Go、无 cgo、可静态链接的 PROJ 子集，把网格坐标与经纬度互转，
供重采样（resampling）和几何处理使用。

- **为什么不用 pyproj**：pyproj 绑定 C 库（cgo），在纯 Go 服务/交叉编译场景
  下是负担；本库只覆盖气象卫星实际用到的 10 个投影，行为与 PROJ 9.8 对齐。
- **怎么保证行为一致**：所有投影与 pyproj 3.8.0 / PROJ 9.8.1 双向对拍
  （33×33 网格 golden + 独立扩展 sweep 两套点集），数值门槛
  Forward ≤ 1e-6 m、Inverse ≤ 1e-9 度；差异以 pyproj 输出为准。
- **开发方式**：双 AI 协作（AI-A：地基 + P0 投影；AI-B：公式 + 五个 P1 投影
  + golden 生成），分工与契约见 `COORDINATION.md`，全程记账于
  `PROGRESS-A.md` / `PROGRESS-B.md`，公式考据于 `FORMULA-NOTES.md`。

## 特性总览

| 能力 | 状态 | 说明 |
|---|---|---|
| 10 个投影（latlong/eqc/geos/stere/laea/lcc/merc/tmerc/aeqd/utm + longlat 别名） | ✅ | 球体 + 椭球双路径 |
| 大地线解（Karney 6 阶级数） | ✅ | `geodesic.go`；aeqd 斜轴用，对拍 pyproj ≤ 3.7e-9 m |
| 单位换算 `+units` / `+to_meter` | ✅ | m/km/ft/us-ft/mi…（框架层换算） |
| WKT2 / WKT1 解析 | ✅ | `ParseWKT`；翻译成等价 proj 串再入 `Parse` |
| CRS→CRS 转换 `Transformer` | ✅ | 纯投影 + `+towgs84` Helmert（3/7 参，WGS84 中转） |
| `Parse` / `CRS.String()` 往返 | ✅ | 参数拼写容错、默认值、别名归一 |
| EPSG 码解析 | ✅ | 4326 / 4269 / 3857 / 900913（特征回识别） |
| 批量 API `ForwardBatch` / `InverseBatch` | ✅ | 1e6 点 ~29-240 ms；D2 错误语义 |
| GRIB shape_of_the_earth → 椭球 | ✅ | shape 0-11 + 双轴 3/7（`GRIBEllipsoidAxes`） |
| 线程安全 | ✅ | Projection 不可变、无状态，可并发共享 |
| 元数据自省 `Info()` | ✅ | proj 名/描述/域规则/有无逆/精度口径（PJ_PROJ_INFO 对应物） |
| 域外原因 `DomainError` | ✅ | `errors.As` 取 far-pole/antipode/ce-limit…；`errors.Is(ErrDomain)` 不变 |
| 依赖 / cgo | 零 / 无 | 仅标准库；4 平台交叉编译验证 |

## 快速上手

要求 Go 1.21+（无第三方依赖）：

```bash
go get github.com/XC-Zero/simple-proj   # 或在 go.mod 中 require
```

```go
crs, err := proj.Parse("+proj=geos +h=35785863 +ellps=WGS84 +sweep=y")
if err != nil { ... }
p, err := crs.NewProjection()

// 单点（度 → 米）
x, y, err := p.Forward(140.7, 30.5)
lon, lat, err := p.Inverse(x, y)          // 定义域外 → (NaN,NaN,ErrDomain)

// 批量（行主序 packed [lon0,lat0,lon1,lat1,...]）
xy, err := proj.ForwardBatch(p, lonLat)   // 域外点写 (NaN,NaN),批末 err=ErrDomain
back, err := proj.InverseBatch(p, xy)
```

```go
crs, err := proj.Parse("EPSG:4326")       // 4326 / 4269 / 3857 / 900913 / 326xx / 327xx
crs.EPSG()                               // 特征回识别
crs.String()                             // 规范 proj 串
```

**CRS 之间转换**（对标 `Transformer.from_crs`，含 `+towgs84` 基准变换）：

```go
from, _ := proj.Parse("EPSG:4326")
to, _ := proj.Parse("+proj=tmerc +lon_0=117 +k=0.9996 +x_0=500000 +ellps=WGS84")
tr, _ := proj.NewTransformer(from, to)
x, y, err := tr.Transform(116.4, 39.9)   // 经纬度 → 投影坐标

// 反向要另建一个 tr（源/目标对调），与 pyproj 的 Transformer 一样是单向的
trBack, _ := proj.NewTransformer(to, from)
lon, lat, err := trBack.Transform(x, y)
```

**WKT 直接进 `Parse`**（WKT2 / WKT1 自动识别，内部翻译成等价 proj 串）：

```go
crs, err := proj.Parse(wktText)          // PROJCRS[...] / PROJCS[...] / GEOGCRS[...]
```

**单位**（`+units=km` 等，输出按该单位，`x_0` 仍按米参与）：

```go
crs, _ := proj.Parse("+proj=merc +a=6378137 +b=6378137 +units=km")
```

## 用法示例

**定义域错误处理**——超出投影范围（如 geos 看不到的地球背面、lcc 的远极点、
tmerc 偏离中央子午线超过约 81.7°）返回 `ErrDomain` + NaN 坐标，用
`errors.Is` 判定，不要当循环中断：

```go
x, y, err := p.Forward(0, 0)             // geos: 地球背面
if errors.Is(err, proj.ErrDomain) {
    // (x, y) == (NaN, NaN); 调用方决定跳过还是记簿
}
```

**域外原因**——`ErrDomain` 保持唯一 sentinel（`errors.Is` 判定不变），B 轨投影
额外返回携带原因的 `DomainError`，批量重采样时可区分“地平线外/远极点/超带”：

```go
var de *proj.DomainError
if errors.As(err, &de) {
    log.Printf("skip: %s (%s)", de.Reason, de.Detail) // far-pole / antipode / ce-limit / ...
} else if errors.Is(err, proj.ErrDomain) {
    // A 轨投影(latlong/eqc/geos)只带 sentinel,无原因
}
```

**元数据自省**——问一个实例“你是什么、有没有逆、域规则是什么”：

```go
info, _ := proj.Info(p)
info.Name        // "tmerc"
info.Domain      // "|Ce| ≤ 2.623395162778 rad(赤道约 ±81.7° 中央子午线圈外)"
info.HasInverse  // true(本库 9 投影全部有逆)
info.Accuracy    // golden 实测精度口径
```

**输入校验**——经度 > 360°、纬度 > 90° 是普通 error（非 ErrDomain）；
NaN 输入同样报错。

**CRS 往返**：

```go
crs, _ := proj.Parse("+proj=tmerc +k=0.9996 +lat_0=0 +lon_0=105 +ellps=WGS84")
crs.String()   // "+proj=tmerc +k=0.9996 +lat_0=0 +lon_0=105 +ellps=WGS84"
               // 原样回放(别名归一/默认值填补只发生在内部 Params,不改写字符串);
               // 只有 "EPSG:xxxx" 进门的 CRS 会回放成该码对应的规范 proj 串
```

内部 `Params` 里才是归一后的形态(`k`/`k0` → `k_0`、缺省补 `k_0=1` 与
`lat_0/lon_0/x_0/y_0=0`),可以用 `crs.Params["k_0"]` 取。

**并发共享一个 Projection**（无状态、不可变）：

```go
var wg sync.WaitGroup
for i := 0; i < runtime.NumCPU(); i++ {
    wg.Add(1)
    go func() {
        defer wg.Done()
        for j := 0; j < 1000; j++ {
            p.Forward(140.7+j*0.01, 30.5)   // 只读内核常量,无数据竞争
        }
    }()
}
wg.Wait()
```

## 支持的投影（v1 冻结清单）

| proj 名 | 别名 / 说明 | 球体+椭球 | 备注 |
|---|---|---|---|
| `latlong`/`longlat` | 等经纬恒等投影（输出度） | — | |
| `eqc` | Equidistant Cylindrical（Plate Carrée） | ✓ | EPSG:1028/1029；k_0 忽略 |
| `geos` | Geostationary Satellite View | ✓ | `+sweep=x/y`（缺省 y）；`+h` 必需 |
| `stere` / `laea` / `lcc` / `merc` / `tmerc` | B 轨投影 | ✓ | 见 PROGRESS-B |
| `aeqd` | Azimuthal Equidistant（极轴 N/S、赤道、斜轴） | ✓ | 椭球斜轴走 Karney 大地线 |
| `utm` | Universal Transverse Mercator | ✓ | `+zone=1..60`、`+south`；= tmerc + 固定参数 |

参数语义（均与 pyproj 3.8.0 / PROJ 9.8.1 实测对齐）：

| 投影 | 关键参数口径 | 域规则 |
|---|---|---|
| `eqc` | `+lat_ts` 定比例纬度；k_0 忽略 | `\|lat\|` > 90 报错；`\|lat_ts\|` = 90 拒收 |
| `geos` | `+h` 卫星高度（必需）；k_0 忽略 | 椭球：视界外 → ErrDomain；球体无可见性检查 |
| `stere` | `+lat_ts` 折进有效 k（`stereEffectiveK`）；极冠 `lat_ts≠90` 且 `k_0≠1` 按 PROJ 拒收 | 远极点 → ErrDomain |
| `laea` | 不用 k_0（PROJ 口径） | 中心对跖点 → ErrDomain |
| `lcc` | 双标准纬；单纬线（只给 lat_ts）按切锥；k_0 缩放 ρ | `\|lat_1+lat_2\|` 须 > 0、`\|lat_i\|` < 90°；远极点 → ErrDomain |
| `merc` | `+lat_ts` 一旦给出（含 0）即忽略 k_0，factor = msfn(φs)；缺省时 factor = k_0 | `\|lat_ts\|` = 90 拒收；极点 y 有限 |
| `tmerc` | PROJ 默认 auto -> Poder/Engsager 6 阶精确序列；`+k`/`+k0` 归一 k_0 | `\|Ce\|` ≤ 2.6234 rad（赤道约 ±81.7°）外 → ErrDomain |
| `aeqd` | `+lat_0` 定方面（±90 极轴 / 0 赤道 / 其余斜轴）；`k_0` 忽略 | 球体：中心对跖点、极轴对侧极点 → ErrDomain；椭球全域可解 |
| `utm` | `+zone` 必需（1..60）；`lon_0`/`x_0`/`y_0` 由 zone 覆盖；`+south` 加 1e7 假北移 | 同 tmerc |

## 精度与对拍

`xcheck/` 消费由 pyproj 3.8.0 / PROJ 9.8.1 生成的 golden（33×33 密集网格
+ 特例），门槛：Forward ≤ 1e-6 m、Inverse ≤ 1e-9 度。实测量级：

| 投影 | golden forward 最大 | golden inverse 最大 | 扩展 sweep forward / inverse 最大 |
|---|---|---|---|
| latlong | 0（恒等） | 0 | 0 / 0 |
| eqc | ≤ 5.4e-9 m | ≤ 1.5e-11 度 | 7.5e-9 m / 5.7e-14 度 |
| geos_x / geos_y | ≤ 4.2e-9 m | ≤ 5.4e-12 度 | 2.3e-9 m / ≤ 2.8e-12 度 |
| stere | 4.5e-4 m @ 1.3e10 m（相对 3.5e-14） | 1.6e-11 度 | 2.1e-5 m（相对）/ 5.7e-14 度 |
| laea | 2.97e-7 m | 1.38e-12 度 | 2.8e-8 m / 3.4e-13 度 |
| lcc | 4.84e-5 m @ 9e7 m（相对 5.4e-13） | 2.56e-13 度 | 4.7e-6 m（相对）/ 1.7e-13 度 |
| merc | 5.96e-8 m | 1.98e-9 度* | 4.7e-9 m / 5.7e-14 度 |
| tmerc | 7.45e-9 m | 3.13e-13 度 | 3.7e-9 m / 1.7e-13 度 |

\* merc 的 1.98e-9 度是 golden 的 (1e12, 1e12) 极端探针：弧度经度 ~1.6e5 的双
精度 ulp 单独就有 ~1.7e-9 度，任何实现对不齐——用 C#3 同款相对下限
`max(1e-9, |x|/a·180/π·2.3e-16)`。同理，外围点（tmerc 偏离中央子午线 ~70°）
的往返门以“与 pyproj 自身往返一致”为准（PROJ 自己也漂 1.4e-8 度）。

新增投影的 sweep 实测（2026-10-22，21 个 case）：

| 投影 | sweep forward 最大 | sweep inverse 最大 | 备注 |
|---|---|---|---|
| aeqd_np / aeqd_sp | 3.7e-9 m | 2.8e-14 度 | 椭球极轴：子午线弧差 |
| aeqd_eq | 3.2e-8 m | 1.6e-12 度 | 椭球赤道：大地线 |
| aeqd_obl | 9.3e-9 m | 5.7e-14 度 | 椭球斜轴：大地线 |
| utm / utm_south | ≤ 5.6e-9 m | ≤ 1.4e-13 度 | 等价 tmerc |
| merc_km | 7.3e-12（km 单位） | 5.7e-14 度 | 单位换算层 |

大地线求解器本身（`geodesic.go`，`xcheck/geodesic_ref.json`：79 反解 + 69 正解，
含近对跖与极区）：距离 **≤ 3.7e-9 m**、方位 ≤ 8.1e-13 度、正解位置 ≤ 8.5e-14 度。

扩展 sweep（`xcheck/sweep_extra.json`，生成器 `xcheck/gen_sweep.py`）是一套
独立于 golden 网格的新点集（结构化边角 + 固定种子伪随机），并覆盖每个投影的多种
**方面**（stere/laea 的赤道 / 斜轴 / 两极方面都单独建 CRS，共 14 例），防止实现
“恰好拟合”fixture 网格而漏掉模式盲点（正是这个升级逮住了 stere 非极轴方面反向
长期错误的 bug）； fixture 手册见 `xcheck/README.md`。

## 公共 API 契约（satpy 侧接入面）

只依赖两个类型 + 三函数：

- 类型：`proj.CRS`、`proj.Projection`
- 函数：`Parse` → `NewProjection` → `Forward`
（批量见 `ForwardBatch`/`InverseBatch`；错误用 `errors.Is(err, proj.ErrDomain)` 判定。）

v0.2.0 追加（不冻结）：

- `ParseWKT(s)` / `Parse(wkt)`：WKT2/WKT1 子集，行为等价于同义 proj 串；
- `NewTransformer(from, to)` → `Transform(x, y)` / `TransformBatch(xy)`：
  CRS→CRS 转换，`+towgs84` 走 Helmert（3/7 参，WGS84 中转）；
- `+units` / `+to_meter`：非米单位在 `NewProjection` 里套换算层，内核仍按米。
- 增强(不冻结,C#14):`proj.Info(p)` 元数据自省;`proj.DomainError` 域外原因
  (`errors.As` 取 `Reason`,`errors.Is(ErrDomain)` 语义不变)。

## simple-grib 桥接（放在 simple-sat 的 geom 包，不在本库）

```go
// simple-grib 已解析模板 3.20/3.30 的投影参数(grid/projected.go projectedParams)
func crsFromGridDef(def *gribgrid.Definition) string {
    // 模板 3.0/3.1(经纬) → "+proj=latlong"
    // 模板 3.30(LCC)     → fmt.Sprintf("+proj=lcc +lat_1=%g +lat_2=%g +lat_0=%g +lon_0=%g", ...)
    // 模板 3.10(墨卡托)   → fmt.Sprintf("+proj=merc +lat_ts=%g +lon_0=%g", ...)
}
```

GRIB shape_of_the_earth → 椭球的换算可用 `proj.GRIBEllipsoid`
（另见 `proj.GRIBEllipsoidAxes` 双轴形状 3/7 的接口；详见 PROGRESS-A.md 之 C#1）。

## 性能

1e6 点批量（AMD Ryzen 7 7840HS，`go test -bench . -benchmem`，确定性伪随机
点集，逆变换输入由正向输出构造以保证域内）：

| 投影 | ForwardBatch | InverseBatch | 备注 |
|---|---|---|---|
| latlong | ~29 ms | ~34 ms | 恒等 |
| eqc | ~74 ms | ~80 ms | |
| geos | ~160 ms | ~149 ms | 三角重算 ~200 ns/点 |
| stere | ~135 ms | ~692 ms | inverse 是不动点迭代（8 次），PROJ 同款 |
| laea | ~161 ms | ~409 ms | inverse Newton 迭代 |
| lcc | ~159 ms | ~572 ms | inverse Newton 迭代 |
| merc | ~96 ms | ~77 ms | 全库最快 |
| tmerc | ~239 ms | ~256 ms | 6 阶 Clenshaw 级数（正反都算） |
| aeqd（椭球斜轴） | ~919 ms | ~361 ms | 正解是大地线 Newton 迭代；utm 与 tmerc 同价 |

（latlong/eqc/merc 落在 DESIGN T8 的 1e6 < 100 ms 预算内；geos/stere/laea/
lcc/tmerc 超预算的仲裁记录见 PROGRESS-A/B.md——批量每点 1 alloc，输入预分配
后无额外分配。数字随机器与载荷波动，以本仓 bench 实测为准。）

## 测试

```bash
go test ./...                          # 全量（80+ 个 Test，含 golden/sweep 消费）
go test -bench . -benchmem ./          # 批量基准
go vet ./... && gofmt -l .             # 静态检查
GOOS=linux GOARCH=arm64 go build ./... # 交叉编译（无 cgo）
```

- **golden 对拍**：`golden_*.json`（pyproj 生成）由各投影的 `*_test.go` 消费；
- **扩展 sweep**：`TestSweepExtra` 走完整公共 API 对拍独立点集；
- **线程安全**：`TestThreadSafety` 8 goroutine 并发同一实例，结果与串行逐位一致
  （`-race` 需 cgo，Windows 本机不可用，留待 linux CI；内核为不可变常量构造，
  竞争由设计排除）；
- **批量语义**：D2 域外逐点 NaN + 批末 ErrDomain、坏输入整批失败；
- **大地线**：`TestGeodesicInverseParity` / `TestGeodesicDirectParity` 对拍
  `xcheck/geodesic_ref.json`（含近对跖、极区、8 m 短线）；
- **WKT**：`TestWKTParity` 把 14 个 CRS × 2 方言的 WKT 解析结果与 pyproj 的
  forward 坐标逐点比对，并验「解析 → proj 串 → 再解析」行为一致；
- **CRS 互转**：`TestHelmert*` / `TestTransform*` 覆盖 3/7 参、投影间转换、
  ballpark、单投影拒收 `+towgs84`、`+proj=pipeline` 拒收。

## 能力边界

**已支持**（v0.2.0）：

- 10 个投影、球/椭球双路径、批量 API、`+units`/`+to_meter`；
- WKT2 / WKT1 子集解析（翻译成等价 proj 串，`String()` 仍只回 proj 串）；
- CRS→CRS 转换 + `+towgs84` 的 3 参/7 参 Helmert（WGS84 中转）。

**明确不做**：

- 网格基准（`+nadgrids` 的 NTv2/HARN）、动态基准与 ITRF 历元（`+deformation_model`）；
- `+proj=pipeline` 操作链语法 —— 两 CRS 之间的转换请用 `NewTransformer`；
- proj.db / 完整 EPSG 库（只认 4326/4269/3857/900913 + WGS84 UTM 326xx/327xx）；
- WKT **输出**、PROJJSON、ESRI 方言的细节差异；
- 3D/4D 坐标（z、时间）、垂直基准、大地水准面网格；
- authority 轴序重排（EPSG:4326 恒为 lon,lat；pyproj 默认是 lat,lon，需要 `always_xy=True` 才对齐）；
- 地理检索、框架层并行投影（批量已高度面向循环优化）。

## 与 pyproj 的已知差异

本库以「与 pyproj 3.8.0 / PROJ 9.8.1 逐点对齐」为验收口径，下面三处是**有意保留**
的差异，都写在对应源码注释里：

1. **aeqd 球面斜轴的刀刃点**：`(lat_0=45, lon_0=10)` 投 `(100,45)` 时，PROJ 的
   `aeqd_s_inverse` 里有 `xy.y == 0 ? lam = 0` 的短路，而该处 y 只剩 ~1e-16 的
   舍入残差，C 侧恰好舍成 0 就把经度判成中心经度（一次往返丢 90°）。本库直接
   用 `atan2(x, y)`，往返正确。
2. **反向 Helmert 的求法**：PROJ 用「参数取负」近似（`s' = 1−ppm·1e-6`），本库
   解 `M·x = X'−T`。两者差 O((ppm+r)²)，DHDN 参数下 0.4 mm（≈4e-9 度）。
3. **不支持的参数 fail-loud**：`+pm`/`+axis`/`+nadgrids`/`+datum`(非 WGS84)/
   `+R_A` 族/`+geoc`/`+over`/`+geoidgrids` 在 pyproj 里会被静默忽略或降级，
   本库在 `Parse` 阶段就报错 —— 静默忽略等于悄悄算错。恒等值
   （`+towgs84=0,0,0`、`+nadgrids=@null`、`+datum=WGS84`、`+type=crs`）照常放行。

## 项目结构

| 文件 | 内容 | Owner |
|---|---|---|
| `crs.go` / `crs_test.go` | `Parse`、proj 串往返、EPSG | A |
| `ellipsoid.go` / `gribshape.go` | 椭球表、GRIB shape → 椭球 | A |
| `proj.go` / `projstr.go` | `Projection` 接口、`register`、批量 | A |
| `impl_latlong.go` / `impl_eqc.go` / `impl_geos.go` | A 轨三投影 | A |
| `mlfn.go` | Karney 矩形纬级数（共享底座） | A |
| `geolib.go` | 球面/椭球公共数学（等角/等积纬等） | B |
| `impl_stere.go` / `impl_laea.go` / `impl_lcc.go` / `impl_merc.go` / `impl_tmerc.go` | B 轨五投影 | B |
| `impl_aeqd.go` / `impl_utm.go` | 方位等距、UTM | — |
| `geodesic.go` / `geodesic_test.go` | Karney 大地线（aeqd 斜轴用） | — |
| `transform.go` / `transform_test.go` | `Transformer`（CRS→CRS + Helmert） | — |
| `wkt.go` / `wkt_test.go` | WKT2/WKT1 子集解析 | — |
| `units_test.go` | `+units`/`+to_meter` 换算层 | — |
| `xcheck/gen_golden.py` / `xcheck/gen_sweep.py` | golden / sweep 生成器 | B |
| `xcheck/gen_geodesic.py` / `xcheck/gen_wkt.py` | 大地线 / WKT 参照生成器 | — |
| `xcheck/golden_*.json` / `sweep_extra.json` | 对拍参照 | B |
| `xcheck/xcheck_test.go` | A 轨消费端 | A |
| `bench_test.go` / `bench_b_test.go` | 批量基准（A / B 轨） | A / B |

## 协作

双轨 AI 协作见 `COORDINATION.md`；A 轨账本 `PROGRESS-A.md`、B 轨 `PROGRESS-B.md`；
设计背景 `DESIGN.md`（只读，冻结）。

## 许可

MIT（见 LICENSE）。
