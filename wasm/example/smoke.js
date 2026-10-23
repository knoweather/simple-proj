// Node smoke test for the WASM front end. Run after building main.wasm and
// copying wasm_exec.js into this directory:
//
//   GOOS=js GOARCH=wasm go build -o wasm/example/main.wasm ./wasm/main
//   cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" wasm/example/
//   node wasm/example/smoke.js
//
// Exits 0 on success; prints the failure and exits 1 otherwise.
"use strict";

const fs = require("fs");
const path = require("path");

require(path.join(__dirname, "wasm_exec.js"));

const go = new Go();
const wasmPath = path.join(__dirname, "main.wasm");

WebAssembly.instantiate(fs.readFileSync(wasmPath), go.importObject)
  .then((result) => {
    go.run(result.instance);
    // Register runs on the Go side before the first JS turn; a short delay is
    // enough for it to install the global object.
    setTimeout(() => {
      try {
        if (typeof simpleProj !== "object") {
          throw new Error("simpleProj was not registered");
        }

        const merc = simpleProj.parse("+proj=merc +a=6378137 +b=6378137 +units=km");
        if (merc.error) throw new Error("parse: " + merc.error);
        const fwd = simpleProj.forward(merc.id, 10, 0);
        if (fwd.error) throw new Error("forward: " + fwd.error);
        if (Math.abs(fwd.x - 1113.1949) > 1e-3 || Math.abs(fwd.y) > 1e-9) {
          throw new Error("forward(10,0) = " + JSON.stringify(fwd));
        }

        const batch = simpleProj.forwardBatch(merc.id, new Float64Array([0, 0, 10, 0]));
        if (batch.error) throw new Error("forwardBatch: " + batch.error);
        if (!(batch.xy instanceof Float64Array) || batch.xy.length !== 4) {
          throw new Error("forwardBatch did not return a Float64Array of length 4");
        }

        const geos = simpleProj.parse("+proj=geos +h=35785863 +lon_0=0 +ellps=WGS84");
        if (geos.error) throw new Error("parse geos: " + geos.error);
        const outside = simpleProj.forward(geos.id, 180, 0);
        if (!outside.domain) {
          throw new Error("far-side geos point should report domain:true");
        }

        const from = simpleProj.parse("EPSG:4326");
        const to = simpleProj.parse("EPSG:3857");
        const tr = simpleProj.transform(from.id, to.id, 0, 0);
        if (tr.error || tr.x !== 0 || tr.y !== 0) {
          throw new Error("transform(0,0) = " + JSON.stringify(tr));
        }

        if (!simpleProj.free(merc.id) || simpleProj.free(merc.id)) {
          throw new Error("free() should return true once, then false");
        }

        console.log("wasm smoke: ok");
        process.exit(0);
      } catch (err) {
        console.error("wasm smoke failed:", err.message);
        process.exit(1);
      }
    }, 300);
  })
  .catch((err) => {
    console.error("wasm smoke failed to load:", err);
    process.exit(1);
  });
