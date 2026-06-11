package httpapi

import (
	"math"
	"strconv"

	"github.com/araki/pibench/internal/model"
)

// appendRangeJSON serializes a range response directly to bytes, reflection-free,
// matching json.Marshal of the rangeOut/pointOut contract byte-for-byte. Field
// order follows the structs: ts, lat, lon, battery (only when present), ax, ay,
// az; then next_cursor, which is null when next is empty.
func appendRangeJSON(dst []byte, pts []model.Point, next string) []byte {
	dst = append(dst, `{"points":[`...)
	for i := range pts {
		if i > 0 {
			dst = append(dst, ',')
		}
		dst = appendPointJSON(dst, pts[i])
	}
	dst = append(dst, `],"next_cursor":`...)
	if next == "" {
		dst = append(dst, "null"...)
	} else {
		dst = appendCursor(dst, next)
	}
	return append(dst, '}')
}

func appendPointJSON(dst []byte, p model.Point) []byte {
	dst = append(dst, `{"ts":`...)
	dst = strconv.AppendInt(dst, p.TS, 10)
	dst = append(dst, `,"lat":`...)
	dst = appendJSONFloat(dst, p.Lat)
	dst = append(dst, `,"lon":`...)
	dst = appendJSONFloat(dst, p.Lon)
	if p.HasBattery {
		dst = append(dst, `,"battery":`...)
		dst = appendJSONFloat(dst, p.Battery)
	}
	dst = append(dst, `,"ax":`...)
	dst = appendJSONFloat(dst, p.Ax)
	dst = append(dst, `,"ay":`...)
	dst = appendJSONFloat(dst, p.Ay)
	dst = append(dst, `,"az":`...)
	dst = appendJSONFloat(dst, p.Az)
	return append(dst, '}')
}

// appendJSONFloat reproduces encoding/json's float formatting: shortest
// round-trippable digits, 'f' notation except 'e' for magnitudes below 1e-6 or
// at/above 1e21, with the e-0X exponent trimmed to e-X.
func appendJSONFloat(dst []byte, f float64) []byte {
	abs := math.Abs(f)
	format := byte('f')
	if abs != 0 && (abs < 1e-6 || abs >= 1e21) {
		format = 'e'
	}
	dst = strconv.AppendFloat(dst, f, format, -1, 64)
	if format == 'e' {
		n := len(dst)
		if n >= 4 && dst[n-4] == 'e' && dst[n-3] == '-' && dst[n-2] == '0' {
			dst[n-2] = dst[n-1]
			dst = dst[:n-1]
		}
	}
	return dst
}

// appendCursor quotes a pagination cursor. Cursors are base64.RawURLEncoding
// tokens ([A-Za-z0-9_-]), so no JSON string escaping is ever required.
func appendCursor(dst []byte, cur string) []byte {
	dst = append(dst, '"')
	dst = append(dst, cur...)
	return append(dst, '"')
}
