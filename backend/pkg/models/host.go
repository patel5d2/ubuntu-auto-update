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
	Tags          []string       `json:"tags" db:"tags"`

	// Agent-reported fields (populated by /api/v1/report). Zero-valued for
	// SSH-only hosts that never run the agent.
	RebootRequired    bool   `json:"reboot_required" db:"reboot_required"`
	PackagesUpdated   int    `json:"packages_updated" db:"packages_updated"`
	PackagesAvailable int    `json:"packages_available" db:"packages_available"`
	OsVersion         string `json:"os_version" db:"os_version"`
	KernelVersion     string `json:"kernel_version" db:"kernel_version"`
	AgentVersion      string `json:"agent_version" db:"agent_version"`

	// OfflineSince is set by the server-side offline sweep when last_seen
	// crosses the threshold; nil = online (or not yet evaluated).
	OfflineSince *time.Time `json:"offline_since" db:"offline_since"`
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
