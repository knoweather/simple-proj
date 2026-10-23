// Package proj implements coordinate reference systems and map projections for
// weather and satellite grids.
//
// It provides a pure-Go subset of PROJ: parse proj strings, WKT2/WKT1 or EPSG
// shortcuts into a CRS, build immutable Projection or Transformer values, and
// convert coordinates one point at a time or in packed batches. Eleven
// projections are supported — latlong, eqc, geos, stere, laea, lcc, merc,
// tmerc, aeqd, utm and ob_tran — with spherical and ellipsoidal paths,
// +units/+to_meter scaling, +towgs84 Helmert transforms, Karney geodesics and
// GRIB shape_of_the_earth conversion.
//
// Projections and transformers are immutable after construction and safe for
// concurrent use. Numerical behaviour is checked against pyproj 3.8.0 /
// PROJ 9.8.1: forward within 1e-6 m and inverse within 1e-9 degrees, with
// relative floors for extreme coordinates. See README.md and the xcheck/
// fixtures for the reference data and regeneration scripts.
package proj
