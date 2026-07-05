// Package playbooks stores named, ordered lists of shell steps and compiles
// them into runnable commands for a host. The run engines (cmd/api run-playbook
// and the bulk coordinator) load a playbook, call CompileSteps, and feed the
// result to the existing streaming runner — playbooks add no execution code of
// their own.
package playbooks

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"ubuntu-auto-update/backend/pkg/db"
)

type Playbook struct {
	ID          int32     `json:"id" db:"id"`
	Name        string    `json:"name" db:"name"`
	Description string    `json:"description" db:"description"`
	Steps       []string  `json:"steps" db:"steps"`
	UseSudo     bool      `json:"use_sudo" db:"use_sudo"`
	CreatedBy   string    `json:"created_by" db:"created_by"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

const cols = `id, name, description, steps, use_sudo, created_by, created_at, updated_at`

func List(ctx context.Context, dbx db.DBTX) ([]Playbook, error) {
	rows, err := dbx.Query(ctx, `SELECT `+cols+` FROM playbooks ORDER BY name`)
	if err != nil {
		return nil, err
	}
	pbs, err := pgx.CollectRows(rows, pgx.RowToStructByName[Playbook])
	if err != nil {
		return nil, err
	}
	if pbs == nil {
		pbs = []Playbook{}
	}
	return pbs, nil
}

// Get returns a playbook by id, or pgx.ErrNoRows when it doesn't exist.
func Get(ctx context.Context, dbx db.DBTX, id int32) (Playbook, error) {
	rows, err := dbx.Query(ctx, `SELECT `+cols+` FROM playbooks WHERE id = $1`, id)
	if err != nil {
		return Playbook{}, err
	}
	return pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[Playbook])
}

func Create(ctx context.Context, dbx db.DBTX, name, description string, steps []string, useSudo bool, createdBy string) (Playbook, error) {
	rows, err := dbx.Query(ctx, `
		INSERT INTO playbooks (name, description, steps, use_sudo, created_by)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+cols,
		name, description, steps, useSudo, createdBy)
	if err != nil {
		return Playbook{}, err
	}
	return pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[Playbook])
}

// Update replaces a playbook's editable fields and bumps updated_at. Returns
// pgx.ErrNoRows if no row matches.
func Update(ctx context.Context, dbx db.DBTX, id int32, name, description string, steps []string, useSudo bool) (Playbook, error) {
	rows, err := dbx.Query(ctx, `
		UPDATE playbooks
		SET name = $2, description = $3, steps = $4, use_sudo = $5, updated_at = NOW()
		WHERE id = $1
		RETURNING `+cols,
		id, name, description, steps, useSudo)
	if err != nil {
		return Playbook{}, err
	}
	return pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[Playbook])
}

func Delete(ctx context.Context, dbx db.DBTX, id int32) (int64, error) {
	tag, err := dbx.Exec(ctx, `DELETE FROM playbooks WHERE id = $1`, id)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// CompileSteps turns raw playbook steps into runnable shell lines for a host.
// Root, or useSudo=false: steps pass through unchanged. Otherwise each step is
// wrapped `sudo -n sh -c '<step>'` (single quotes escaped as '\”) so a compound
// step (&&, |, ;) escalates as a whole — a bare `sudo -n <step>` prefix would
// only cover the first command. -n keeps buildUpdateScript's fail-fast behavior
// when passwordless sudo is missing.
func CompileSteps(steps []string, sshUser string, useSudo bool) []string {
	if !useSudo || sshUser == "" || sshUser == "root" {
		// Copy so callers can't mutate the playbook's stored slice.
		return append([]string(nil), steps...)
	}
	out := make([]string, len(steps))
	for i, s := range steps {
		out[i] = "sudo -n sh -c '" + strings.ReplaceAll(s, "'", `'\''`) + "'"
	}
	return out
}
