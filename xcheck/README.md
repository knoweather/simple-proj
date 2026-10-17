# xcheck/ — golden 对拍 fixture(owner: AI-B,C#8)

> 本目录 owner=AI-B(COORDINATION §1:生成器与 fixture 归 B;A 只读消费)。
> `xcheck_test.go`(A 轨消费端)读取本目录的 `golden_*.json` 做对拍断言。
> Golden truth = **pyproj 3.8.0 / PROJ 9.8.1**(见每个文件头的 `generator` 字段)。

## 1. 这是什么

9 个投影的权威参照点集:33×33 经纬网格 + 每例固定的附加点 + 域界探针,
由 pyproj 双向(正/逆)生成。Go 侧实现(`impl_*.go`)对它们逐点对拍,
是 COORDINATION §2.5 数值门槛的落地手段。**权威永远在 pyproj 一侧**:
对拍不一致时,以重跑 pyproj 的输出为仲裁(§5.4)。

## 2. fixture 清单

| 文件 | fwd | inv | specials | CRS |
|---|---|---|---|---|
| `golden_latlong.json` | 1099 | 1089 | 0 | `+proj=latlong +ellps=WGS84` |
| `golden_eqc.json` | 1099 | 1090 | 0 | `+proj=eqc +lat_ts=0 +lat_0=0 +lon_0=180 +x_0=0 +y_0=0 +ellps=WGS84 +units=m` |
| `golden_geos_x.json` | 435 | 431 | 660 | `+proj=geos +lon_0=140.7 +h=35785863 +x_0=0 +y_0=0 +a=6378137 +b=6356752.3 +units=m +sweep=x` |
| `golden_geos_y.json` | 407 | 405 | 686 | `+proj=geos +lon_0=0 +h=35785863 +ellps=WGS84 +sweep=y` |
| `golden_stere.json` | 1099 | 1090 | 0 | `+proj=stere +lat_0=90 +lat_ts=60 +lon_0=-100 +ellps=WGS84` |
| `golden_laea.json` | 1098 | 1089 | 2 | `+proj=laea +lat_0=45 +lon_0=10 +ellps=WGS84 +x_0=0 +y_0=0` |
| `golden_lcc.json` | 1099 | 1090 | 0 | `+proj=lcc +lat_1=30 +lat_2=60 +lat_0=0 +lon_0=100 +ellps=WGS84` |
| `golden_merc.json` | 1099 | 1090 | 0 | `+proj=merc +a=6378137 +b=6378137 +lat_ts=0.0 +lon_0=0.0 +x_0=0 +y_0=0 +units=m`(正球) |
| `golden_tmerc.json` | 1091 | 1081 | 9 | `+proj=tmerc +lat_0=0 +lon_0=105 +k=0.9996 +ellps=WGS84` |

计数含义:网格点(1089)里被 pyproj 判域外的会降级进 `specials`(所以
geos/tmerc 的 fwd < 1099);每例的域界探针,有限应答留在 `forward`/`inverse`,
非有限应答进 `specials`。

## 3. JSON schema(冻结于 COORDINATION §2.4)

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

- `forward`/`inverse`:普通参照点;字段即输入 → pyproj 输出。
- `specials`:域界探针,**只存输入坐标 + `desc` + `expectNaN`**。它有**两种形态**:
  - `{lon, lat}` = 正向域外探针(输入是经纬度);
  - `{x, y}` = 逆向域外探针(输入是米;geos_y 的 "inverse far outside visible disc"
    里 lon/lat 字字段装的其实是 x/y 米值)。
  消费端按字段存在性分派(A 的 xcheck_test.go 已如此处理)。
- `expectNaN: true`:pyproj 对该探针返回非有限值(PROJ HUGE_VAL)。JSON 不能写
  inf/NaN,所以只记"期望非有限",Go 侧对应 `ErrDomain` + (NaN, NaN)。
- 输出经度已归一到 `[-180, 180)`(生成器 `norm_lon`);极点纬度若 pyproj 给出
  ±90.0000001 之类不归一,按原值记录,Go 侧按度比较。

## 4. 生成器用法

```bash
# 依赖: pyproj(3.8.0 起;pip install pyproj)
python xcheck/gen_golden.py     # 从仓库根运行,覆写同目录 golden_*.json
```

口径(与 `gen_golden.py` docstring 一致):

- 网格:lon ∈ [−180, 180] 33 点、lat ∈ [−85, 85] 33 步长(DESIGN §3.1);
  每点正向变换后立即用其输出做逆向,记录往返两侧。
- 附加点(每例固定):赤道/本初/近极 ±89.9/±180 经线/±85 等 9 个 sanity 点。
- 域界探针:`SPECIALS` 表按投影定制(极点、对跖点、卫星视野外、远距逆向等);
  有限应答升级为普通记录,非有限应答进 `specials`。
- 源 CRS:`+proj=longlat` + 仅保留椭球 token(`a/b/R/rf/f/es/ellps/datum`)——
  投影参数(如 +h/+lat_ts/+sweep)混进 longlat 会让 PROJ 建出另一条管线,数值错位。
- 输出采用 `Transformer.from_crs(geo, tgt, always_xy=True)`,并做"中心点必须落到
  (x_0, y_0)"的健全性校验(挡 datum shift 混入)。
- 行格式:一条记录一行,便于 diff 评审。

## 5. 消费侧容差口径

两层门槛,以 COORDINATION §2.5 为准:

| 方向 | 门槛 | 备注 |
|---|---|---|
| forward | ≤ 1e-6 m 绝对 | 超大坐标用相对下限 `max(1e-6, \|want\|·1e-12)`(C#3) |
| inverse | ≤ 1e-9 度绝对 | 极端位移用相对下限 `max(1e-9, \|x\|/a·180/π·2.3e-16)`(C#9) |
| 往返 | ≤ 1e-9 度 | 域界/病态点只校单向(§3.2 回退,C#7) |
| 经度 | 圆周比对 | `lonDiff()`:−180 与 180 是同一条经线(C#5) |

共享助手(`golden_test.go`,B 拥有):`loadGolden` / `goldenParams` /
`ellipsoidFor` / `tol` / `lonDiff`。A 的 `xcheck_test.go` 有自己的 loader,
包内不得出现同名定义(C#6)。

## 6. 再生策略(改 fixture = 改两轨的断言)

1. **默认不要重跑生成器**。换 pyproj/PROJ 版本、改 `SPECIALS`/网格、或任何
   fixture 内容变化,都必须先在 PROGRESS-B.md 记账(受影响文件列表),并通知 A
   (A 的消费端自动收紧,失败即 A 的事故)。
2. 重跑前确认 `generator` 字段的版本变化;fixture 内容如与 pyproj 输出不符,
   以**重跑的 pyproj 输出**为准修 Go 侧实现,而不是手改 JSON。
3. JSON 里不得手写 inf/NaN(非法 JSON);域外一律走 `expectNaN`。
4. 仓库 `core.autocrlf=true`,工作区会是 CRLF、仓库存 LF;评审 fixture diff 时
   用 `git diff`(自动归一),勿据此判"全文件变更"(C#11)。

## 7. 已知坑(消费/生成时都咬过人)

- **`expectNaN` 键名自带 "NaN" 子串**:A 的容错解码曾用宽替换把键名一起替换,
  直接炸解析。替换只能针对值位。
- specials 的两种探针形态(§3),按字段存在性分派,别假设字段名。
- geos 的 `specials` 里混着"域外正向"和"域外逆向"两种,判据是字段不是 desc。
