package webapi

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// jsonOK writes a 200 response with JSON-encoded data.
func jsonOK(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		http.Error(w, `{"error":"encode response"}`, http.StatusInternalServerError)
	}
}

// jsonError writes an error response with the given HTTP status code
// and a JSON body of {"error": "message"}.
func jsonError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// decodeJSON reads and decodes the request body as JSON into v.
// Limits the body to 64KB to prevent abuse. Returns an error if the
// body cannot be decoded.
func decodeJSON(r *http.Request, v interface{}) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 64*1024) // 64KB max
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return fmt.Errorf("decode request body: %w", err)
	}
	return nil
}
