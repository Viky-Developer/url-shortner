// Package utils provides shared utility types used across internal packages.
package utils

import (
	"strings"
	"time"
)

// OptionalTime is a time that also accepts null or empty-string JSON values,
// treating them as "not provided". It is used for optional request fields so an
// omitted or empty expiresAt never triggers a type conversion error.
type OptionalTime struct {
	Time  time.Time
	Valid bool
}

// UnmarshalJSON parses an RFC3339 timestamp. Null and empty strings set the
// value as invalid instead of raising a conversion error.
func (t *OptionalTime) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		t.Valid = false
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return err
	}
	t.Time = parsed
	t.Valid = true
	return nil
}
