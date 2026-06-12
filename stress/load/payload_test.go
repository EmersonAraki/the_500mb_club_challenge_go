package load

import (
	"encoding/json"
	"math/rand"
	"testing"

	"github.com/araki/pibench/internal/model"
)

func TestRandomPointAlwaysValid(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 1000; i++ {
		if _, err := model.ParsePoint(RandomPoint(rng)); err != nil {
			t.Fatalf("RandomPoint produced an invalid point: %v\n%s", err, RandomPoint(rng))
		}
	}
}

func TestRandomBatchValidAndSized(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	body := RandomBatch(rng, 50)
	var parsed struct {
		Points []json.RawMessage `json:"points"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("batch not valid JSON: %v", err)
	}
	if len(parsed.Points) != 50 {
		t.Fatalf("batch size: got %d want 50", len(parsed.Points))
	}
	for i, raw := range parsed.Points {
		if _, err := model.ParsePoint(raw); err != nil {
			t.Errorf("batch point %d invalid: %v", i, err)
		}
	}
}
