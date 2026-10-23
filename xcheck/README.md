# xcheck — reference fixtures for numerical parity

This directory holds the reference data used by the Go tests. The fixtures are
generated once with **pyproj 3.8.0 / PROJ 9.8.1**, committed as plain JSON, and
consumed by the test suite without Python or pyproj installed.

The rule is simple: **pyproj is the oracle**. If an implementation disagrees
with a fixture, the implementation is wrong unless the fixture itself is shown
to be generated incorrectly.

## Fixtures

| File | Content | Generator | Consumer |
|---|---|---|---|
| `golden_latlong.json` | 33×33 lon/lat grid, forward + inverse | `gen_golden.py` | `xcheck_test.go` |
| `golden_eqc.json` | same grid for Equidistant Cylindrical | `gen_golden.py` | `xcheck_test.go` |
| `golden_geos_x.json` | geostationary sweep=x, visibility/NaN probes | `gen_golden.py` | `xcheck_test.go` |
| `golden_geos_y.json` | geostationary sweep=y, visibility/NaN probes | `gen_golden.py` | `xcheck_test.go` |
| `golden_stere.json` | polar stereographic | `gen_golden.py` | `stere_test.go` |
| `golden_laea.json` | Lambert azimuthal equal-area | `gen_golden.py` | `laea_test.go` |
| `golden_lcc.json` | Lambert conformal conic (2 standard parallels) | `gen_golden.py` | `lcc_test.go` |
| `golden_merc.json` | spherical Mercator | `gen_golden.py` | `merc_test.go` |
| `golden_tmerc.json` | transverse Mercator (Poder/Engsager) | `gen_golden.py` | `tmerc_test.go` |
| `sweep_extra.json` | 22 independent cases: structured edges plus a fixed-seed LCG, including domain edges and unit scaling | `gen_sweep.py` | `sweep_extra_test.go` |
| `geodesic_ref.json` | 86 inverse + 71 direct geodesic cases (near-antipodal, polar, 8 m lines, prolate ellipsoid) | `gen_geodesic.py` | `geodesic_test.go` |
| `wkt_ref.json` | 14 CRS × WKT2/WKT1 text plus the forward coordinates of each dialect | `gen_wkt.py` | `wkt_test.go` |
| `obtran_ref.json` | 7 rotated-pole configurations × both directions | `gen_obtran.py` | `impl_obtran_test.go` |

`wkt_ref.json` stores the reference produced by parsing each WKT dialect with
pyproj, not by parsing the equivalent proj string: WKT1 loses some datum
information, and the fixture intentionally captures that difference.

## Tolerances

| Axis | Gate | Detail |
|---|---|---|
| forward | ≤ 1e-6 m absolute | huge coordinates use a relative floor `max(1e-6, |expected|·1e-12)` |
| inverse | ≤ 1e-9 deg absolute | extreme displacements use a relative floor `max(1e-9, |x|/a·180/π·2.3e-16)` |
| round trip | ≤ 1e-9 deg | at domain edges or ill-conditioned points only the single direction is checked |
| longitude | circular comparison | −180 and +180 are the same meridian |

The relative floors exist because at 1e9–1e12 m the double-precision ulp alone
is far above the absolute gate; the individual residuals are recorded in the
matrix tests and in `Info().Accuracy`.

## Regenerating

```bash
pip install "pyproj==3.8.0"
python xcheck/gen_golden.py
python xcheck/gen_sweep.py
python xcheck/gen_geodesic.py
python xcheck/gen_wkt.py
python xcheck/gen_obtran.py
go test ./...
```

Regenerate only when the reference set intentionally changes. A regenerated
fixture must be reviewed together with the code change that consumes it; a
fixture diff without a code diff usually means the local pyproj version moved.
