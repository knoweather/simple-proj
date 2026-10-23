package wasm

import (
	"errors"
	"math"
	"strings"
	"sync"
	"testing"
)

func TestParseForwardInverse(t *testing.T) {
	e := NewEngine()
	id, info, err := e.Parse("+proj=merc +a=6378137 +b=6378137 +units=km")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if id == 0 {
		t.Fatal("Parse returned handle 0")
	}
	if info.Name != "merc" || !info.Spherical || info.Geographic || info.EPSG != 0 {
		t.Fatalf("info = %+v", info)
	}
	if !strings.Contains(info.Units, "km") {
		t.Errorf("Units = %q, want km", info.Units)
	}

	x, y, err := e.Forward(id, 10, 0)
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	if math.Abs(x-1113.1949) > 1e-3 || math.Abs(y) > 1e-9 {
		t.Errorf("Forward(10,0) = (%v,%v)", x, y)
	}
	lon, lat, err := e.Inverse(id, x, y)
	if err != nil {
		t.Fatalf("Inverse: %v", err)
	}
	if math.Abs(lon-10) > 1e-9 || math.Abs(lat) > 1e-9 {
		t.Errorf("Inverse round trip = (%v,%v)", lon, lat)
	}
}

func TestBatchAndDomain(t *testing.T) {
	e := NewEngine()
	id, _, err := e.Parse("+proj=geos +h=35785863 +lon_0=0 +ellps=WGS84")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	out, domain, err := e.ForwardBatch(id, []float64{0, 0, 180, 0})
	if err != nil {
		t.Fatalf("ForwardBatch: %v", err)
	}
	if !domain {
		t.Error("domain = false, want true for a far-side point")
	}
	if len(out) != 4 || math.IsNaN(out[0]) || !math.IsNaN(out[2]) || !math.IsNaN(out[3]) {
		t.Fatalf("ForwardBatch out = %v", out)
	}

	back, domain, err := e.ForwardBatch(id, []float64{0, 0, 10, 10})
	if err != nil || domain {
		t.Fatalf("ForwardBatch valid: %v domain=%v", err, domain)
	}
	ll, domain, err := e.InverseBatch(id, back)
	if err != nil || domain {
		t.Fatalf("InverseBatch: %v domain=%v", err, domain)
	}
	if math.Abs(ll[0]) > 1e-9 || math.Abs(ll[1]) > 1e-9 {
		t.Errorf("InverseBatch round trip = %v", ll)
	}

	if _, _, err := e.ForwardBatch(id, []float64{1}); err == nil {
		t.Error("odd batch length should fail")
	}
}

func TestTransform(t *testing.T) {
	e := NewEngine()
	from, _, err := e.Parse("EPSG:4326")
	if err != nil {
		t.Fatalf("Parse from: %v", err)
	}
	to, _, err := e.Parse("EPSG:3857")
	if err != nil {
		t.Fatalf("Parse to: %v", err)
	}
	x, y, err := e.Transform(from, to, 0, 0)
	if err != nil || x != 0 || y != 0 {
		t.Fatalf("Transform(0,0) = (%v,%v), %v", x, y, err)
	}
	out, domain, err := e.TransformBatch(from, to, []float64{0, 0, 10, 0})
	if err != nil || domain {
		t.Fatalf("TransformBatch: %v domain=%v", err, domain)
	}
	if out[0] != 0 || out[1] != 0 || out[2] <= 0 {
		t.Errorf("TransformBatch out = %v", out)
	}
}

func TestInfoAndFree(t *testing.T) {
	e := NewEngine()
	id, _, err := e.Parse("+proj=ob_tran +o_proj=longlat +o_lat_p=39.25 +lon_0=18 +ellps=WGS84")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	info, err := e.Info(id)
	if err != nil || info.Name != "ob_tran" || info.Geographic {
		t.Fatalf("Info = %+v, %v", info, err)
	}

	if !e.Free(id) {
		t.Error("Free(existing) = false")
	}
	if e.Free(id) {
		t.Error("Free(second time) = true")
	}
	if _, _, err := e.Forward(id, 0, 0); err == nil {
		t.Error("Forward after Free should fail")
	}
	if _, err := e.Info(id); err == nil {
		t.Error("Info after Free should fail")
	}
}

func TestInvalidHandleAndInput(t *testing.T) {
	e := NewEngine()
	if _, _, err := e.Forward(42, 0, 0); err == nil {
		t.Error("unknown handle should fail")
	}
	if _, _, err := e.Parse(""); err == nil {
		t.Error("empty definition should fail")
	}
	if _, _, err := e.Parse("+proj=does_not_exist"); err == nil {
		t.Error("unknown projection should fail")
	}
}

func TestConcurrentUse(t *testing.T) {
	e := NewEngine()
	id, _, err := e.Parse("+proj=tmerc +lon_0=105 +k=0.9996 +ellps=WGS84")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	x, y, err := e.Forward(id, 105, 30)
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				gotX, gotY, err := e.Forward(id, 105, 30)
				if err != nil || gotX != x || gotY != y {
					errs <- errors.New("concurrent result drift")
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}
