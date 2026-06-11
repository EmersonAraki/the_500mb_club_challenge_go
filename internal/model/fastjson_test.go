package model

import (
	"encoding/json"
	"math"
	"testing"
)

// referenceScan is the stdlib oracle: scanPointJSON must agree with what
// json.Unmarshal into pointJSON produces, both on success and on rejection.
func referenceScan(b []byte) (pointJSON, error) {
	var pj pointJSON
	err := json.Unmarshal(b, &pj)
	return pj, err
}

func f64eq(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return math.Float64bits(*a) == math.Float64bits(*b)
}

func i64eq(a, b *int64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func pjEqual(a, b pointJSON) bool {
	return i64eq(a.TS, b.TS) && f64eq(a.Lat, b.Lat) && f64eq(a.Lon, b.Lon) &&
		f64eq(a.Battery, b.Battery) && f64eq(a.Ax, b.Ax) && f64eq(a.Ay, b.Ay) &&
		f64eq(a.Az, b.Az)
}

func TestScanPointJSONMatchesStdlib(t *testing.T) {
	cases := []string{
		// Valid shapes.
		`{"ts":1715800000000,"lat":12.5,"lon":-40.25,"ax":0.1,"ay":0.2,"az":9.8,"battery":0.75}`,
		`{"ts":1,"lat":0,"lon":0,"ax":0,"ay":0,"az":0}`,                 // battery absent
		`{"ts":1,"lat":0,"lon":0,"ax":0,"ay":0,"az":0,"battery":null}`,  // battery null -> unset
		`{"ts":1,"lat":null,"lon":0,"ax":0,"ay":0,"az":0}`,             // required null -> unset
		`  {  "ts" : 1 , "lat":0,"lon":0,"ax":0,"ay":0,"az":0 }  `,      // whitespace
		`{"ts":1,"lat":0,"lon":0,"ax":0,"ay":0,"az":0,"extra":"hi"}`,    // unknown string
		`{"ts":1,"extra":{"a":[1,2,{"b":"c"}]},"lat":0,"lon":0,"ax":0,"ay":0,"az":0}`, // unknown nested
		`{"ts":1,"extra":[true,false,null,1.5],"lat":0,"lon":0,"ax":0,"ay":0,"az":0}`,
		`{"ts":1,"lat":-90,"lon":180,"ax":-1e3,"ay":1E2,"az":0.0e1}`,    // number forms
		`{}`,                                                            // empty object
		`{"ts":1,"lat":0,"lon":0,"ax":0,"ay":0,"az":0,"ts":2}`,         // duplicate key (last wins)
		`{"battery":-0.0,"ts":1,"lat":0,"lon":0,"ax":0,"ay":0,"az":0}`,  // signed zero
		`{"TS":1,"Lat":0,"LON":0,"aX":0,"Ay":0,"aZ":0,"BATTERY":0.5}`,   // case-insensitive keys
		// Inputs both must reject.
		`{"ts":01,"lat":0,"lon":0,"ax":0,"ay":0,"az":0}`,   // leading zero
		`{"ts":1.5,"lat":0,"lon":0,"ax":0,"ay":0,"az":0}`,  // ts not integer
		`{"ts":1e3,"lat":0,"lon":0,"ax":0,"ay":0,"az":0}`,  // ts exponent form
		`{"ts":1,"lat":1e999,"lon":0,"ax":0,"ay":0,"az":0}`, // float overflow rejected
		`{"ts":"5","lat":0,"lon":0,"ax":0,"ay":0,"az":0}`,  // ts wrong type
		`{"lat":true,"ts":1}`,                              // bool into float
		`{"ts":1,"lat":0,"lon":0,"ax":0,"ay":0,"az":0}trailing`,
		`{"ts":1,"lat":1..2,"lon":0,"ax":0,"ay":0,"az":0}`,
		`{not json`,
		`[1,2,3]`,
		`5`,
		`{"ts":1,}`,
		``,
	}
	for _, in := range cases {
		wantPJ, wantErr := referenceScan([]byte(in))
		gotPJ, gotErr := scanPointJSON([]byte(in))
		if (wantErr == nil) != (gotErr == nil) {
			t.Errorf("input %q: stdlib err=%v, scan err=%v", in, wantErr, gotErr)
			continue
		}
		if wantErr == nil && !pjEqual(wantPJ, gotPJ) {
			t.Errorf("input %q: pj mismatch\n stdlib=%+v\n scan  =%+v", in, wantPJ, gotPJ)
		}
	}
}

func FuzzScanPointJSON(f *testing.F) {
	for _, s := range []string{
		`{"ts":1,"lat":0,"lon":0,"ax":0,"ay":0,"az":0,"battery":0.5}`,
		`{"ts":1,"lat":null,"x":[1,{}]}`,
		`{}`, `{"ts":01}`, `5`, `{not`,
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		wantPJ, wantErr := referenceScan(b)
		gotPJ, gotErr := scanPointJSON(b)
		// Safe invariant: whenever stdlib accepts, the fast scanner must accept
		// the identical fields. (It may reject some exotic inputs stdlib accepts,
		// which only ever yields a stricter 400 -- never a misparse.)
		if wantErr == nil {
			if gotErr != nil {
				t.Fatalf("stdlib accepted but scan rejected: %q (%v)", b, gotErr)
			}
			if !pjEqual(wantPJ, gotPJ) {
				t.Fatalf("field mismatch for %q\n stdlib=%+v\n scan  =%+v", b, wantPJ, gotPJ)
			}
		}
	})
}
