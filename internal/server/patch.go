package server

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
)

// decodePatch preserves omitted fields and clears explicit nulls to their zero
// value. Only fields declared by the write DTO may be changed. Callers must
// reset Version before decoding so an omitted version cannot bypass locking.
func decodePatch(r *http.Request, target any) error {
	var fields map[string]json.RawMessage
	if err := decodeJSON(r, &fields); err != nil {
		return err
	}
	value := reflect.ValueOf(target).Elem()
	allowed := make(map[string]int, value.NumField())
	for i := 0; i < value.NumField(); i++ {
		allowed[strings.Split(value.Type().Field(i).Tag.Get("json"), ",")[0]] = i
	}
	for key, raw := range fields {
		index, ok := allowed[key]
		if !ok {
			return &APIError{Code: "INVALID_REQUEST", Message: "The request includes a field that cannot be edited."}
		}
		field := value.Field(index)
		replacement := reflect.New(field.Type())
		if err := json.Unmarshal(raw, replacement.Interface()); err != nil {
			return &APIError{Code: "INVALID_REQUEST", Message: "A field has an incorrect data type."}
		}
		field.Set(replacement.Elem())
	}
	return nil
}
