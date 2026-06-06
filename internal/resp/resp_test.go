package resp

import (
	"bufio"
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestEncodeCommand(t *testing.T) {
	got := EncodeCommand([][]byte{[]byte("SET"), []byte("k"), []byte("v")})
	want := "*3\r\n$3\r\nSET\r\n$1\r\nk\r\n$1\r\nv\r\n"
	if string(got) != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestEncodeCommandBinarySafe(t *testing.T) {
	member := []byte{0x00, 0x01, 0x0d, 0x0a, 0xff}
	got := EncodeCommand([][]byte{[]byte("ZADD"), member})
	// length prefix must reflect raw byte count, body passes through untouched.
	if !bytes.Contains(got, append([]byte("$5\r\n"), append(member, '\r', '\n')...)) {
		t.Errorf("binary member not encoded raw: %q", got)
	}
}

func readOne(t *testing.T, wire string) (any, error) {
	t.Helper()
	return ReadReply(bufio.NewReader(strings.NewReader(wire)))
}

func TestReadSimpleString(t *testing.T) {
	v, err := readOne(t, "+OK\r\n")
	if err != nil || v != "OK" {
		t.Errorf("got %v, %v want OK,nil", v, err)
	}
}

func TestReadInteger(t *testing.T) {
	v, err := readOne(t, ":42\r\n")
	if err != nil || v.(int64) != 42 {
		t.Errorf("got %v, %v want 42,nil", v, err)
	}
}

func TestReadError(t *testing.T) {
	_, err := readOne(t, "-WRONGTYPE nope\r\n")
	if err == nil {
		t.Fatal("expected error reply to surface as error")
	}
	var re *Error
	if !errors.As(err, &re) {
		t.Errorf("expected *resp.Error, got %T", err)
	}
}

func TestReadBulkString(t *testing.T) {
	v, err := readOne(t, "$5\r\nhello\r\n")
	if err != nil || string(v.([]byte)) != "hello" {
		t.Errorf("got %v, %v want hello,nil", v, err)
	}
}

func TestReadNullBulk(t *testing.T) {
	v, err := readOne(t, "$-1\r\n")
	if err != nil || v != nil {
		t.Errorf("got %v, %v want nil,nil", v, err)
	}
}

func TestReadArray(t *testing.T) {
	v, err := readOne(t, "*2\r\n$1\r\na\r\n:7\r\n")
	if err != nil {
		t.Fatal(err)
	}
	arr := v.([]any)
	if len(arr) != 2 || string(arr[0].([]byte)) != "a" || arr[1].(int64) != 7 {
		t.Errorf("unexpected array: %#v", arr)
	}
}

func TestReadNullArray(t *testing.T) {
	v, err := readOne(t, "*-1\r\n")
	if err != nil || v != nil {
		t.Errorf("got %v, %v want nil,nil", v, err)
	}
}

func TestReadBulkPreservesBinary(t *testing.T) {
	body := []byte{0x00, 0xff, 0x0d, 0x0a, 0x00}
	var buf bytes.Buffer
	buf.WriteString("$5\r\n")
	buf.Write(body)
	buf.WriteString("\r\n")
	v, err := ReadReply(bufio.NewReader(&buf))
	if err != nil || !bytes.Equal(v.([]byte), body) {
		t.Errorf("binary bulk corrupted: got %v err %v", v, err)
	}
}
