package model

import (
	"errors"
	"strconv"
)

// errScan marks any JSON that the fast scanner rejects. The caller maps it to
// ErrInvalidPoint, so the specific reason never leaves the package.
var errScan = errors.New("model: malformed point json")

// scanPointJSON tokenizes a flat telemetry-point object into pointJSON without
// reflection, matching encoding/json's behaviour on the shapes the API accepts:
// unknown fields are skipped, an explicit null leaves a field unset, and a
// numeric overflow (e.g. 1e999) is kept as +Inf for toPoint to reject. It does
// not allocate. Validation (ranges, required fields) stays in toPoint.
//
// Fidelity boundary: a known key written with JSON escapes (e.g. "lat") is
// treated as unknown and skipped, where stdlib would match it. That only ever
// makes the scanner stricter (a 400, never a misparse), and real clients do not
// escape ASCII keys.
func scanPointJSON(b []byte) (pointJSON, error) {
	var pj pointJSON
	i := skipWS(b, 0)
	if i >= len(b) || b[i] != '{' {
		return pj, errScan
	}
	i++
	i = skipWS(b, i)
	if i < len(b) && b[i] == '}' {
		i++
		return pj, trailingOnlyWS(b, i)
	}
	for {
		i = skipWS(b, i)
		key, ni, err := readKey(b, i)
		if err != nil {
			return pj, err
		}
		i = skipWS(b, ni)
		if i >= len(b) || b[i] != ':' {
			return pj, errScan
		}
		i = skipWS(b, i+1)
		if i, err = readField(b, i, key, &pj); err != nil {
			return pj, err
		}
		i = skipWS(b, i)
		if i >= len(b) {
			return pj, errScan
		}
		switch b[i] {
		case ',':
			i++
		case '}':
			return pj, trailingOnlyWS(b, i+1)
		default:
			return pj, errScan
		}
	}
}

// readField parses the value at i for the given key, storing it when the key is
// known, and returns the index past the value. Keys match field names
// case-insensitively, as encoding/json does (so "AX" fills ax).
func readField(b []byte, i int, key []byte, pj *pointJSON) (int, error) {
	switch fieldName(key) {
	case "ts":
		return readInt(b, i, &pj.TS)
	case "lat":
		return readFloat(b, i, &pj.Lat)
	case "lon":
		return readFloat(b, i, &pj.Lon)
	case "battery":
		return readFloat(b, i, &pj.Battery)
	case "ax":
		return readFloat(b, i, &pj.Ax)
	case "ay":
		return readFloat(b, i, &pj.Ay)
	case "az":
		return readFloat(b, i, &pj.Az)
	default:
		return skipValue(b, i)
	}
}

// fieldName returns the canonical lowercase field name a key maps to, or "" if
// the key is unknown. Matching is ASCII case-insensitive to mirror
// encoding/json's tag matching.
func fieldName(key []byte) string {
	switch len(key) {
	case 2:
		for _, n := range [...]string{"ts", "ax", "ay", "az"} {
			if eqFoldASCII(key, n) {
				return n
			}
		}
	case 3:
		for _, n := range [...]string{"lat", "lon"} {
			if eqFoldASCII(key, n) {
				return n
			}
		}
	case 7:
		if eqFoldASCII(key, "battery") {
			return "battery"
		}
	}
	return ""
}

// eqFoldASCII reports whether s equals the all-lowercase ASCII string lower,
// ignoring ASCII case in s.
func eqFoldASCII(s []byte, lower string) bool {
	if len(s) != len(lower) {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != lower[i] {
			return false
		}
	}
	return true
}

func skipWS(b []byte, i int) int {
	for i < len(b) {
		switch b[i] {
		case ' ', '\t', '\n', '\r':
			i++
		default:
			return i
		}
	}
	return i
}

func trailingOnlyWS(b []byte, i int) error {
	if skipWS(b, i) != len(b) {
		return errScan
	}
	return nil
}

// readKey reads a JSON string key and returns its raw inner bytes (escapes left
// intact: a known field name never contains them, so an escaped key simply fails
// to match and is treated as unknown). The returned slice aliases b -- no
// allocation. Returns the index past the closing quote.
func readKey(b []byte, i int) ([]byte, int, error) {
	if i >= len(b) || b[i] != '"' {
		return nil, i, errScan
	}
	i++
	start := i
	for i < len(b) {
		switch b[i] {
		case '\\':
			i += 2 // skip the escaped char; we only need correct termination
		case '"':
			return b[start:i], i + 1, nil
		default:
			i++
		}
	}
	return nil, i, errScan
}

// readInt parses a JSON integer (or null) into dst. A null or absent value
// leaves dst unset; a non-integer numeric form (1.5, 1e3) is rejected, matching
// json.Unmarshal into *int64.
func readInt(b []byte, i int, dst **int64) (int, error) {
	if ni, ok := consumeNull(b, i); ok {
		return ni, nil
	}
	lit, ni, err := scanNumberLiteral(b, i)
	if err != nil {
		return i, err
	}
	// The string conversion does not escape ParseInt, so it is not allocated.
	v, perr := strconv.ParseInt(string(lit), 10, 64)
	if perr != nil {
		return i, errScan
	}
	*dst = &v
	return ni, nil
}

// readFloat parses a JSON number (or null) into dst. An out-of-range magnitude
// (e.g. 1e999) is rejected, matching json.Unmarshal, which returns an error
// rather than storing +/-Inf.
func readFloat(b []byte, i int, dst **float64) (int, error) {
	if ni, ok := consumeNull(b, i); ok {
		return ni, nil
	}
	lit, ni, err := scanNumberLiteral(b, i)
	if err != nil {
		return i, err
	}
	// The string conversion does not escape ParseFloat, so it is not allocated.
	v, perr := strconv.ParseFloat(string(lit), 64)
	if perr != nil {
		return i, errScan
	}
	*dst = &v
	return ni, nil
}

func consumeNull(b []byte, i int) (int, bool) {
	if i+4 <= len(b) && string(b[i:i+4]) == "null" {
		return i + 4, true
	}
	return i, false
}

// scanNumberLiteral validates a JSON number per the grammar (no leading zeros,
// optional fraction and exponent) and returns the literal slice plus the index
// past it. Rejecting leading zeros matches encoding/json, which would error.
func scanNumberLiteral(b []byte, i int) ([]byte, int, error) {
	start := i
	if i < len(b) && b[i] == '-' {
		i++
	}
	// Integer part.
	if i >= len(b) || b[i] < '0' || b[i] > '9' {
		return nil, start, errScan
	}
	if b[i] == '0' {
		i++ // a leading zero must stand alone
	} else {
		for i < len(b) && b[i] >= '0' && b[i] <= '9' {
			i++
		}
	}
	// Fraction.
	if i < len(b) && b[i] == '.' {
		i++
		if i >= len(b) || b[i] < '0' || b[i] > '9' {
			return nil, start, errScan
		}
		for i < len(b) && b[i] >= '0' && b[i] <= '9' {
			i++
		}
	}
	// Exponent.
	if i < len(b) && (b[i] == 'e' || b[i] == 'E') {
		i++
		if i < len(b) && (b[i] == '+' || b[i] == '-') {
			i++
		}
		if i >= len(b) || b[i] < '0' || b[i] > '9' {
			return nil, start, errScan
		}
		for i < len(b) && b[i] >= '0' && b[i] <= '9' {
			i++
		}
	}
	return b[start:i], i, nil
}

// skipValue advances past one JSON value of any type, used for unknown fields.
func skipValue(b []byte, i int) (int, error) {
	if i >= len(b) {
		return i, errScan
	}
	switch b[i] {
	case '"':
		return skipString(b, i)
	case '{':
		return skipContainer(b, i, '{', '}')
	case '[':
		return skipContainer(b, i, '[', ']')
	case 't':
		return consumeLiteral(b, i, "true")
	case 'f':
		return consumeLiteral(b, i, "false")
	case 'n':
		return consumeLiteral(b, i, "null")
	default:
		_, ni, err := scanNumberLiteral(b, i)
		return ni, err
	}
}

func skipString(b []byte, i int) (int, error) {
	i++ // opening quote
	for i < len(b) {
		switch b[i] {
		case '\\':
			i += 2
		case '"':
			return i + 1, nil
		default:
			i++
		}
	}
	return i, errScan
}

// skipContainer skips a balanced object or array, honouring strings so that a
// brace inside a string does not affect nesting depth.
func skipContainer(b []byte, i int, open, close byte) (int, error) {
	depth := 0
	for i < len(b) {
		switch b[i] {
		case '"':
			ni, err := skipString(b, i)
			if err != nil {
				return i, err
			}
			i = ni
			continue
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return i + 1, nil
			}
		}
		i++
	}
	return i, errScan
}

func consumeLiteral(b []byte, i int, lit string) (int, error) {
	if i+len(lit) <= len(b) && string(b[i:i+len(lit)]) == lit {
		return i + len(lit), nil
	}
	return i, errScan
}
