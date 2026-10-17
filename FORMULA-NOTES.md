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


## 3. lcc(待写)

出处:Snyder PP 1395 ch. 14(Conic projections,Lambert conformal conic);PROJ `src/projections/lcc.cpp`。
`m` / `n` 由 `lat_1`,`lat_2`(或 `lat_ts`)推出;`lat_1 = lat_2` 或 `lat_ts` 时退化为单标准纬。

## 4. merc(待写)

出处:Snyder PP 1395 ch. 7(Mercator);PROJ `src/projections/merc.cpp`。
`y = a·(1 - e²)... ` 形式即 `a·(gd⁻¹(φ) + (e/2)ln((1-e sinφ)/(1+e sinφ)))` —— 正好是 geolib 的 `IsometricLat`,
所以 merc 的 kernel 直接复用 geolib,不另写公式。

## 5. tmerc(待写)

出处:Snyder PP 1395 ch. 7(Transverse Mercator,级数解);PROJ 9 用 Krüger 级数(`src/projections/tmerc.cpp`,
`Tmerc` 结构里的 `c`/`d` 系数表)。气象尺度先做 6 阶(误差 <1 mm @ <1000 km,DESIGN §6.3);
若要与 PROJ 逐位对拍,需要 Krüger 10 阶或 Karney 2023 的 `TransverseMercator`。
golden 的 tmerc case 会决定截断阶数 —— 先按 6 阶跑,超差再升阶。
