# FORMULA-NOTES.md — 公式出处考据

> Owner: AI-B (COORDINATION §1). 每个投影一节:出处 + 实现选择 + 对拍结论。
> 只参考公式(Snyder 是 public domain;PROJ 源码思路可借鉴,代码一律重写 —— COORDINATION §6 红线 2)。

---

## 0. geolib.go(共享球面/椭球数学)

**出处**:Snyder, *Map Projections — A Working Manual*, USGS PP 1395, ch. 1
(isometric latitude / conformal latitude / Gudermannian);PROJ 对应 `pj_tsfn` / `pj_msfn` / `pj_qsfn`。

- `ψ(φ) = ln tan(π/4 + φ/2) + (e/2)·ln((1 - e sinφ)/(1 + e sinφ))` — 等积纬无关,这是**等角**用的等比纬(isometric latitude)。
- `X = gd(ψ)` — 共形纬(conformal latitude)。
- `exp(-ψ) = tan(π/4 - X/2)` — **PROJ 在 stere 极冠分支里用的就是这个值**。

**踩坑记录(值得留档)**:把 `exp(-ψ)` 写成常见的 `tan(π/4 - φ/2)·((1-e sinφ)/(1-e... ))` 形式时,幂因子符号容易写反。
用 `tan(π/4-φ/2)·((1-e sinφ)/(1+e sinφ))^{e/2}` 在 lat=80 处与 pyproj 差 **83 km**;用 `exp(-ψ)` 则逐位一致(相对误差 3.5e-14)。
所以 geolib 里 `ExpNegIsometric` 一律走 `exp(-IsometricLat(...))`,不做"化简"。

`LatFromIsometric` 用 Newton:`dψ/dφ = (1-e²)/(cosφ·(1-e² sin²φ))`,初值取共形纬,4 步内收敛到 1e-13。

### 0.1 等积纬(authalic latitude)

**出处**:定义是"带面积相等":椭球上赤道到大地纬 φ 的带面积 = 等积球上赤道到 μ 的带面积。
带面积元 `dA = 2π r ds`,`r = N cosφ`,`ds = M dφ`,被积函数是 `(1 - e² sin²t)^-2` —— 注意是 **2 次幂**,
不是子午线曲率半径的 3/2 次幂。这就是系数里 `(i+1)` 的来源。

```
C_i = (i+1)/(2i+1) · e^{2i}
q(φ) = 2(1-e²) Σ C_i sin^{2i+1} φ
sin μ = q(φ) / q(π/2)
```

**踩坑记录(值得留档,这一条花掉一整个调试时段)**:常被转录成 `C_i = ((2i-1)!!/(2i)!!)/(2i+1)` 的是
**子午线弧长**级数(被积函数 `(1-e² sin²t)^{-3/2}`),不是带面积级数。用错系数后 `authalic(45°) = 44.9812°`
而不是 `44.8717°`,laea forward 差 **6.4e6 m**。Snyder 的直接级数 `μ = φ - (e²/3)sin³φ - (3e⁴/45)sin⁵φ - ...`
给出第三个值 `44.9548°`。只有 `(i+1)/(2i+1)` 形式与 PROJ/pyproj 一致。

- 项数:PROJ 用 `P->n`;实测 n ≥ 8 才与 golden 一致(n=6 差 1.3e-6 m,n=8 起收敛到 6.1e-7 m)。
  `geolib` 固定取 `AuthalicMaxTerms = 12`,对 fixture 里所有椭球都已收敛。
- 反解:Newton on `f(φ) = q(φ) - qp·sin μ`,`f'(φ) = 2(1-e²)cosφ Σ (i+1)e^{2i} sin^{2i}φ`。
  注意求和用两张系数表(q 用 `C_i`,导数用 `(i+1)e^{2i}`)——写错这张表反解会偏 0.25°。
- `e² = 0` 时 `q(φ) = 2 sinφ`,`qp = 2`,`rq = 1`,`dd = 1`,laea 自动退化成球面公式(已用 pyproj 验证)。

---

## 1. stere(已实现,对拍通过)

**出处**:Snyder PP 1395, ch. 13(Azimuthal projections),椭球版 polar / general aspect;
PROJ 9.8.1 `src/projections/stere.cpp`(mode 枚举 `S_POLE / N_POLE / OBLIQ / EQUIT`)。

实现结构(与 PROJ 一致,代码重写):

- mode 选择:`|lat_0| = 90` → 极冠;`lat_0 ≠ 0` → OBLIQ;`lat_0 = 0` → EQUIT。用 `EPS10 = 1e-10`。
- `akm1`(椭球):
  - 极冠 + `lat_ts = 90`:`akm1 = 2k₀ / √((1+e)^{1+e}(1-e)^{1-e})`
  - 极冠 + `lat_ts ≠ 90`:`akm1 = cos(φs) / exp(-ψ(φs)) / √(1 - (e sin φs)²)`
  - OBLIQ/EQUIT:`akm1 = 2k₀ cos(φ₀) / √(1 - e² sin²φ₀)`
- Forward(极冠,`λ = lon - lon₀`):`ρ = a·akm1·exp(-ψ(φ))`,`x = ρ sinλ`,`y = -ρ cosλ`
  (S_POLE 先把 φ 和 cosλ 取反再走 N_POLE 分支 —— 与 PROJ 的 fallthrough 等价)。
- Inverse(极冠):`tp = -ρ/a / akm1`,`φ = 2 atan(tp·((1+e sinφ)/(1-e sinφ))^{-e/2}) + π/2`
  的不动点迭代(8 步,CONV=1e-10);`λ = atan2(x, -y)`(N_POLE;y 取反是 PROJ 的做法)。
  直接走 `ψ = -ln(ρ/(a·akm1))` + `LatFromIsometric` 与 PROJ 迭代等价(已验证)。

**对拍结论**(pyproj 3.8.0 / PROJ 9.8.1,`xcheck/golden_stere.json`,1099 forward + 1090 inverse):

- forward 最大绝对差 **4.5e-4 m**,出现在 `(lon=0, lat=-89.9)`(坐标量级 1.3e10 m);相对差 **3.5e-14**。
  → 公式是双精度精确的,1e-6 m 的绝对容差在超大坐标上不可达,见 PROGRESS-B.md **C#3**。
- inverse 最大绝对差 **1.6e-11 deg**,在 1e-9 容差内。
- 定义域:pyproj 在 `(9e9, -9e9)` 处**不报错**,返回 `lon=-55, lat=-89.89391267552834`。
  即椭球版极冠 stere 没有硬定义域(只有迭代不收敛才报错)。golden 里这条落在 `inverse` 而不是 `specials`。

**与 Snyder 的差异**:Snyder 的极冠椭球版用 `t = tan(π/4-φ/2)·(...)`,PROJ 用 `exp(-ψ)`;两者在 e>0 时不等。
本实现按 PROJ/pyproj 口径(golden 是权威),并在 geolib 里把两种形式分开命名,避免以后混用。

---

## 2. laea(已实现,对拍通过)

**出处**:Snyder PP 1395 ch. 13(Lambert azimuthal equal-area,general aspect);
PROJ 9.8.1 `src/projections/laea.cpp`(mode 枚举 `S_POLE / N_POLE / EQUIT / OBLIQ`)。
等积纬用 §0.1 的级数(不是 `pj_qsfn`——`pj_qsfn` 是 `q(φ)` 的另一种写法,PROJ 9 已经换成
`pj_authalic_lat_compute_coeffs`)。

实现结构(与 PROJ 一致,代码重写):

- mode 选择用 `EPS10 = 1e-10`;`|lat_0| = 90` → 极冠,`lat_0 = 0` → EQUIT,否则 OBLIQ。
- 常数:`qp = q(π/2)`,`rq = √(qp/2)`,
  - EQUIT:`dd = 1/rq`,`xmf = 1`,`ymf = qp/2`
  - OBLIQ:`dd = cosφ₀ / (√(1-e² sin²φ₀) · rq · cos μ₁)`,`xmf = rq·dd`,`ymf = rq/dd`
  - 极冠:`dd = 1`
- Forward(OBLIQ):`b₀ = 1 + sinμ₁ sinμ + cosμ₁ cosμ cosλ`;`b₀ ≤ EPS10` → 定义域外(中心的对跖点);
  `b₀ = √(2/b₀)`,`x = a·xmf·b₀·cosμ·sinλ`,`y = a·ymf·b₀·(cosμ₁ sinμ - sinμ₁ cosμ cosλ)`。
- Forward(极冠):`q = qp ∓ q(φ)`(N_POLE 减,S_POLE 加),`ρ = a√q`;`x = ρ sinλ`,`y = ∓ρ cosλ`。
  `q < 1e-15` 时 PROJ 直接返回 (0,0),本实现照抄。
- Inverse(OBLIQ):`x/a/dd`,`y/a·dd`,`ρ = hypot`,`sCe = 2 asin(ρ/(2rq))`;
  `ab = cos(sCe) sinμ₁ + y' sin(sCe) cosμ₁/ρ`;
  **`y` 必须在 atan2 前重算**:`y₂ = ρ cosμ₁ cos(sCe) - y' sinμ₁ sin(sCe)`,
  `λ = atan2(x' sin(sCe), y₂)`,μ = asin(ab),φ = 反解(§0.1)。
  这一行漏掉经度就错(我第一版就是漏了它,`golden_laea` 的 inverse 全红)。
- Inverse(极冠):N_POLE 先把 y 取反,`ab = 1 - ρ²/qp`(S_POLE 取反),`λ = atan2(x, y)`。

**+k_0 不参与**:pyproj 实测 `+k_0=2` 与 `+k_0=1` 输出逐位相同 —— PROJ 的 laea 不用 k_0。
§2.3 的冻结签名里 laea 也没有 k0 参数,一致。适配器因此不乘 k_0。

**对拍结论**(pyproj 3.8.0 / PROJ 9.8.1,`xcheck/golden_laea.json`,1098 forward + 1089 inverse + 2 specials):

- forward 最大绝对差 **2.97e-7 m**(在 1e-6 m 内),inverse 最大绝对差 **1.38e-12 deg**(在 1e-9 deg 内)。
- 定义域:中心的对跖点 `(-170, -45)` → `1 + cos(z) ≤ EPS10` → ErrDomain ✓。
- 极点分支(本 fixture 不覆盖)单独用 pyproj 校过:`+lat_0=90` 下 `(0,45) → (0, -4889334.802955)`,
  `+lat_0=-90` 下 `(0,-45) → (0, 4889334.802955)`,`+lat_0=0` 下 `(0,45) → (0, 4857538.576747)`。
- 病态点:`(-170, -44.9)` 处 `1 + cos(z) ≈ 1.5e-6`,`cos` 的 1e-16 绝对误差放大成 7e-11 相对误差,
  本实现与 pyproj 差 4.6e-4 m;pyproj 自己往返也回不到 1e-9 deg(它返回 -44.899998958)。
  → 见 PROGRESS-B.md **C#7**。


## 3. lcc(已实现,对拍通过)

**出处**:Snyder PP 1395 ch. 14;PROJ 9.8.1 `src/projections/lcc.cpp`(本轨拉了源码逐字核对)。

推导(不背 Snyder 公式,从共形条件直推):把映射写成极坐标形式 (ρ(φ), n(λ−λ0)),
共形条件 ⟺ dρ/dφ = −n·ρ·M/(N·cosφ),而 M/(N cosφ) 的不定积分正是等角纬 ψ
(geolib.IsometricLat),故:

```
m(φ) = cosφ / √(1 − e² sin²φ)          (pj_msfn)
t(φ) = e^(−ψ(φ))                       (pj_tsfn = geolib.ExpNegIsometric)
n   = ln(m1/m2) / ln(t1/t2)             (secant; 切锥 |lat1−lat2|<EPS10 → n = sin φ1)
C   = a·k0·m1·e^(n·ψ1) / n
ρ(φ) = C·e^(−n·ψ(φ)),  ρ0 = ρ(φ0)
x = ρ·sin(n·Δλ),  y = ρ0 − ρ·cos(n·Δλ)   (Δλ = adjlon(λ−λ0))
inverse: ρ = hypot(x, ρ0−y)  (n<0 时三者先取反), ψ = ln(C/ρ)/n, φ = LatFromIsometric(ψ), λ = atan2(x, ρ0−y)/n
```

- **注意 pj_tsfn = e^(−ψ)**,不是 e^(+ψ);符号搞反 n 和 ρ0 会翻符号。
- **南锥(n<0)不是另一套公式**:n 的符号从 ln(m1/m2)/(ψ2−ψ1) 自然流出,ρ0 同号;
  只有 inverse 要先取反 (x, y, ρ)——PROJ `lcc_e_inverse` 原样如此(否则 ρ/c < 0 取对数出 NaN)。
- **PROJ 的极点规则**:φ·n > 0 的极点是锥尖 → (0, ρ0);另一极点是定义域外
  (PROJ_ERR_COORD_TRANSFM_OUTSIDE_PROJECTION_DOMAIN)。pyproj 默认 errcheck=False,
  对外显示 (inf, inf),别被误导——我们内核返 NaN,适配层转 ErrDomain。
- **守卫与 PROJ 逐字一致**:`|lat_1 + lat_2| < EPS10` 拒收(lat_1 = −lat_2 → n=0)、
  `|lat_1|,|lat_2| < 90°` 拒收;`+k_0` 缩放 ρ。
- **坑:经度必须弧度制归约**。θ = n·Δλ 对 Δλ 不是 2π 周期(n≠1)。golden_lcc 第一个点
  Δλ = −280°,必须归约成 +80° 才对得上——原来没归约,forward 差 5000 万米级。
  geolib 新增 `geoAdjLonPi`((−π,π],PROJ 管线 adjlon 顺序)。
- **与 §2.3 契约的偏差(记录在案)**:PROJ 9.8.1 对"只给 +lat_ts 的 lcc"直接拒收
  (lat_1 默认 0 → |lat_1+lat_2| 守卫);按 §2.3 单纬线约定,适配层把 lat_ts 读成切锥
  (lat1 = lat2 = lat_ts)。golden 不覆盖该路径。
- PROJ 9.8.1 的 `lat_2` 缺失分支行为无法复现(既非 phi0=0 也非 phi0=lat_1,怀疑与
  proj.ini/上下文默认有关),本实现统一 lat_2 缺省 = lat_1、phi0 缺省 = 0,并记录于此。

**对拍结论**(golden_lcc.json,1099 fwd + 1090 inv):forward 最大 4.84e-5 m(坐标 ~9e7 m,
相对 5.4e-13,C#3 相对容差内),inverse 最大 2.56e-13 deg。切锥/南锥/球面退化用 pyproj
参考点单独校过(lcc_test.go: TestLccTangentAndSouthern / TestLccSphere)。

## 4. merc(已实现,对拍通过)

**出处**:Snyder PP 1395 ch. 7;PROJ 9.8.1 `src/projections/merc.cpp`。

```
factor = |lat_ts|>0 ? msfn(lat_ts) : 1        (msfn = cosφs/√(1−e² sin²φs);球面 = cosφs)
x = a·factor·Δλ,   y = a·factor·ψ(φ)
ψ = asinh(tan φ) − e·atanh(e·sin φ)            (PROJ 稳定形式;e=0 → asinh(tan φ))
inverse: ψ = y/(a·factor), φ = LatFromIsometric(ψ);  λ = x/(a·factor) + λ0
```

**比例因子口径(day0 笔记写反了,本轨用 pyproj 重探,以此为准)**:

- `+lat_ts` 一旦给出(**值是多少都行,含 0**):factor = **msfn(φs)**,且 **`+k_0` 被彻底忽略**
  (PROJ 是*替换* k0,不是相乘——pyproj 实测 k0=1/2/3 输出逐位相同)。
- `+lat_ts` 缺省:factor = `+k_0`。
- `|lat_ts| = 90` 拒收(msfn 退化为 0);`|lat| > 90` 我方输入约定报错;`|lat| = 90` 是域内点,
  y 有限(见下)。

**正解必须用 asinh 形式**:`asinh(tanφ) − e·atanh(e·sinφ)` 与
`ln tan(π/4+φ/2) + (e/2)·ln((1−e sinφ)/(1+e sinφ))` 数学等价,但 φ = π/2(双精度)时
前者给 38.0250、后者给 37.3319。golden_merc 的极点记录 `(0, 90) → y = 242528680.94374272`
(= 38.025003373828866·a)锁的是前者。geolib.IsometricLat 保留 ln-tan 形式(laea/stere 的
golden 已按它绿),merc 内核因此自带 `mercIsometricLat`,不污染共享库。

**极点/极端值**:lat=±90 → y = ±38.0250·a(有限,与 pyproj 一致);(1e12,1e12) inverse →
λ 的弧度值 ~1.6e5,ulp ~1.7e-9 deg——inverse 容差需要相对下限(见 C#9)。

**对拍结论**(golden_merc.json,1099 fwd + 1090 inv):forward 最大 5.96e-8 m, inverse 最大
1.98e-9 deg(C#9 相对容差内含 1e12 极端探针)。椭球 lat_ts 各值、k0 忽略、x_0/y_0 移位、
EPSG:3857 别名均用 pyproj 参考点单独校过(merc_test.go)。

## 5. tmerc(已实现,对拍通过)

**出处**:PROJ 9.8.1 `src/projections/tmerc.cpp`(Poder/Engsager "exact"/etmerc 路径)+
`src/latitudes.cpp`(辅助纬度级数,Maxima 表);数学根源 Koenig & Weise / Engsager-Poder
(ICC2007)。**不是** Karney 2023 的椭圆函数精确版,也**不是** DESIGN §6.3 预设的自研
Krüger 6 阶——PROJ 9 的默认 tmerc(auto)在 |Δλ|>3° 时就走这套 6 阶精确序列,直接对齐它。

结构(归一化坐标里的 Gauss-Schreiber 复合):

```
正: φ --(cbg)--> 高斯纬 χ --> 复球面 (atan2/asinh) --> clenS(gtu) --> 归一化 (Cn, Ce)
    x = Qn·Ce,  y = Qn·Cn + Zb
反: Cn = (y−Zb)/Qn, Ce = x/Qn; clenS(utg); Ce = atan2(sinhCe, cosCn);
    Cn = atan2(sinCn, hypot(sinhCe, cosCn)); φ = auxlat(cgb, Cn, sinCn/rr, mod/rr); λ = Ce
Qn = k0·a·pj_rectifying_radius(n)   (n = (a−b)/(a+b) 第三扁率)
Zb = −Qn·auxlat(gtu, auxlat(cbg, φ0))
域: |Ce| ≤ 2.623395162778 rad(~150°),双向
```

- auto 调度(|Δλ|>3° 精确 / 否则 Evenden-Snyder)在 fixture 上**不触发**:网格步长 11.25°,
  所有点 |Δλ| ≥ 5°;两路在 3° 边界一致到 ~1e-9 m,故椭球路径恒定走精确序列。
- **系数块标签是 `C[输出, 输入]`**(latitudes.cpp 的注释排版):`cgb`(GEO→GAUSS,φ→χ)用
  标着 `C[chi,phi]` 的块(coeffs[87:108]),`cgb`(GEO→GAUSS)与 `cbg`(GAUSS→GEO)、`utg` 与 `gtu` 两对容易装反。
  装反的判据:φ→χ 首项应为 **−2n**(45° 的共形纬 ≈ 44.8°,小于大地纬),不是 +2n。
- 四周转换全走"n 的奇次幂"分支(F[l] = n^l·polyval(n, block, 5−l));
  `pj_auxlat_coeffs` 的 `ptrs[]` 索引是 `6·auxout + auxin`。
- **inverse 别丢实部**:PROJ 写 `Cn += clenS(utg, …)`——clenS 的**返回值**(实部)要加进
  Cn,指针参数拿的是虚部 dCe。只加 dCe 的话往返 lat 偏 0.05°。
- 球面(a==b)PROJ 强制走 `tmerc_spherical_*`(exp 型 x = k0·atanh(cosφ sinΔλ)、acos 型 y),
  且框架统一乘 `P->a`——我们内核自产米制,球面分支要显式 ×a、inverse 要 /a。
- 经度同样要 `geoAdjLonPi`;inverse 返回的是相对 λ0 的 Δλ,适配层 `adjlon(λ+λ0)` 后
  再转度(顺序反了会在 1e12 m 级坐标上丢 1e-9 deg 以上的精度)。
- **fixture 的 `+k=` 别名**:A 的 Parse 把 k/k0 归一到 `k_0`,但 B 测试侧的
  `goldenParams` 没有别名表,golden_tmerc 读的是 `p["k"]=0.9996`——测试自己回退。

**对拍结论**(golden_tmerc.json,1091 fwd + 1081 inv + 9 specials):forward 最大 **7.45e-9 m**,
inverse 最大 **3.13e-13 deg**;specials(Δλ 82.5°~97.5° 的前向 + (9e9,−9e9) 反向)全部
按 |Ce| 域界正确 NaN。球面分支、k0/x_0/y_0/lat_0≠0 变体用 pyproj 参考点单独校过
(tmerc_test.go: TestTmercSphere / TestTmercVariants)。
