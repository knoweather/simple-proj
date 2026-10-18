#!/usr/bin/env python3
# Owner: AI-B (single-track: 本会话)
# 生成 xcheck/golden_*.json —— pyproj 是金标唯一事实源(参照 DESIGN.md §3)。
# 运行: python xcheck/gen_golden.py
import json
import math
import sys
from pathlib import Path
from datetime import datetime, timezone

import pyproj

CASES = {
    "latlong": ("deg", "+proj=latlong +ellps=WGS84"),
    "eqc":     ("m",   "+proj=eqc +lat_ts=0 +lat_0=0 +lon_0=0 +x_0=0 +y_0=0 +ellps=WGS84 +units=m"),
    "geos_x":  ("m",   "+proj=geos +lon_0=140.7 +h=35785863 +x_0=0 +y_0=0 +a=6378137 +b=6356752.3142 +units=m +sweep=x"),
    "geos_y":  ("m",   "+proj=geos +lon_0=0 +h=35785863 +x_0=0 +y_0=0 +ellps=WGS84 +units=m +sweep=y"),
    "stere":   ("m",   "+proj=stere +lat_0=90 +lat_ts=60 +lon_0=-100 +ellps=WGS84 +units=m"),
    "laea":    ("m",   "+proj=laea +lat_0=45 +lon_0=10 +x_0=0 +y_0=0 +ellps=WGS84 +units=m"),
    "lcc":     ("m",   "+proj=lcc +lat_1=30 +lat_2=60 +lat_0=40 +lon_0=100 +x_0=0 +y_0=0 +ellps=WGS84 +units=m"),
    "merc":    ("m",   "+proj=merc +lat_ts=45 +lon_0=0 +x_0=0 +y_0=0 +ellps=WGS84 +units=m"),
    "tmerc":   ("m",   "+proj=tmerc +lat_0=0 +lon_0=105 +k=0.9996 +x_0=0 +y_0=0 +ellps=WGS84 +units=m"),
}

# 每个 CRS 的采样网格范围与过滤(视野/极点奇异/半球外一律剔除)
def mask(name, lon, lat):
    dlon = (lon - {"geos_x": 140.7, "geos_y": 0.0, "stere": -100.0, "laea": 10.0,
                   "lcc": 100.0, "tmerc": 105.0}.get(name, 0.0) + 180) % 360 - 180
    if name in ("geos_x", "geos_y") and (abs(dlon) > 70 or abs(lat) > 70):
        return False
    if name == "merc" and abs(lat) > 80:
        return False
    if name == "laea":
        cd = math.radians(90 - 45)  # 与球心的角距
        d = math.acos(min(1.0, math.sin(math.radians(45)) * math.sin(math.radians(lat)) +
                          math.cos(math.radians(45)) * math.cos(math.radians(lat)) * math.cos(math.radians(dlon))))
        if d > math.radians(165):
            return False
    return True


def main():
    n_lon, n_lat = 33, 33
    lons = [-180 + 360 * i / (n_lon - 1) for i in range(n_lon)]
    lats = [-85 + 170 * j / (n_lat - 1) for j in range(n_lat)]

    out_dir = Path(__file__).parent
    n_total = n_ok = 0
    for name, (units, proj_str) in CASES.items():
        crs = pyproj.CRS(proj_str)
        fwdt = pyproj.Transformer.from_crs("EPSG:4326", crs, always_xy=True)
        invt = pyproj.Transformer.from_crs(crs, "EPSG:4326", always_xy=True)

        forward, inverse = [], []
        for lat in lats:
            for lon in lons:
                n_total += 1
                if not mask(name, lon, lat):
                    continue
                x, y = fwdt.transform(lon, lat)
                if math.isnan(x) or math.isnan(y) or abs(x) > 5e7 or abs(y) > 5e7:
                    continue
                forward.append({"lon": round(lon, 6), "lat": round(lat, 6), "x": x, "y": y, })
                lon2, lat2 = invt.transform(x, y)
                # 往返自检(生成器自己的质量门)
                wl = abs(((lon2 - lon) + 180) % 360 - 180)
                if max(wl, abs(lat2 - lat)) < 1e-6:
                    inverse.append({"x": x, "y": y, "lon": lon2, "lat": lat2})
        n_ok += len(forward)

        specials = []
        if name in ("geos_x", "geos_y"):
            # 地平线外:从星下点往东偏 100°(远超 81° 的地平角)
            sx, sy = fwdt.transform(140.7 + (-100 if name == "geos_x" else 100), 0)
            specials.append({"desc": "below horizon", "lon": 0, "lat": 0, "x": sx, "y": sy,
                             "expectNaN": math.isnan(sx) or math.isnan(sy) or abs(sx) > 5e7})
        if name == "stere":
            sx, sy = fwdt.transform(-100, -85)  # 远端
            specials.append({"desc": "far side", "lon": -100, "lat": -85, "x": sx, "y": sy,
                             "expectNaN": math.isnan(sx)})
        doc = {
            "crs": proj_str,
            "generator": f"pyproj {pyproj.proj_version_str}",
            "generated": datetime.now(timezone.utc).isoformat(timespec="seconds"),
            "units": units,
            "forward": forward,
            "inverse": inverse,
            "specials": specials,
        }
        (out_dir / f"golden_{name}.json").write_text(json.dumps(doc), encoding="utf-8")
        print(f"{name:8s} forward={len(forward):4d} inverse={len(inverse):4d} units={units}")
    js = json.dumps
    print(f"total grid pts={n_total}, kept={n_ok}")


if __name__ == "__main__":
    sys.exit(main())
