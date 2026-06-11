package resp

import (
	"bufio"
	"bytes"
	"errors"
	"strconv"
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

// fixedArrayWire builds an array reply of `count` bulk strings, each exactly
// `size` bytes, where member i is filled with byte value i (binary, incl CRLF).
func fixedArrayWire(count, size int) ([]byte, [][]byte) {
	var buf bytes.Buffer
	buf.WriteString("*")
	buf.WriteString(strconv.Itoa(count))
	buf.WriteString("\r\n")
	want := make([][]byte, count)
	for i := 0; i < count; i++ {
		m := make([]byte, size)
		for j := range m {
			m[j] = byte(i)
		}
		want[i] = m
		buf.WriteString("$")
		buf.WriteString(strconv.Itoa(size))
		buf.WriteString("\r\n")
		buf.Write(m)
		buf.WriteString("\r\n")
	}
	return buf.Bytes(), want
}

func TestReadFixedBulkArrayReturnsContiguousMembers(t *testing.T) {
	wire, want := fixedArrayWire(3, 65)
	backing, count, err := ReadFixedBulkArray(bufio.NewReader(bytes.NewReader(wire)), 65)
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("count = %d, want 3", count)
	}
	if len(backing) != 3*65 {
		t.Fatalf("backing len = %d, want %d", len(backing), 3*65)
	}
	for i := 0; i < count; i++ {
		got := backing[i*65 : (i+1)*65]
		if !bytes.Equal(got, want[i]) {
			t.Errorf("member %d mismatch:\n got %x\nwant %x", i, got, want[i])
		}
	}
}

func TestReadFixedBulkArrayEmpty(t *testing.T) {
	backing, count, err := ReadFixedBulkArray(bufio.NewReader(strings.NewReader("*0\r\n")), 65)
	if err != nil || count != 0 || len(backing) != 0 {
		t.Errorf("empty array: got backing len %d count %d err %v, want 0,0,nil", len(backing), count, err)
	}
}

func TestReadFixedBulkArrayNull(t *testing.T) {
	_, count, err := ReadFixedBulkArray(bufio.NewReader(strings.NewReader("*-1\r\n")), 65)
	if err != nil || count != 0 {
		t.Errorf("null array: got count %d err %v, want 0,nil", count, err)
	}
}

func TestReadFixedBulkArrayRejectsWrongMemberSize(t *testing.T) {
	// A member declares 64 bytes when 65 is required.
	wire := "*1\r\n$64\r\n" + strings.Repeat("x", 64) + "\r\n"
	if _, _, err := ReadFixedBulkArray(bufio.NewReader(strings.NewReader(wire)), 65); err == nil {
		t.Error("expected error on member size mismatch")
	}
}

func TestReadFixedBulkArraySurfacesErrorReply(t *testing.T) {
	// WRONGTYPE etc. must not be silently read as an array.
	if _, _, err := ReadFixedBulkArray(bufio.NewReader(strings.NewReader("-WRONGTYPE x\r\n")), 65); err == nil {
		t.Error("expected error reply to surface")
	}
}

func TestReadFixedBulkArrayAllocatesOnce(t *testing.T) {
	wire, _ := fixedArrayWire(100, 65)
	allocs := testing.AllocsPerRun(50, func() {
		r := bufio.NewReader(bytes.NewReader(wire))
		if _, _, err := ReadFixedBulkArray(r, 65); err != nil {
			t.Fatal(err)
		}
	})
	// One backing allocation for all members. bufio.NewReader itself allocates in
	// the closure (reader + its buffer), so allow a small fixed overhead but far
	// below the ~3*100 the generic []any path costs.
	if allocs > 4 {
		t.Fatalf("ReadFixedBulkArray allocs/op = %v, want <= 4 (members must share one backing)", allocs)
	}
}
