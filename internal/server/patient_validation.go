package server

import (
	"net/mail"
	"strings"
	"time"
)

func validatePatient(input *patientPayload) string {
	input.FirstName, input.LastName = strings.TrimSpace(input.FirstName), strings.TrimSpace(input.LastName)
	if err := requireFields(map[string]string{"First name": input.FirstName, "Last name": input.LastName}); err != nil {
		return err.Error()
	}
	if len(input.FirstName) > 200 || len(input.LastName) > 200 {
		return "Patient names must be 200 characters or fewer."
	}
	input.Sex = strings.TrimSpace(input.Sex)
	if !allowedValue(input.Sex, []string{"female", "male", "other"}) {
		return "Sex must be female, male, other, or left blank."
	}
	input.DateOfBirth = strings.TrimSpace(input.DateOfBirth)
	if input.DateOfBirth != "" {
		born, err := time.Parse("2006-01-02", input.DateOfBirth)
		if err != nil || born.After(time.Now().UTC()) {
			return "Date of birth must be a valid date in YYYY-MM-DD format and cannot be in the future."
		}
	}
	input.Email = strings.TrimSpace(input.Email)
	if input.Email != "" {
		address, err := mail.ParseAddress(input.Email)
		if err != nil || address.Address != input.Email || len(input.Email) > 254 {
			return "Enter a valid email address or leave it blank."
		}
	}
	if input.Tags == nil {
		input.Tags = []string{}
	}
	return normaliseDemographics(input)
}
