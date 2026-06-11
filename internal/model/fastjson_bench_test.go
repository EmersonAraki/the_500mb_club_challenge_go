package model

import (
	"encoding/json"
	"testing"
)

var benchPoint = []byte(`{"ts":1715800000000,"lat":12.5,"lon":-40.25,"ax":0.1,"ay":0.2,"az":9.81,"battery":0.75}`)

// BenchmarkParsePoint_Fast is the production reflection-free path.
func BenchmarkParsePoint_Fast(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := ParsePoint(benchPoint); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParsePoint_Stdlib is the prior reflection-based path, kept here only
// to quantify the delta.
func BenchmarkParsePoint_Stdlib(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var pj pointJSON
		if err := json.Unmarshal(benchPoint, &pj); err != nil {
			b.Fatal(err)
		}
		if _, err := pj.toPoint(); err != nil {
			b.Fatal(err)
		}
	}
}
