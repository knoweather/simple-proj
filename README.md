# simple-proj

**English** | [简体中文](README.zh-CN.md)

[![CI](https://github.com/knoweather/simple-proj/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/knoweather/simple-proj/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/knoweather/simple-proj.svg)](https://pkg.go.dev/github.com/knoweather/simple-proj)
[![Go version](https://img.shields.io/github/go-mod/go-version/knoweather/simple-proj)](go.mod)
[![License](https://img.shields.io/github/license/knoweather/simple-proj)](LICENSE)
[![Latest tag](https://img.shields.io/github/v/tag/knoweather/simple-proj?sort=semver)](https://github.com/knoweather/simple-proj/tags)

Pure-Go coordinate projections for weather and satellite grids: no cgo, no
third-party dependencies, only the standard library.

`simple-proj` implements the subset of PROJ that GRIB and satellite products
actually use — lon/lat, Mercator, Lambert conformal conic, polar stereographic,
geostationary view, UTM, rotated-pole grids — plus WKT input and CRS-to-CRS
transforms. Numerical behaviour is checked against **pyproj 3.8.0 / PROJ 9.8.1**
on every commit.

**Status:** v0.2.0 · Go 1.27+ · 11 projections · statement coverage 98.3%

## Why another projection library?

- **No cgo.** A pure-Go build cross-compiles and statically links anywhere; no
  PROJ shared library, no C toolchain, no runtime data directory.
- **Runs everywhere Go does.** No cgo means trivial cross-compilation; the library
  and its full test suite also build and run as **js/wasm** (verified under Node).
- **Built for grids.** Packed batch APIs, per-point domain errors, and GRIB
  `shape_of_the_earth` → ellipsoid conversion are first-class.
- **Verified, not approximated.** Every projection is compared point-by-point
  with pyproj on two independent point sets: a 33×33 golden grid and a 22-case
  edge/random sweep. The fixtures are committed, so `go test` needs no Python.

## Install

```bash
go get github.com/knoweather/simple-proj
```

```go
import proj "github.com/knoweather/simple-proj"
```

## Quick start

```go
crs, err := proj.Parse("+proj=merc +a=6378137 +b=6378137 +units=km")
if err != nil {
    // bad definition
}
p, err := crs.NewProjection()   // build the instance once, then reuse it
if err != nil {
    // unsupported projection or parameter
}

x, y, err := p.Forward(10, 0)   // degrees -> kilometres
lon, lat, err := p.Inverse(x, y)
```

Batch conversion uses packed `[lon0,lat0,lon1,lat1,...]` slices:

```go
xy, err := proj.ForwardBatch(p, lonLat) // out-of-domain points become NaN
if errors.Is(err, proj.ErrDomain) {
    // the batch completed; (NaN,NaN) marks the points outside the domain
}
```

CRS-to-CRS conversion, including `+towgs84` Helmert transforms:

```go
from, _ := proj.Parse("EPSG:4326")
to, _ := proj.Parse("EPSG:3857")
tr, _ := proj.NewTransformer(from, to)
x, y, err := tr.Transform(0, 0)
```

Runnable versions of these snippets live in [example_test.go](example_test.go)
and run as part of `go test`.

## WebAssembly (WASM)

The `wasm/` subpackage runs the same projection code in browsers and Node.js,
with parsed CRS handles kept on the Go side:

```bash
pwsh wasm/build.ps1      # or ./wasm/build.sh
```

```js
const p = simpleProj.parse("+proj=geos +h=35785863 +lon_0=140.7 +ellps=WGS84");
const { x, y } = simpleProj.forward(p.id, 140.7, 30.5);
const { xy, domain } = simpleProj.forwardBatch(p.id, new Float64Array([0, 0, 140.7, 30.5]));
```

`simpleProj` exposes `parse`, `forward`, `inverse`, `forwardBatch`,
`inverseBatch`, `transform`, `transformBatch`, `info` and `free`. See
[wasm/README.md](wasm/README.md) for the full API, batch semantics and the
browser demo.

## Supported projections

| `+proj=` | Description | Sphere / ellipsoid | Notes |
|---|---|---|---|
| `latlong` / `longlat` | longitude/latitude identity | — | output in degrees |
| `eqc` | Equidistant Cylindrical (Plate Carrée) | ✓ | `+lat_ts`; `k_0` ignored |
| `geos` | Geostationary Satellite View | ✓ | `+h` required; `+sweep=x/y` |
| `stere` | Stereographic (polar/oblique/equatorial) | ✓ | `+lat_ts` folds into scale |
| `laea` | Lambert Azimuthal Equal-Area | ✓ | `k_0` ignored |
| `lcc` | Lambert Conformal Conic | ✓ | one or two standard parallels |
| `merc` | Mercator | ✓ | `+lat_ts`; sphere or ellipsoid |
| `tmerc` | Transverse Mercator (Poder/Engsager 6th order) | ✓ | `|Ce| ≤ 2.6234 rad` |
| `aeqd` | Azimuthal Equidistant | ✓ | ellipsoidal oblique uses Karney geodesics |
| `utm` | Universal Transverse Mercator | ✓ | `+zone=1..60`, `+south` |
| `ob_tran` | Rotated pole / oblique transverse | ✓ | GRIB2 templates 3.1/3.3 |

Also supported:

- `+units=` / `+to_meter=` (m, km, ft, us-ft, mi, …), applied at the API layer;
- WKT2 and WKT1 subset parsing (`Parse` auto-detects WKT);
- EPSG shortcuts: 4326, 4269, 3857, 900913, and WGS84 UTM 32601–32660 / 32701–32760;
- `+towgs84` 3- and 7-parameter Helmert transforms through `NewTransformer`;
- GRIB `shape_of_the_earth` 0–11 plus dual-axis shapes 3/7 (`GRIBEllipsoid`,
  `GRIBEllipsoidAxes`);
- `Info()` metadata (`Name`, `Domain`, `HasInverse`, `Accuracy`, …) and
  reason-carrying `DomainError` (`errors.As` gives `far-pole`, `antipode`,
  `ce-limit`, …) while `errors.Is(err, ErrDomain)` stays the primary check.

## Correctness

The reference data was generated with **pyproj 3.8.0 / PROJ 9.8.1** and is
committed under [xcheck/](xcheck/README.md):

- golden grids: 33×33 points for each projection, forward and inverse;
- 22-case extension sweep with structured edges and fixed-seed random points,
  including domain boundaries and unit scaling;
- WKT parity: 14 CRS × 2 dialects, parsed and compared against pyproj output;
- geodesics: 86 inverse + 71 direct cases, including near-antipodal and 8 m lines;
- rotated pole: 7 configurations, both directions.

Acceptance gates enforced by the test suite:

| Axis | Gate |
|---|---|
| forward | ≤ 1e-6 m absolute, with a relative floor for coordinates above ~1e6 m |
| inverse | ≤ 1e-9 deg absolute, with a relative floor for extreme displacements |
| round trip | ≤ 1e-9 deg, except at documented ill-conditioned edges |
| longitude | compared on the circle (−180° and +180° are the same meridian) |

The package has **98.3% statement coverage**; the remaining paths are defensive
branches (nil receivers, impossible constructor states) covered by dedicated
tests where reachable.

## Concurrency

`CRS`, `Projection` and `Transformer` are immutable after construction and can
be shared by any number of goroutines. Batch calls allocate only their output
slice; no per-point allocations, no locks, no global caches.

`TestThreadSafety` hammers every projection from 8 goroutines and compares the
results bit-for-bit with a serial run; the race detector runs on every push in
CI (`go test -race ./...`).

One rule: treat the `CRS` you get from `Parse` as read-only. Its exported
`Params` / `UnknownParams` / `Towgs84` fields are shared with the projections
and metadata helpers, so mutating them concurrently is a data race.

## Performance

1e6 points per batch, packed input, output allocation included. All rows are
**1 alloc/op** (the 16 MB output slice). Measured on Windows 11, AMD Ryzen 7
7840HS, Go 1.27.3, 2026-10-23; numbers vary with machine and load.

| Projection | ForwardBatch | InverseBatch |
|---|---|---|
| `latlong` | 18 ms | 18 ms |
| `eqc` | 48 ms | 43 ms |
| `geos` | 108 ms | 91 ms |
| `ob_tran` | 84 ms | 80 ms |
| `aeqd` (ellipsoidal oblique) | 898 ms | 356 ms |

`aeqd` is the outlier by design: its ellipsoidal oblique/equatorial forward path
solves a geodesic per point, matching PROJ's algorithm. For equal-area,
conformal and Mercator workloads the per-point cost stays in the tens of
nanoseconds.

Reproduce with:

```bash
go test -run '^$' -bench . -benchmem
```

## API overview

The core surface is small:

- types: `CRS`, `Projection`, `Transformer`, `Ellipsoid`, `DomainError`;
- constructors: `Parse`, `ParseWKT`, `CRS.NewProjection`, `NewTransformer`;
- conversion: `Forward`, `Inverse`, `Transform`, and the batch variants;
- errors: `ErrDomain` plus `errors.As` for `*DomainError`;
- introspection: `CRS.String/EPSG/IsGeographic/Units/Equals`, `Info`;
- GRIB: `GRIBEllipsoid`, `GRIBEllipsoidAxes`.

Until v1.0, minor releases may still adjust edge-case behaviour; exported names
and signatures are kept stable unless a release note says otherwise.

## GRIB mapping

Typical GRIB2 `grid_template_number` mapping:

| Template | `+proj=` |
|---|---|
| 3.0 / 3.1 regular and rotated lat/lon | `latlong`, or `ob_tran` for rotated grids |
| 3.10 Mercator | `merc` |
| 3.20 / 3.30 Lambert conformal | `lcc` |
| 3.40 / 3.41 geostationary | `geos` |

Rotated grids: GRIB gives the south pole `(LaSP, LoSP)`; use
`+o_lat_p = -LaSP` and `+lon_0 = LoSP` (for example, ECMWF's north pole at
39.25°N, 162°W becomes `+o_lat_p=39.25 +lon_0=18`).

## Limitations

Deliberately out of scope:

- grid-shift datums (`+nadgrids` NTv2/HARN), dynamic/ITRF datums;
- `+proj=pipeline` syntax — use `NewTransformer` between two CRSs;
- a full EPSG database (only the shortcuts listed above);
- WKT output, PROJJSON, and ESRI dialect details;
- 3D/4D coordinates, vertical datums, geoid grids;
- authority axis-order swapping: coordinates are always `lon,lat` / `x,y`;
- framework-level parallelism (the batch loops are already tight and
  allocation-free).

Parsing is strict: parameters whose semantics are not implemented (`+pm`,
`+axis`, `+nadgrids`, non-WGS84 `+datum`, the `+R_A` family, `+geoc`, `+over`,
…) fail at `Parse` instead of being silently ignored.

## Related projects

- [simple-grib](https://github.com/knoweather/simple-grib) — pure-Go GRIB1/GRIB2
  reader/writer, the source of grid definitions this library consumes;
- simple-sat — the satellite data stack that uses `simple-proj` for resampling.

## License

MIT — see [LICENSE](LICENSE).
