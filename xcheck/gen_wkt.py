#!/usr/bin/env python3
"""
xcheck/gen_wkt.py — WKT2 / WKT1 round-trip reference generator.

Purpose: the Go side parses WKT by TRANSLATING it to an equivalent proj string
and re-entering Parse (see wkt.go). The risk of that design is silent parameter
mix-ups (a WKT parameter landing on the wrong proj key, a unit applied twice,
the pseudo-Mercator sphere case). This fixture closes the loop end-to-end:
pyproj emits WKT2 and WKT1 for each CRS, and the Go test must reproduce pyproj's
own forward coordinates from that WKT.

Run from repo root:  python xcheck/gen_wkt.py
Deps:                pyproj
"""

import datetime
import json
import math
import os
import sys

from pyproj import CRS, Transformer

# (name, proj4) — EPSG codes are used where the CRS has one, so the fixture also
# covers the "ID[...] fast path"; proj strings cover the translation path.
CASES = [
    ("epsg4326", "EPSG:4326"),
    ("epsg3857", "EPSG:3857"),
    ("epsg32650", "EPSG:32650"),
    ("epsg32733", "EPSG:32733"),
    ("tmerc", "+proj=tmerc +lat_0=0 +lon_0=105 +k=0.9996 +ellps=WGS84"),
    ("lcc2sp", "+proj=lcc +lat_1=30 +lat_2=60 +lat_0=0 +lon_0=100 +ellps=WGS84"),
    ("laea", "+proj=laea +lat_0=45 +lon_0=10 +ellps=WGS84"),
    ("stere_b", "+proj=stere +lat_0=90 +lat_ts=60 +lon_0=-100 +ellps=WGS84"),
    ("geos", "+proj=geos +h=35785863 +lon_0=140.7 +ellps=WGS84"),
    ("eqc", "+proj=eqc +lat_ts=30 +lon_0=180 +ellps=WGS84"),
    ("aeqd", "+proj=aeqd +lat_0=45 +lon_0=10 +ellps=WGS84"),
    ("merc_km", "+proj=merc +a=6378137 +b=6378137 +units=km"),
    ("lcc_usft", "+proj=lcc +lat_1=33 +lat_2=45 +lat_0=23 +lon_0=-96 "
                 "+ellps=WGS84 +units=us-ft"),
]

# Hand-written WKT2 for cases PROJ cannot express as a proj string (1SP LCC has
# no +proj spelling: PROJ's lcc always needs lat_1, so the 1SP form only exists
# in WKT/EPSG land). pyproj parses the WKT itself to produce the reference.
HAND_WKT2 = {
    "lcc1sp": (
        'PROJCRS["LCC 1SP probe",'
        'BASEGEOGCRS["WGS 84",'
        'DATUM["World Geodetic System 1984",'
        'ELLIPSOID["WGS 84",6378137,298.257223563,LENGTHUNIT["metre",1]]],'
        'PRIMEM["Greenwich",0,ANGLEUNIT["degree",0.0174532925199433]]],'
        'CONVERSION["unnamed",'
        'METHOD["Lambert Conic Conformal (1SP)"],'
        'PARAMETER["Latitude of natural origin",25,ANGLEUNIT["degree",0.0174532925199433]],'
        'PARAMETER["Longitude of natural origin",-95,ANGLEUNIT["degree",0.0174532925199433]],'
        'PARAMETER["Scale factor at natural origin",0.9999,SCALEUNIT["unity",1]],'
        'PARAMETER["False easting",1000000,LENGTHUNIT["metre",1]],'
        'PARAMETER["False northing",0,LENGTHUNIT["metre",1]]],'
        'CS[Cartesian,2],'
        'AXIS["easting",east,ORDER[1],LENGTHUNIT["metre",1]],'
        'AXIS["northing",north,ORDER[2],LENGTHUNIT["metre",1]]]'
    ),
}

# Forward probes (lon, lat) — one per projection region, chosen to stay inside
# every domain (aeqd/stere/lcc/geos all fine at these).
PROBES = [
    (10.0, 20.0), (100.0, 30.0), (-95.0, 40.0), (140.0, 10.0), (0.0, 0.0),
    (-170.0, 45.0), (45.0, 80.0), (179.9, -30.0), (-60.0, -45.0),
    (120.0, -60.0), (30.0, 60.0), (150.0, 0.0),
]


def main():
    out_dir = os.path.dirname(os.path.abspath(__file__))
    import pyproj

    cases = []
    all_cases = [(n, s) for n, s in CASES] + [(n, None) for n in HAND_WKT2]
    for name, src in all_cases:
        if src is None:
            base = CRS.from_wkt(HAND_WKT2[name])
            wkt2 = HAND_WKT2[name]
        else:
            base = CRS.from_user_input(src)
            wkt2 = base.to_wkt(version="WKT2_2019")
        try:
            wkt1 = base.to_wkt(version="WKT1_GDAL")
        except Exception:
            wkt1 = None

        # N.B. the reference is computed from EACH WKT separately, not from the
        # source CRS: WKT1 is lossy (e.g. GDAL's EPSG:3857 WKT1 says
        # "Mercator_1SP" and drops the pseudo-Mercator sphere), so "what pyproj
        # gets out of this exact WKT" is the only fair comparison target.
        variants = []
        for kind, wkt in (("WKT2", wkt2), ("WKT1", wkt1)):
            if not wkt:
                continue
            crs = CRS.from_wkt(wkt)
            fwd = Transformer.from_crs(CRS.from_epsg(4326), crs, always_xy=True)
            pts = []
            for lon, lat in PROBES:
                x, y = fwd.transform(lon, lat)
                if not (math.isfinite(x) and math.isfinite(y)):
                    # JSON has no inf/NaN; out-of-domain probes live in the
                    # sweep/golden fixtures instead.
                    continue
                pts.append({"lon": lon, "lat": lat, "x": x, "y": y})
            variants.append({"kind": kind, "wkt": wkt, "points": pts})

        cases.append({
            "name": name,
            "source": src if src is not None else "hand-written WKT2",
            "variants": variants,
        })

    out = {
        "generator": "pyproj %s / PROJ %s" % (pyproj.__version__, pyproj.proj_version_str),
        "generated": datetime.datetime.now(datetime.timezone.utc)
        .strftime("%Y-%m-%dT%H:%M:%SZ"),
        "note": "WKT2/WKT1 fixtures for wkt.go; each variant's x/y come from pyproj "
                "parsing THAT WKT and transforming from EPSG:4326 (always_xy)",
        "cases": cases,
    }

    path = os.path.join(out_dir, "wkt_ref.json")
    with open(path, "w", encoding="utf-8") as f:
        json.dump(out, f, ensure_ascii=False, indent=1)
        f.write("\n")
    print("wrote %s (%d cases, %d variants)"
          % (path, len(cases), sum(len(c["variants"]) for c in cases)))
    return 0


if __name__ == "__main__":
    sys.exit(main())
