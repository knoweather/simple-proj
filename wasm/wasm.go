//go:build js && wasm

// Package wasm is the WebAssembly front end of simple-proj for browsers and
// Node.js: it keeps parsed CRS handles on the Go side and exposes projection
// math through the global `simpleProj` object. See wasm/README.md for the API
// and build instructions.
package wasm

import (
	"errors"
	"syscall/js"
	"unsafe"

	proj "github.com/knoweather/simple-proj"
)

// defaultEngine holds the CRS handles for the page / Node process.
var defaultEngine = NewEngine()

// jsFuncs keeps the js.Func values alive for the lifetime of the module.
var jsFuncs []js.Func

// Register installs the `simpleProj` object on the JavaScript global object:
//
//	simpleProj.parse(def)                     -> { id, name, ... } | { error }
//	simpleProj.forward(id, lon, lat)          -> { x, y } | { error, domain? }
//	simpleProj.inverse(id, x, y)              -> { lon, lat } | { error }
//	simpleProj.forwardBatch(id, lonlat)       -> { xy, domain? } | { error }
//	simpleProj.inverseBatch(id, xy)           -> { xy, domain? } | { error }
//	simpleProj.transform(from, to, x, y)      -> { x, y } | { error }
//	simpleProj.transformBatch(from, to, xy)   -> { xy, domain? } | { error }
//	simpleProj.info(id)                       -> metadata | { error }
//	simpleProj.free(id)                       -> bool
//
// Batch arrays may be Float64Array, ArrayBuffer or a plain Array of numbers;
// results are Float64Array. Domain errors mark the affected points as NaN and
// set domain:true while keeping the rest of the batch valid.
func Register() {
	api := object()
	add := func(name string, fn js.Func) {
		api.Set(name, fn)
		jsFuncs = append(jsFuncs, fn)
	}
	add("parse", js.FuncOf(parseFunc))
	add("forward", js.FuncOf(forwardFunc))
	add("inverse", js.FuncOf(inverseFunc))
	add("forwardBatch", js.FuncOf(forwardBatchFunc))
	add("inverseBatch", js.FuncOf(inverseBatchFunc))
	add("transform", js.FuncOf(transformFunc))
	add("transformBatch", js.FuncOf(transformBatchFunc))
	add("info", js.FuncOf(infoFunc))
	add("free", js.FuncOf(freeFunc))
	js.Global().Set("simpleProj", api)
}

func parseFunc(_ js.Value, args []js.Value) any {
	if len(args) < 1 || args[0].Type() != js.TypeString {
		return errObj(errors.New("simpleProj.parse: expected a CRS string"))
	}
	id, info, err := defaultEngine.Parse(args[0].String())
	if err != nil {
		return errObj(err)
	}
	o := infoObj(info)
	o.Set("id", id)
	return o
}

func forwardFunc(_ js.Value, args []js.Value) any {
	id, lon, lat, err := pointArgs(args)
	if err != nil {
		return errObj(err)
	}
	x, y, err := defaultEngine.Forward(id, lon, lat)
	if err != nil {
		return errObj(err)
	}
	return pointObj("x", x, "y", y)
}

func inverseFunc(_ js.Value, args []js.Value) any {
	id, x, y, err := pointArgs(args)
	if err != nil {
		return errObj(err)
	}
	lon, lat, err := defaultEngine.Inverse(id, x, y)
	if err != nil {
		return errObj(err)
	}
	return pointObj("lon", lon, "lat", lat)
}

func forwardBatchFunc(_ js.Value, args []js.Value) any {
	id, coords, err := batchArgs(args)
	if err != nil {
		return errObj(err)
	}
	out, domain, err := defaultEngine.ForwardBatch(id, coords)
	if err != nil {
		return errObj(err)
	}
	return batchObj(out, domain)
}

func inverseBatchFunc(_ js.Value, args []js.Value) any {
	id, coords, err := batchArgs(args)
	if err != nil {
		return errObj(err)
	}
	out, domain, err := defaultEngine.InverseBatch(id, coords)
	if err != nil {
		return errObj(err)
	}
	return batchObj(out, domain)
}

func transformFunc(_ js.Value, args []js.Value) any {
	if len(args) < 4 {
		return errObj(errors.New("simpleProj.transform: expected (fromId, toId, x, y)"))
	}
	from, ok := intArg(args[0])
	if !ok {
		return errObj(errors.New("simpleProj.transform: fromId must be an integer handle"))
	}
	to, ok := intArg(args[1])
	if !ok {
		return errObj(errors.New("simpleProj.transform: toId must be an integer handle"))
	}
	x, y, err := defaultEngine.Transform(from, to, args[2].Float(), args[3].Float())
	if err != nil {
		return errObj(err)
	}
	return pointObj("x", x, "y", y)
}

func transformBatchFunc(_ js.Value, args []js.Value) any {
	if len(args) < 3 {
		return errObj(errors.New("simpleProj.transformBatch: expected (fromId, toId, xy)"))
	}
	from, ok := intArg(args[0])
	if !ok {
		return errObj(errors.New("simpleProj.transformBatch: fromId must be an integer handle"))
	}
	to, ok := intArg(args[1])
	if !ok {
		return errObj(errors.New("simpleProj.transformBatch: toId must be an integer handle"))
	}
	coords, err := floatsFromJS(args[2])
	if err != nil {
		return errObj(err)
	}
	out, domain, err := defaultEngine.TransformBatch(from, to, coords)
	if err != nil {
		return errObj(err)
	}
	return batchObj(out, domain)
}

func infoFunc(_ js.Value, args []js.Value) any {
	if len(args) < 1 {
		return errObj(errors.New("simpleProj.info: expected an integer handle"))
	}
	id, ok := intArg(args[0])
	if !ok {
		return errObj(errors.New("simpleProj.info: expected an integer handle"))
	}
	info, err := defaultEngine.Info(id)
	if err != nil {
		return errObj(err)
	}
	o := infoObj(info)
	o.Set("id", id)
	return o
}

func freeFunc(_ js.Value, args []js.Value) any {
	if len(args) < 1 {
		return false
	}
	id, ok := intArg(args[0])
	if !ok {
		return false
	}
	return defaultEngine.Free(id)
}

// ---- argument parsing ------------------------------------------------------

func pointArgs(args []js.Value) (id int, a, b float64, err error) {
	if len(args) < 3 {
		err = errors.New("expected (handle, a, b)")
		return
	}
	id, ok := intArg(args[0])
	if !ok {
		err = errors.New("handle must be an integer")
		return
	}
	if args[1].Type() != js.TypeNumber || args[2].Type() != js.TypeNumber {
		err = errors.New("coordinates must be numbers")
		return
	}
	return id, args[1].Float(), args[2].Float(), nil
}

func batchArgs(args []js.Value) (id int, coords []float64, err error) {
	if len(args) < 2 {
		err = errors.New("expected (handle, packed coordinates)")
		return
	}
	id, ok := intArg(args[0])
	if !ok {
		err = errors.New("handle must be an integer")
		return
	}
	coords, err = floatsFromJS(args[1])
	return
}

func intArg(v js.Value) (int, bool) {
	if v.Type() != js.TypeNumber {
		return 0, false
	}
	f := v.Float()
	i := v.Int()
	if float64(i) != f || i < 0 {
		return 0, false
	}
	return i, true
}

// floatsFromJS copies a Float64Array / ArrayBuffer / plain Array of numbers.
func floatsFromJS(v js.Value) ([]float64, error) {
	switch {
	case v.InstanceOf(js.Global().Get("ArrayBuffer")):
		return copyFloat64(js.Global().Get("Float64Array").New(v)), nil
	case v.InstanceOf(js.Global().Get("Float64Array")):
		return copyFloat64(v), nil
	case v.InstanceOf(js.Global().Get("Array")):
		n := v.Length()
		out := make([]float64, n)
		for i := 0; i < n; i++ {
			e := v.Index(i)
			if e.Type() != js.TypeNumber {
				return nil, errors.New("coordinates must be numbers")
			}
			out[i] = e.Float()
		}
		return out, nil
	default:
		return nil, errors.New("expected Float64Array, ArrayBuffer or Array of numbers")
	}
}

// copyFloat64 copies the bytes of a Float64Array into Go memory. WebAssembly
// and every supported JS engine use little-endian float64, so the copy is a
// direct reinterpretation.
func copyFloat64(f js.Value) []float64 {
	n := f.Length()
	out := make([]float64, n)
	if n == 0 {
		return out
	}
	u8 := js.Global().Get("Uint8Array").New(f.Get("buffer"), f.Get("byteOffset"), f.Get("byteLength"))
	js.CopyBytesToGo(unsafeBytes(out), u8)
	return out
}

// floatsToJS copies float64 values into a JS-owned Float64Array.
func floatsToJS(v []float64) js.Value {
	arr := js.Global().Get("Float64Array").New(len(v))
	if len(v) == 0 {
		return arr
	}
	u8 := js.Global().Get("Uint8Array").New(arr.Get("buffer"))
	js.CopyBytesToJS(u8, unsafeBytes(v))
	return arr
}

// ---- result helpers --------------------------------------------------------

func object() js.Value { return js.Global().Get("Object").New() }

func infoObj(info *Info) js.Value {
	o := object()
	o.Set("name", info.Name)
	o.Set("description", info.Description)
	o.Set("crs", info.CRS)
	o.Set("units", info.Units)
	o.Set("domain", info.Domain)
	o.Set("accuracy", info.Accuracy)
	o.Set("epsg", info.EPSG)
	o.Set("spherical", info.Spherical)
	o.Set("ellipsoidal", info.Ellipsoidal)
	o.Set("hasInverse", info.HasInverse)
	o.Set("geographic", info.Geographic)
	return o
}

func pointObj(k1 string, v1 float64, k2 string, v2 float64) js.Value {
	o := object()
	o.Set(k1, v1)
	o.Set(k2, v2)
	return o
}

func batchObj(out []float64, domain bool) js.Value {
	o := object()
	o.Set("xy", floatsToJS(out))
	if domain {
		o.Set("domain", true)
	}
	return o
}

func errObj(err error) js.Value {
	o := object()
	o.Set("error", err.Error())
	if errors.Is(err, proj.ErrDomain) {
		o.Set("domain", true)
		var de *proj.DomainError
		if errors.As(err, &de) {
			o.Set("reason", de.Reason)
			o.Set("detail", de.Detail)
		}
	}
	return o
}

// unsafeBytes reinterprets a []float64 as its little-endian byte view.
func unsafeBytes(v []float64) []byte {
	if len(v) == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(&v[0])), len(v)*8)
}
