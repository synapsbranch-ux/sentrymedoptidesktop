package server

import (
	"bytes"
	"encoding/json"
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
	const maxJSONBody = 2 << 20
	body, err := io.ReadAll(io.LimitReader(r.Body, maxJSONBody+1))
	if err != nil || len(body) > maxJSONBody {
		return &APIError{Code: "INVALID_JSON", Message: "The JSON request could not be read or exceeds 2 MiB."}
	}
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return &APIError{Code: "EMPTY_BODY", Message: "A JSON request body is required."}
	}
	if body[0] != '{' {
		return &APIError{Code: "INVALID_JSON", Message: "The request body must be a JSON object."}
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return &APIError{Code: "INVALID_JSON", Message: "Invalid request fields or data types. Please check the form."}
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
