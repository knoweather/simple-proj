# Build the WASM front end for the browser (Windows).
# Usage:  pwsh wasm/build.ps1
# Set GO_BIN to a specific go.exe if "go" is not on PATH.
$ErrorActionPreference = "Stop"

$go = $env:GO_BIN
if (-not $go) { $go = "go" }

$goroot = & $go env GOROOT
$env:GOOS = "js"
$env:GOARCH = "wasm"
$env:CGO_ENABLED = "0"

& $go build -o wasm/example/main.wasm ./wasm/main
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

Copy-Item "$goroot/lib/wasm/wasm_exec.js" -Destination wasm/example/wasm_exec.js -Force
Write-Host "built wasm/example/main.wasm (+ wasm_exec.js)"
