package proj_test

import (
	"errors"
	"fmt"

	proj "github.com/knoweather/simple-proj"
)

// Parse a projection string, build the projection once, then use it as a
// value handle (the instance is immutable and safe for concurrent use).
func ExampleParse() {
	crs, _ := proj.Parse("+proj=merc +a=6378137 +b=6378137 +units=km")
	p, _ := crs.NewProjection()

	x, y, _ := p.Forward(10, 0)
	fmt.Printf("%s %.3f %.3f\n", crs.Proj, x, y)
	// Output: merc 1113.195 0.000
}

// ForwardBatch takes packed [lon0,lat0,lon1,lat1,...] degrees and returns
// packed [x0,y0,...] in the CRS unit. Out-of-domain points become (NaN,NaN)
// and the call still returns ErrDomain at the end.
func ExampleForwardBatch() {
	crs, _ := proj.Parse("+proj=eqc +lat_ts=0 +lon_0=0 +ellps=WGS84")
	p, _ := crs.NewProjection()

	xy, err := proj.ForwardBatch(p, []float64{0, 0, 90, 0})
	fmt.Println(len(xy), err)
	fmt.Printf("%.0f\n", xy[2])
	// Output:
	// 4 <nil>
	// 10018754
}

// NewTransformer converts between two CRSs. Here EPSG:4326 (degrees) goes to
// EPSG:3857 (spherical Web Mercator, metres).
func ExampleNewTransformer() {
	from, _ := proj.Parse("EPSG:4326")
	to, _ := proj.Parse("EPSG:3857")
	tr, _ := proj.NewTransformer(from, to)

	x, y, _ := tr.Transform(0, 0)
	fmt.Printf("%.3f %.3f\n", x, y)
	// Output: 0.000 0.000
}

// Out-of-domain forward calls return ErrDomain together with NaN coordinates;
// use errors.Is so the same check works for the richer DomainError type.
func ExampleErrDomain() {
	crs, _ := proj.Parse("+proj=geos +h=35785863 +lon_0=0 +ellps=WGS84")
	p, _ := crs.NewProjection()

	_, _, err := p.Forward(180, 0) // far side of the Earth
	fmt.Println(errors.Is(err, proj.ErrDomain))
	// Output: true
}
