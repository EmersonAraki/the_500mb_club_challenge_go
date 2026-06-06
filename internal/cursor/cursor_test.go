package cursor

import "testing"

func TestRoundTrip(t *testing.T) {
	enc := Encode(1715800000000, 3)
	ts, skip, err := Decode(enc)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if ts != 1715800000000 || skip != 3 {
		t.Errorf("got ts=%d skip=%d want 1715800000000,3", ts, skip)
	}
}

func TestDecodeEmptyIsError(t *testing.T) {
	if _, _, err := Decode(""); err == nil {
		t.Error("expected error decoding empty cursor")
	}
}

func TestDecodeGarbageIsError(t *testing.T) {
	for _, bad := range []string{"!!!", "notbase64==", "Zm9v`"} {
		if _, _, err := Decode(bad); err == nil {
			t.Errorf("expected error decoding %q", bad)
		}
	}
}

func TestEncodeIsOpaqueNonEmpty(t *testing.T) {
	enc := Encode(1, 0)
	if enc == "" {
		t.Error("expected non-empty cursor")
	}
}
