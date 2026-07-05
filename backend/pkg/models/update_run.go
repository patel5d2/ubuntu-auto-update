package models

import (
	"database/sql"
	"encoding/json"
	"time"
)

// RunKind separates read-only previews ("apt list --upgradable") from real
// upgrades ("apt-get upgrade -y"). Persisted as a CHECK-constrained text
// column.
type RunKind string

const (
	RunKindPreview  RunKind = "preview"
	RunKindUpdate   RunKind = "update"
	RunKindPlaybook RunKind = "playbook"
)

// RunStatus tracks lifecycle. CHECK constraint in the schema enforces the
// allowed values.
type RunStatus string

const (
	RunStatusRunning   RunStatus = "running"
	RunStatusSucceeded RunStatus = "succeeded"
	RunStatusFailed    RunStatus = "failed"
	RunStatusCancelled RunStatus = "cancelled"
)

type UpdateRun struct {
	ID          int32          `json:"id"           db:"id"`
	HostID      int32          `json:"host_id"      db:"host_id"`
	RunGroupID  sql.NullString `json:"-"           db:"run_group_id"`
	TriggeredBy string         `json:"triggered_by" db:"triggered_by"`
	Kind        RunKind        `json:"kind"         db:"kind"`
	Status      RunStatus      `json:"status"       db:"status"`
	ExitCode    sql.NullInt32  `json:"-"            db:"exit_code"`
	StartedAt   time.Time      `json:"started_at"   db:"started_at"`
	FinishedAt  sql.NullTime   `json:"-"            db:"finished_at"`
	Output      string         `json:"output"       db:"output"`
	Error       sql.NullString `json:"-"           db:"error"`
	PlaybookID  sql.NullInt32  `json:"-"           db:"playbook_id"`
}

// MarshalJSON renders nullable columns as plain JSON null instead of the
// default sql.Null* shapes.
func (r UpdateRun) MarshalJSON() ([]byte, error) {
	type Alias UpdateRun

	var (
		exit interface{}
		fin  interface{}
		errV interface{}
		grp  interface{}
		pb   interface{}
	)
	if r.ExitCode.Valid {
		exit = r.ExitCode.Int32
	}
	if r.FinishedAt.Valid {
		fin = r.FinishedAt.Time
	}
	if r.Error.Valid {
		errV = r.Error.String
	}
	if r.RunGroupID.Valid {
		grp = r.RunGroupID.String
	}
	if r.PlaybookID.Valid {
		pb = r.PlaybookID.Int32
	}

	return json.Marshal(&struct {
		Alias
		ExitCode   interface{} `json:"exit_code"`
		FinishedAt interface{} `json:"finished_at"`
		Error      interface{} `json:"error"`
		RunGroupID interface{} `json:"run_group_id"`
		PlaybookID interface{} `json:"playbook_id"`
	}{
		Alias:      Alias(r),
		ExitCode:   exit,
		FinishedAt: fin,
		Error:      errV,
		RunGroupID: grp,
		PlaybookID: pb,
	})
}
