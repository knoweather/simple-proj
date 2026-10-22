#!/usr/bin/env python3
"""
xcheck/gen_geodesic.py — geodesic reference generator (Karney solver parity).

Why: aeqd's ellipsoidal oblique/equatorial aspects are defined by PROJ through
its bundled GeographicLib geodesic solver (geod_inverse / geod_direct). The Go
side ports that solver (geodesic.go), so it must be cross-checked on its own —
before aeqd ever gets involved — otherwise an aeqd mismatch cannot be attributed
to the projection or to the geodesic.

Golden truth: pyproj's Geod (same GeographicLib code as PROJ's src/geodesic.c).

Two case families:
  * inverse: (lat1, lon1, lat2, lon2) -> (s12, azi1, azi2)
      N.B. pyproj's Geod.inv returns the BACK azimuth at point 2, while
      GeographicLib's geod_inverse (what PROJ calls) returns the FORWARD
      azimuth at point 2. The fixture stores the forward one (az2 + 180,
      normalized) so the Go port can be compared directly.
  * direct:  (lat1, lon1, azi1, s12) -> (lat2, lon2, azi2)

Run from repo root:  python xcheck/gen_geodesic.py
Deps:                pyproj
"""

import datetime
import json
import math
import os
import sys

from pyproj import Geod


class LCG:
    """Deterministic LCG (Numerical Recipes constants), same as gen_sweep.py."""

    def __init__(self, seed):
        self.s = seed & 0x7FFFFFFF

    def next(self):
        self.s = (1664525 * self.s + 1013904223) & 0x7FFFFFFF
        return self.s / 0x7FFFFFFF

    def uniform(self, a, b):
        return a + (b - a) * self.next()


def norm180(x):
    """Wrap an angle to (-180, 180]."""
    y = math.fmod(x + 180.0, 360.0)
    if y <= 0:
        y += 360.0
    return y - 180.0


# Structured inverse pairs: identity, meridians, equator, poles, antipodes,
# very short lines, and the classic hard cases from the GeographicLib test set.
INVERSE_EDGES = [
    (0, 0, 0, 0),            # identical points
    (0, 0, 0, 1),            # 1 deg of longitude on the equator
    (0, 0, 1, 0),            # 1 deg of latitude
    (0, 0, 45, 45),
    (20, 10, 60, 30),
    (0, 0, 0, 180),          # antipodal on the equator
    (0, 0, 0.0001, 180),     # nearly antipodal
    (0, 0, 0.5, 179.9),
    (45, 0, -45, 180),       # antipodal, non-equatorial
    (89, 0, 89, 180),        # near the north pole
    (-89.9, 0, -89.9, 179.9),
    (90, 0, -90, 0),         # pole to pole
    (90, 0, 0, 0),
    (40, -75, 40, -75.0001),  # 8 m apart
    (52.784459512564, 0, -52.784459512563990912, 179.634407464943777557),
    (48.522876735459, 0, -48.52287673545898293, 179.599720456223079643),
    (-33.9, 151.2, 51.5, -0.13),  # Sydney -> London
    (35.7, 139.7, -33.9, 151.2),  # Tokyo -> Sydney
    (0, 179.999, 0, -179.999),    # across the antimeridian
]

DIRECT_EDGES = [
    (0, 0, 90, 111319.49079327357),
    (0, 0, 45, 1000000.0),
    (0, 0, 0, 10001965.729),      # quarter meridian
    (45, 10, 35.410058905, 6662472.718210),
    (-33.9, 151.2, 300.0, 5000000.0),  # heading north-west
    (89.5, 0, 0, 200000.0),       # over the pole
    (0, 179.5, 90, 200000.0),     # across the antimeridian
    (10, 20, -90, 1234567.89),
    (0, 0, 180, 20003931.458625), # half the equator
]

N_RANDOM = 60


def main():
    out_dir = os.path.dirname(os.path.abspath(__file__))
    import pyproj

    g = Geod(ellps="WGS84")
    rng = LCG(20261022)

    inverse = []
    for lat1, lon1, lat2, lon2 in INVERSE_EDGES:
        az1, az2, s12 = g.inv(lon1, lat1, lon2, lat2)
        inverse.append({"lat1": lat1, "lon1": lon1, "lat2": lat2, "lon2": lon2,
                        "s12": s12, "azi1": az1, "azi2": norm180(az2 + 180.0)})

    direct = []
    for lat1, lon1, azi1, s12 in DIRECT_EDGES:
        lon2, lat2, az2 = g.fwd(lon1, lat1, azi1, s12)
        direct.append({"lat1": lat1, "lon1": lon1, "azi1": azi1, "s12": s12,
                       "lat2": lat2, "lon2": lon2, "azi2": norm180(az2 + 180.0)})

    # Random pairs, including a band that stresses the near-antipodal branch.
    for _ in range(N_RANDOM):
        lat1 = rng.uniform(-89.5, 89.5)
        lon1 = rng.uniform(-180.0, 180.0)
        if rng.next() < 0.35:
            # near-antipodal: flip both signs and stay within ~2 deg
            lat2 = -lat1 + rng.uniform(-2.0, 2.0)
            lon2 = lon1 + 180.0 + rng.uniform(-2.0, 2.0)
        else:
            lat2 = rng.uniform(-89.5, 89.5)
            lon2 = rng.uniform(-180.0, 180.0)
        lat2 = max(-89.9, min(89.9, lat2))
        lon2 = norm180(lon2)
        az1, az2, s12 = g.inv(lon1, lat1, lon2, lat2)
        if not all(math.isfinite(v) for v in (az1, az2, s12)):
            continue
        inverse.append({"lat1": lat1, "lon1": lon1, "lat2": lat2, "lon2": lon2,
                        "s12": s12, "azi1": az1, "azi2": norm180(az2 + 180.0)})
        lon3, lat3, az3 = g.fwd(lon1, lat1, az1, s12)
        direct.append({"lat1": lat1, "lon1": lon1, "azi1": az1, "s12": s12,
                       "lat2": lat3, "lon2": lon3, "azi2": norm180(az3 + 180.0)})

    out = {
        "generator": "pyproj %s / PROJ %s (Geod, ellps=WGS84)"
                     % (pyproj.__version__, pyproj.proj_version_str),
        "generated": datetime.datetime.now(datetime.timezone.utc)
        .strftime("%Y-%m-%dT%H:%M:%SZ"),
        "note": "geodesic solver parity for geodesic.go (consumed by geodesic_test.go); "
                "azi2 is the FORWARD azimuth at point 2 (pyproj returns the back azimuth)",
        "ellipsoid": {"a": 6378137.0, "rf": 298.257223563},
        "inverse": inverse,
        "direct": direct,
    }

    path = os.path.join(out_dir, "geodesic_ref.json")
    with open(path, "w", encoding="utf-8") as f:
        json.dump(out, f, ensure_ascii=False, indent=1)
        f.write("\n")
    print("wrote %s (inverse=%d direct=%d)" % (path, len(inverse), len(direct)))
    return 0


if __name__ == "__main__":
    sys.exit(main())
