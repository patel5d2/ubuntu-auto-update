package models

import (
	"database/sql"
	"encoding/json"
	"time"
)

type Host struct {
	ID            int32          `json:"id"`
	Hostname      string         `json:"hostname"`
	SshUser       string         `json:"ssh_user"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	LastSeen      time.Time      `json:"last_seen"`
	UpdateOutput  string         `json:"update_output"`
	UpgradeOutput string         `json:"upgrade_output"`
	Error         sql.NullString `json:"-"` // Exclude from default marshaling
}

// MarshalJSON implements custom JSON marshaling for Host.
// This produces a clean "error" field as a string or null instead of
// the default sql.NullString struct {"String":"","Valid":false}.
func (h Host) MarshalJSON() ([]byte, error) {
	type Alias Host // Avoid infinite recursion

	var errorValue interface{}
	if h.Error.Valid {
		errorValue = h.Error.String
	} else {
		errorValue = nil
	}

	return json.Marshal(&struct {
		Alias
		Error interface{} `json:"error"`
	}{
		Alias: Alias(h),
		Error: errorValue,
	})
}
