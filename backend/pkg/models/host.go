package models

import (
	"database/sql"
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
	Error         sql.NullString `json:"error"`
}
