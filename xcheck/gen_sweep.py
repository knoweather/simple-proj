#!/usr/bin/env python3
"""
xcheck/gen_sweep.py — extension sweep generator. Owner: AI-B (COORDINATION §1).

Purpose: the frozen golden_*.json fixtures use one fixed 33x33 grid per CRS
(DESIGN §3.1). This generator produces a SECOND, independent point set —
structured edges plus a seeded LCG (no numpy) — so the Go side is cross-checked
against pyproj on points the fixtures never touch. Golden truth is still pyproj;
COORDINATION §2.4's frozen schema is NOT extended, this is a separate file
(xcheck/sweep_extra.json) consumed only by sweep_extra_test.go (B track).

Run from repo root:  python xcheck/gen_sweep.py
Deps:                pyproj (same as gen_golden.py)

Semantics (identical plumbing to gen_golden.py):
  * geographic source CRS keeps only ellipsoid tokens (local geostring helper),
    so the source and target share the ellipsoid and the transform is a PURE
    projection — this is what the Go implementation computes (it projects the
    given lon/lat on the target ellipsoid with no datum shift);
  * Transformer.from_crs(geo, tgt, always_xy=True) => pure projection;
  * non-finite pyproj answers become specials with expectNaN (JSON cannot hold
    inf/NaN); inverse records take x/y from pyproj forward and ask pyproj back;
  * output longitudes are normalized to [-180, 180) (local norm_lon).

NOTE: gen_golden.py was rewritten (commit 06d1455) and no longer exposes the
geostring / norm_lon / finite helpers this script used. They are re-implemented
locally below — single source of truth now lives here, matched-ellipsoid
semantics preserved (see the note above about why we must NOT switch to a bare
"EPSG:4326" source: Go does no datum shift, so the reference must share the
target's ellipsoid or the sweep drifts ~mm off Go for e.g. geos_x).
"""

import datetime
import json
import math
import os
import re
import sys

from pyproj import CRS, Transformer


def geostring(proj):
    """Build a geographic (latlong) CRS carrying the SAME ellipsoid tokens as
    `proj`, so that from_crs(geo, tgt) is a pure projection with no datum
    shift. Tokens kept: ellps, a, b, datum, R, rf, f, towgs84."""
    parts = []
    for m in re.finditer(r"\+([A-Za-z_]+)=([^\s+]*)", proj):
        key, val = m.group(1), m.group(2)
        if key in ("ellps", "a", "b", "datum", "R", "rf", "f", "towgs84"):
            parts.append("+%s=%s" % (key, val))
    base = "+proj=latlong " + " ".join(parts)
    if not parts:
        base += " +ellps=WGS84"
    return base


def norm_lon(lon):
    """Wrap a longitude to [-180, 180)."""
    return (lon + 180.0) % 360.0 - 180.0


def finite(p):
    """True iff every coordinate in the tuple is finite."""
    return all(math.isfinite(v) for v in p)


class LCG:
    """Deterministic linear congruential generator (Numerical Recipes constants)."""

    def __init__(self, seed):
        self.s = seed & 0x7FFFFFFF

    def next(self):
        self.s = (1664525 * self.s + 1013904223) & 0x7FFFFFFF
        return self.s / 0x7FFFFFFF

    def uniform(self, a, b):
        return a + (b - a) * self.next()


# 9 CRSs, same projection setups as the golden fixtures (but this is an
# independent reference set, not a copy of them).
CASES = {
    "latlong": "+proj=latlong +ellps=WGS84",
    "eqc": "+proj=eqc +lat_ts=0 +lat_0=0 +lon_0=180 +x_0=0 +y_0=0 +ellps=WGS84 +units=m",
    "geos_x": "+proj=geos +lon_0=140.7 +h=35785863 +x_0=0 +y_0=0 "
              "+a=6378137 +b=6356752.3 +units=m +sweep=x",
    "geos_y": "+proj=geos +lon_0=0 +h=35785863 +ellps=WGS84 +sweep=y",
    "stere": "+proj=stere +lat_0=90 +lat_ts=60 +lon_0=-100 +ellps=WGS84",
    "stere_eq": "+proj=stere +lat_0=0 +lon_0=0 +ellps=WGS84",
    "stere_obl": "+proj=stere +lat_0=45 +lon_0=0 +ellps=WGS84",
    "laea": "+proj=laea +lat_0=45 +lon_0=10 +ellps=WGS84 +x_0=0 +y_0=0",
    "laea_eq": "+proj=laea +lat_0=0 +lon_0=0 +ellps=WGS84 +x_0=0 +y_0=0",
    "laea_np": "+proj=laea +lat_0=90 +lon_0=0 +ellps=WGS84 +x_0=0 +y_0=0",
    "laea_sp": "+proj=laea +lat_0=-90 +lon_0=0 +ellps=WGS84 +x_0=0 +y_0=0",
    "lcc": "+proj=lcc +lat_1=30 +lat_2=60 +lat_0=0 +lon_0=100 +ellps=WGS84",
    "merc": "+proj=merc +a=6378137 +b=6378137 +lat_ts=0.0 +lon_0=0.0 +x_0=0 +y_0=0 +units=m",
    "tmerc": "+proj=tmerc +lat_0=0 +lon_0=105 +k=0.9996 +ellps=WGS84",
    # aeqd 四种方面(极轴 N/S、赤道、斜轴)+ 球体斜轴;utm 北/南半球
    "aeqd_np": "+proj=aeqd +lat_0=90 +lon_0=0 +ellps=WGS84",
    "aeqd_sp": "+proj=aeqd +lat_0=-90 +lon_0=0 +ellps=WGS84",
    "aeqd_eq": "+proj=aeqd +lat_0=0 +lon_0=0 +ellps=WGS84",
    "aeqd_obl": "+proj=aeqd +lat_0=45 +lon_0=10 +ellps=WGS84",
    "aeqd_sph": "+proj=aeqd +lat_0=45 +lon_0=10 +R=6371000",
    "utm": "+proj=utm +zone=50 +ellps=WGS84",
    "utm_south": "+proj=utm +zone=33 +south +ellps=WGS84",
}

# Structured edge points (lon, lat) per case: centre, meridians, near-pole,
# near-antimeridian, standard parallels, etc. Random points are added on top.
EDGES = {
    "latlong": [(0, 0), (180, 0), (-180, 0), (0, 90), (0, -90), (90, 45), (-90, -45),
                (179.999, 30), (-179.999, -30), (45, 89.5), (-45, -89.5)],
    "eqc": [(180, 0), (180, 45), (180, -45), (0, 0), (0, 89), (0, -89), (170, 30),
            (-170, -30), (185, 15), (-175, -15), (90, 85), (-90, -85)],
    "geos_x": [(140.7, 0), (140.7, 45), (140.7, -45), (140.7, 65), (140.7, -65),
               (75.7, 0), (205.7, 0), (100, 30), (180, -30), (140.7, 80), (120, -60)],
    "geos_y": [(0, 0), (0, 45), (0, -45), (0, 65), (0, -65), (65, 0), (-65, 0),
               (30, 30), (-30, -30), (0, 80), (55, -60)],
    "stere": [(-100, 90), (-100, 0), (-100, 45), (-100, -45), (-10, 60), (-190, -60),
              (80, 30), (-170, 10), (150, -80), (100, -85), (-100, 89), (-100, -89)],
    "stere_eq": [(0, 0), (0, 45), (0, -45), (90, 30), (-90, -30), (180, 60),
                (-180, -60), (45, 80), (-45, -80), (10, 89), (-10, -89)],
    "stere_obl": [(0, 45), (0, -45), (0, 0), (-170, 45), (170, -45), (100, 45),
                 (100, 80), (10, 88), (-80, -10), (55, -80), (10, -88), (170, 80)],
    "laea": [(10, 45), (10, -45), (10, 0), (-170, 45), (-170, -45), (100, 45),
             (100, 80), (10, 88), (-80, -10), (55, -80), (10, -88), (170, 80)],
    "laea_eq": [(0, 0), (0, 45), (0, -45), (90, 30), (-90, -30), (180, 60),
                (-180, -60), (45, 80), (-45, -80), (10, 89), (-10, -89)],
    "laea_np": [(0, 90), (0, -45), (0, 45), (90, 45), (-90, -45), (10, 80),
                (170, 80), (-170, 80), (45, 85), (-45, 85), (10, 30)],
    "laea_sp": [(0, -90), (0, 45), (0, -45), (90, 45), (-90, -45), (10, 80),
                (170, 80), (-170, 80), (45, 85), (-45, 85), (10, 30)],
    "lcc": [(100, 0), (100, 30), (100, 60), (100, 89), (100, -89), (100, 45),
            (10, 0), (-170, 0), (10, 45), (-170, -45), (100, -45), (55, 85)],
    "merc": [(0, 0), (0, 89.9), (0, -89.9), (179.9, 45), (-179.9, -45), (90, 60),
             (-90, -60), (0, 85), (180, 0), (-180, 0), (45, -85), (-45, 85)],
    "tmerc": [(105, 0), (105, 45), (105, -45), (105, 88), (105, -88), (35, 0), (175, 0),
              (105, 89), (30, 30), (180, -30), (40, -85), (170, 85)],
    "aeqd_np": [(0, 90), (0, 0), (0, 45), (90, 45), (-90, -45), (180, 10), (10, 80),
                (170, 80), (-170, 80), (45, 85), (-45, -85), (0, -90)],
    "aeqd_sp": [(0, -90), (0, 0), (0, 45), (90, 45), (-90, -45), (180, 10), (10, 80),
                (170, 80), (-170, 80), (45, 85), (-45, -85), (0, 90)],
    "aeqd_eq": [(0, 0), (0, 45), (0, -45), (90, 30), (-90, -30), (180, 0), (180, 0.001),
                (45, 80), (-45, -80), (10, 89), (-10, -89), (179.9, 0)],
    "aeqd_obl": [(10, 45), (10, -45), (10, 0), (-170, 45), (170, -45), (100, 45),
                 (100, 80), (10, 88), (-80, -10), (55, -80), (10, -88), (170, 80)],
    # N.B. (100, 45) is deliberately absent: that is the knife-edge where
    # PROJ's `xy.y == 0 ? lam = 0` shortcut in aeqd_s_inverse loses 90 deg of
    # longitude (the residual y is ~1e-16 and C rounds it to exactly 0).
    # Go keeps atan2 there, so a round-trip assertion would flag PROJ, not us.
    "aeqd_sph": [(10, 45), (10, -45), (10, 0), (-170, 45), (170, -45), (100, 44.5),
                 (100, 80), (10, 88), (-80, -10), (55, -80), (10, -88), (170, 80)],
    "utm": [(117, 0), (117, 45), (117, -45), (117, 88), (117, -88), (114, 0), (120, 0),
            (60, 30), (180, -30), (117, 89), (117, -89), (30, 85)],
    "utm_south": [(15, 0), (15, 45), (15, -45), (15, 88), (15, -88), (12, 0), (18, 0),
                  (60, 30), (180, -30), (15, 89), (15, -89), (30, 85)],
}

# Random-point domain boxes per case: (lon_lo, lon_hi, lat_lo, lat_hi) — chosen
# to stay inside each projection's valid region (see PROGRESS-B.md notes:
# tmerc ~81.7 deg of the central meridian, geos the visible disc, lcc/merc the
# non-pole band, etc.).
BOXES = {
    "latlong": (-180, 180, -89.9, 89.9),
    "eqc": (-180, 180, -89.9, 89.9),
    "geos_x": (140.7 - 64, 140.7 + 64, -64, 64),
    "geos_y": (-64, 64, -64, 64),
    "stere": (-180, 180, -88, 88),
    "stere_eq": (-180, 180, -88, 88),
    "stere_obl": (-180, 180, -88, 88),
    "laea": (-180, 180, -88, 88),
    "laea_eq": (-180, 180, -88, 88),
    "laea_np": (-180, 180, -89, 89),
    "laea_sp": (-180, 180, -89, 89),
    "lcc": (-180, 180, -89, 89),
    "merc": (-180, 180, -89.5, 89.5),
    "tmerc": (30, 180, -88, 88),
    "aeqd_np": (-180, 180, -89, 89),
    "aeqd_sp": (-180, 180, -89, 89),
    "aeqd_eq": (-180, 180, -89, 89),
    "aeqd_obl": (-180, 180, -89, 89),
    "aeqd_sph": (-180, 180, -89, 89),
    "utm": (50, 180, -88, 88),
    "utm_south": (-30, 100, -88, 88),
}

# Domain probes: lon/lat => forward probe, x/y => inverse probe. pyproj decides
# expectNaN (non-finite => true), exactly like gen_golden's SPECIALS handling.
SPECIALS = {
    "geos_x": [{"desc": "far side of Earth, outside satellite view", "lon": 0.0, "lat": 0.0},
               {"desc": "inverse far outside visible disc", "x": 9e9, "y": -9e9}],
    "geos_y": [{"desc": "far side of Earth, outside satellite view", "lon": 180.0, "lat": 0.0},
               {"desc": "inverse far outside visible disc", "x": 9e9, "y": -9e9}],
    "tmerc": [{"desc": "beyond ~81.7 deg of the central meridian at the equator",
               "lon": 105.0 + 85.0, "lat": 0.0},
              {"desc": "inverse far outside", "x": 9e9, "y": 9e9}],
    "lcc": [{"desc": "far pole (outside PROJ lcc domain)", "lon": 100.0, "lat": -90.0}],
    "laea": [{"desc": "antipode of lat_0/lon_0 (infinite radius)", "lon": -170.0, "lat": -45.0}],
    "aeqd_sph": [{"desc": "antipode of the spherical oblique centre (infinite radius)",
                  "lon": -170.0, "lat": -45.0},
                 {"desc": "inverse far outside the sphere radius (>pi)",
                  "x": 3.0e7, "y": 3.0e7}],
    "aeqd_np": [{"desc": "opposite pole has no defined azimuth (sphere branch only)",
                 "lon": 0.0, "lat": -90.0}],
    "utm": [{"desc": "beyond ~81.7 deg of the central meridian at the equator",
             "lon": 117.0 + 85.0, "lat": 0.0}],
}

N_RANDOM = 24


def main():
    out_dir = os.path.dirname(os.path.abspath(__file__))
    import pyproj
    generator = "pyproj %s / PROJ %s" % (pyproj.__version__, pyproj.proj_version_str)

    out = {
        "generator": generator,
        "generated": datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "note": "extension sweep beyond the frozen golden grids; consumed by sweep_extra_test.go (owner AI-B)",
        "cases": {},
    }

    for name, proj in CASES.items():
        tgt = CRS.from_user_input(proj)
        geo = CRS.from_user_input(geostring(proj))
        fwd = Transformer.from_crs(geo, tgt, always_xy=True)
        inv = Transformer.from_crs(tgt, geo, always_xy=True)

        rng = LCG(20261017)  # fixed seed, deterministic
        lon_lo, lon_hi, lat_lo, lat_hi = BOXES[name]
        pts = list(EDGES[name])
        for _ in range(N_RANDOM):
            pts.append((rng.uniform(lon_lo, lon_hi), rng.uniform(lat_lo, lat_hi)))

        forward, inverse, specials = [], [], []
        for lon, lat in pts:
            x, y = fwd.transform(lon, lat)
            if not finite((x, y)):
                # random point slipped out of the assumed domain box: drop it
                # (the SPECIALS list is where out-of-domain probes belong).
                continue
            rl, la = inv.transform(x, y)
            if not finite((rl, la)):
                continue
            forward.append({"lon": lon, "lat": lat, "x": x, "y": y,
                            # pyproj's OWN forward-then-inverse round trip. At the
                            # projection periphery (e.g. tmerc ~70-80 deg from the
                            # central meridian) even PROJ's own round trip drifts
                            # ~1e-8 deg, so the Go side checks CONSISTENCY with
                            # pyproj's round trip there instead of the absolute
                            # 1e-9 deg gate (C#7/C#13 philosophy).
                            "rt_lon": norm_lon(rl), "rt_lat": la})
            inverse.append({"x": x, "y": y,
                            "lon": norm_lon(rl), "lat": la})

        for probe in SPECIALS.get(name, []):
            if "lon" in probe:
                x, y = fwd.transform(probe["lon"], probe["lat"])
                rec = {"desc": probe["desc"], "lon": probe["lon"], "lat": probe["lat"],
                       "expectNaN": not finite((x, y))}
            else:
                rl, la = inv.transform(probe["x"], probe["y"])
                rec = {"desc": probe["desc"], "x": probe["x"], "y": probe["y"],
                       "expectNaN": not finite((rl, la))}
            if rec["expectNaN"]:
                specials.append(rec)

        out["cases"][name] = {
            "crs": proj,
            "forward": forward,
            "inverse": inverse,
            "specials": specials,
        }

    path = os.path.join(out_dir, "sweep_extra.json")
    with open(path, "w", encoding="utf-8") as f:
        json.dump(out, f, ensure_ascii=False, indent=1)
        f.write("\n")
    print("wrote %s" % path)
    for name, c in out["cases"].items():
        print("  %-8s forward=%3d inverse=%3d specials=%d"
              % (name, len(c["forward"]), len(c["inverse"]), len(c["specials"])))


if __name__ == "__main__":
    sys.exit(main())
