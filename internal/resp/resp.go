// Package resp implements the minimal subset of the Redis serialization
// protocol (RESP2) needed by this service, with no external dependencies.
package resp

import (
	"bufio"
	"errors"
	"io"
	"strconv"
)

// Error is a Redis error reply (a line beginning with '-').
type Error struct{ Msg string }

func (e *Error) Error() string { return "redis: " + e.Msg }

// EncodeCommand serializes a command as a RESP array of bulk strings. Arguments
// are length-prefixed by raw byte count, so binary members pass through intact.
func EncodeCommand(args [][]byte) []byte {
	// Pre-size: header + per-arg ($len\r\n ... \r\n).
	n := 1 + len(strconv.Itoa(len(args))) + 2
	for _, a := range args {
		n += 1 + len(strconv.Itoa(len(a))) + 2 + len(a) + 2
	}
	b := make([]byte, 0, n)
	b = append(b, '*')
	b = strconv.AppendInt(b, int64(len(args)), 10)
	b = append(b, '\r', '\n')
	for _, a := range args {
		b = append(b, '$')
		b = strconv.AppendInt(b, int64(len(a)), 10)
		b = append(b, '\r', '\n')
		b = append(b, a...)
		b = append(b, '\r', '\n')
	}
	return b
}

// ReadReply reads one RESP value. Returns string for simple strings, int64 for
// integers, []byte for bulk strings, []any for arrays, nil for null bulk/array,
// and an *Error for error replies.
func ReadReply(r *bufio.Reader) (any, error) {
	prefix, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	line, err := readLine(r)
	if err != nil {
		return nil, err
	}
	switch prefix {
	case '+':
		return string(line), nil
	case '-':
		return nil, &Error{Msg: string(line)}
	case ':':
		return parseInt(line)
	case '$':
		return readBulk(r, line)
	case '*':
		return readArray(r, line)
	default:
		return nil, errors.New("resp: unknown reply type")
	}
}

func readArray(r *bufio.Reader, line []byte) (any, error) {
	n, err := parseInt(line)
	if err != nil {
		return nil, err
	}
	if n < 0 {
		return nil, nil
	}
	out := make([]any, n)
	for i := range out {
		v, err := ReadReply(r)
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

func readBulk(r *bufio.Reader, line []byte) (any, error) {
	n, err := parseInt(line)
	if err != nil {
		return nil, err
	}
	if n < 0 {
		return nil, nil
	}
	buf := make([]byte, n+2) // include trailing CRLF
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf[:n], nil
}

// ReadFixedBulkArray reads an array reply whose members are each exactly `size`
// bytes into a single backing buffer, returning it and the member count; member
// i is backing[i*size:(i+1)*size]. This is the hot read path: it allocates one
// backing for the whole array instead of one buffer per member, and skips the
// per-member interface boxing the generic ReadReply incurs. An error reply ('-')
// surfaces as an error; a null or empty array returns (nil, 0, nil); any member
// whose declared length is not `size` is an error.
func ReadFixedBulkArray(r *bufio.Reader, size int) ([]byte, int, error) {
	prefix, err := r.ReadByte()
	if err != nil {
		return nil, 0, err
	}
	if prefix == '-' {
		line, err := readLine(r)
		if err != nil {
			return nil, 0, err
		}
		return nil, 0, &Error{Msg: string(line)}
	}
	if prefix != '*' {
		return nil, 0, errors.New("resp: expected array reply")
	}
	n, err := parseIntLine(r)
	if err != nil {
		return nil, 0, err
	}
	if n <= 0 {
		return nil, 0, nil
	}
	backing := make([]byte, int(n)*size)
	for i := int64(0); i < n; i++ {
		bp, err := r.ReadByte()
		if err != nil {
			return nil, 0, err
		}
		if bp != '$' {
			return nil, 0, errors.New("resp: expected bulk member in array")
		}
		// parseIntLine avoids the per-line allocation that readLine's ReadBytes
		// incurs -- the dominant cost when reading a large member array.
		ln, err := parseIntLine(r)
		if err != nil {
			return nil, 0, err
		}
		if ln != int64(size) {
			return nil, 0, errors.New("resp: unexpected member size")
		}
		if _, err := io.ReadFull(r, backing[i*int64(size):(i+1)*int64(size)]); err != nil {
			return nil, 0, err
		}
		// Skip the member's trailing CRLF without allocating (Discard avoids the
		// stack-array-to-slice escape that io.ReadFull on a local buffer causes).
		if _, err := r.Discard(2); err != nil {
			return nil, 0, err
		}
	}
	return backing, int(n), nil
}

// parseIntLine reads a decimal integer (optional leading '-') terminated by CRLF
// directly off the reader, allocating nothing. Used on the hot member-read path.
func parseIntLine(r *bufio.Reader) (int64, error) {
	var n int64
	neg, seen := false, false
	for {
		c, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		switch {
		case c == '\r':
			nl, err := r.ReadByte()
			if err != nil {
				return 0, err
			}
			if nl != '\n' {
				return 0, errors.New("resp: malformed line terminator")
			}
			if !seen {
				return 0, errors.New("resp: empty integer")
			}
			if neg {
				n = -n
			}
			return n, nil
		case c == '-' && !seen && !neg:
			neg = true
		case c >= '0' && c <= '9':
			n = n*10 + int64(c-'0')
			seen = true
		default:
			return 0, errors.New("resp: invalid integer")
		}
	}
}

// readLine reads up to and including CRLF, returning the content without CRLF.
func readLine(r *bufio.Reader) ([]byte, error) {
	line, err := r.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	if len(line) < 2 || line[len(line)-2] != '\r' {
		return nil, errors.New("resp: malformed line terminator")
	}
	return line[:len(line)-2], nil
}

func parseInt(line []byte) (int64, error) {
	return strconv.ParseInt(string(line), 10, 64)
}
