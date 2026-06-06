// Package cursor encodes opaque pagination cursors for time-window queries.
// A cursor carries the last timestamp returned plus how many points sharing
// that exact timestamp were already emitted, so keyset pagination is stable
// even when multiple points share a ts.
package cursor

import (
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
)

// ErrInvalid indicates a malformed cursor.
var ErrInvalid = errors.New("invalid cursor")

// Encode builds an opaque cursor from the last ts and same-ts skip count.
func Encode(ts int64, skip int) string {
	raw := strconv.FormatInt(ts, 10) + ":" + strconv.Itoa(skip)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// Decode parses a cursor back into ts and skip, or returns ErrInvalid.
func Decode(s string) (ts int64, skip int, err error) {
	if s == "" {
		return 0, 0, ErrInvalid
	}
	raw, derr := base64.RawURLEncoding.DecodeString(s)
	if derr != nil {
		return 0, 0, ErrInvalid
	}
	parts := strings.SplitN(string(raw), ":", 2)
	if len(parts) != 2 {
		return 0, 0, ErrInvalid
	}
	ts, err = strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, 0, ErrInvalid
	}
	skip, err = strconv.Atoi(parts[1])
	if err != nil || skip < 0 {
		return 0, 0, ErrInvalid
	}
	return ts, skip, nil
}
