#!/usr/bin/env bash
# Build the WASM front end for the browser.
# Usage:  ./wasm/build.sh
# Set GO_BIN to a specific go binary if "go" is not on PATH.
set -euo pipefail

GO="${GO_BIN:-go}"
export GOOS=js GOARCH=wasm CGO_ENABLED=0

"$GO" build -o wasm/example/main.wasm ./wasm/main
cp "$("$GO" env GOROOT)/lib/wasm/wasm_exec.js" wasm/example/wasm_exec.js
echo "built wasm/example/main.wasm (+ wasm_exec.js)"
