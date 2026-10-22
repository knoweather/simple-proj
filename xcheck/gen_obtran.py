#!/usr/bin/env python3
"""
xcheck/gen_obtran.py — ob_tran（旋转极点）参照生成器。

Purpose: the hand-written table in impl_obtran_test.go documents five representative
parameter sets on a handful of points. This generator adds a wider, seeded sweep so
the rotation is checked against pyproj on points nobody hand-picked (including the
neighbourhood of the rotated poles and the antimeridian).

Both directions are recorded:
  * rotated -> geographic  (our Projection.Forward)
  * geographic -> rotated  (our Projection.Inverse)

Run from repo root:  python xcheck/gen_obtran.py
Deps:                pyproj

Note: ob_tran + o_proj=longlat is a pure angular rotation — the ellipsoid never
enters the math (PROJ also ignores it), so the reference source is plain
EPSG:4326 and no ellipsoid matching is needed.
"""

import datetime
import json
import math
import os
import sys

from pyproj import CRS, Transformer

# (name, proj string) — the pole form only; the o_alpha / o_lat_c family is not
# implemented (Parse rejects it, see crs.go).
CASES = [
    ("np30", "+proj=ob_tran +o_proj=longlat +o_lat_p=30 +lon_0=0 +ellps=WGS84"),
    ("np30_lonp40", "+proj=ob_tran +o_proj=longlat +o_lat_p=30 +o_lon_p=40 +lon_0=0 +ellps=WGS84"),
    ("np30_lon170", "+proj=ob_tran +o_proj=longlat +o_lat_p=30 +o_lon_p=40 +lon_0=170 +ellps=WGS84"),
    ("ecmwf", "+proj=ob_tran +o_proj=longlat +o_lat_p=39.25 +lon_0=18 +ellps=WGS84"),
    ("sp30", "+proj=ob_tran +o_proj=longlat +o_lat_p=-30 +lon_0=-170 +ellps=WGS84"),
    ("np90", "+proj=ob_tran +o_proj=longlat +o_lat_p=90 +o_lon_p=10 +lon_0=30 +ellps=WGS84"),
    ("oblique", "+proj=ob_tran +o_proj=longlat +o_lat_p=55.5 +o_lon_p=-25 +lon_0=-100 +ellps=WGS84"),
]

# Structured rotated-grid points: poles, antimeridian, equator, both hemispheres.
STRUCTURED = [
    (0.0, 0.0), (0.0, 90.0), (0.0, -90.0), (180.0, 0.0), (-180.0, 0.0),
    (90.0, 0.0), (-90.0, 0.0), (0.0, 45.0), (0.0, -45.0),
    (179.999, 0.0), (-179.999, 0.0), (45.0, 89.0), (45.0, -89.0),
    (10.0, 20.0), (-30.0, -60.0), (120.0, 60.0), (-135.0, 15.0), (30.0, -75.0),
]

N_RANDOM = 60


class LCG:
    """Deterministic 64-bit LCG (no numpy), same spirit as gen_sweep.py."""

    def __init__(self, seed):
        self.s = seed & 0xFFFFFFFFFFFFFFFF

    def next(self):
        self.s = (self.s * 6364136223846793005 + 1442695040888963407) & 0xFFFFFFFFFFFFFFFF
        return (self.s >> 11) / float(1 << 53)

    def uniform(self, lo, hi):
        return lo + (hi - lo) * self.next()


def norm_lon(lon):
    """Wrap a longitude to [-180, 180)."""
    return (lon + 180.0) % 360.0 - 180.0


def main():
    out_dir = os.path.dirname(os.path.abspath(__file__))
    import pyproj

    cases = []
    for name, src in CASES:
        crs = CRS.from_user_input(src)
        fwd = Transformer.from_crs(crs, CRS.from_epsg(4326), always_xy=True)   # rotated -> geo
        inv = Transformer.from_crs(CRS.from_epsg(4326), crs, always_xy=True)   # geo -> rotated

        pts = list(STRUCTURED)
        rng = LCG(20261022)
        for _ in range(N_RANDOM):
            pts.append((rng.uniform(-180.0, 180.0), rng.uniform(-89.5, 89.5)))
        # A few points hugging the rotated pole: longitude there is degenerate,
        # but the latitude answer must still match.
        for lon in (-180.0, -90.0, 0.0, 90.0, 179.9):
            pts.append((lon, 89.999))

        rotated_to_geo, geo_to_rotated = [], []
        for rlon, rlat in pts:
            glon, glat = fwd.transform(rlon, rlat)
            if not (math.isfinite(glon) and math.isfinite(glat)):
                continue
            rotated_to_geo.append({
                "rlon": rlon, "rlat": rlat,
                "lon": norm_lon(glon), "lat": glat,
            })

        # geographic -> rotated: use a plain lat/lon grid (that is what a GRIB
        # consumer asks for when it maps a geographic point into the grid frame).
        grid = [(lon, lat) for lat in (-89.0, -60.0, -30.0, 0.0, 30.0, 60.0, 89.0)
                for lon in (-179.0, -120.0, -60.0, 0.0, 60.0, 120.0, 179.0)]
        rng2 = LCG(20261023)
        for _ in range(N_RANDOM):
            grid.append((rng2.uniform(-180.0, 180.0), rng2.uniform(-89.5, 89.5)))
        for lon, lat in grid:
            rlon, rlat = inv.transform(lon, lat)
            if not (math.isfinite(rlon) and math.isfinite(rlat)):
                continue
            geo_to_rotated.append({
                "lon": lon, "lat": lat,
                "rlon": norm_lon(rlon), "rlat": rlat,
            })

        cases.append({
            "name": name,
            "source": src,
            "rotated_to_geo": rotated_to_geo,
            "geo_to_rotated": geo_to_rotated,
        })

    out = {
        "generator": "pyproj %s / PROJ %s" % (pyproj.__version__, pyproj.proj_version_str),
        "generated": datetime.datetime.now(datetime.timezone.utc)
        .strftime("%Y-%m-%dT%H:%M:%SZ"),
        "note": "ob_tran (o_proj=longlat) parity sweep. rotated_to_geo feeds our "
                "Projection.Forward, geo_to_rotated feeds Inverse. Longitudes are "
                "normalized to [-180,180); the antimeridian therefore reads -180 "
                "where pyproj may print +180.",
        "cases": cases,
    }

    path = os.path.join(out_dir, "obtran_ref.json")
    # newline="\n":与 .gitattributes 的 eol=lf 一致,免得工作树 CRLF 反复 churn
    with open(path, "w", encoding="utf-8", newline="\n") as f:
        json.dump(out, f, ensure_ascii=False, indent=1)
        f.write("\n")
    print("wrote %s (%d cases, %d + %d points)"
          % (path, len(cases),
             sum(len(c["rotated_to_geo"]) for c in cases),
             sum(len(c["geo_to_rotated"]) for c in cases)))
    return 0


if __name__ == "__main__":
    sys.exit(main())
