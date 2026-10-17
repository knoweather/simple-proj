# simple-proj 设计稿

> 定位:纯 Go 的地图投影引擎,对标 Python 生态里的 **pyproj**(作为库使用时关心的子集:proj 字符串解析 + Forward/Inverse 变换)。
> 它是 simple-sat(satpy 复刻)的地基模块之一,但**不依赖任何 simple-\* 模块,也不依赖 simple-sat**——只认识数学。
> 总体设计见 `D:\simple-sat\DESIGN.md`(§2b 拆分动机)。

---

## 0. 背景与动机

Python 分层:satpy → pyresample → pyproj(C 绑定)。
Go 复刻分层:simple-sat → `geom`(重采样几何)→ **simple-proj**(本模块)。

**为什么独立成库而不是 simple-sat 的子包:**

1. 投影数学 self-contained,可以立刻开工、独立对拍,不拖框架进度;
2. "lon/lat ⇄ x/y" 是通用需求——simple-grib 的 grid 包、simple-netcdf 的 CF 坐标、前端 wasm viewer、tsias/tc-track 等项目都能直接 import;
3. 简化 simple-sat 的 API 面:geom 只持 `proj.CRS`,不关心 CRS 内部;
4. 纯 Go(无 cgo)——保住 "simple-\*" 血统:cgo 绑 PROJ 会带来交叉编译噩梦,与 simple-grib/simple-netcdf 的纯 Go 哲学冲突。

**上游参考**:
- 行为标准:PROJ.4 / EPSG 定义公式(Snyder《Map Projections — A Working Manual》是最权威的公式出处);
- 对拍基准:本机 `D:\satpy` 环境里的 pyproj,生成 golden JSON;
- 兜底备选:`github.com/go-spatial/proj`(纯 Go 的 PROJ.4 移植,不完整、停更)——接口设计保持可互换。

---

## 1. 范围:v0 能力清单

### 1.1 做什么

| 能力 | 说明 |
|---|---|
| proj 字符串解析 | `+proj=geos +lon_0=140.7 +h=35785863 +ellps=WGS84 +units=m +no_defs` → `CRS` 结构 |
| 参数归一化 | 拼写容错(`+lat_1`/`+lat1`)、默认值填补(lat_0=0 等)、未知参数不报错仅保留 |
| 椭球表 | WGS84、GRS80、International 1924、Clarke 1866、**正球体(R=6371000 默认)**、airy…;`+a/+b/+rf/+f/+es/+R` 覆盖逻辑同 PROJ |
| GRIB shape_of_earth 映射 | GRIB2 模板 3.x octet "shape of the earth" 1-9(+residual scale)→ 椭球:a/b/sphere —— 参数抄 simple-grib `grid/definition.go` 的 earthDimension 逻辑 |
| Forward 变换 | `Forward(lon, lat) → (x, y)`(单位:米,latlong 投影为度) |
| Inverse 变换 | `Inverse(x, y) → (lon, lat)` **与 Forward 同为 t0 必做**(重采样反算目标区格点坐标全靠它) |
| 批量 API | `ForwardBatch/InverseBatch([]float64)`,行主序两两成对,给 resample 大循环用(单次暖机、避免接口调度开销) |
| CRS 互转辅助 | `CRS.EPSG() int` 识别常用 EPSG(4326/3857/4269…识别不到返回 0,不做数据库查询) |

### 1.2 投影子集(7 个,覆盖气象 95% 场景)

按开工顺序(前 3 个总计 1.5 人日即可端到端跑 Himawari + GRIB 经纬图):

| # | proj 名 | 用户 | 变换 | 公式量 | 优先级 |
|---|---|---|---|---|---|
| 1 | `latlong` | GRIB 经纬、CF 1D 坐标 | 恒等(度) | 50 行 | P0 |
| 2 | `eqc` | GRIB→satpy on-the-fly、NWP 常见 | 等距圆柱(椭球版 Equirectangular) | 150 行 | P0 |
| 3 | `geos` | Himawari/GOES-R/SEVIRI 全盘 | geostationary(satpy _geos_area 的正式版) | 300 行 | P0 |
| 4 | `stere` | 极地项目、NWC-SAF、lalb | Stereographic(+trueLAT_0/+ellps) | 250 行 | P1 |
| 5 | `laea` | Lambert 方位等积 | 250 行 | P1 |
| 6 | `lcc` | GRIB 模板 3.30 Lambert 投影 | 250 行 | P1 |
| 7 | `merc` | EPSG:3857 web 墨卡托 | 150 行 | P1 |
| 8 | `tmerc` | 本地区域(CMA 区域常用) | 250 行 | P2(完整高精度版需 Karney/Krüger 级数) |

GRIB 模板 3.20(AE)与 3.62 未列入——遇到再登记。

### 1.3 不做什么(明确出界)

- 基准变换(datum shift/Helmert 7 参数/NAD 系列)——气象场景默认 WGS84 或球体即可;
- WKT 解析(satpy 极少从 WKT 走,遇 CF 文件再议;`CRS.String()` 能导出 proj 串即可);
- proj.db / EPSG 完整库、管线操作链(`|pipeline` 语法);
- 大地水准面/垂直基准、地理检索(Nominatim 之类);
- 并行投影(内部 batch 已有 SIP 并行,框架层再并行)。

---

## 2. 目录结构与代码骨架

```
github.com/XC-Zero/simple-proj
├── go.mod
├── DESIGN.md
├── README.md                 ← 用法示例 + 能力边界
├── crs.go                    CRS 类型、Parse、参数归一化
├── crs_test.go
├── ellipsoid.go              椭球表 + a/b/es/rf 运算
├── ellipsoid_test.go
├── gribshape.go              GRIB shape_of_earth 1-9 → 椭球(供 simple-grib/grid 直接复用)
├── proj.go                   Projection 接口 +注册机制(工厂函数 map)
├── proj_test.go
├── impl_latlong.go
├── impl_eqc.go
├── impl_geos.go
├── impl_stere.go
├── impl_laea.go
├── impl_lcc.go
├── impl_merc.go
├── impl_tmerc.go
├── projstring.go             CRS.String() 重新生成 proj 串(往返不变性测试)
└── xcheck/                   golden 对拍
    ├── gen_golden.py          Python 侧生成脚本(pyproj)
    ├── golden_latlong.json
    ├── golden_eqc.json
    ├── golden_geos.json
    ├── golden_stere.json
    ├── ...
    ├── xcheck_test.go         Go 消费 golden 断言
    └── README.md
```

### 2.1 核心类型

```go
package proj

// CRS 是投影定义:解析后的参数集合。零值不可用,必须经 Parse 构造。
type CRS struct {
    Proj     string    // "geos", "stere", ...
    Params   map[string]float64 // 归一化后的参数(lon_0, lat_0, k_0, x_0, y_0, h, lat_1, lat_2...)
    Ellipsoid Ellipsoid
    units    string             // "m" 只认米;度仅 latlong 返回
    projString string           // 原始串(往返用)
}

// Parse 解析 proj 字符串或 "EPSG:4326" 形式。
// 未知 +key 不报错,存 OriginalParams 保留(警告语义),需要时未来扩展。
func Parse(s string) (*CRS, error)

// Projection 是某一种投影的实例化(绑定 CRS)。
type Projection interface {
    // CRS 返回绑定的坐标系。
    CRS() *CRS
    // Forward 经纬度(度) → 投影坐标(x, y 单位米;latlong 投影返回度)。
    // 非法输入(经度超 ±180 放宽到 ±360、纬度超 ±90)返回 error。
    Forward(lon, lat float64) (x, y float64, err error)
    // Inverse Forward 的逆。超出投影定义域(如 stere 极外侧、geos 视野之外)
    // 返回 NaN 坐标 + ErrOutsideDomain(重采样用 Is 判定,不中断循环)。
    Inverse(x, y float64) (lon, lat float64, err error)
}

// New 依 CRS.Proj 在工厂表里查找实现,返回实例。
// 批量推荐: crs.NewProjection() 一次然后持句柄。
func (c *CRS) NewProjection() (Projection, error)

// 批量接口:行主序 packed 数组 [lon0,lat0,lon1,lat1,...] → [x0,y0,...]
// 大数组时避免接口调用开销,内部按 chunk 常驻计算。
func ForwardBatch(p Projection, lonlat []float64) (xy []float64, err error)
func InverseBatch(p Projection, xy []float64) (lonlat []float64, err error)
```

设计原则(与 simple-grib 风格一致——fail-loud、纯函数):
1. **Forward/Inverse 无副作用**,线程安全(所有状态在 CRS/Projection 构造时定型);
2. **错误值:定义域外**不 panic,不返回部分脏值——统一 `ErrDomain`,`(NaN, NaN)`;
3. **CRS 不可变**:暴露的只有 getter;
4. 数值全是 float64;内存布局无 interface 面(连续 packed slice)。

### 2.2 椭球

```go
type Ellipsoid struct {
    A, B float64  // 长短半轴;球体 A==B
    F    float64  // 扁率 = (A-B)/A
    ES   float64  // 第一偏心率平方 = (A²-B²)/A²
    E    float64  // e
    Sphere bool
}

// 从 +a/+b / +a/+rf / +a/+f / +a/+e² / +R 全组合推导;
// +ellps= 名查内置表(WGS84/GRS80/Int1924/Clarke1866/sphere)。
// 顺序按 PROJ 约定: +R > +ellps > +a/b 组合 > 默认 WGS84。
func EllipsoidFromParams(params map[string]float64, ellpsName string) (Ellipsoid, error)

// GRIB shape_of_the_earth(WMO 码表 4.2 系列)1-9 + 缩放八位组 → Ellipsoid
// 逻辑抄 simple-grib/grid/definition.go 的 earthDimension():编码 + scaleFactor。
func GRIBEllipsoid(shape int, scale uint8, raw uint32) (Ellipsoid, error)
```

### 2.3 每个 impl 的固定模板

```go
// impl_geos.go 示例
type geosProj struct {
    crs   *CRS
    h     float64 // +h 语义按 PROJ:卫星到椭球面的高度;内部一律换算成 地心距 供公式用
    sweep string  // "x" or "y"(PROJ ≥6 默认 y;老数据常见 x,两者都支持)
    // 预计算量(椭球第一/第二偏心率等)
}
func newGeos(c *CRS) (Projection, error) { ... 参数校验(+h 必填,lon_0 默认 0)... }
func (g *geosProj) Forward(lon, lat float64)...{...}
func (g *geosProj) Inverse(x, y float64)...{...}
```

每个 impl ≤ 300 行;公式一律从 Snyder/EPSG 抄现成推导,不重新发明;每个投影至少 3 条 sanity(赤道、极点附近、本初子午线)单测,不写单测不能合入。

---

## 3. 对拍(xcheck)方案

**原则:金标由 pyproj 生成,Go 只做消费者。** 没有对拍能力就合入 = 禁止。

### 3.1 golden 生成(gen_golden.py,本地跑)

```python
# 在 D:\satpy 的环境里执行(satpy 依赖 pyproj 已装好)
from pyproj import CRS, Transformer
import json

CASES = {
  "latlong":  "+proj=latlong +ellps=WGS84",
  "eqc":     "+proj=eqc +lat_ts=0 +lat_0=0 +lon_0=180 +x_0=0 +y_0=0 +ellps=WGS84 +units=m",
  "geos_fy":  "+proj=geos +lon_0=140.7 +h=35785863 +x_0=0 +y_0=0 +a=6378137 +b=6356752.3 +units=m +sweep=x",
  "geos_nod": "+proj=geos +lon_0=0 +h=35785863 +ellps=WGS84 +sweep=y",   # GOES
  "stere_p":  "+proj=stere +lat_0=90 +lat_ts=60 +lon_0=-100 +ellps=WGS84",  # NSIDC 极地
  "laea_mb":  "+proj=laea +lat_0=45 +lon_0=10 +ellps=WGS84 +x_0=0 +y_0=0",
  "lcc_grib": "+proj=lcc +lat_1=30 +lat_2=60 +lat_0=0 +lon_0=100 +ellps=WGS84",  # GRIB 3.30
  "merc_3857":"+proj=merc +a=6378137 +b=6378137 +lat_ts=0.0 +lon_0=0.0 +x_0=0 +y_0=0 +units=m",  # 3857 球版
  "tmerc_cm": "+proj=tmerc +lat_0=0 +lon_0=105 +k=0.9996 +ellps=WGS84",  # CMA 区域
}
grids = {}
# 每案例 33×33 网格点:lon ∈ [-180,180], lat ∈ [-85,85] + 赤道、+边界点若干
for name, p in CASES.items():
    fwd, inv = Transformer.from_pipeline(...), ...
    # 每案例生成 {(lon,lat) → (x,y)} 正向用例,和 {(x,y) → (lon,lat)} 反向用例
with open("golden_xxx.json","w") as f: json.dump(case_set, f)
```

**重点用例**(正确性盲区,全部进 golden):
1. `geos` 的 sweep=x 与 sweep=y 两种(历史上 satpy/pyproj 在此踩坑);
2. 高斯/GRIB 球体椭球(`+a=+b=6371229` 这类);
3. `stere` +lat_0=90 极点(北极)、lat_0=-90;
4. `lcc` 两标准纬线 (30/60),含 lon_0=100(东半球);
5. Inverse 定义域外(geos 地平线以下、stere 投影圆外)→ 期望 NaN;
6. 半球过渡带:lat 接近 ±90 的数值稳定性。

### 3.2 Go 断言(xcheck_test.go)

```go
func TestGoldenGeos(t *testing.T) {
    g := loadGolden("golden_geos.json")
    for _, pt := range g.Cases {
        x, y, err := projForward(g.CRS, pt.Lon, pt.Lat)
        require.NoError(t, err)
        assertNear(t, x, pt.X, 1e-6 /* 米 */)   // 相对 1e-6 绝对 1e-3 米,先严后松
        lon, lat, err := projInverse(g.CRS, pt.X, pt.Y)
        assertNear(t, lon, pt.Lon, 1e-7)
    }
}
// + 往返不变性: Forward∘Inverse == id,误差 < 1e-9 度
```

**拿不到 pyproj 环境?** CI 里就不做对拍,用 repo 里已生成好的 JSON(提交进仓库),Python 脚本只在你本机更新 fixture 时用。

---

## 4. 任务切分(独立可验收)

| 任务 | 内容 | 验收 | 人日 |
|---|---|---|---|
| T0 | go.mod / DESIGN / README / 目录 | `go build ./...` | 0.2 |
| T1 | `ellipsoid.go` + `gribshape.go`(抄 simple-grib 逻辑)| 单测 + grib grid 包引用可达 | 1 |
| T2 | `crs.go`:Parse + 参数归一化 + EPSG 识别 | proj 串往返测试 | 1.5 |
| T3 | `latlong` + `eqc` impl | golden 对拍通过 | 1 |
| T4 | `geos`(sweep=x/y 双模式)| golden(satpy 用例)+ Inverse 视野外 NaN | 2 |
| T5 | `stere` + `laea` | golden 对拍 | 2 |
| T6 | `lcc` + `merc` | golden 对拍(含与 GRIB 3.30 参数匹配) | 2 |
| T7 | `tmerc` | golden 对拍 | 1.5 |
| T8 | Batch API + 基准(1e6 点 < 100ms)| bench 文件 | 1 |
| T9 | README/示例(含 simple-grib `grid.Definition → proj.CRS` 桥接文档) | — | 0.5 |

合计 **≈ 12.7 人日**(P1 里预算基本对齐)。

### 验收标准(整个项目)

- [ ] `go test ./...` 全绿,覆盖率 ≥ 85%(数学库标准线)
- [ ] 7 个投影全部过 pyproj golden(误差:x/y ∈ 1e-6 相对,lon/lat ∈ 1e-9 度)
- [ ] 无 cgo,`GOOS=linux/windows/darwin GOARCH=amd64/arm64` 交叉编译通过
- [ ] `go vet` 零警告;`gofumpt` 已跑
- [ ] (加分)simple-grib `grid` 引用本库替代它目前的投影内联公式后的回归全绿

---

## 5. simple-sat / simple-grib 对接契约(提前写死,防止互等)

**方向一:GRIB → proj(高频)**
simple-grib 已解析模板 3.20/3.30 的投影参数(`grid/projected.go` projectedParams)。桥接逻辑**放在 simple-sat 的 geom 包**(不在 simple-proj,避免反向依赖):

```go
func crsFromGridDef(def *gribgrid.Definition) string {
    // 模板 3.0/3.1(经纬) → "+proj=eqc" (参照 satpy 的 _get_cyl_area_info)
    // 模板 3.20(AE)      → "+proj=aeqd +lon_0=%g +lat_0=%g"(模板 3.20 是方位等距,当前 v1 未实现 aeqd;若实际数据用到再补进 impl 清单)
    // 模板 3.30(LCC)     → "+proj=lcc +lat_1=%g +lat_2=%g +lat_0=%g +lon_0=%g"
    // 模板 3.10(墨卡托)  → "+proj=merc +lat_ts=%g +lon_0=%g"
}
```

> 措辞提醒:**3.10 墨卡托我没列入 impl 清单但 GRIB 会遇到,加为 T6.5**(0.5 人日,公式与 merc 相同仅参数语义差)。

**方向二:satpy 侧 `geom.AreaDefinition` 持 proj.CRS**:
仅依赖 `proj.CRS` 和 `proj.Projection` 两个类型 + 三函数(Parse/NewProjection/Forward)。只这一条写在 simple-proj 的 README「公共 API 契约」节里。

**方向三:未来 nc/CF 文件→ CRS**:proj 串或 WKT 由 nccf reader 拼好传入本库 Parse,本库不读文件。

---

## 6. 风险与开放问题

1. **proj/geos +h 语义**:PROJ 里 `+h` 是卫星到椭球面的高度(不是到地心);与 satpy `_geos_area.py` 内部的 地心距折算直接对拍留档,避免两种约定打架;
2. **sweep 语义**:PROJ ≥ 6 有 `+sweep=y`(默认,沿卫星 y 轴扫),旧的 wrf-python 库/GOES 早期文件常写 sweep=x——**双向都支持**,golden 各一组;
3. `tmerc` 的 Karney vs Krüger 级数截断:气象尺度 km 级误差足够,先 6 阶系列(误差 <1 mm @ <1000km),将来如有 geodesy 用户再升级;
4. go-spatial/proj 兜底要不要接?**不预接**,接口留好;等 simple-proj 对拍完成后再决定是否除名;
5. EPSG 识别(4326 等)只做几种特例,遇未知原样返回 proj 串——不做 proj.db。

---

## 7. 开工指引(双 AI 并行轨)

> 分工已拆为 **AI-A(地基+P0投影)× AI-B(公式+fixture)双轨**,完整规程、文件所有权表、冻结接口代码、日程与质量 Gate 见 **`COORDINATION.md`**;本文档 §2b/§3.1 是其输入。
> 启动提示词在 COORDINATION.md §7,开两个新会话各自粘贴即可同时开工。单轨模式(只开一个会话)也写明在末尾。

1. 读 `COORDINATION.md`,确认自己角色(A 或 B),执行 Day0 的契约首提交(A)/ fixture 生成(B);
2. 之后按 `COORDINATION.md` §3 各自推进,只写自家文件(所有权见其 §1);
3. 单轨模式(只开一个会话):按本文档 §4 任务表 T0-T9 顺序做,忽略双轨约束。
