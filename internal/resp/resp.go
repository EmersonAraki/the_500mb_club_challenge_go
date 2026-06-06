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
