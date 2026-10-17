# PROGRESS-B.md — AI-B 轨进度(公式 + P1 投影)

> 本文件 owner=AI-B,A 只读。跨轨事项走 COORDINATION §5。
> C 编号与 PROGRESS-A.md **共享同一序列**(A 已用到 C#2),B 的新条目从 **C#3** 起。
> 环境:go1.27.1(`C:\Users\64517\go\go1.27.1\bin`,不在 PATH);pyproj 3.8.0 / PROJ 9.8.1
> (装在系统 Python 3.13:`C:\Users\64517\AppData\Local\Programs\Python\Python313\python.exe`,
> 不在本机默认 python 里);Python 3.13;工作目录 D:\simple-proj。

## 状态(COORDINATION §3 B 轨清单)

- [x] Day0 上午:`xcheck/gen_golden.py` + 9 个 fixture(清单见下表)
- [x] Day0 下午:`geolib.go` + `impl_stere.go`(内核 + register 适配)+ `stere_test.go` → 合流(绿)
- [x] Day1:`impl_laea.go` + `laea_test.go`(绿)
- [x] Day1:`impl_lcc.go` + `lcc_test.go` —— 双标准纬 + 切锥 + 南锥(n<0),golden 全绿
- [x] Day2:`impl_merc.go` + `merc_test.go` —— 椭球/球 + lat_ts 比例纬度,golden 全绿
- [x] Day3:`impl_tmerc.go` + `tmerc_test.go` —— PROJ etmerc(Poder/Engsager 6 阶精确路径)
      + 球面分支,golden 全绿(1091 fwd / 1081 inv / 9 specials)
- [x] Day4 门(B 侧):`go vet` 0 / gofmt 内容干净(见"C#11 行尾符说明")/ 包覆盖率 **92.0%**
      / 交叉编译 4 组(linux+windows+darwin × amd64/arm64)全过;`-race` 需 cgo,本机受限,
      与 A 的 day4 记录同因,待 linux CI

**五个 B 轨投影全部收口。** 上一会话的"文件未提交"注记已过时——304dea3 起 B 的文件都在库里了。

## golden fixture 现状(权威 = pyproj 3.8.0 / PROJ 9.8.1)

| fixture | fwd | inv | specials | crs |
|---|---|---|---|---|
| golden_latlong.json | 1099 | 1089 | 0 | `+proj=latlong +ellps=WGS84` |
| golden_eqc.json | 1099 | 1090 | 0 | `+proj=eqc +lat_ts=0 +lat_0=0 +lon_0=180 +ellps=WGS84 +units=m` |
| golden_geos_x.json | 435 | 431 | 660 | `+proj=geos +lon_0=140.7 +h=35785863 +a=6378137 +b=6356752.3 +sweep=x` |
| golden_geos_y.json | 407 | 405 | 686 | `+proj=geos +lon_0=0 +h=35785863 +ellps=WGS84 +sweep=y` |
| golden_stere.json | 1099 | 1090 | 0 | `+proj=stere +lat_0=90 +lat_ts=60 +lon_0=-100 +ellps=WGS84` |
| golden_laea.json | 1098 | 1089 | 2 | `+proj=laea +lat_0=45 +lon_0=10 +ellps=WGS84` |
| golden_lcc.json | 1099 | 1090 | 0 | `+proj=lcc +lat_1=30 +lat_2=60 +lat_0=0 +lon_0=100 +ellps=WGS84` |
| golden_merc.json | 1099 | 1090 | 0 | `+proj=merc +a=6378137 +b=6378137 +lat_ts=0 +lon_0=0 +units=m`(正球) |
| golden_tmerc.json | 1091 | 1081 | 9 | `+proj=tmerc +lat_0=0 +lon_0=105 +k=0.9996 +ellps=WGS84` |

生成器口径(写在 `gen_golden.py` docstring 里):输出经度归一到 `[-180,180)`;
非有限值不能写进 JSON,所以 `specials` 存输入坐标 + `expectNaN: true`。

## B 侧已完成的实现与对拍结论

### geolib.go(共享,已由 day0 会话落地)
`geoDegToRad` / `geoRadToDeg` / `geoNormLon`、**`geoAdjLonPi`(本会话新增,弧度制 (−π,π] 归约)**、
`Gudermannian` / `InvGudermannian` / `IsometricLat` / `ExpNegIsometric` / `ConformalLat` /
`LatFromIsometric`(Newton,1e-13)、authalic 级数(`AuthalicSeries` + `Q`/`AuthalicLat`/`LatFromAuthalic`,
`AuthalicMaxTerms = 12`)。

authalic 系数坑(FORMULA-NOTES §0.1):`C_i = (i+1)/(2i+1)·e^{2i}`,项数 n≥8 才收敛。

### impl_stere.go(对拍通过)
forward 最大绝对差 **4.539e-4 m** at `(0,-89.9)`(坐标 1.3e10 m),相对 **3.5e-14**(C#3);
inverse 最大 **1.639e-11 deg**。`stereEffectiveK` 口径已用 pyproj 校正(见 C#3 一带)。

### impl_laea.go(对拍通过)
forward 最大 **2.971e-7 m**,inverse 最大 **1.378e-12 deg**,2 个 specials 全过。

### impl_lcc.go(本会话新增,对拍通过)
- golden_lcc:forward 最大 **4.840e-05 m**(出现在坐标 ~9e7 m 的边角点,相对 5.4e-13,纯双精度
  下限,走 C#3 相对容差);inverse 最大 **2.558e-13 deg**。
- 公式从共形条件直推:极坐标形式 (ρ(φ), n(λ−λ0)) 共形 ⟺ dρ/dφ = −nρ·M/(N cosφ),
  而 M/(N cosφ) 的不定积分正是等角纬 ψ,故 **ρ = C·exp(−n·ψ(φ))**;两标准纬等比例条件给出
  `n = ln(m1/m2)/ln(t1/t2)`(t = pj_tsfn = exp(−ψ)),切锥极限 `n = sin φ1`。
- **PROJ 9.8.1 lcc.cpp 逐条对齐**(本次拉了源码逐字核对):
  `|lat_1+lat_2| < EPS10` 拒收、`|lat_i| < 90°` 拒收、`+k_0` 参与缩放、远极点(φ·n ≤ 0)是
  **定义域外**(pyproj 显示的 (inf,inf) 是 errcheck=False 的假象,真身是 PROJ 报错)、
  近极点映射到 `(0, ρ0)`、**n<0(南锥)时 inverse 要先取反 (x, y, ρ)**(PROJ 源码原样)。
- 南锥 n 的符号天然从公式流出(ln(m1/m2)/(ψ2−ψ1)),不需要分支;ρ0 同号。
- **经度必须先在弧度制归约**:θ = n·Δλ 不是 2π 周期函数(n≠1),golden 第一个点 Δλ=−280°
  必须归约成 +80° 才对得上。为此在 geolib 加了 `geoAdjLonPi`(PROJ 管线的 adjlon 顺序)。

### impl_merc.go(本会话新增,对拍通过)
- golden_merc:forward 最大 **5.960e-08 m**,inverse 最大 **1.976e-09 deg**(见 C#9:1e12 m 极端
  探针上的双精度下限,用了 inverse 相对容差)。
- **比例因子口径修正**(day0 笔记写反了,本会话用 pyproj 重新探明,FORMULA-NOTES §4):
  - `+lat_ts` 一旦给出(**含 0**):factor = **msfn(φs) = cosφs/√(1−e²sin²φs)**(球面退化为
    cosφs),且 **`+k_0` 被忽略(PROJ 是替换不是相乘**——k0=2 与 k0=1 输出逐位相同)。
  - `+lat_ts` 缺省:factor = `+k_0`。
  - `|lat_ts| = 90` 拒收(msfn 退化)。
- 内核就是 geolib 等角纬的薄封装,**但正解的值要用 PROJ 的稳定形式**:
  `ψ = asinh(tanφ) − e·atanh(e·sinφ)`,不是 `ln tan(π/4+φ/2) + e/2·ln(...)`——两者数学等价,
  但 φ=π/2(双精度)时前者给 38.0250、后者给 37.3319,golden 的极点记录
  `(0,90) → y = 242528680.94374272` 锁的是前者。lat_ts=0 + k0=2 探针证明"lat_ts=0 也折叠 k0"。

### impl_tmerc.go(本会话新增,对拍通过)
- golden_tmerc:forward 最大 **7.451e-09 m**,inverse 最大 **3.126e-13 deg**,9 个 specials
  (8 个 Δλ∈[82.5°,97.5°] 前向 + 1 个 (9e9,−9e9) 反向)全过。
- 算法 = **PROJ 9.8.1 默认 tmerc(auto)的精确路径:Poder/Engsager(etmerc)**,不是 Karney 2023。
  源码结构:`geodetic φ --(cbg)--> 高斯(共形)纬 --> 复球面经纬 -->(clenS+gtu)--> 归一化 N/E
  -->(Qn, Zb)--> 米`;反之为 utg + atan2 复合 + `pj_auxlat_convert(sin/cos)` 三参版。
- auto 模式(|Δλ|>3° 走精确、否则 Evenden/Snyder)在 fixture 上不触发:网格步长 11.25°,
  **所有 fixture 点 |Δλ| ≥ 5°**,整份 fixture 都是精确路径;两条路在 3° 边界一致到 ~1e-9 m,
  所以实现里椭球路径恒定走精确序列。
- 系数:PROJ `src/latitudes.cpp` 的 Maxima 表(4 组 × 21 项,6 阶 Clenshaw,第三扁率 n),
  **标签是 `C[输出,输入]`**(这个坑见 FORMULA-NOTES §5:一开始装反了,φ→χ 的首项是 −2n 不是 +2n)。
- 域:复球面东移 `|Ce| ≤ 2.623395162778 rad`(~150°);赤道上看就是 |Δλ| ≲ 81.7° 才合法,
  与 specials(82.5° 起 NaN、75° 合法)完全吻合。
- 球面(a==b)走 PROJ 的 `tmerc_spherical_*`(`exp` 型 x、`acos` 型 y),并补上框架的 ×a。
- `Zb = −Qn·auxlat(gtu, auxlat(cbg, φ0))`,`Qn = k0·a·pj_rectifying_radius(n)`
  (复用了 A 的 `mlfn.go` 同包函数,见 C#10)。

## 协作事项(§5)

- **C#1-ACK(回执 A)**:同上,B 无须改动。
- **C#2 回执(覆盖率)**:包覆盖率 90.7% → **92.0%**(三个新投影 + golden 消费端)。
- **C#3 容差相对下限**:维持;lcc 的 4.8e-5 m @ 9e7 m 坐标同属此类(相对 5.4e-13)。
- **C#4 authalic 归 geolib(共享)**:接口冻结不变。
- **C#5 ±180 经度比对**:不变。
- **C#6 golden_test.go 归属**:不变。
- **C#7 往返容差在定义域边缘不可达**:不变。
- **C#8 `xcheck/README.md`**:仍未写(B 提议认领,排期在收尾之后)。
- **C#9 inverse 相对容差(新)**:§2.5 的 inverse 门槛 1e-9 deg 在极端坐标上不可达——
  golden_merc 的 (1e12, 1e12) 探针弧度经度 ~1.6e5,其双精度 ulp 单独就 ~1.7e-9 deg,
  任何实现对不上。merc_test 用 `max(1e-9, |x|/a·180/π·2.3e-16)`,即 C#3 相对下限思想
  延伸到 inverse 轴。建议 §2.5 补一句"inverse 容差同 forward,极端坐标按相对下限"。
- **C#10 复用 A 的 mlfn.go 助手(新)**:impl_tmerc.go 复用了同包的 `polyval` /
  `clenshawDelta` / `rectifyingRadius`(A 在 day2 为 eqc 落地,数学与 PROJ 的
  pj_polyval/pj_clenshaw/pj_rectifying_radius 逐位一致),没有重复定义也没有改 A 的文件。
  注意 `mlfn.go` 不在 COORDINATION §1 所有权表里,建议补登("A 拥有"应无争议)。
- **C#11 行尾符(新,环境性)**:仓库 `core.autocrlf=true`,工作区是 CRLF、仓库存 LF;
  go1.27.1 的 gofmt 输出 LF,于是 `gofmt -l` 会把所有 CRLF 文件都列出来(内容并没有问题)。
  本会话把 B 的文件统一成 LF(gofmt 规范形)后提交。建议 §4 的 gofmt 自查加一句
  "以 LF 规范形为准,勿用 `gofmt -w` 全仓重写(会踩 A 的文件)"。

## 覆盖率(B 文件,`go tool cover -func`)

包级 **92.0%**(A 轨 day3 时 90.7%)。本会话新增文件的逐函数情况:
impl_lcc.go:newLccKernel 96.4%,inverse 81.8%,newLcc 86.2%,adapter Inverse 75%(kernel inverse
对 lcc 实际不可能失败,该分支为防御性死代码);impl_merc.go:newMercKernel 88.9%,inverse 80.0%,
newMerc 95.0%,Forward/Inverse 83.3%/75.0%;impl_tmerc.go:newTmercKernel 96.4%,
forwardSph 70.0%,inverseSph 91.7%,newTmerc 81.8%(守卫分支)。其余函数 100%。

## 日志

- 2026-10-17 [B] day0-上午:`gen_golden.py` + 9 fixture 生成(见上表)。
- 2026-10-17 [B] day0-下午:`geolib.go` + `impl_stere.go` + `stere_test.go`;踩坑见 FORMULA-NOTES §0/§0.1。
- 2026-10-17 [B] day1:`impl_laea.go` + `laea_test.go` 全绿。三步坑留档 §2。
- 2026-10-17 [B] day1-附:stereEffectiveK 口径修正 + 极冠拒绝规则;补 C#2 直测;vet/gofmt/90.7%。
- 2026-10-17 [B] **交接点**:lcc / merc / tmerc 未开工。
- 2026-10-17 [B·续] **恢复现场**:发现 B 的提交(304dea3/d7ca952)实际落在 `ai-a` 分支上,
  `ai-b` 被重置成了空 commit(cc7c5f5)。`git reset --hard 304dea3` 把工作树恢复到 ai-b,基线
  `go test ./...` 全绿后接着干。
- 2026-10-17 [B·续] **lcc**:公式从共形条件直推;拉 PROJ 9.8.1 lcc.cpp 逐条对齐(极点定义域、
  n<0 取反、|lat_1+lat_2| 守卫);发现 conic 需要弧度制经度归约(geolib 加 `geoAdjLonPi`);
  golden_lcc 1099/1090 全绿(4.8e-5 m @ C#3 容差)。
- 2026-10-17 [B·续] **merc**:pyproj 重探发现 day0 笔记把比例因子写反了——**+lat_ts 给出即忽略
  k_0,factor = msfn(φs)**;正解等角纬必须用 asinh 稳定形式(极点 FP 行为不同);极点/域/守卫
  按 pyproj 校准;golden_merc 1099/1090 全绿,含极点记录 (0,90)→242528680.94374272。
- 2026-10-17 [B·续] **tmerc**:选定 PROJ etmerc(Poder/Engsager)而非自研 Krüger 级数;抓齐
  tmerc.cpp + latitudes.cpp 源码;踩掉的坑:辅助纬度系数块标签是 [输出,输入](装反过一次)、
  inverse 漏 `Cn +=` 实部、球面分支漏框架 ×a、测试侧 `+k`→`k_0` 别名;golden_tmerc
  1091/1081/9 全绿(7.5e-9 m)。**B 轨五投影自此全部收口。**
- 2026-10-17 [B·续] Day4 门(B 侧):vet 0 / gofmt 内容干净(LF 规范形,C#11)/ 覆盖率 92.0% /
  交叉编译 4 组过 / race 待 linux CI(cgo 限制)。

## HANDOVER(下个会话可直接接)

1. B 轨五投影(latlong 归 A;stere/laea/lcc/merc/tmerc 归 B)全部 golden 绿,
   剩余 B 侧待办只有 **C#8 的 `xcheck/README.md`**(fixture schema + 生成器用法 + 容差口径)。
2. Day4 全仓门:双方 merge 回 `main`(契约未改,应零冲突)、A 打 tag
   `v0.1.0-projsubset`;`go test -race` 留待 linux CI。
3. 跨轨未决:C#9(inverse 相对容差入 §2.5)、C#10(mlfn.go 补登所有权)、C#11(gofmt 行尾符口径)。
4. 工具环境提醒:`go` 不在 PATH,用 `C:\Users\64517\go\go1.27.1\bin\go.exe`;
   **pyproj 在系统 Python 里**(`C:\Users\64517\AppData\Local\Programs\Python\Python313\python.exe`),
   默认的 `python`/`pip` 是 WorkBuddy 托管 Python,没有 pyproj。
