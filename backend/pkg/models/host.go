package models

import (
	"database/sql"
	"encoding/json"
	"time"
)

type Host struct {
	ID            int32          `json:"id" db:"id"`
	Hostname      string         `json:"hostname" db:"hostname"`
	SshUser       string         `json:"ssh_user" db:"ssh_user"`
	CreatedAt     time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at" db:"updated_at"`
	LastSeen      time.Time      `json:"last_seen" db:"last_seen"`
	UpdateOutput  string         `json:"update_output" db:"update_output"`
	UpgradeOutput string         `json:"upgrade_output" db:"upgrade_output"`
	Error         sql.NullString `json:"-" db:"error"`
}

// MarshalJSON renders Error as a plain string-or-null instead of the default
// sql.NullString shape ({"String":"","Valid":false}).
func (h Host) MarshalJSON() ([]byte, error) {
	type Alias Host

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
