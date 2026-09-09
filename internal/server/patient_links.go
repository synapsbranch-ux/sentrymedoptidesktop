package server

import (
	"context"
	"database/sql"
)

type rowQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// The queries are fixed by callers, never assembled from request values.
type patientLink struct{ query, id string }

func validatePatientLinks(ctx context.Context, db rowQuerier, patientID string, links ...patientLink) error {
	var id string
	if err := db.QueryRowContext(ctx, "SELECT id FROM patients WHERE id=? AND archived_at IS NULL", patientID).Scan(&id); err != nil {
		if err == sql.ErrNoRows {
			return &APIError{Code: "INVALID_PATIENT", Message: "Choose an active patient."}
		}
		return err
	}
	for _, link := range links {
		if link.id == "" {
			continue
		}
		var owner string
		err := db.QueryRowContext(ctx, link.query, link.id).Scan(&owner)
		if err == sql.ErrNoRows || (err == nil && owner != patientID) {
			return &APIError{Code: "PATIENT_LINK_MISMATCH", Message: "The linked record must belong to the selected patient and be available."}
		}
		if err != nil {
			return err
		}
	}
	return nil
}
