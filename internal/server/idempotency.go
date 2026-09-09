package server

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"
)

// Reserve the key, perform the mutation and save the response in ONE SQLite
// transaction. A lost response or concurrent retry cannot duplicate a payment.
// Keys are optional for older clients; the maintained UI always sends them.
func (s *Server) idempotentTx(r *http.Request, input any, mutate func(*sql.Tx) error, response func() any) (json.RawMessage, error) {
	key := r.Header.Get("Idempotency-Key")
	if key == "" {
		return nil, s.db.WithTx(r.Context(), mutate)
	}
	if len(key) > 128 {
		return nil, &APIError{Code: "INVALID_IDEMPOTENCY_KEY", Message: "The request identifier is too long."}
	}
	user, _ := userFromContext(r.Context())
	body, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	hash := tokenHash(string(body))
	scope := user.ID + ":" + r.Method + ":" + r.URL.Path + ":" + key
	var replay json.RawMessage
	err = s.db.WithTx(r.Context(), func(tx *sql.Tx) error {
		// Write first: SQLite serializes simultaneous retries before either reads.
		_, err := tx.ExecContext(r.Context(), "INSERT OR IGNORE INTO idempotency_records(scope,request_hash,response_json,created_at) VALUES(?,?,'',?)", scope, hash, time.Now().UTC().Format(time.RFC3339Nano))
		if err != nil {
			return err
		}
		var savedHash, saved string
		if err := tx.QueryRowContext(r.Context(), "SELECT request_hash,response_json FROM idempotency_records WHERE scope=?", scope).Scan(&savedHash, &saved); err != nil {
			return err
		}
		if savedHash != hash {
			return &APIError{Code: "IDEMPOTENCY_CONFLICT", Message: "This request identifier was already used for a different operation."}
		}
		if saved != "" {
			replay = json.RawMessage(saved)
			return nil
		}
		if err := mutate(tx); err != nil {
			return err
		}
		data, err := json.Marshal(response())
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(r.Context(), "UPDATE idempotency_records SET response_json=? WHERE scope=?", string(data), scope)
		return err
	})
	return replay, err
}
