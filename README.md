# simple-proj

轻量 Go 投影库（无 cgo、零第三方依赖，仅标准库），面向气象卫星/GRIB 场景的
等经纬、等距圆柱、Geostationary 视图投影的正反变换。与 pyproj/PROJ 9.8 对拍
（xcheck/golden_*.json），数值门槛：Forward ≤ 1e-6 m，Inverse ≤ 1e-9 度。

## 快速上手

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
crs, err := proj.Parse("EPSG:4326")       // 4326 / 4269 / 3857 / 900913
crs.EPSG()                               // 特征回识别
crs.String()                             // 规范 proj 串
```

## 支持的投影（v1 冻结清单）

| proj 名 | 别名 / 说明 | 球体+椭球 | 备注 |
|---|---|---|---|
| `latlong`/`longlat` | 等经纬恒等投影（输出度） | — | |
| `eqc` | Equidistant Cylindrical（Plate Carrée） | ✓ | EPSG:1028/1029；k_0 忽略 |
| `geos` | Geostationary Satellite View | ✓ | `+sweep=x/y`（缺省 y）；`+h` 必需 |
| `stere` / `laea` / `lcc` / `merc` / `tmerc` | B 轨投影 | ✓ | 见 PROGRESS-B |

## 能力边界（v0 明确不做）

- 基准变换（datum shift / Helmert / NAD 系列）
- WKT 解析（`CRS.String()` 只回 proj 串）
- proj.db / 完整 EPSG 库、`|pipeline` 操作链
- 大地水准面 / 垂直基准、地理检索
- 框架层并行投影（批量已高度面向循环优化）

## 精度与对拍

`xcheck/` 消费由 pyproj 3.8.0 / PROJ 9.8.1 生成的 golden（33×33 密集网格
+ 特例），门槛：Forward ≤ 1e-6 m、Inverse ≤ 1e-9 度。实测量级：
latlong/eqc/geos forward 偏差 ≤ 5e-9 m，inverse ≤ 1.5e-11 度。

## 公共 API 契约（satpy 侧接入面）

只依赖两个类型 + 三函数：

- 类型：`proj.CRS`、`proj.Projection`
- 函数：`Parse` → `NewProjection` → `Forward`
（批量见 `ForwardBatch`/`InverseBatch`；错误用 `errors.Is(err, proj.ErrDomain)` 判定。）

## simple-grib 桥接（放在 simple-sat 的 geom 包，不在本库）

```go
// simple-grib 已解析模板 3.20/3.30 的投影参数(grid/projected.go projectedParams)
func crsFromGridDef(def *gribgrid.Definition) string {
    // 模板 3.0/3.1(经纬) → "+proj=latlong"
    // 模板 3.30(LCC)     → fmt.Sprintf("+proj=lcc +lat_1=%g +lat_2=%g +lat_0=%g +lon_0=%g", ...)
    // 模板 3.10(墨卡托)   → fmt.Sprintf("+proj=merc +lat_ts=%g +lon_0=%g", ...)
}
```

GRIB shape_of_earth → 椭球的换算可用 `proj.GRIBEllipsoid`
（另见 `proj.GRIBEllipsoidAxes` 双轴形状 3/7 的接口；详见 PROGRESS-A.md 之 C#1）。

## 性能

1e6 点批量（AMD Ryzen 7 7840HS，`go test -bench . -benchmem`）：

| 投影 | ForwardBatch | InverseBatch |
|---|---|---|
| latlong | ~29 ms | ~41 ms |
| eqc | ~84 ms | ~96 ms |
| geos | ~211 ms | ~188 ms |

（geos 为三角重算;latlong/eqc 落在 DESIGN T8 的 1e6 < 100 ms 预算内。）

## 协作

双轨 AI 协作见 `COORDINATION.md`；A 轨账本 `PROGRESS-A.md`、B 轨 `PROGRESS-B.md`；
设计背景 `DESIGN.md`（只读，冻结）。

## 许可

MIT（见 LICENSE）。
