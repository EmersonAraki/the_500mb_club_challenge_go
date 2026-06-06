package model

import (
	"testing"
)

func validRaw() map[string]any {
	return map[string]any{
		"ts":      float64(1715800000000),
		"lat":     -23.5505,
		"lon":     -46.6333,
		"battery": 0.82,
		"ax":      0.11,
		"ay":      -0.04,
		"az":      9.81,
	}
}

func TestParsePointAcceptsValidPayload(t *testing.T) {
	p, err := ParsePoint(mustJSON(t, validRaw()))
	if err != nil {
		t.Fatalf("expected valid point, got error: %v", err)
	}
	if p.TS != 1715800000000 {
		t.Errorf("ts: got %d want 1715800000000", p.TS)
	}
	if !p.HasBattery || p.Battery != 0.82 {
		t.Errorf("battery: got present=%v value=%v want present=true value=0.82", p.HasBattery, p.Battery)
	}
}

func TestParsePointBatteryOptional(t *testing.T) {
	raw := validRaw()
	delete(raw, "battery")
	p, err := ParsePoint(mustJSON(t, raw))
	if err != nil {
		t.Fatalf("battery is optional, got error: %v", err)
	}
	if p.HasBattery {
		t.Errorf("expected HasBattery=false when battery absent")
	}
}

func TestParsePointRejectsMissingRequiredField(t *testing.T) {
	for _, field := range []string{"ts", "lat", "lon", "ax", "ay", "az"} {
		raw := validRaw()
		delete(raw, field)
		if _, err := ParsePoint(mustJSON(t, raw)); err == nil {
			t.Errorf("expected error when %q missing", field)
		}
	}
}

func TestParsePointRejectsOutOfRange(t *testing.T) {
	cases := map[string]any{
		"lat":     91.0,
		"lon":     -181.0,
		"battery": 1.5,
		"ts":      float64(0),
	}
	for field, bad := range cases {
		raw := validRaw()
		raw[field] = bad
		if _, err := ParsePoint(mustJSON(t, raw)); err == nil {
			t.Errorf("expected error for %s=%v", field, bad)
		}
	}
}

func TestParsePointRejectsNonFiniteAccel(t *testing.T) {
	// JSON has no Infinity literal, but an overflowing exponent decodes to +Inf.
	for _, field := range []string{"ax", "ay", "az"} {
		raw := []byte(`{"ts":1715800000000,"lat":0,"lon":0,"ax":0,"ay":0,"az":0,"` +
			field + `":1e999}`)
		if _, err := ParsePoint(raw); err == nil {
			t.Errorf("expected error for non-finite %s", field)
		}
	}
}

func TestParsePointRejectsMalformedJSON(t *testing.T) {
	if _, err := ParsePoint([]byte("{not json")); err == nil {
		t.Error("expected error for malformed JSON")
	}
}

func TestCodecRoundTrip(t *testing.T) {
	p, err := ParsePoint(mustJSON(t, validRaw()))
	if err != nil {
		t.Fatal(err)
	}
	encoded := p.Encode(42)
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if decoded.TS != p.TS || decoded.Lat != p.Lat || decoded.Lon != p.Lon ||
		decoded.Ax != p.Ax || decoded.Ay != p.Ay || decoded.Az != p.Az ||
		decoded.HasBattery != p.HasBattery || decoded.Battery != p.Battery {
		t.Errorf("round trip mismatch: got %+v want %+v", decoded, p)
	}
}

func TestEncodeOrdersByTimestamp(t *testing.T) {
	a := Point{TS: 1000, Az: 9.8}
	b := Point{TS: 2000, Az: 9.8}
	ea := a.Encode(1)
	eb := b.Encode(2)
	// Member bytes must sort by ts (big-endian prefix) for ZSET tiebreaks.
	if string(ea) >= string(eb) {
		t.Errorf("expected encoding of earlier ts to sort first")
	}
}

func TestEncodeUniquePerSequence(t *testing.T) {
	p := Point{TS: 1000, Az: 9.8}
	if string(p.Encode(1)) == string(p.Encode(2)) {
		t.Error("expected different sequence to yield distinct members")
	}
}
