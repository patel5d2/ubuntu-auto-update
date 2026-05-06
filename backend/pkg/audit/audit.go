// Package audit writes a permanent record of every state-changing action.
// Reads are paginated and filterable so the UI can render an "activity"
// feed and ops can answer "who deleted host X" without grepping logs.
package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"ubuntu-auto-update/backend/pkg/db"
)

// Action constants. Centralised so every callsite logs the same string for
// the same logical operation. New actions: add a constant, never a literal.
const (
	ActionLoginSuccess    = "login.success"
	ActionLoginFailure    = "login.failure"
	ActionLogout          = "logout"

	ActionUserCreate      = "user.create"
	ActionUserUpdate      = "user.update"
	ActionUserDelete      = "user.delete"
	ActionUserPassword    = "user.password_reset"
	ActionUserDisable     = "user.disable"
	ActionUserEnable      = "user.enable"

	ActionHostCreate      = "host.create"
	ActionHostUpdate      = "host.update"
	ActionHostDelete      = "host.delete"
	ActionHostBootstrap   = "host.bootstrap"
	ActionHostKeyRotate   = "host.key_rotate"
	ActionHostKeyInstall  = "host.key_install"
	ActionHostTestConn    = "host.test_connection"

	ActionRunPreview      = "run.preview"
	ActionRunUpdate       = "run.update"
	ActionRunBulkUpdate   = "run.bulk_update"
	ActionRunScript       = "run.script"

	ActionWebhookCreate   = "webhook.create"
	ActionAgentEnroll     = "agent.enroll"
)

// Event is what callers hand to Log. Keep it small — JSON details are for
// the long tail.
type Event struct {
	ActorUserID *int32                 // nil for unauthenticated / agent contexts
	ActorLabel  string                 // copy of username, kept even if user deleted
	Action      string
	TargetType  string
	TargetID    string
	RequestID   string
	IP          string
	UserAgent   string
	Details     map[string]interface{} // optional structured payload
}

// Log inserts a single audit record. Best-effort — callers should not fail
// the user-facing operation on audit-write errors, but they should log them.
func Log(ctx context.Context, db db.DBTX, e Event) error {
	if e.Action == "" {
		return fmt.Errorf("audit: action is required")
	}
	details := []byte("{}")
	if len(e.Details) > 0 {
		b, err := json.Marshal(e.Details)
		if err != nil {
			return fmt.Errorf("audit: marshal details: %w", err)
		}
		details = b
	}
	_, err := db.Exec(ctx, `
		INSERT INTO audit_log
		    (actor_user_id, actor_label, action, target_type, target_id,
		     request_id, ip, user_agent, details)
		VALUES ($1, NULLIF($2, ''), $3, NULLIF($4, ''), NULLIF($5, ''),
		        NULLIF($6, ''), NULLIF($7, ''), NULLIF($8, ''), $9::jsonb)`,
		e.ActorUserID, e.ActorLabel, e.Action,
		e.TargetType, e.TargetID,
		e.RequestID, e.IP, e.UserAgent, string(details),
	)
	return err
}

// Record is the row returned by List/queries. JSON details are decoded for
// the consumer's convenience.
type Record struct {
	ID          int64                  `json:"id"`
	OccurredAt  time.Time              `json:"occurred_at"`
	ActorUserID *int32                 `json:"actor_user_id,omitempty"`
	ActorLabel  string                 `json:"actor_label,omitempty"`
	Action      string                 `json:"action"`
	TargetType  string                 `json:"target_type,omitempty"`
	TargetID    string                 `json:"target_id,omitempty"`
	RequestID   string                 `json:"request_id,omitempty"`
	IP          string                 `json:"ip,omitempty"`
	UserAgent   string                 `json:"user_agent,omitempty"`
	Details     map[string]interface{} `json:"details,omitempty"`
}

// ListOptions controls pagination and filtering for List.
type ListOptions struct {
	Limit      int
	Action     string // exact match if non-empty
	TargetType string // exact match if non-empty
	TargetID   string // exact match if non-empty (and TargetType set)
}

// List returns recent audit records, newest first. Defaults: limit=100.
func List(ctx context.Context, db db.DBTX, opts ListOptions) ([]Record, error) {
	limit := opts.Limit
	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	// Build a tiny query — we only support exact-match filters for now to keep
	// the index plan obvious. Additional filters can land later as the UI needs.
	args := []interface{}{}
	where := ""
	if opts.Action != "" {
		args = append(args, opts.Action)
		where += fmt.Sprintf(" AND action = $%d", len(args))
	}
	if opts.TargetType != "" {
		args = append(args, opts.TargetType)
		where += fmt.Sprintf(" AND target_type = $%d", len(args))
		if opts.TargetID != "" {
			args = append(args, opts.TargetID)
			where += fmt.Sprintf(" AND target_id = $%d", len(args))
		}
	}
	args = append(args, limit)

	q := fmt.Sprintf(`
		SELECT id, occurred_at, actor_user_id, COALESCE(actor_label, ''), action,
		       COALESCE(target_type, ''), COALESCE(target_id, ''),
		       COALESCE(request_id, ''), COALESCE(ip, ''), COALESCE(user_agent, ''),
		       details
		FROM audit_log
		WHERE 1=1%s
		ORDER BY occurred_at DESC
		LIMIT $%d`, where, len(args))

	rows, err := db.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Record{}
	for rows.Next() {
		var r Record
		var detailsBytes []byte
		if err := rows.Scan(
			&r.ID, &r.OccurredAt, &r.ActorUserID, &r.ActorLabel, &r.Action,
			&r.TargetType, &r.TargetID,
			&r.RequestID, &r.IP, &r.UserAgent,
			&detailsBytes,
		); err != nil {
			return nil, err
		}
		if len(detailsBytes) > 0 {
			_ = json.Unmarshal(detailsBytes, &r.Details)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

