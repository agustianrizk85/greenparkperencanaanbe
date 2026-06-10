// Package http provides the HTTP transport layer: router, handlers and
// middleware. It depends on the service layer only.
package http

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"greenpark/perencanaan/internal/service"
)

type errorBody struct {
	Error string `json:"error"`
}

// writeJSON serialises v as JSON with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("perencanaan: encode response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorBody{Error: msg})
}

// decodeJSON reads the request body into dst, writing a 400 on failure.
// It returns false when the caller should stop (an error was already written).
func decodeJSON[T any](w http.ResponseWriter, r *http.Request, dst *T) bool {
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return false
	}
	return true
}

// writeServiceError maps service sentinel errors to HTTP status codes.
func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrNotFound):
		writeError(w, http.StatusNotFound, "resource not found")
	case errors.Is(err, service.ErrValidation):
		writeError(w, http.StatusBadRequest, "validation failed: required field is missing")
	case errors.Is(err, service.ErrInvalidCredentials):
		writeError(w, http.StatusUnauthorized, "invalid username or password")
	case errors.Is(err, service.ErrForbidden):
		writeError(w, http.StatusForbidden, "you are not permitted to perform this action")
	default:
		log.Printf("perencanaan: unexpected error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}
