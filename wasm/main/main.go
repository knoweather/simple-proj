//go:build js && wasm

// Command wasm is the WebAssembly entry point. Build it with
//
//	GOOS=js GOARCH=wasm go build -o main.wasm ./wasm/main
//
// to get a module that, once instantiated with wasm_exec.js, exposes the
// `simpleProj` object on the JavaScript global scope (see wasm.Register).
package main

import "github.com/knoweather/simple-proj/wasm"

func main() {
	wasm.Register()
	// Block forever so the Go runtime — and the JS callbacks it registered —
	// stay alive for the lifetime of the page.
	select {}
}
