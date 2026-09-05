package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

type APIError struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

func (e *APIError) Error() string { return e.Message }

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, APIError{Code: code, Message: message})
}

func decodeJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, (2<<20)+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		if errors.Is(err, io.EOF) {
			return &APIError{Code: "EMPTY_BODY", Message: "A JSON request body is required."}
		}
		return &APIError{Code: "INVALID_JSON", Message: fmt.Sprintf("Invalid request body: %v", err)}
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return &APIError{Code: "INVALID_JSON", Message: "The request body must contain one JSON object."}
	}
	return nil
}

func requireFields(fields map[string]string) error {
	for label, value := range fields {
		if value == "" {
			return &APIError{Code: "VALIDATION_ERROR", Message: label + " is required."}
		}
	}
	return nil
}
