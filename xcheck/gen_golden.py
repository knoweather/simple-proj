#!/usr/bin/env python3
"""
xcheck/gen_golden.py — golden fixture generator. Owner: AI-B (COORDINATION §1).

Golden truth = pyproj; the Go side is only a consumer (DESIGN §3). Output schema is
the frozen one in COORDINATION §2.4 (crs / generator / generated / forward / inverse /
specials). `specials` entries may additionally carry lon/lat when the probe is
forward-direction — Go's decoder ignores unknown keys, so this is additive, not a
contract change.

Run from repo root:  python xcheck/gen_golden.py
Deps:                pyproj (pip install pyproj)

Notes on semantics we lock in here (agreed numeric rules, COORDINATION §2.5):
  * API units: lon/lat in degrees, x/y in metres (latlong returns degrees).
  * Out-of-domain: pyproj returns inf (PROJ HUGE_VAL); our Go contract returns NaN.
    The fixture therefore records `expectNaN: true` and never writes raw inf/NaN
    into JSON (invalid JSON).
  * Longitude normalization on output: [-180, 180). pyproj does not always normalize,
    so the generator normalizes and records the normalized value; the Go side must
    match that. Points where pyproj's own output was already normalized are unaffected.
"""

import datetime
import json
import math
import os
import re
import sys

from pyproj import CRS, Transformer

# --- cases (DESIGN §3.1) -----------------------------------------------------
CASES = {
    "golden_latlong": "+proj=latlong +ellps=WGS84",
    "golden_eqc": "+proj=eqc +lat_ts=0 +lat_0=0 +lon_0=180 +x_0=0 +y_0=0 +ellps=WGS84 +units=m",
    "golden_geos_x": "+proj=geos +lon_0=140.7 +h=35785863 +x_0=0 +y_0=0 "
                     "+a=6378137 +b=6356752.3 +units=m +sweep=x",
    "golden_geos_y": "+proj=geos +lon_0=0 +h=35785863 +ellps=WGS84 +sweep=y",
    "golden_stere": "+proj=stere +lat_0=90 +lat_ts=60 +lon_0=-100 +ellps=WGS84",
    "golden_laea": "+proj=laea +lat_0=45 +lon_0=10 +ellps=WGS84 +x_0=0 +y_0=0",
    "golden_lcc": "+proj=lcc +lat_1=30 +lat_2=60 +lat_0=0 +lon_0=100 +ellps=WGS84",
    "golden_merc": "+proj=merc +a=6378137 +b=6378137 +lat_ts=0.0 +lon_0=0.0 "
                   "+x_0=0 +y_0=0 +units=m",
    "golden_tmerc": "+proj=tmerc +lat_0=0 +lon_0=105 +k=0.9996 +ellps=WGS84",
}

# 33 x 33 grid: lon in [-180, 180], lat in [-85, 85] (DESIGN §3.1)
N = 33
LON_MIN, LON_MAX = -180.0, 180.0
LAT_MIN, LAT_MAX = -85.0, 85.0

# Extra sanity points shared by every case (equator / prime meridian / near-pole).
EXTRA_POINTS = [
    (0.0, 0.0), (0.0, 89.9), (0.0, -89.9), (180.0, 0.0), (-180.0, 0.0),
    (170.0, 85.0), (-170.0, -85.0), (100.0, 0.0), (-100.0, 0.0),
]

# Per-case domain probes. Each probe is run in both directions; whatever pyproj
# returns decides expectNaN (non-finite => true). Points that pyproj answers with
# finite values are recorded as ordinary forward/inverse entries instead.
SPECIALS = {
    "golden_latlong": [
        {"desc": "identity at pole", "lon": 0.0, "lat": 90.0},
    ],
    "golden_eqc": [
        {"desc": "eqc at pole (y = R2 * pi/2, finite)", "lon": 0.0, "lat": 90.0},
        {"desc": "eqc inverse far outside band", "x": 9e9, "y": -9e9},
    ],
    "golden_geos_x": [
        {"desc": "far side of Earth, outside satellite view", "lon": 0.0, "lat": 0.0},
        {"desc": "inverse far outside visible disc", "x": 9e9, "y": -9e9},
        {"desc": "just inside horizon, equator", "lon": 140.7 + 70.0, "lat": 0.0},
    ],
    "golden_geos_y": [
        {"desc": "far side of Earth, outside satellite view", "lon": 180.0, "lat": 0.0},
        {"desc": "inverse far outside visible disc", "x": 9e9, "y": -9e9},
        {"desc": "sweep=y sign check at lon_0+70", "lon": 70.0, "lat": 0.0},
    ],
    "golden_stere": [
        {"desc": "stere at north pole (lat_0)", "lon": -100.0, "lat": 90.0},
        {"desc": "inverse outside projection circle", "x": 9e9, "y": -9e9},
    ],
    "golden_laea": [
        {"desc": "antipode of lat_0/lon_0 (infinite radius)", "lon": -170.0, "lat": -45.0},
        {"desc": "inverse far outside", "x": 9e9, "y": -9e9},
    ],
    "golden_lcc": [
        {"desc": "lcc at pole (infinite radius)", "lon": 100.0, "lat": 90.0},
        {"desc": "inverse far outside", "x": 9e9, "y": -9e9},
    ],
    "golden_merc": [
        {"desc": "merc at pole (infinite y)", "lon": 0.0, "lat": 90.0},
        {"desc": "inverse far outside", "x": 1e12, "y": 1e12},
    ],
    "golden_tmerc": [
        {"desc": "tmerc at pole (infinite y)", "lon": 105.0, "lat": 90.0},
        {"desc": "inverse far outside", "x": 9e9, "y": -9e9},
    ],
}

PARAM_RE = re.compile(r"\+([a-zA-Z_0-9]+)=([-0-9.e+]+)")


def params(proj):
    out = {}
    for k, v in PARAM_RE.findall(proj):
        try:
            out[k] = float(v)
        except ValueError:
            pass  # non-numeric params (ellps name, sweep, units) are not needed here
    return out


def norm_lon(lon):
    """Contract §2.5: output longitude in [-180, 180)."""
    v = (lon + 180.0) % 360.0 - 180.0
    return v


def finite(pair):
    return math.isfinite(pair[0]) and math.isfinite(pair[1])


def num(v):
    return repr(float(v))


def existing_generated(path):
    if os.path.exists(path):
        try:
            with open(path, "r", encoding="utf-8") as f:
                return json.load(f).get("generated")
        except Exception:
            pass
    return None


def write_json(path, obj):
    """Stable, diff-friendly layout: header pretty, one record per line."""
    lines = ["{"]
    for key in ("crs", "generator", "generated"):
        lines.append('  "%s": %s,' % (key, json.dumps(obj[key])))
    for key in ("forward", "inverse", "specials"):
        lines.append('  "%s": [' % key)
        recs = obj[key]
        for i, rec in enumerate(recs):
            body = ", ".join('"%s": %s' % (k, json.dumps(v) if isinstance(v, str)
                                           else ("true" if v is True else num(v)))
                             for k, v in rec.items())
            lines.append("    {" + body + "}" + ("," if i < len(recs) - 1 else ""))
        lines.append("  ]" + ("," if key != "specials" else ""))
    lines.append("}")
    with open(path, "w", encoding="utf-8") as f:
        f.write("\n".join(lines) + "\n")


def main():
    out_dir = os.path.dirname(os.path.abspath(__file__))
    import pyproj
    generator = "pyproj %s / PROJ %s" % (pyproj.__version__, pyproj.proj_version_str)

    summary = []
    for name, proj in CASES.items():
        tgt = CRS.from_user_input(proj)
        # Geographic source built from the *same* string with only +proj swapped:
        # identical ellipsoid/datum => pure projection, no datum shift.
        geo = CRS.from_user_input(re.sub(r"\+proj=\w+", "+proj=longlat", proj))
        fwd = Transformer.from_crs(geo, tgt, always_xy=True)
        inv = Transformer.from_crs(tgt, geo, always_xy=True)

        p = params(proj)
        lon0 = p.get("lon_0", 0.0)
        lat0 = p.get("lat_0", 0.0)
        # Sanity: the projection centre must map to (x_0, y_0). A small offset would
        # mean the transformer is doing a datum shift (fixture invalid). A huge offset
        # is just longitude wrap (pyproj maps input lon 180 to -180), which is fine.
        cx, cy = fwd.transform(lon0, lat0)
        if not finite((cx, cy)):
            raise SystemExit("%s: centre sanity check failed: (%r, %r)" % (name, cx, cy))
        dx, dy = cx - p.get("x_0", 0.0), cy - p.get("y_0", 0.0)
        if abs(dx) > 1e-6 and abs(dx) < 1e6:
            raise SystemExit("%s: centre sanity check failed (datum shift?): (%r, %r)" % (name, cx, cy))
        if abs(dy) > 1e-6 and abs(dy) < 1e6:
            raise SystemExit("%s: centre sanity check failed (datum shift?): (%r, %r)" % (name, cx, cy))

        forward, inverse, specials = [], [], []

        # grid
        for i in range(N):
            lat = LAT_MIN + (LAT_MAX - LAT_MIN) * i / (N - 1)
            for j in range(N):
                lon = LON_MIN + (LON_MAX - LON_MIN) * j / (N - 1)
                x, y = fwd.transform(lon, lat)
                if not finite((x, y)):
                    specials.append({"desc": "grid point outside domain",
                                     "lon": lon, "lat": lat, "expectNaN": True})
                    continue
                forward.append({"lon": lon, "lat": lat, "x": x, "y": y})
                rl, la = inv.transform(x, y)
                if not finite((rl, la)):
                    specials.append({"desc": "inverse of own forward point failed",
                                     "x": x, "y": y, "expectNaN": True})
                    continue
                inverse.append({"x": x, "y": y, "lon": norm_lon(rl), "lat": la})

        # extra sanity points
        for lon, lat in EXTRA_POINTS:
            x, y = fwd.transform(lon, lat)
            if finite((x, y)):
                forward.append({"lon": lon, "lat": lat, "x": x, "y": y})

        # domain probes
        for probe in SPECIALS[name]:
            if "lon" in probe:
                x, y = fwd.transform(probe["lon"], probe["lat"])
                rec = dict(probe)
                if finite((x, y)):
                    rec.update({"x": x, "y": y, "expectNaN": False})
                    forward.append(rec)
                else:
                    specials.append({"desc": probe["desc"], "lon": probe["lon"],
                                     "lat": probe["lat"], "expectNaN": True})
            else:
                rl, la = inv.transform(probe["x"], probe["y"])
                rec = dict(probe)
                if finite((rl, la)):
                    rec.update({"lon": norm_lon(rl), "lat": la, "expectNaN": False})
                    specials.append(rec)
                else:
                    specials.append({"desc": probe["desc"], "x": probe["x"],
                                     "y": probe["y"], "expectNaN": True})

        path = os.path.join(out_dir, name + ".json")
        obj = {
            "crs": proj,
            "generator": generator,
            "generated": existing_generated(path) or datetime.datetime.now(
                datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
            "forward": forward,
            "inverse": inverse,
            "specials": specials,
        }
        write_json(path, obj)
        summary.append((name, len(forward), len(inverse), len(specials)))

    print("wrote %d golden files to %s" % (len(summary), out_dir))
    for name, nf, ni, ns in summary:
        print("  %-16s forward=%4d inverse=%4d specials=%d" % (name, nf, ni, ns))


if __name__ == "__main__":
    sys.exit(main())
