#!/usr/bin/env python3
# 模型探针:用 pyproj 逐个候选公式试数,锁定 geos/eqc/stere/laea 的准确数学形式。
# 跑: python xcheck/probe_models.py   (结果直接看 stdout)
import math
import pyproj

A, B = 6378137.0, 6356752.314245
E2 = 1 - (B * B) / (A * A)


def proj(crs_str):
    return pyproj.Transformer.from_crs("EPSG:4326", pyproj.CRS(crs_str), always_xy=True)


print("=== geos: model identification (ray -> z-plane) ===")
# 参数: lon_0=140.7, h, 全圆盘。测试点(相对 lon_0 的 dlon, lat)
t = proj("+proj=geos +lon_0=140.7 +h=35785863 +x_0=0 +y_0=0 +a=6378137 +b=6356752.3142 +units=m +sweep=x")
h = 35785863.0
D = A + h
pts = [(10, 0), (30, 0), (60, 0), (0, 10), (0, 30), (0, 60), (30, 30), (-10, 5)]
py = [t.transform(140.7 + dl, lat) for dl, lat in pts]
for (dl, lat), (x, y) in zip(pts, py):
    print(f"  dl={dl:+3d} lat={lat:+3d} -> x={x:15.6f} y={y:15.6f}")

print("  候选: ray-plane z=z_p:")
def ray_plane(dlon_deg, lat_deg, zp):
    lam = math.radians(dlon_deg)
    phi = math.radians(lat_deg)
    N = A / math.sqrt(1 - E2 * math.sin(phi) ** 2)
    Px = N * math.cos(phi) * math.sin(lam)
    Py = N * math.cos(phi) * math.cos(lam)
    Pz = N * (1 - E2) * math.sin(phi)
    tpar = (zp - D) / (Pz - D)
    return tpar * Px, tpar * Py

for zp in (0, A):
    errs = []
    for (dl, lat), (x, y) in zip(pts, py):
        u, v = ray_plane(dl, lat, zp)
        errs.append((x - u, y - v))
    print(f"    z_p={zp}: x_errs={[f'{e[0]:.6e}' for e in errs]}")

# 归一化平角模型: x = tan(theta_x) * z_pDIST
print("  候选: x = D*tan(theta_x) 等价检查 (对 φ=0 行应成立)")

print()
print("=== geos sweep=y 对照 (lon_0=0) ===")
t2 = proj("+proj=geos +lon_0=0 +h=35785863 +x_0=0 +y_0=0 +ellps=WGS84 +units=m +sweep=y")
for dl, lat in [(10, 0), (0, 30), (30, 30)]:
    x, y = t2.transform(dl, lat)
    print(f"  dl={dl:+3d} lat={lat:+3d} -> x={x:15.6f} y={y:15.6f}")

print()
print("=== eqc: 半径身份 ===")
t3 = proj("+proj=eqc +lat_ts=0 +lat_0=0 +lon_0=0 +x_0=0 +y_0=0 +ellps=WGS84 +units=m")
x, y = t3.transform(10, 45)
math_lam = math.radians(10)
print(f"  fwd(10,45)        x={x:.6f} y={y:.6f}")
print(f"    a*lam           = {A * math_lam:.6f}   (球等距,R=a)")
def authalic_radius(a, b, e2):
    e = math.sqrt(e2)
    qp = 1 - (1 - e2) * math.log((1 - e) / (1 + e)) / (2 * e)
    return a * a * (1 + (1 - e2) * math.log((1 - e) / (1 + e)) / e) / (2 * qp) / 1.0
# 精确 R_q: Rq = sqrt( ( (A^2 + (1-e2) A^2 * ln[(1-e)/(1+e)]/e) ) / (2 + (1-e2) ln[(1-e)/(1+e)]/e) )
qp = 2 + (1 - E2) / E * math.log((1 - E) / (1 + E)) if False else None
e = math.sqrt(E2)
qp = 2 + (1 - E2) * math.log((1 - e) / (1 + e)) / e
rq = A * math.sqrt(A * A * (1 + (1 - E2) * math.log((1 - e) / (1 + e)) / e) / (1 / 1) / 1 / 1) if False else math.sqrt(
    (A * A) * (1 + (1 - E2) * math.log((1 - e) / (1 + e)) / e) / qp)
print(f"    R_authalic      = {rq:.6f} -> x = {rq * math_lam:.6f}")

print()
print("=== stere: 半径/比例识别 (lat_0=0,lat_ts=0 版本最简) ===")
t4 = proj("+proj=stere +lat_0=0 +lon_0=0 +lat_ts=0 +ellps=WGS84 +units=m")
for lat, lon in [(1, 0), (10, 0), (30, 0), (0, 10), (45, 45), (30, 60), (-30, 25)]:
    x, y = t4.transform(lon, lat)
    print(f"  fwd({lon},{lat})  x={x:15.6f} y={y:15.6f}")
# 球心角模型: φ=10, λ=0: 球面 stere x=0, y = 2 R tan(phi/2)? (切于球心点)
phi = math.radians(10)
for lbl, R in (("a", A), ("authalic", rq)):
    print(f"    2*{lbl}*tan(pi/4+phi/2)... y_sph模型 = {2 * R * math.tan(math.pi / 4 + phi / 2 - math.pi / 4):.6f}")

print()
print("=== laea: 半径识别 ===")
t5 = proj("+proj=laea +lat_0=0 +lon_0=0 +ellps=WGS84 +units=m")
for lat, lon in [(30, 0), (0, 30), (45, 45), (30, 90), (-60, 70)]:
    x, y = t5.transform(lon, lat)
    print(f"  fwd({lon},{lat})  x={x:15.6f} y={y:15.6f}")
phi = math.radians(30)
rqv = rq
print(f"    球 laea 中心点: y = 2 R sin(pi/4 - phi/2) ·? 标准球 laea(φ0=0,height长): y=2R sin(pi/4 - φ/2)? =")
th = math.pi / 4 - phi / 2
print(f"      R=a   -> {2 * A * math.sin(th):.6f}")
print(f"      R=rq  -> {2 * rqv * math.sin(th):.6f}")

print()
print("=== tmerc: Snyder 近似式的可行域标定 (UTM型) ===")
t6 = proj("+proj=tmerc +lat_0=0 +lon_0=105 +k=0.9996 +ellps=WGS84 +units=m")
# UTM zone 47, 中点(0,105),(55,115)
M0 = 6367448.3142453216  # 不精确的 meridional 部分半径
e4 = E2 * E2
e6 = e4 * E2
Mcoef = (1 - E2 / 4 - 3 * e4 / 64 - 5 * e6 / 256)
def meridian_arc(phi):
    return A * Mcoef * phi - A * (3 * E2 / 8 + 3 * e4 / 32 + 45 * e6 / 1024) * math.sin(2 * phi) + \
           A * (15 * e4 / 256 + 45 * e6 / 1024) * math.sin(4 * phi) - A * (35 * e6 / 3072) * math.sin(6 * phi)
for lat, lon in [(0, 105), (55, 115), (40, 99), (-23, 110)]:
    x, y = t6.transform(lon, lat)
    dl = math.radians(lon - 105)
    ep2 = E2 / (1 - E2)
    sin0_, cos0_ = math.sin(math.radians(0)), math.cos(math.radians(0))
    T0 = math.tan(math.radians(0)) ** 2
    C0 = ep2 * cos0_ ** 2
    phi = math.radians(lat)
    NN = A / math.sqrt(1 - E2 * math.sin(phi) ** 2)
    AA = dl * math.cos(phi)
    snyder_x = 0.9996 * NN * (AA + math.tan(phi) * AA ** 2 * 0 / 2 + ...)
