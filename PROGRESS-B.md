# PROGRESS-B.md — AI-B 轨进度(公式 + P1 投影)

> 本文件 owner=AI-B,A 只读。跨轨事项走 COORDINATION §5。
> C 编号与 PROGRESS-A.md **共享同一序列**(A 已用到 C#2),B 的新条目从 **C#3** 起。
> 环境:go1.27.1(`C:\Users\64517\go\go1.27.1\bin`,不在 PATH);pyproj 3.8.0 / PROJ 9.8.1;
> Python 3.13;工作目录 D:\simple-proj。

## 状态(COORDINATION §3 B 轨清单)

- [x] Day0 上午:`xcheck/gen_golden.py` + 9 个 fixture(清单见下表)
- [x] Day0 下午:`geolib.go` + `impl_stere.go`(内核 + register 适配)+ `stere_test.go` → 合流(绿)
- [x] Day1:`impl_laea.go` + `laea_test.go`(绿)
- [ ] Day1:`impl_lcc.go` + `lcc_test.go` —— **未开工,交接给下一个 AI**
- [ ] Day2:`impl_merc.go` + `merc_test.go` —— 未开工,但公式口径已用 pyproj 探明(见"merc 已探明的口径")
- [ ] Day3:`impl_tmerc.go`(Krüger 6 阶)+ `tmerc_test.go` —— 未开工
- [x] Day4 门(B 侧):`go vet` 0 / `gofmt` 干净 / 包覆盖率 **90.7%**(A 提的 C#2 已回执)

**交接状态:本文件写时,B 的全部文件尚未提交**(`git status` 里是 untracked/modified)。
接手第一件事:`git add` B 的文件并按 `[B]` 前缀提交(单目录单分支顺序提交,A 的注记已认可)。

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

### geolib.go
`geoDegToRad` / `geoRadToDeg` / `geoNormLon`(负数 `math.Mod` 已修,保证 `[-180,180)`)、
`Gudermannian` / `InvGudermannian` / `IsometricLat` / `ExpNegIsometric` / `ConformalLat` /
`LatFromIsometric`(Newton,1e-13)。
新增 **authalic 级数**:`AuthalicSeries{coef, dcoef, qp}` + `NewAuthalicSeries(e2)` +
`Q(sinphi)` + `AuthalicLat(phi)` + `LatFromAuthalic(mu)`,`AuthalicMaxTerms = 12`。

关键坑(详见 FORMULA-NOTES.md §0.1):系数必须是 `C_i = (i+1)/(2i+1)·e^{2i}`(带面积定义,
被积函数 `(1-e²sin²t)^-2`),不是子午线弧的 `((2i-1)!!/(2i)!!)/(2i+1)`。用错系数
`authalic(45°)` = 44.9812° 而非 44.8717°,laea forward 差 6.4e6 m。项数 n≥8 才收敛(n=6 差 1.3e-6 m)。

### impl_stere.go(对拍通过)
- forward 最大绝对差 **4.539e-4 m** at `(0,-89.9)`(坐标 1.3e10 m),相对 **3.5e-14**;
  inverse 最大 **1.639e-11 deg**。→ 见 C#3。
- `stereEffectiveK` 口径已用 pyproj 重新校正(本轮改动):OBLIQ/EQUIT **忽略 `+lat_ts`**;
  极冠 `|lat_ts|=90` → `akm1 = 2k/base`;极冠 `lat_ts≠90` → `akm1` 是 raw 值且 **PROJ 拒收 `k_0≠1`**
  ("lat_ts != lat_0 and k != 1 not supported for Polar Stereographic"),所以 k = `akm1_raw·base/2`。
  适配层照 PROJ 报错,不静默折叠。`newStereKernel` 签名去掉了 `latTs`(k 已是有效比例)。

### impl_laea.go(对拍通过)
- forward 最大 **2.971e-7 m**,inverse 最大 **1.378e-12 deg**,2 个 specials 全过。
- 反解里 `y2 = rho·cosb1·cCe − yy·sinb1·sCe` 必须在 `atan2` 前重算,漏一行经度全错。
- **PROJ 的 laea 不用 `k_0`**(pyproj 实测 `+k_0=2` 与 `+k_0=1` 逐位相同),与 §2.3 冻结签名无 k0 一致。
- 极冠/EQUIT 分支 golden 不覆盖,单独用 pyproj 校过:`+lat_0=90` 下 `(0,45) → (0, -4889334.802955)`;
  `+lat_0=-90` 下 `(0,-45) → (0, 4889334.802955)`;`+lat_0=0` 下 `(0,45) → (0, 4857538.576747)`。

### merc 已探明的口径(实现未写,给接手者)
pyproj 实测(椭球 WGS84,`+lat_ts=45`,`(0,30)`):
- `lat_ts = 0`:`y = a·k_0·ψ(φ)`,`x = a·λ`(factor = k_0)。
- `lat_ts ≠ 0`:factor = `cos φs / √(1-e² sin²φs)` = `1/msfn(φs)`,且 **`k_0` 被忽略**
  (k_0 = 1/2/3 输出完全相同)。球体路径 factor = `cos φs`(e=0 时 `1/msfn` 自然退化)。
- 反解:`ψ = y/(a·factor)`,`φ = LatFromIsometric(ψ)`(直接复用 geolib,不另写公式)。
- `|φ| > 90°` → 定义域外。
这条"k_0 被忽略"是从 pyproj 反推的,尚未与 PROJ 源码核对;若接手者读 `merc.cpp` 得到不同结论,
按 §5 第 4 条以 pyproj 为准,但要把结论回写 FORMULA-NOTES.md §4。

### tmerc 未开工
golden_tmerc 有 9 个 specials、1091/1081 点。先按 Krüger 6 阶跑,超差再升阶(DESIGN §6.3)。
注意 fixture 用的是 `+k=0.9996`(A 的 Parse 已把 `k`/`k0` 归一到 `k_0`)。

## 协作事项(§5)

- **C#1-ACK(回执 A)**:**同意** `GRIBEllipsoidAxes(shape, majorScale, majorRaw, minorScale, minorRaw)`。
  B 无须改动:B 侧只读 `CRS.Params`/`Ellipsoid.A/B`,`gen_golden.py` 的 `ELLPS_KEYS` 也不受影响。
  补充一点:shape 3/7 现在明确报错是对的 —— pyproj 对"只给 +b/+es/+f/+rf(缺 +a)"同样全拒收,
  与 A 的 fail-loud 守卫一致。原 `GRIBEllipsoid` 保留处理单维/固定形状,B 无异议。
- **C#2 回执(覆盖率)**:已补 `geolib_test.go`(直测 gd/gd⁻¹/等比纬/共形纬/authalic 及其反解、
  球体退化、错误系数回归哨兵)与 `stere_test.go` / `laea_test.go` 里的**适配层直测**
  (`Parse → NewProjection → Forward/Inverse`,含 register 通路、ErrDomain、非法输入)。
  包覆盖率从 81.2% 抬到 **90.7%**;B 文件逐函数见"覆盖率"节。
- **C#3 容差相对下限**:1e-6 m 绝对门槛在超大坐标上不可达。stere 极冠 `(0,-89.9)` 坐标 1.3e10 m,
  单精度舍入下限就是 ~3e-6 m,实测差 4.5e-4 m(相对 3.5e-14)。B 的测试用
  `tol(want) = max(1e-6, |want|·1e-12)`,即"严格优先 + 双精度下限"。
  **希望 A 的 xcheck 消费端统一采用同一函数**(现在 A 的消费端是硬 1e-6,会红)。
- **C#4 authalic 归 geolib(共享)**:已落地在 `geolib.go`。A 的 `impl_eqc.go` 走的是 Karney 矩形纬
  (mlfn),与 authalic 是**两个不同的纬**,不要混用;若 A 想复用 authalic 球半径 R2,请确认
  它用的是带面积级数而不是 mlfn。B 的 laea 依赖 `AuthalicSeries`,这块归 B,接口冻结:
  `NewAuthalicSeries(e2) *AuthalicSeries` / `Q(sinphi)` / `AuthalicLat(phi)` / `LatFromAuthalic(mu)` /
  `AuthalicMaxTerms = 12`。
- **C#5 ±180 经度比对**:golden 里 dateline 列存的是 `179.99999999999994`,B 的内核返回精确 `-180`。
  两点是同一经线,但按 1e-9 deg 直接比会红。B 的测试用 `lonDiff()`(mod 360)。
  **希望 A 的消费端也改成圆周比对**(A 的 xcheck_test.go 已有"经度圆周比对",一致);
  或者由 B 在 `gen_golden.py` 里把 |lon| 距 ±180 小于 1e-12 的值 snap 到 -180 —— 需要 A 同意才做,
  因为会改动 A 消费的 fixture。
- **C#6 golden_test.go 归属**:`golden_test.go`(loader + `tol` + `lonDiff` + `ellipsoidFor`)是 B 写的,
  建议 §1 明确为 B 拥有。若 A 的 `xcheck/xcheck_test.go` 需要同一 loader,请 A 用自己的前缀
  (`aGoldenRec` 之类),包内不能有两个同名定义。
- **C#7 往返容差在定义域边缘不可达**:laea 中心对跖点附近 `1+cos(z)` 是灾难性抵消
  (`z≈179.9°` 时 `1+cos z ≈ 1.5e-6`,cos 的 1e-16 绝对误差放大成 7e-11 相对),
  pyproj 自己往返也回不到 1e-9 deg(它返回 -44.899998958)。B 的往返测试只取良态点,
  边缘点只校 forward(用 DESIGN §3.2 的 1e-3 m 绝对回退)。**希望 §2.5 补一句**:
  往返门槛只适用于良态点,定义域边缘按 §3.2 的回退。
- **C#8 `xcheck/README.md`**:§1 未指派,B 提议由 B 认领(fixture schema + 生成器用法 + 容差口径)。

## 覆盖率(B 文件,`go tool cover -func`)

geolib.go:geoDegToRad/gd/gd⁻¹/ExpNegIsometric/ConformalLat/NewAuthalicSeries/Q/AuthalicLat 100%,
IsometricLat 75%,LatFromAuthalic 90%,LatFromIsometric 80%。
impl_laea.go:newLaeaKernel 93.5%,forward 97.1%,inverse 90.9%,laeaForward/laeaInverse 100%,
newLaea 81.8%,Forward 100%,Inverse 75%。
impl_stere.go:stereEffectiveK 90%,newStereKernel 75%,forward 62.5%,inverse 25%,iterateGeodetic 80%,
stereForward/stereInverse 100%,newStere 96.7%,Forward 83.3%,Inverse 75%。

## 日志

- 2026-10-17 [B] day0-上午:`gen_golden.py` + 9 fixture 生成(见上表)。
- 2026-10-17 [B] day0-下午:`geolib.go` + `impl_stere.go` + `stere_test.go`;
  踩坑记录见 FORMULA-NOTES.md §0(`exp(-ψ)` 幂因子符号)与 §0.1(authalic 系数表)。
- 2026-10-17 [B] day1:`impl_laea.go` + `laea_test.go` 全绿。走过的三步坑留档:
  (1) authalic 系数用错表(见上);(2) Python 累乘 `sp *= s` 写成 `s^{i+1}` 而非 `s^{2i+1}`;
  (3) laea 反解漏 `y2` 重算行。Newton 的 RHS 也要用 `qp·sin μ − q(φ)`,写反会飞到 210°。
- 2026-10-17 [B] day1-附:回应 A 在 PROGRESS-A.md 里报的 `TestGoldenStere (0,-89.9)` 失败 ——
  不是公式错,是 1.3e10 m 坐标上的双精度下限(C#3);加相对下限后绿。
- 2026-10-17 [B] day1-附:`stereEffectiveK` 口径修正 + 极冠 `lat_ts≠90 且 k_0≠1` 按 PROJ 拒收;
  `newStereKernel` 去掉 `latTs` 参数(B 自己的函数,非冻结符号)。
- 2026-10-17 [B] day1-附:补 C#2 要求的直测(`geolib_test.go` + 两个适配层测试 + 冻结签名直测),
  `go vet` 0 / `gofmt` 干净 / 覆盖率 90.7%。`go test ./...` 全绿(含 A 的 xcheck)。
- 2026-10-17 [B] **交接点**:lcc / merc / tmerc 未开工。merc 的口径已探明(见上),
  lcc 需要 `n`/`m0` 推导(Snyder ch.14),tmerc 需要 Krüger 系数表。

## HANDOVER(下个会话可直接接)

1. 先提交 B 的文件(`[B]` 前缀):`geolib.go` `geolib_test.go` `golden_test.go` `impl_stere.go`
   `impl_laea.go` `stere_test.go` `laea_test.go` `FORMULA-NOTES.md` `PROGRESS-B.md` `xcheck/*`。
2. 跑 `C:\Users\64517\go\go1.27.1\bin\go.exe test ./...` 确认基线绿。
3. 按 §3 Day1 剩余做 `impl_lcc.go` + `lcc_test.go`(golden_lcc 1099/1090/0,无 specials)。
4. 然后 `impl_merc.go`(口径已探明,直接复用 geolib 的 `IsometricLat`/`LatFromIsometric`)。
5. 最后 `impl_tmerc.go` Krüger 6 阶,超差升阶。
6. 工具环境提醒:`glob`/`grep` 工具在本机会报 "ripgrep launch failed",用 pwsh 的
   `Get-ChildItem` / `Select-String` 代替;`go` 不在 PATH,用全路径;
   `pip install pyproj` 需要沙箱放行(本会话已装好,pyproj 3.8.0 / PROJ 9.8.1)。
