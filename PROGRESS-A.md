# PROGRESS-A.md — AI-A 轨进度(地基 + P0 投影)

> 本文件 owner=AI-A,B 只读。跨轨事项编号 C#x 在本文件独立递增(COORDINATION §5)。
> 参照:COORDINATION.md §3 日程;本文件是 A 轨唯一记账。
> 环境:go1.27.1(`C:\Users\64517\go\go1.27.1\bin`,系统 PATH 未含,构建需自带);
> git 2.45.1;工作目录 D:\simple-proj。

## 状态(COORDINATION §3 A 轨清单)

- [x] Day0 上午:git init + main 空提交(cc7c5f5);建分支 `ai-a`/`ai-b`;A 停在 `ai-a`
- [x] Day0 上午:契约首提交 —— §2 符号逐字落地(proj.go / crs.go / ellipsoid.go /
  gribshape.go / projstr.go),函数体骨架;同提交 .gitignore / PROGRESS-A.md
- [x] Day0 下午:ellipsoid.go 全量实现 + 单测(3d10229)
- [x] Day0 下午:gribshape.go(GRIB shape 1-11 → 椭球,平移 simple-grib 逻辑)+ 单测(3d10229)
- [x] Day1:crs.go Parse 全量 + projstr.go 往返 + impl_latlong(+ mlfn.go Karney 矩形纬)
- [x] Day2 上半:impl_eqc(球/椭球、k_0 忽略、Δλ 环绕、|lat_ts|=90 拒收)
  + probe7/probe8 高精度参照内嵌;forward 最大偏差 vs pyproj 3.0e-9 m
  (门槛 1e-6)、inverse 1.4e-14 度(门槛 1e-9)。
- [x] Day2 下半:xcheck/xcheck_test.go 消费端(密集 golden:latlong/eqc
  全绿 —— forward 最大 5.4e-9 m / inverse 1.5e-11°;geos skip 待 Day3 收紧;
  NaN 字面量 ": NaN"→"__NaN__" 容错解码,expectNaN 兼容 true/0.0)
- [x] Day3:impl_geos(sweep x/y,球/椭球)—— golden 收紧全绿:geos_x/geos_y
  forward ≤4.2e-9 m、inverse ≤5.4e-12°、全部 659/685 特例(含逆向域外探针)
  过;球体路径 probe9/probe9b 双参照;批量语义(D2)测试即 proj_batch_test.go。
- [ ] Day3 剩余:bench + README/LICENSE;Day4 门槛:race/vet/交叉编译/coverage ≥85%。
- [ ] Day3:impl_geos(sweep x/y)+ ForwardBatch/InverseBatch + bench + README

## 决策记录(A 轨,B 侧关注的部分)

- D1 非数值参数存储:冻结契约只开放 Params[float64] / UnknownParams[string] 两口。
  Parse 把识别到的非数值参数(sweep、datum、towgs84、no_defs 等旗标)连同真正的
  未知参数统一存 UnknownParams;适配层经 proj.go 未导出助手
  `paramString(crs,"sweep","y")` / `paramFloat(...)` 取用。B 的 impl_*.go 直接用即可
  (sweep 缺省 "y",PROJ ≥6 语义);若认为需要契约化(如专职字段),走 §5 提 C#x。
- D2 批量错误语义(DESIGN §1.1 重采样场景):单点 ErrDomain → 该点写 (NaN,NaN)
  继续;其他错误(非法输入等)立即整批失败并返回;批内出现任一 ErrDomain → 批末
  err 为 ErrDomain(errors.Is 可判,有效点结果仍可用)。
- D3 椭球组合优先级:+R > +ellps > +a/b 组合 > 默认 WGS84;仅给 +a 视作正球体;
  +rf=0 同义正球体(PROJ 语义)。
- D4 Parse 已知 proj 名(冻结 v1):latlong(含 longlat 别名 → 常规化 latlong)、
  eqc、geos、stere、laea、lcc、merc、tmerc;未知 +proj 报错(DESIGN §1.1 校验)。
  B 未移交的投影在 NewProjection 阶段报 "尚未注册"。
- D5 [在写] gen_golden 缺位期间,xcheck 消费端对缺失的 golden_*.json errno-跳过,
  不阻塞 A 侧 go test 绿;golden 到位后自动收紧为真实断言(B 侧生成即可,无协作锁)。

## 协作事项(§5 流程)

- C#1 **GRIBEllipsoid 签名装不下双轴形状**(冻结签名 `GRIBEllipsoid(shape int,
  scale uint8, raw uint32)` 只有一组 (scale,raw)):shape 3(major/minor km 双轴)
  与 shape 7(m 双轴)需要两组编码维度,且 minor 维度取自 octets 26-30
  (simple-grib grid/definition.go 同表)。当前对 3/7 返回明确错误;提议新增
  `GRIBEllipsoidAxes(shape int, majorScale uint8, majorRaw uint32, minorScale uint8, minorRaw uint32) (Ellipsoid, error)`
  (owner=A 落地;原 GRIBEllipsoid 保留处理单维/固定形状;B 无须改动)。
  **请 B 在 PROGRESS-B.md 回执 C#1-ACK 或异议。**

## 日志

- 2026-10-17 [A] day0-上午:git init、main 空提交 cc7c5f5、ai-a/ai-b 分支、
  契约骨架首提交(§2 逐字)、.gitignore、本档建档。
- 2026-10-17 [A] **给 B 的提示(共享工作目录实况)**:契约首提交 aef5c7a 是用
  `git add -A` 做的,把当时已出现在 xcheck/ 下的 B 侧文件(gen_golden.py、
  golden_eqc.json、golden_latlong.json)一并扫了进去 —— 内容未做任何修改,
  纯属同目录并发的扫描效应;golden 的权威版本仍以 B 自己的提交/生成器为准,
  Day4 合流时该三件以 B 提交的哈希对齐(内容相同时 git 自动化解)。
  另注意:双方目前都落在同一 branch(ai-a)上提交 —— 单目录只有一份 HEAD,
  若要真分离分支需要 B 放弃在本会话内 checkout ai-b(会动走 A 的文件);
  建议简化为:**同一目录同一分支顺序提交,commit 前缀 [A]/[B] 区分归属**,
  Day4 合流的"零冲突"目标不变。A 不改 B 文件、B 不改 A 文件的红线不变。
- 2026-10-17 [A] day0-下午:ellipsoid.go 全量(表 +f/rf/es/R/b 组合推导、
  fail-loud 校验)+ gribshape.go(shape 0-11 平移 simple-grib earthDimension
  逻辑;3/7 双轴缺口按 C#1 明确报错)+ 两文件单测;go test 全绿。
- 2026-10-17 [A] day1:crs.go Parse 全量(别名归一/默认值/EPSG 特例/'r' 弧度后缀/
  域校验)、projstr.go String() 往返、impl_latlong.go(恒等投影 + validLonLat/
  normLonDeg 共享域助手)、mlfn.go(Karney 六阶 φ↔μ 级数 + rectifyingRadius +
  clenshawDelta,为 eqc/geos 供子午线弧);配套 crs_test.go / impl_latlong_test.go。
  全绿。转轨期间修了两处自己测试/口径 bug:(1) xcheck 前的 EPSG 特征比对 near()
  容差 1e-9 太松,GRS80 与 WGS84 的 b 差 ~1.05e-4 m 被吞掉 → 4269 误判 4326,
  收紧至 1e-12(≈1.3e-5 m);(2) 弧度后缀测试误用无类型常量精确折叠当期望值,
  与实现的双舍入口径差 2e-14 度,改为与实现同序的运行期表达式。
- 2026-10-17 [A] day2-上半:impl_eqc.go 全量 + impl_eqc_test.go(probe7/probe8
  pyproj 3.8.0 实测值内嵌)。走过的弯路记两笔(mlfn 通用件,geos 复用前务必吃透):
  (1) Karney auxlat 系数起始幂:d 从 n 起(PROJ pj_auxlat_coeffs `double d = n`),
  我最初写 d=1 导致 F 表放大;(2) 系数块归属:PROJ coeffs 数组里 ptrs 的
  (auxout,auxin) 行主序,φ→μ(R.GEOGRAPHIC→RECTIFYING)取 offset 54 的
  负首块(−3/2, 9/16, …),offset 0 正首块(3/2, −27/32, …)其实是 μ→φ ——
  两块形似、方向相反,注释命名 C[phi,mu]/C[mu,phi] 与数组物理顺序相反,
  第一次接 就是 ICH 坑。修后 forward ≤3e-9 m、inverse ≤1.4e-14°,全绿。
- 2026-10-17 [A] day2-下半:xcheck/xcheck_test.go 消费端上线(golden 缺失 →
  Skip、impl 未注册 → Skip、经度圆周比对、expectNaN 容忍 true/0.0、JSON 裸
  NaN 以 ": NaN" → `"__NaN__"` 值位替换解码 —— **键名 expectNaN 自带 "NaN"
  子串,宽替换会连键名一起炸**,踩过一次)。latlong forward 1099 点/inverse
  1089 点全过(偏 0);eqc 1099/1090 全过(forward ≤5.4e-9 m、inverse ≤1.5e-11°)。
  geos_x/geos_y 两个 runGolden 在注册前 skip,Day3 合入后自动收紧。
- 2026-10-17 [A] day3:impl_geos.go(PROJ 9.8 geos.cpp 语义移植:球体无可见性
  检查、椭球 (rg−Vx)·Vx−Vy²−Vz²/rp²<0 → ErrDomain、inverse 三次方程 det<0 →
  ErrDomain;sweep 缺省 y、k_0 忽略)全过。**消费端收紧**:TestXCheckGeos 从
  skip 变为真断言,geos_x 435 fwd + 431 inv + 659 特例、geos_y 407+405+685
  全绿 —— 最大偏差 forward 4.2e-9 m、inverse 5.4e-12°。踩坑记一笔:golden
  specials 有两种形态,{lon,lat}=正向域外探针、{x,y}=逆向域外探针
  ("inverse far outside visible disc" 的 lon/lat 字段实际装的是 x/y 米值),
  消费端按字段存在性分派(errors.Is ErrDomain)。
- 2026-10-17 [A] day3-附:proj_batch_test.go 落 D2 批量语义(1e-9-度门);
  impl_geos_test.go 球体 probe9b 网格(fwd 1.9e-9 m/inv 1.4e-14°)+ sweep
  x/y 差异哨兵 + h 缺省报错对齐(pyproj 亦拒)+ 域内值 1e-6 m 门。
- 2026-10-17 [A] 注记(给 B):共享 go test 中 `TestGoldenStere`
  (stere_test.go:113)失败 —— 点 (0,-89.9) forward x 差 ≈ +4.6e-4 m
  (13297575846.740891 vs 13297575846.740435),超出 1e-6 m 门槛约两个数量级
  且 y 也有 8e-11 级差,呈现系统性而非舍入噪声(疑 stereEffectiveK 或
  lat_ts 处理与 golden 生成侧口径不一致)。属 B 轨文件,B 自查。C#1 仍待 B 回执。
