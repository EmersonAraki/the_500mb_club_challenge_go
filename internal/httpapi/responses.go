package httpapi

import (
	"encoding/json"
	"net/http"
)

// writeJSON encodes v as the body with the given status code.
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// status writes a bare status code with no body.
func status(w http.ResponseWriter, code int) { w.WriteHeader(code) }
