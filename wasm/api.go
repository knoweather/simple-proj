// Package wasm exposes simple-proj to WebAssembly / JavaScript.
//
// The WASM build keeps a small registry of parsed CRS handles on the Go side
// and turns each JavaScript call into a projection operation: parse a CRS once,
// then forward, inverse, batch-convert or transform coordinates by handle.
//
// The JS-facing bindings live in wasm.go (build tag js && wasm) and are wired
// up by Register(); this file holds the framework-independent engine so it can
// be unit tested on a normal host.
package wasm

import (
	"errors"
	"fmt"
	"sync"

	proj "github.com/knoweather/simple-proj"
)

// Info is the metadata returned to JavaScript for one parsed CRS.
type Info struct {
	Name        string
	Description string
	CRS         string
	Units       string
	Domain      string
	Accuracy    string
	EPSG        int
	Spherical   bool
	Ellipsoidal bool
	HasInverse  bool
	Geographic  bool
}

// engine keeps the CRS/projection handles and the transformer cache. All
// access is serialized: JS is single-threaded, but host tests (and future
// workers) may call it concurrently.
type Engine struct {
	mu         sync.Mutex
	nextID     int
	handles    map[int]*handle
	transforms map[[2]int]*proj.Transformer
}

type handle struct {
	crs *proj.CRS
	p   proj.Projection
}

// NewEngine returns an empty registry.
func NewEngine() *Engine {
	return &Engine{
		nextID:     1,
		handles:    make(map[int]*handle),
		transforms: make(map[[2]int]*proj.Transformer),
	}
}

// Parse parses a proj string / WKT / EPSG code, builds its Projection, stores
// the pair under a stable numeric handle, and returns the handle plus metadata.
func (e *Engine) Parse(def string) (int, *Info, error) {
	if def == "" {
		return 0, nil, errors.New("empty CRS definition")
	}
	crs, err := proj.Parse(def)
	if err != nil {
		return 0, nil, err
	}
	p, err := crs.NewProjection()
	if err != nil {
		return 0, nil, err
	}
	meta, err := proj.Info(p)
	if err != nil {
		return 0, nil, err
	}

	info := &Info{
		Name:        meta.Name,
		Description: meta.Description,
		CRS:         meta.CRS,
		Units:       meta.Units,
		Domain:      meta.Domain,
		Accuracy:    meta.Accuracy,
		EPSG:        meta.EPSG,
		Spherical:   meta.Spherical,
		Ellipsoidal: meta.Ellipsoidal,
		HasInverse:  meta.HasInverse,
		Geographic:  crs.IsGeographic(),
	}

	e.mu.Lock()
	id := e.nextID
	e.nextID++
	e.handles[id] = &handle{crs: crs, p: p}
	e.mu.Unlock()
	return id, info, nil
}

// get returns the handle or an error for unknown / freed ids.
func (e *Engine) get(id int) (*handle, error) {
	e.mu.Lock()
	h, ok := e.handles[id]
	e.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("invalid CRS handle %d (parse a CRS first, and do not use freed handles)", id)
	}
	return h, nil
}

// Info returns metadata for an existing handle.
func (e *Engine) Info(id int) (*Info, error) {
	h, err := e.get(id)
	if err != nil {
		return nil, err
	}
	meta, err := proj.Info(h.p)
	if err != nil {
		return nil, err
	}
	return &Info{
		Name:        meta.Name,
		Description: meta.Description,
		CRS:         meta.CRS,
		Units:       meta.Units,
		Domain:      meta.Domain,
		Accuracy:    meta.Accuracy,
		EPSG:        meta.EPSG,
		Spherical:   meta.Spherical,
		Ellipsoidal: meta.Ellipsoidal,
		HasInverse:  meta.HasInverse,
		Geographic:  h.crs.IsGeographic(),
	}, nil
}

// Forward converts (lon, lat) degrees to (x, y) in the CRS unit.
func (e *Engine) Forward(id int, lon, lat float64) (float64, float64, error) {
	h, err := e.get(id)
	if err != nil {
		return 0, 0, err
	}
	return h.p.Forward(lon, lat)
}

// Inverse converts (x, y) in the CRS unit back to (lon, lat) degrees.
func (e *Engine) Inverse(id int, x, y float64) (float64, float64, error) {
	h, err := e.get(id)
	if err != nil {
		return 0, 0, err
	}
	return h.p.Inverse(x, y)
}

// ForwardBatch converts packed [lon0,lat0,...] degrees to packed [x0,y0,...].
// domain reports that at least one point fell outside the projection domain;
// those points are NaN and the rest of the batch is still valid.
func (e *Engine) ForwardBatch(id int, lonlat []float64) (out []float64, domain bool, err error) {
	h, err := e.get(id)
	if err != nil {
		return nil, false, err
	}
	out, err = proj.ForwardBatch(h.p, lonlat)
	if errors.Is(err, proj.ErrDomain) {
		return out, true, nil
	}
	if err != nil {
		return nil, false, err
	}
	return out, false, nil
}

// InverseBatch is the inverse of ForwardBatch.
func (e *Engine) InverseBatch(id int, xy []float64) (out []float64, domain bool, err error) {
	h, err := e.get(id)
	if err != nil {
		return nil, false, err
	}
	out, err = proj.InverseBatch(h.p, xy)
	if errors.Is(err, proj.ErrDomain) {
		return out, true, nil
	}
	if err != nil {
		return nil, false, err
	}
	return out, false, nil
}

// transformer returns the cached Transformer for a handle pair, building it on
// first use.
func (e *Engine) transformer(from, to int) (*proj.Transformer, error) {
	key := [2]int{from, to}
	e.mu.Lock()
	if tr, ok := e.transforms[key]; ok {
		e.mu.Unlock()
		return tr, nil
	}
	fromH, ok1 := e.handles[from]
	toH, ok2 := e.handles[to]
	e.mu.Unlock()
	if !ok1 {
		return nil, fmt.Errorf("invalid CRS handle %d", from)
	}
	if !ok2 {
		return nil, fmt.Errorf("invalid CRS handle %d", to)
	}

	tr, err := proj.NewTransformer(fromH.crs, toH.crs)
	if err != nil {
		return nil, err
	}
	e.mu.Lock()
	e.transforms[key] = tr
	e.mu.Unlock()
	return tr, nil
}

// Transform converts one point from the source handle's CRS to the target's.
func (e *Engine) Transform(from, to int, x, y float64) (float64, float64, error) {
	tr, err := e.transformer(from, to)
	if err != nil {
		return 0, 0, err
	}
	return tr.Transform(x, y)
}

// TransformBatch converts packed [x0,y0,...] coordinates between two CRSs.
// domain has the same meaning as in ForwardBatch.
func (e *Engine) TransformBatch(from, to int, xy []float64) (out []float64, domain bool, err error) {
	tr, err := e.transformer(from, to)
	if err != nil {
		return nil, false, err
	}
	out, err = tr.TransformBatch(xy)
	if errors.Is(err, proj.ErrDomain) {
		return out, true, nil
	}
	if err != nil {
		return nil, false, err
	}
	return out, false, nil
}

// Free drops a handle and any cached transformer that references it. It
// reports whether the handle existed.
func (e *Engine) Free(id int) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.handles[id]; !ok {
		return false
	}
	delete(e.handles, id)
	for key := range e.transforms {
		if key[0] == id || key[1] == id {
			delete(e.transforms, key)
		}
	}
	return true
}
