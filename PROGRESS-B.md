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

- [x] C#8:`xcheck/README.md` 落地(fixture schema + 生成器用法 + 容差口径 + 再生策略)。
- [x] Day4 合流:由 A 落锤 —— main fast-forward 至 50f53db(含 B 全部提交)、tag
      `v0.1.0-projsubset`(5deef93);C#1/C#2/C#9/C#10/C#11 全部回执关闭。

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

**C#15(本会话):非极轴方面反向曾经完全错。** golden 与旧 sweep 都只用 `lat_0=90`
一套 CRS(模式盲),赤道/斜轴方面的反向(lat→0、经度打乱)长期没被测出——`go test`
全绿、覆盖率 92% 也盖不住,因为 `inverse` 分支覆盖率只有 25%(只跑了极轴模式)。
修复两处(详见 `impl_stere.go` 注释):
1. `newStereKernel` 的 EQUIT 分支漏初始化 `sinX1`/`cosX1`(Go 零值 → 纬度迭代收敛到 0);
2. `inverse` 用了被重赋值的球面近似 `tp` 算经度(且 OBLIQ 分支多挂了一个 `cosX1`),
   并漏掉 `/a` 归一(坐标以米为单位未缩放回椭球 a 单位)。
补 `TestStereAspects` 直测赤道(`lat_0=0`)与斜轴(`lat_0=45`);新多方面 sweep 一并覆盖。
末态 sweep:`stere_eq` forward 最大 **1.5e-8 m** / `stere_obl` 最大 **2.0e-6 m**,
inverse 均 ≤ **4e-13 deg**。**已结案。**

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
- **C#8 `xcheck/README.md`**:已由 B 认领完成(`xcheck/README.md`:fixture 清单、
  schema 两种 specials 形态、生成器用法、消费侧容差口径 C#3/C#5/C#9、再生策略、
  已知坑)。**已结案。**
- **C#9 inverse 相对容差**:A 已 C#9-ACK 回执:merc_test 内沿用相对下限即可,
  冻结契约文本不动,以"侧写注记"留论。**已结案。**
- **C#10 mlfn.go 所有权补登**:A 已 C#10-ACK 并在 COORDINATION §1 补登
  mlfn.go → A。**已结案。**
- **C#11 行尾符/gofmt 口径**:A 已 C#11-ACK:仓库存 LF、工作区 CRLF 属
  autocrlf 预期,gofmt 自查以"LF 规范形无 diff"为准,双方都不 `gofmt -w` 全仓
  重写。**已结案。**
- **C#1-ACK**:上一轮已回执;A 已在 50f53db 落地 `GRIBEllipsoidAxes`(shape 3/7
  双轴)。**已结案。**
- **C#2 覆盖率**:整体 92.0% ≥ 85%,A 已结案。
- **C#12 建议 gitignore 追加 `.workbuddy-ai/`(新,待 A 决定)**:B 的会话记忆落在
  `D:\simple-proj\.workbuddy-ai/`(daily log / MEMORY.md,agent 私有),不该进仓库;
  `.gitignore` 是 A 的文件(B 不动),建议 A 补一行 `.workbuddy-ai/`——否则哪天一次
  `git add -A`(day0 就发生过一次扫描效应)会把 B 的私有笔记扫进提交。
  *(后续 `.gitignore` 已含 `.workbuddy-ai/` 与 `.idea/`,本项实际已落地。)*

- **C#15 stere 非极轴方面反向 bug(本会话发现并修复)**:详见 `impl_stere.go` 注释与
  上文 `impl_stere.go` 段。golden + 旧 sweep 每投影仅一套 CRS,犯了"模式盲",使赤道 /
  斜轴方面反向(lat→0、经度打乱)漏测。`newStereKernel` 的 EQUIT 分支漏初始化
  `sinX1`/`cosX1`;`inverse` 用重赋值的球面 `tp` 算经度且 OBLIQ 多挂 `cosX1`、漏 `/a`
  归一。修复 + 加 `TestStereAspects` 直测赤道/斜轴。**已结案。**

- **C#16 sweep 多方面升级 + gen_sweep/gen_golden 解耦修复**:旧 sweep 与 golden 同犯
  模式盲,升级 `CASES` 增 `stere_eq`/`stere_obl`/`laea_eq`/`laea_np`/`laea_sp` 五套
  多方面 CRS,消费端 `sweep_extra_test.go` 遍历名单同步扩展(含 `TestThreadSafety`)。
  A 的 06d1455 把 `gen_golden.py` 重写为 EPSG:4326 源并删掉 `gen_sweep.py` 依赖的
  `geostring`/`norm_lon`/`finite`,导致 sweep 再生 `AttributeError`。本地在
  `gen_sweep.py` 重实现这三个 helper(**matched-ellipsoid 语义,不能用裸 EPSG:4326——
  Go 不做基准变换,参考必须与目标椭球一致,否则 geos_x 等偏 ~mm**),重新生成
  `sweep_extra.json`(14 例)。**已结案。**

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
  交叉编译 4 组过 / race 待 linux CI(cgo 限制)。提交 80c70f4。
- 2026-10-17 [A] day4-合流落锤:A 已把 main fast-forward 至 50f53db(含 B 全部提交),
  落地 GRIBEllipsoidAxes(C#1),C#9/C#10/C#11 全部 ACK 回执,覆盖率结案(92.0%),
  打 tag **v0.1.0-projsubset**(5deef93)。
- 2026-10-17 [B·续] **C#8 收尾**:`xcheck/README.md` 落地——9 fixture 清单与计数、
  schema(specials 两种探针形态 + expectNaN 语义)、生成器用法(网格/附加点/域探针/
  源 CRS 过滤/健全性校验)、消费侧容差口径(§2.5 + C#3/C#5/C#9)、再生策略(改 fixture
  须双方记账)、已知坑(expectNaN 键名宽替换炸解析等)。C#8 结案。
- 2026-10-17 [B·续] **全篇测试(用户指示)**:新增扩展对拍体系与 B 轨 bench——
  - `xcheck/gen_sweep.py` + `xcheck/sweep_extra.json` + `sweep_extra_test.go`:
    独立于 golden 网格的第二套点集(结构化边角 + 固定种子 LCG,9 例共 320 fwd /
    320 inv / 8 specials),走完整公共 API 对拍 pyproj,防"恰好拟合 fixture"。
    结果:forward 最大 eqc 7.5e-9 m / tmerc 3.7e-9 m / stere 2.1e-5 m(相对,C#3)/
    lcc 4.7e-6 m(相对),inverse 全部 ≤ 2.8e-12 deg。
  - `TestThreadSafety`:8 goroutine 并发同一 Projection,与串行逐位一致(内核不可变,
    竞争由设计排除;-race 待 linux CI)。
  - `bench_b_test.go`:B 轨五投影 Forward/InverseBatch 1e6 点基准
    (fwd 96-239 ms;inv merc 77 ms → stere 692 ms,stere/laea/lcc 的 inverse 是
    不动点/Newton 迭代的真实成本,PROJ 同款)。A 轨 bench 复跑数值同量级。
  - **C#13(新,外围点往返门)**:sweep 抓到 tmerc 在 Δλ=−70° 的 F→I 往返漂
    1.36e-8 deg——**pyproj 自己也是 1.36e-8**(PROJ etmerc 域边固有),我们与其
    一致到 2e-14 deg。故 sweep 的往返门改为"与 pyproj 自身往返一致"
    (参考文件记 rt_lon/rt_lat),良态点自动退化为 §2.5 绝对 1e-9 deg 门。
    建议 §2.5 侧写:往返门适用于良态点,外围点以 pyproj 往返为参照。
  - 末态门:vet 0 / 测试全绿 / 覆盖率 92.2% / gofmt(LF 规范形)干净。
- 2026-10-17 [B·续] **goproj 融合(C#14,用户指示"把好的部分融进来")**:
  评审 `C://Users//64517//GolandProjects//goproj`(用户手写的 cgo 版,617 行)后,
  融合两项、明确不做四项:
  - **已融合**:
    ① `impl_info.go` `proj.Info(p) (ProjectionInfo, error)` —— goproj `Info()`
       (PJ_PROJ_INFO)的对应物:proj 名/描述/规范串/EPSG/球椭球/HasInverse/单位/
       域规则/精度口径。**包级函数而非接口方法**——§2.1 接口冻结
       (CRS/Forward/Inverse),加方法是 §5 契约变更;B 侧新增文件即不碰冻结面。
    ② `domain_reason.go` `DomainError{Reason,Detail}`(Unwrap 到 ErrDomain)——
       goproj `proj_context_errno_string` 的思想:B 轨五投影的域外返回从裸
       sentinel 升级为带原因(far-pole/antipode/ce-limit/degenerate/outside),
       `errors.Is(ErrDomain)` 与 proj.go 的 D2 批量语义零影响(A 轨消费端与
       xcheck 全部照旧通过);tmerc_test.go 的 errorsIsDomain 辅助随之改为
       errors.Is(原来写 `== ErrDomain` 是个潜在坑,本次修正)。
  - **明确不做(记录理由)**:z/t 4D 坐标(Forward 签名冻结,重采样场景 2D 够用)、
    datum/crs_to_crs/pipeline(README 能力边界明文的战略选择)、LogLevel
    (goproj 自己也没接线,且日志钩子要动 A 的 proj.go 批量路径)、
    map 构造入口(Parse 串已覆盖,避免 API 膨胀)。
  - 顺手记录一个 PROJ 语义细节:stere 斜轴对跖点**不**报域错(kernel denom==0
    是精确比较,FP 残差 ~1e-16,给巨大有限值;PROJ 9.8.1 同款行为;只有 laea
    用 EPS10 拦得住)。
  - 末态门:测试全绿 / vet 0 / 覆盖率 92.2% / gofmt(LF)干净。
- 2026-10-17 [B·续] **README.md 扩写(应用户要求;§1 属 A 的文件,请 A review)**:
  在**完整保留 A 原文**(快速上手/能力边界/精度与对拍/公共 API 契约/simple-grib
  桥接/协作/许可)的基础上,补:项目背景(为什么做/为什么不用 pyproj/双 AI 协作)、
  特性总览表、参数语义与域规则表、9 投影精度表(golden + sweep 双列)、批量性能表
  (A+B 共 8 投影)、测试指南、项目结构与所有权表。

- 2026-10-18 [B·续] **健康检查 + 收口(用户:"有问题别管 ab 了 都你干就行了")**:
  - **发现并修复 stere 非极轴方面反向 bug(C#15)**:覆盖率分析暴露 `impl_stere`
    forward 62.5% / inverse 25% 只覆盖极轴模式;加 `TestStereAspects`(赤道/斜轴)
    立刻复现 (10,40)→(90,0) 的离谱反向。根因两处:`newStereKernel` EQUIT 漏初始化
    `sinX1/cosX1`;`inverse` 用重赋值的球面 `tp` 算经度 + OBLIQ 多挂 `cosX1` + 漏 `/a`
    归一。修复后两测试 PASS。
  - **sweep 多方面升级(C#16)**:给 stere/laea 增赤道/斜轴/两极 5 套 CRS,消费端遍历
    名单扩展(含 `TestThreadSafety`),重新生成 `sweep_extra.json`(14 例)。sweep 全
    绿,forward 最大 stere_obl 2.0e-6 m / stere_eq 1.5e-8 m / laea_* ≤ 4.5e-8 m,
    inverse ≤ 4e-13 deg。这层升级正是为了把"模式盲"挡在 CI 外。
  - **修 gen_sweep.py 再生断裂**:A 的 06d1455 把 `gen_golden.py` 改写成 EPSG:4326 源
    并删了 `geostring`/`norm_lon`/`finite`,`gen_sweep.py` 一 import 就 `AttributeError`。
    在 `gen_sweep.py` 本地重实现这三个 helper(保留 matched-ellipsoid 语义,不用裸
    EPSG:4326)。
  - 末态门:**vet 0 / 全量测试绿 / gofmt(LF)干净 / 包覆盖率 94.4%**(stere/laea 分支
    补满后从 92.2% 升上来)。
  - 行尾符:本机 `core.autocrlf=true` 使工作树 CRLF、仓库 blob 实际也是 CRLF,与
    C#11 声称的"仓库存 LF"不符,导致 `gofmt` 反复报 diff。本会话加 `.gitattributes`
    锁定源码 `eol=lf` 并 `git add --renormalize`,把全仓统一成 LF,根除该 churn
    (C#11 意图的真正落地,用户已授权越过 A/B 协作约束)。


- 2026-10-22 **[全项] 对齐 pyproj 的功能缺口(用户:"看一下这里和 pyproj 还有什么
  功能上区别" → "一项一项做吧")**。五项按序落地,每项独立提交 + 对拍:
  - **① 静默参数改 fail-loud**(62a44e9):`+pm`/`+axis`/`+nadgrids`/`+towgs84`(当时)/
    `+datum`(非 WGS84)/`+R_*` 族/`+geoc`/`+over`/`+geoidgrids`/`+init` 在 Parse
    阶段报错 —— 实测这些键此前被静默丢进 UnknownParams,`+pm=paris` 会给出错经度
    却不报错。恒等值(`+towgs84=0,0,0`、`+nadgrids=@null`、`+datum=WGS84`、
    `+type=crs`)放行。顺带修 eqc/geos **漏掉 x_0/y_0**(A 轨历史遗漏,B 轨五投影
    早就支持),用 pyproj 参照值锁死。
  - **② aeqd + utm**(48212b3):aeqd 四方面(极轴 N/S、赤道、斜轴)× 球/椭球;
    椭球斜轴按 PROJ aeqd.cpp 走大地线(x = s12·sin(azi1))。为此移植
    **geodesic.go**(GeographicLib 6 阶级数,只留 inverse/direct),对拍
    `xcheck/geodesic_ref.json`:Δs12 ≤ 3.7e-9 m、Δ方位 ≤ 8.1e-13 度。
    踩坑:GeographicLib 的 `C3f` 里 `mult *= eps` 在 polyval **之前**(与 C1f/C2f
    相反),装反导致经度差 0.024°。utm = tmerc + zone 派生参数,含 EPSG 326xx/327xx。
  - **③ +units/+to_meter**(7b167fc):框架层 `unitScaled` 装饰器,内核仍按米
    (含 x_0);`+units` 胜 `+to_meter`;latlong 不换算;EPSG 特征识别在非米单位下
    返回 0。
  - **④ WKT2/WKT1**(e5d1dd1):`wkt.go` 递归下降 → **翻译成等价 proj 串**再入
    Parse(复用全部归一化/校验/单位层)。踩坑三处:WKT1 的 PROJCS 里有两个 UNIT
    (内层 GEOGCS 的 0.01745 会被当成线单位)、WKT2 的 LENGTHUNIT 挂在 AXIS 而非
    CS、GDAL WKT1 的 `Polar_Stereographic` 把 latitude_of_origin 写成标准纬线。
    角度单位必须按名字短路成 1(0.0174532925199433 乘 180/π 得 1.0000000000000002,
    会把 lon_0=180 顶成 180.00000000000003,eqc 的 x 直接翻号)。
  - **⑤ Transformer + towgs84**(37ed86d):CRS→CRS(替代 `+proj=pipeline` 的文本
    DSL),Helmert 3/7 参经 WGS84 中转;反向解 `M·x = X'−T` 而非 PROJ 的参数取负
    (差 0.4 mm,记入 README 已知差异)。单投影带 towgs84 明确拒收。
  - 末态门:vet 0 / gofmt 干净 / 全量测试绿 / 覆盖率 **91.9%** / 交叉编译 3 组过。
    sweep 扩到 22 个 case;新增 `xcheck/gen_geodesic.py`、`gen_wkt.py`。


- 2026-10-22 **[复审] 五项落地后的复查(用户:"再次检查一下")** —— 逮到 5 处:
  1. **椭球组合优先级装反**(真 bug,历史遗留):`+ellps=WGS84 +a=6371000` 之前
     按 DESIGN §2.2 让 ellps 全胜(a=6378137);pyproj 实测是 **+a/+b/+rf 覆盖
     ellps 打底、+f/+es 在有 ellps 时被忽略**。已按实测重写 `EllipsoidFromParams`
     并补 3 条用例(含"DESIGN 原文与实测相反"的注记)。
  2. **裸旗标漏网**:`+ellps` / `+units` / `+towgs84` 不带值时此前静默收进
     UnknownParams(PROJ 三者都报错)。已在旗标分支显式拒收。
  3. `transform.go` 的 `isErrDomain` 手写包装换成 `errors.Is`(原实现漏了
     `fmt.Errorf("%w")` 包装的情形,且是死代码)。
  4. 清死代码:`utmZoneLon0`、`benignNoopKeys`、`geodesic.go` 里 `_ = a12`/`_ = E`
     的占位(`a12` 是 PROJ geod_geninverse 的返回值,本库用不到,已连声明一起删)。
  5. `xcheck/README.md` 补 §2.1:三个新参照集(sweep/geodesic/wkt)的清单与口径。
  另核对通过:aeqd 极轴 inverse 的超远点(含 lat=-194.6 的外推形态)、ENSEMBLE /
  GEODCRS 形式的 WKT、`+units=km` 叠加 utm、WKT 前后空白、Info() 的单位显示。
  末态:全量测试绿 / vet 0 / gofmt 干净 / 覆盖率 **92.2%**。


- 2026-10-22 **[细查] 第三轮(用户:"仔细检查")** —— 又逮到 4 处,全是边界/冷路径:
  1. **`+ellps=`(空值)静默回落 WGS84**(PROJ 报错)。已拒收;顺带补 `+units=`/
     `+towgs84=` 空值用例(那两个本来就报错,补测锁住)。
  2. **`EPSG()` 把带 `+towgs84` 的 CRS 认成 4326**:特征比对只看椭球,没看基准。
     带 towgs84 时直接返回 0。
  3. **WKT 逐参数单位**:WKT2 允许每个 PARAMETER 自带 ANGLEUNIT/LENGTHUNIT,且
     可与 CS 不同(如 CS 用米、false easting 写美国测量英尺;或中央经线用 gon)。
     原实现只按全局单位换算 → 算错。改为「PARAMETER 自带单位 > CS 单位」,
     并用 pyproj 对同两份 WKT 取参照值锁死。为此把 `wktAngularUnitToDeg` 放开
     到任意角单位(只有「地理 CRS 本身」仍要求度)。
  4. `wgs84Ellipsoid` 从「手写字段 + init 补 ES」改为走 `newEllipsoidFromRF`
     (init 里 panic 兜底)。改的时候自己踩了一次 `rf` 与 `f` 混用,被 init panic
     当场逮住。
  另:WKT 参照点从 5 个扩到 12 个(每方言 336 点比对);补 `TestTransformConcurrent`
  并发冒烟。末态:全量绿 / vet 0 / gofmt 干净 / 覆盖率 **92.3%** / 交叉编译过。


- 2026-10-22 **[覆盖率] 92.3% → 95.0%(用户:"覆盖率 92.3% 为啥不是 100%")**。
  先把 213 条未覆盖语句按"能不能真的走到"分了四类,再把**可达**的那批补测:
  - 删死代码:`geodSinCosSeries` 的 cos 分支只服务 C4 面积项(本库不移植),
    14 个调用点全走 sin —— 改成 `geodSinSeries`,一半代码不再存在。
  - 补测(12 个新测试,覆盖 74 条真实可达语句):aeqd 球体极轴 N/S + 赤道、
    球面半径边界(π·R 夹取)、`clampUnit`;WKT 畸形输入 13 例 + 括号式 WKT1 +
    内嵌引号 + 非 EPSG 的 ID 三种变体 + 伪墨卡托强制球体 + 南半球极射;
    `Info()` 的 Units 随 `+units`/`+to_meter` 变;`InverseBatch` 错误面;
    Transformer 的 nil/构造期/执行期报错、批量域外、地心极点、退化尺度(s=0)。
  - 剩余 138 条(5.0%)的构成:geodesic 49(f<0 长球体分支 ~14、astroid 极端根 ~5、
    退化/短线程兜底 ~12、|f|>0.01 的 Newton 修正 6、NaN 守卫 ~6);
    B 轨五投影 43(内核 NaN 兜底,适配层已先校验);wkt 13(词法错误路径);
    其余 33(nil 守卫、`mustEllipsoid` panic、`Info` 未知投影等不可达分支)。
  **结论:剩下的不是"没测",是"测不到"——除非为不可达分支写断言,或为测试
  开后门。** 行为正确性由对拍保证(21×36 sweep + 9 套 golden 网格 + 148 条大地线
  + 336 点 WKT),不是由语句覆盖率保证。


- 2026-10-22 **[覆盖率 95% → 98.6%] 顺带挖出 stere 两个真 bug(用户:"需要 做完")**。
  这轮不是刷数字,是"为了覆盖冷路径,必须先把它的行为说清楚",结果说清了两个错的:
  1. **球面极射赤面反变换给出 180°−φ**(impl_stere.go `iterateGeodetic`):
     e==0 时直接返回 phiL,而极轴分支的 phiL = π/2 − 2·atan(tp) 是镜像纬度
     (lat=60 反算回来变 120)。椭球走迭代式看不出来,golden/sweep 又只有椭球极轴,
     所以从 day0 一直没暴露。修法:e==0 时返回 `2·atan(tp) − halfpi`(迭代式的
     极限形式),斜轴/赤道分支化简后仍是 phiL,行为不变。
  2. **`+lat_0=90` 不给 `+lat_ts` 时多乘了 0.5**:`newStere` 把缺省的 lat_ts=0
     喂给 `stereEffectiveK`,折出一个 0.5 的 k。pyproj 实测:缺省 lat_ts 时
     PROJ 直接用 k_0(`+lat_0=90` 与 `+lat_ts=90` 逐位相同;`+lat_0=90 +k=2` 是
     k=1 的两倍且**不**拒收)。改为 hasLatTs 时才走 effectiveK,那条"lat_ts≠90
     且 k≠1 拒收"的守卫也只在显式给 lat_ts 时生效。
  - 这两条都在**球体**与**缺省参数**上 —— 正是 golden/sweep 的模式盲区
    (与 C#15 的 stere 非极轴反向 bug 同源:fixture 每个投影只覆盖一种模式)。
  - 覆盖率的推进手段:prolate(f<0)大地线参照(pyproj 的 Geod 支持 f<0,补了
    79×2 反解 + 79×2 正解,Δs12 ≤ 3.7e-9 m —— 那些 f<0 分支终于有金标了);
    亚米级短线与极点正解(压 inverseStart 的 shortline / position 的 cbet2==0);
    新增 `coverage_extra_test.go`(WKT 词法错误 13 例、各投影域外、内核 b>a 夹
    e²、构造器第二道守卫、nil 守卫、矩阵奇异兜底、球体 tmerc/stere 全方面)。
  - 剩余 38 条(1.4%):geodesic 12(Newton 二分兜底、赤道退化、角度边界)、
    stere 7(内核 NaN 守卫 + 迭代不收敛)、geolib 3、laea/tmerc/wkt 各 3、
    aeqd 2(大地线不可能失败)、lcc/crs/merc/transform 各 1-2。全是"要么输入
    构造不出来,要么只能白盒直接调内部函数"的分支。


- 2026-10-22 **[goproj 二次评审] 融合 IsGeographic/Units/Equals(用户:"本地还有个
  goproj 是以前写一半的,看看有没有用")**。C#14 那轮已融合 Info() 与 DomainError,
  这次把 `C:/Users/64517/GolandProjects/goproj`(617 行 cgo 包装)重读了一遍,
  按"现在有没有用"重新过:
  - **新融合 3 项**:
    ① `CRS.IsGeographic()` + `CRS.Units()` —— 对应 goproj 包的
       `proj_angular_input/proj_degree_input/proj_angular_output/proj_degree_output`
       那组 PROJ 查询。本库收发约定固定,不需要那四个函数,但调用方(simple-sat 的
       geom)需要机器可读地知道"喂度还是米"。
    ② `CRS.Equals()` —— pyproj 有 `CRS.equals`,本库一直没有;goproj 没实现
       (只注释了 proj_is_equivalent_to)。口径比 pyproj 宽松:只比"算出的坐标是否
       一致",不比 datum/名称元数据(本库不建模),注释里写明了这个差异。
    ③ 顺带把 `paramOr` 的缺省表补上(to_meter 缺省 1,否则 `+units=m` 与不写单位
       会被判不等)。
  - **仍然不做**(与 C#14 结论一致,理由更新):4D 坐标(goproj 的 `Trans(lat,
    long, zt...)`;本库 2D 契约够用,且它把 z 也当角度转弧度了 —— 见下)、
    LogLevel、Area(无 datum 库可选区)、Type 枚举(CRS 分类用 IsGeographic 就够)、
    `CreateCrsToCrs(source, target string)`(等价于 Parse×2 + NewTransformer,
    不必再开一个入口)。
  - **goproj 里不该抄的 6 处**(记账,免得以后回头参考时踩):
    ① `Trans` 里 `DDToR(z)` —— 高程是米,转弧度是错的;
    ② `NewGoProjFromMap` 用 map 拼 proj 串 → 参数顺序随机、无校验;
    ③ `proj_create` 失败不判 nil,后面直接解引用;
    ④ 每点 `log.Println` + `spew.Dump`,且 `checkCoord` 失败只打日志;
    ⑤ `Trans` 拿写锁(`sync.RWMutex.Lock`)却只读状态,把所有调用串行化;
    ⑥ `CreateCrsToCrs` 里 `&area.area` 不判 nil。
  - 末态:测试 160 → 162,覆盖率仍 98.6%,全绿。


- 2026-10-22 **[对照 PROJ 的功能盘点 + 大样本差分] 发现 DESIGN §5 两处 GRIB 模板映射写错**。
  用户说"对比 proj 源码看看还缺什么,没问题就测一下"。先纠正前提:
  `GolandProjects/proj-master` 不是 C 版 PROJ 源码,是 **go-spatial/proj**(PROJ 5 的
  Go 移植,只做了 aea/aeqd/airy/august/eqc/etmerc/merc 七个),参考价值有限。
  于是改用 pyproj 里的 **PROJ 9.8.1** 做基准,逐个试建 proj 串枚举出 ≥84 个投影名。
  - 投影覆盖:本库 10 个 vs PROJ ≥84 个。按气象相关性排序的缺口见下(P0 只有一项)。
  - **P0(会影响数据正确性)**:`ob_tran`(旋转极点)。GRIB2 模板 3.1/3.3 是旋转
    经纬网格(ECMWF/ICON/区域模式常见),本库没有该投影,Parse 会报
    "未知 +proj=ob_tran"。**更严重的是 DESIGN §5 把模板 3.1 映射成 `+proj=eqc`**
    —— 旋转网格当普通经纬网格投出来会整体错位,而且是静默的。
  - **DESIGN §5 第二处错**:模板 3.20 注释成 "(AE) 方位等距 → +proj=aeqd"。
    WMO 表 3.1 里 **3.20 是极射赤面(polar stereographic)**,正确映射是
    `+proj=stere +lat_0=±90 +lat_ts=LaD +lon_0=LoV`(正是 golden_stere 那组);
    方位等距是 3.110。DESIGN 是冻结文档,不改,记在这里;下游 simple-sat 实现
    crsFromGridDef 时按本条走。
  - P1(常见但非必须):`aea`/`eqdc`(区域圆锥)、`omerc`/`somerc`、`ortho`/`nsper`
    (极轨卫星视角)、`sinu`/`moll`/`robin`/`igh`/`eck*`(全球出图投影)。
  - P2(基础设施):`Geod` 公开 API(距离/方位/面积;Karney 求解器已有,面积 C4 未移植)、
    `+proj=geocent`(transform.go 内部有 geodetic↔geocentric,未暴露)。
  - 明确出界(不打算做):pipeline 文本语法、网格基准(hgridshift/vgridshift)、
    molodensky、3D/4D、proj.db/完整 EPSG、CRS 元数据(名称/datum/ID)。
  - **大样本差分**(临时把 sweep 的 N_RANDOM 从 24 提到 300,跑完还原):
    22 个 case × 312 点 ≈ 6.9k 点,forward 最大 2.15e-5 m(stere,已知特性)、
    inverse 最大 2.8e-12 度,全绿,无新问题。


- 2026-10-22 **[ob_tran 落地] 补上唯一的 P0 缺口(用户:"需要")**。
  `impl_obtran.go`(约 150 行)+ 接入 Parse/单位层/Info + 对拍。
  - **口径来源**:网络抓不到 ob_tran.cpp(curl/urllib 都被 TLS 拦),于是拿
    pyproj 反推并用 7 组参数 × 192 点验证。最终公式(旋转是绕 Y 轴的正交变换,
    逆变换即其转置):
    ```
    λ' = λr − o_lon_p
    sin φ = sin φp·sin φ' + cos φp·cos φ'·cos λ'
    λ     = lon_0 + atan2(cos φ'·sin λ', sin φp·cos φ'·cos λ' − cos φp·sin φ')
    ```
    踩坑:atan2 的分母符号与 CF/教科书相反 —— PROJ 的约定下**旋转北极落到
    (lon_0+180, o_lat_p)**。照抄教科书写法会让整幅网格沿经线错 180°(静默)。
  - **GRIB2 模板 3.1 的换算**(写进 README 与测试注释):模板给的南极点
    (LaSP, LoSP) → `+o_lat_p=-LaSP`、`+lon_0=LoSP`(**不是** LoSP+180)。
    用 ECMWF 的 (39.25N, 162W) 锁死:南极点 (−39.25, 18) → 旋转北极必须
    正好落在 (−162, 39.25)。这条同时**关闭**了上一轮记的 DESIGN §5 错误:
    3.1 不该映射成 `+proj=eqc`,下游 simple-sat 的 crsFromGridDef 现在有正确落点。
  - **两处有意收紧**(README「已知差异」第 3 条):
    ① PROJ 对 `+o_lat_p=95` 照算(实测给出 lat=−5 的无意义结果),本库报错;
    ② PROJ 允许 `+o_proj=<任意投影>`,本库只支持 `longlat` —— 实测
       `+o_proj=merc +lon_0=100` 时内层中央经线是 0(即 lon_0 被旋转吃掉),
       其余参数归属要逐项核对,不猜。遇到直接报错并指向 NewTransformer。
    另:PROJ 的 `+o_alpha`/`+o_lat_c`/`+o_lon_c`/`+o_lon_w`/`+o_lat_1..2`/
    `+o_lon_1..2` 那套"中心点+方位"参数化未实现,Parse 阶段点名拒收。
  - **单位层**:ob_tran 输出是度,`+units` 不生效(pyproj 实测)。新增
    `CRS.angularOutput()`,让 `NewProjection` 的单位装饰器、`Units()`、`EPSG()`
    三处统一按"角度输出"处理;`IsGeographic()` 仍为 false(旋转系不是地理坐标系)。
  - **对拍**:`xcheck/gen_obtran.py` → `obtran_ref.json`,7 组(含 np30、
    o_lon_p=40、lon_0=170、ECMWF、南半球、o_lat_p=90 退化、斜轴 55.5°)×
    (83 正解 + 109 反解),正反解最大偏差 **1.3e-13 度**;另有全域网格往返
    (25×35 点)与批量一致性。基准 1e6 点 ~87 ms(在 100 ms 预算内)。
  - 测试 162 → 172,覆盖率 **98.6%** 持平。剩余 P0 缺口清零,下一档是 P1
    (aea/eqdc、omerc/somerc、ortho/nsper、sinu/moll/robin/igh/eck*)。

## HANDOVER(下个会话可直接接)

1. **B 轨全部结案**:五投影(stere/laea/lcc/merc/tmerc)+ geolib + golden 生成器 +
   fixture + xcheck/README.md 全部落地并 golden 绿;Day4 合流由 A 完成
   (main → 50f53db,tag `v0.1.0-projsubset`),跨轨事项 C#1/C#2/C#8/C#9/C#10/C#11
   全部关闭。
2. 全仓遗留(非 B 专属):`go test -race` 待 linux CI(windows 无 CGO);
   geos bench ~200 ns/点超 T8 预算的 Skip 仲裁(A 留的人工复测项)。
3. 工具环境提醒:`go` 不在 PATH,用 `C:\Users\64517\go\go1.27.1\bin\go.exe`;
   **pyproj 在系统 Python 里**(`C:\Users\64517\AppData\Local\Programs\Python\Python313\python.exe`),
   默认的 `python`/`pip` 是 WorkBuddy 托管 Python,没有 pyproj。
4. 后继 simple-sat 按 DESIGN §5 / README「公共 API 契约」接入本库。
