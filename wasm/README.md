# simple-proj · WebAssembly (WASM)

A small WebAssembly module that runs the projection math in the **browser** and
in **Node.js**, with no cgo and no native build step. The Go side keeps parsed
CRS handles; JavaScript calls projection operations by handle.

## Build

From the repository root:

```bash
# Windows (PowerShell)
pwsh wasm/build.ps1

# Linux / macOS
./wasm/build.sh
```

This produces `wasm/example/main.wasm` and copies the Go runtime glue
(`wasm_exec.js`) next to it. Set `GO_BIN` if `go` is not on `PATH`.

Serve the demo (WASM cannot be fetched over `file://`):

```bash
cd wasm/example
python3 -m http.server 8080
# open http://localhost:8080/
```

## JavaScript API

After `main.wasm` is instantiated with `wasm_exec.js`, the global `simpleProj`
object is available:

```js
const p = simpleProj.parse("+proj=geos +h=35785863 +lon_0=140.7 +ellps=WGS84");
// { id, name, description, crs, units, domain, accuracy,
//   epsg, spherical, ellipsoidal, hasInverse, geographic }  |  { error }

const { x, y } = simpleProj.forward(p.id, 140.7, 30.5);   // degrees -> x/y
const { lon, lat } = simpleProj.inverse(p.id, x, y);      // x/y -> degrees

const batch = simpleProj.forwardBatch(p.id, new Float64Array([0, 0, 140.7, 30.5]));
// { xy: Float64Array, domain?: true }  |  { error }
```

Full surface:

| Call | Result |
|---|---|
| `parse(def)` | handle + metadata, or `{ error }` |
| `forward(id, lon, lat)` | `{ x, y }`, or `{ error, domain?, reason?, detail? }` |
| `inverse(id, x, y)` | `{ lon, lat }`, or an error object |
| `forwardBatch(id, coords)` | `{ xy: Float64Array, domain? }` |
| `inverseBatch(id, xy)` | `{ xy: Float64Array, domain? }` (packed lon/lat) |
| `transform(fromId, toId, x, y)` | `{ x, y }` |
| `transformBatch(fromId, toId, xy)` | `{ xy: Float64Array, domain? }` |
| `info(id)` | metadata object |
| `free(id)` | `true` if the handle existed |

Notes:

- Batch input may be a `Float64Array`, an `ArrayBuffer`, or a plain array of
  numbers; batch results are always `Float64Array`.
- `domain: true` means at least one point fell outside the projection domain;
  those points are `NaN` and the remaining points are still valid.
- Handles stay valid until `free(id)`. Transformers are cached per handle pair
  and dropped automatically when either handle is freed.
- `parse` accepts proj strings, WKT2/WKT1 and `EPSG:xxxx` shortcuts, exactly
  like the Go API.
- Coordinates are always `lon,lat` / `x,y`; the values use the CRS unit
  (`+units=km` yields kilometres, `latlong` yields degrees).

## Package layout

| File | Purpose |
|---|---|
| `wasm/api.go` | framework-independent engine (handle registry, batch, transforms) — unit tested on the host |
| `wasm/wasm.go` | `//go:build js && wasm` `syscall/js` bindings + `Register()` |
| `wasm/main/main.go` | `//go:build js && wasm` entry point (build target) |
| `wasm/api_test.go` | host tests (no browser needed) |
| `wasm/example/index.html` | browser demo |
| `wasm/example/smoke.js` | Node smoke test used by CI |
| `wasm/build.ps1` / `wasm/build.sh` | build helpers |

The generated `wasm/example/main.wasm` and `wasm/example/wasm_exec.js` are
build artifacts and are not committed.
