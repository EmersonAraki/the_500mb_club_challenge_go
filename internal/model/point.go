// Package model defines the telemetry point, its validation against the API
// contract, and a compact binary codec used as the Redis sorted-set member.
package model

import (
	"encoding/binary"
	"errors"
	"math"
)

// Point is a single validated telemetry sample.
type Point struct {
	TS         int64
	Lat        float64
	Lon        float64
	Battery    float64
	HasBattery bool
	Ax         float64
	Ay         float64
	Az         float64
}

// ErrInvalidPoint indicates a payload that violates the contract.
var ErrInvalidPoint = errors.New("invalid telemetry point")

type pointJSON struct {
	TS      *int64   `json:"ts"`
	Lat     *float64 `json:"lat"`
	Lon     *float64 `json:"lon"`
	Battery *float64 `json:"battery"`
	Ax      *float64 `json:"ax"`
	Ay      *float64 `json:"ay"`
	Az      *float64 `json:"az"`
}

// ParsePoint decodes and validates a single telemetry point from JSON. The JSON
// is tokenized by a reflection-free scanner (scanPointJSON); validation stays in
// toPoint.
func ParsePoint(raw []byte) (Point, error) {
	pj, err := scanPointJSON(raw)
	if err != nil {
		return Point{}, ErrInvalidPoint
	}
	return pj.toPoint()
}

func (pj pointJSON) toPoint() (Point, error) {
	if pj.TS == nil || pj.Lat == nil || pj.Lon == nil ||
		pj.Ax == nil || pj.Ay == nil || pj.Az == nil {
		return Point{}, ErrInvalidPoint
	}
	if *pj.TS <= 0 {
		return Point{}, ErrInvalidPoint
	}
	if *pj.Lat < -90 || *pj.Lat > 90 {
		return Point{}, ErrInvalidPoint
	}
	if *pj.Lon < -180 || *pj.Lon > 180 {
		return Point{}, ErrInvalidPoint
	}
	if !finite(*pj.Ax) || !finite(*pj.Ay) || !finite(*pj.Az) {
		return Point{}, ErrInvalidPoint
	}
	p := Point{
		TS:  *pj.TS,
		Lat: *pj.Lat,
		Lon: *pj.Lon,
		Ax:  *pj.Ax,
		Ay:  *pj.Ay,
		Az:  *pj.Az,
	}
	if pj.Battery != nil {
		if *pj.Battery < 0 || *pj.Battery > 1 {
			return Point{}, ErrInvalidPoint
		}
		p.Battery = *pj.Battery
		p.HasBattery = true
	}
	return p, nil
}

func finite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

// EncodedLen is the fixed binary member size.
const EncodedLen = 65

// Encode serializes the point into a fresh fixed 65-byte member. The leading 8
// bytes are the big-endian timestamp so members sort by ts within a ZSET score
// tie; seq guarantees uniqueness across concurrent writers.
func (p Point) Encode(seq uint64) []byte {
	b := make([]byte, EncodedLen)
	p.EncodeInto(b, seq)
	return b
}

// EncodeInto writes the member into dst, which must be at least EncodedLen bytes,
// allocating nothing. Callers batching many points share one backing array and
// slice it per point, collapsing N allocations into one. Layout matches Encode.
func (p Point) EncodeInto(dst []byte, seq uint64) {
	_ = dst[EncodedLen-1] // bounds-check once; fail fast on a short buffer
	binary.BigEndian.PutUint64(dst[0:8], uint64(p.TS))
	binary.BigEndian.PutUint64(dst[8:16], seq)
	binary.LittleEndian.PutUint64(dst[16:24], math.Float64bits(p.Lat))
	binary.LittleEndian.PutUint64(dst[24:32], math.Float64bits(p.Lon))
	binary.LittleEndian.PutUint64(dst[32:40], math.Float64bits(p.Ax))
	binary.LittleEndian.PutUint64(dst[40:48], math.Float64bits(p.Ay))
	binary.LittleEndian.PutUint64(dst[48:56], math.Float64bits(p.Az))
	binary.LittleEndian.PutUint64(dst[56:64], math.Float64bits(p.Battery))
	dst[64] = 0
	if p.HasBattery {
		dst[64] = 1
	}
}

// Decode reconstructs a Point from its binary member form.
func Decode(b []byte) (Point, error) {
	if len(b) != EncodedLen {
		return Point{}, ErrInvalidPoint
	}
	p := Point{
		TS:      int64(binary.BigEndian.Uint64(b[0:8])),
		Lat:     math.Float64frombits(binary.LittleEndian.Uint64(b[16:24])),
		Lon:     math.Float64frombits(binary.LittleEndian.Uint64(b[24:32])),
		Ax:      math.Float64frombits(binary.LittleEndian.Uint64(b[32:40])),
		Ay:      math.Float64frombits(binary.LittleEndian.Uint64(b[40:48])),
		Az:      math.Float64frombits(binary.LittleEndian.Uint64(b[48:56])),
		Battery: math.Float64frombits(binary.LittleEndian.Uint64(b[56:64])),
	}
	p.HasBattery = b[64] == 1
	return p, nil
}
