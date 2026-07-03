package db_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
	"ubuntu-auto-update/backend/pkg/crypto"
	"ubuntu-auto-update/backend/pkg/db"
)

// setTestKey points crypto at an in-env key so tests don't depend on an
// encryption.key file existing in the working directory.
func setTestKey(t *testing.T) {
	t.Helper()
	t.Setenv("ENCRYPTION_KEY", "0000000000000000000000000000000000000000000000000000000000000000")
}

func TestUpsertHost(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("error creating mock: %v", err)
	}
	defer mock.Close()

	now := time.Now()
	// Success path
	rows := mock.NewRows([]string{"id", "hostname", "ssh_user", "created_at", "updated_at", "last_seen", "update_output", "upgrade_output", "error", "tags", "reboot_required", "packages_updated", "packages_available", "os_version", "kernel_version", "agent_version"}).
		AddRow(int32(1), "test-host", "root", now, now, now, "out", "out", nil, []string{}, false, 0, 0, "", "", "")

	mock.ExpectQuery(`INSERT INTO hosts`).
		WithArgs("test-host", "root", "out", "out", sql.NullString{}, false, 0, 0, "", "", "").
		WillReturnRows(rows)

	_, err = db.UpsertHost(context.Background(), mock, "test-host", "root", db.ReportData{UpdateOutput: "out", UpgradeOutput: "out"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Error path
	mock.ExpectQuery(`INSERT INTO hosts`).
		WithArgs("test-host-2", "root", "out", "out", sql.NullString{String: "err", Valid: true}, false, 0, 0, "", "", "").
		WillReturnError(errors.New("db error"))

	_, err = db.UpsertHost(context.Background(), mock, "test-host-2", "root", db.ReportData{UpdateOutput: "out", UpgradeOutput: "out", Error: "err"})
	if err == nil {
		t.Error("expected error")
	}
}

func TestListHosts(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("error creating mock: %v", err)
	}
	defer mock.Close()

	now := time.Now()
	// Success path
	rows := mock.NewRows([]string{"id", "hostname", "ssh_user", "created_at", "updated_at", "last_seen", "update_output", "upgrade_output", "error", "tags", "reboot_required", "packages_updated", "packages_available", "os_version", "kernel_version", "agent_version"}).
		AddRow(int32(1), "test-host", "root", now, now, now, "", "", nil, []string{}, false, 0, 0, "", "", "")

	mock.ExpectQuery(`SELECT (.+) FROM hosts ORDER BY hostname`).
		WillReturnRows(rows)

	_, err = db.ListHosts(context.Background(), mock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Error path
	mock.ExpectQuery(`SELECT (.+) FROM hosts ORDER BY hostname`).
		WillReturnError(errors.New("db error"))
	_, err = db.ListHosts(context.Background(), mock)
	if err == nil {
		t.Error("expected error")
	}

	// CollectRows error path
	mock.ExpectQuery(`SELECT (.+) FROM hosts ORDER BY hostname`).
		WillReturnRows(mock.NewRows([]string{"id"}).AddRow("not-an-int"))
	_, err = db.ListHosts(context.Background(), mock)
	if err == nil {
		t.Error("expected error from CollectRows")
	}

	// 0 rows path
	mock.ExpectQuery(`SELECT (.+) FROM hosts ORDER BY hostname`).
		WillReturnRows(mock.NewRows([]string{"id", "hostname", "ssh_user", "created_at", "updated_at", "last_seen", "update_output", "upgrade_output", "error", "tags", "reboot_required", "packages_updated", "packages_available", "os_version", "kernel_version", "agent_version"}))
	hosts, err := db.ListHosts(context.Background(), mock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hosts == nil {
		t.Error("expected non-nil empty slice")
	}
}

func TestCreateHost(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("error creating mock: %v", err)
	}
	defer mock.Close()

	now := time.Now()
	// Success
	rows := mock.NewRows([]string{"id", "hostname", "ssh_user", "created_at", "updated_at", "last_seen", "update_output", "upgrade_output", "error", "tags", "reboot_required", "packages_updated", "packages_available", "os_version", "kernel_version", "agent_version"}).
		AddRow(int32(1), "test-host", "root", now, now, now, "", "", nil, []string{}, false, 0, 0, "", "", "")

	mock.ExpectQuery(`INSERT INTO hosts`).
		WithArgs("test-host", "root").
		WillReturnRows(rows)

	_, err = db.CreateHost(context.Background(), mock, "test-host", "root")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Test ErrDuplicateHostname
	mock.ExpectQuery(`INSERT INTO hosts`).
		WithArgs("test-host", "root").
		WillReturnError(&pgconn.PgError{Code: "23505"})

	_, err = db.CreateHost(context.Background(), mock, "test-host", "root")
	if err != db.ErrDuplicateHostname {
		t.Errorf("expected ErrDuplicateHostname, got %v", err)
	}

	// Test general error
	mock.ExpectQuery(`INSERT INTO hosts`).
		WithArgs("test-host", "root").
		WillReturnError(errors.New("db error"))

	_, err = db.CreateHost(context.Background(), mock, "test-host", "root")
	if err == nil {
		t.Errorf("expected error")
	}

	// Test CollectExactlyOneRow error
	mock.ExpectQuery(`INSERT INTO hosts`).
		WithArgs("test-host", "root").
		WillReturnRows(mock.NewRows([]string{"id"}).AddRow("invalid"))

	_, err = db.CreateHost(context.Background(), mock, "test-host", "root")
	if err == nil {
		t.Errorf("expected CollectExactlyOneRow error")
	}
}

func TestUpdateHostSSHUser(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("error creating mock: %v", err)
	}
	defer mock.Close()

	now := time.Now()
	rows := mock.NewRows([]string{"id", "hostname", "ssh_user", "created_at", "updated_at", "last_seen", "update_output", "upgrade_output", "error", "tags", "reboot_required", "packages_updated", "packages_available", "os_version", "kernel_version", "agent_version"}).
		AddRow(int32(1), "test-host", "ubuntu", now, now, now, "", "", nil, []string{}, false, 0, 0, "", "", "")

	mock.ExpectQuery(`UPDATE hosts SET ssh_user = \$2 WHERE id = \$1`).
		WithArgs(int32(1), "ubuntu").
		WillReturnRows(rows)

	_, err = db.UpdateHostSSHUser(context.Background(), mock, 1, "ubuntu")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Error path
	mock.ExpectQuery(`UPDATE hosts SET ssh_user = \$2 WHERE id = \$1`).
		WithArgs(int32(2), "ubuntu").
		WillReturnError(errors.New("db error"))
	_, err = db.UpdateHostSSHUser(context.Background(), mock, 2, "ubuntu")
	if err == nil {
		t.Error("expected error")
	}
}

func TestDeleteHost(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("error creating mock: %v", err)
	}
	defer mock.Close()

	mock.ExpectExec(`DELETE FROM hosts WHERE id = \$1`).
		WithArgs(int32(1)).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))

	_, err = db.DeleteHost(context.Background(), mock, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mock.ExpectExec(`DELETE FROM hosts WHERE id = \$1`).
		WithArgs(int32(2)).
		WillReturnError(errors.New("db error"))
	_, err = db.DeleteHost(context.Background(), mock, 2)
	if err == nil {
		t.Error("expected error")
	}
}

func TestGetHost(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("error creating mock: %v", err)
	}
	defer mock.Close()

	now := time.Now()
	// Success path
	rows := mock.NewRows([]string{"id", "hostname", "ssh_user", "created_at", "updated_at", "last_seen", "update_output", "upgrade_output", "error", "tags", "reboot_required", "packages_updated", "packages_available", "os_version", "kernel_version", "agent_version"}).
		AddRow(int32(1), "test-host", "root", now, now, now, "", "", nil, []string{}, false, 0, 0, "", "", "")

	mock.ExpectQuery(`SELECT (.+) FROM hosts WHERE id = \$1`).
		WithArgs(int32(1)).
		WillReturnRows(rows)

	_, err = db.GetHost(context.Background(), mock, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Error path
	mock.ExpectQuery(`SELECT (.+) FROM hosts WHERE id = \$1`).
		WithArgs(int32(2)).
		WillReturnError(errors.New("db error"))
	_, err = db.GetHost(context.Background(), mock, 2)
	if err == nil {
		t.Error("expected error")
	}
}

func TestGetSSHKey(t *testing.T) {
	setTestKey(t)
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("error creating mock: %v", err)
	}
	defer mock.Close()

	// Need a valid encrypted key
	rows := mock.NewRows([]string{"host_id", "private_key"}).
		AddRow(int32(1), "invalid-encrypted-key")

	mock.ExpectQuery(`SELECT host_id, private_key FROM ssh_keys WHERE host_id = \$1`).
		WithArgs(int32(1)).
		WillReturnRows(rows)

	_, err = db.GetSSHKey(context.Background(), mock, 1)
	if err == nil {
		t.Errorf("expected error decrypting invalid key")
	}

	// DB error
	mock.ExpectQuery(`SELECT host_id, private_key FROM ssh_keys WHERE host_id = \$1`).
		WithArgs(int32(2)).
		WillReturnError(errors.New("db error"))
	_, err = db.GetSSHKey(context.Background(), mock, 2)
	if err == nil {
		t.Error("expected error")
	}

	// ErrNoRows error
	mock.ExpectQuery(`SELECT host_id, private_key FROM ssh_keys WHERE host_id = \$1`).
		WithArgs(int32(3)).
		WillReturnError(pgx.ErrNoRows)
	_, err = db.GetSSHKey(context.Background(), mock, 3)
	if err != pgx.ErrNoRows {
		t.Error("expected ErrNoRows")
	}

	// Success path
	encrypted, _ := crypto.Encrypt("secret")
	mock.ExpectQuery(`SELECT host_id, private_key FROM ssh_keys WHERE host_id = \$1`).
		WithArgs(int32(4)).
		WillReturnRows(mock.NewRows([]string{"host_id", "private_key"}).AddRow(int32(4), encrypted))

	key, err := db.GetSSHKey(context.Background(), mock, 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key.PrivateKey != "secret" {
		t.Errorf("expected 'secret', got %s", key.PrivateKey)
	}
}

func TestAddSSHKey(t *testing.T) {
	setTestKey(t)
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("error creating mock: %v", err)
	}
	defer mock.Close()

	mock.ExpectExec(`INSERT INTO ssh_keys`).
		WithArgs(int32(1), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	err = db.AddSSHKey(context.Background(), mock, 1, "private-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mock.ExpectExec(`INSERT INTO ssh_keys`).
		WithArgs(int32(2), pgxmock.AnyArg()).
		WillReturnError(errors.New("db error"))
	err = db.AddSSHKey(context.Background(), mock, 2, "private-key")
	if err == nil {
		t.Error("expected error")
	}
}

func TestSetSSHKeyAndUser(t *testing.T) {
	setTestKey(t)
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("error creating mock: %v", err)
	}
	defer mock.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO ssh_keys`).
		WithArgs(int32(1), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec(`UPDATE hosts SET ssh_user = \$1, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs("ubuntu", int32(1)).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectCommit()

	err = db.SetSSHKeyAndUser(context.Background(), mock, 1, "ubuntu", "private-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Begin error
	mock.ExpectBegin().WillReturnError(errors.New("db error"))
	err = db.SetSSHKeyAndUser(context.Background(), mock, 2, "ubuntu", "private-key")
	if err == nil {
		t.Error("expected error")
	}

	// Insert error
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO ssh_keys`).
		WithArgs(int32(3), pgxmock.AnyArg()).
		WillReturnError(errors.New("db error"))
	mock.ExpectRollback()
	err = db.SetSSHKeyAndUser(context.Background(), mock, 3, "ubuntu", "private-key")
	if err == nil {
		t.Error("expected error")
	}

	// Update error
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO ssh_keys`).
		WithArgs(int32(4), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec(`UPDATE hosts SET ssh_user = \$1, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs("ubuntu", int32(4)).
		WillReturnError(errors.New("db error"))
	mock.ExpectRollback()
	err = db.SetSSHKeyAndUser(context.Background(), mock, 4, "ubuntu", "private-key")
	if err == nil {
		t.Error("expected error")
	}
}

func TestGetWebhooks(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("error creating mock: %v", err)
	}
	defer mock.Close()

	rows := mock.NewRows([]string{"id", "url", "event"}).
		AddRow(int32(1), "http://test", "update_success")

	mock.ExpectQuery(`SELECT id, url, event FROM webhooks WHERE event = \$1`).
		WithArgs("update_success").
		WillReturnRows(rows)

	_, err = db.GetWebhooks(context.Background(), mock, "update_success")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mock.ExpectQuery(`SELECT id, url, event FROM webhooks WHERE event = \$1`).
		WithArgs("update_fail").
		WillReturnError(errors.New("db error"))
	_, err = db.GetWebhooks(context.Background(), mock, "update_fail")
	if err == nil {
		t.Error("expected error")
	}

	// CollectRows error path
	mock.ExpectQuery(`SELECT id, url, event FROM webhooks WHERE event = \$1`).
		WithArgs("update_success").
		WillReturnRows(mock.NewRows([]string{"id"}).AddRow("not-an-int"))
	_, err = db.GetWebhooks(context.Background(), mock, "update_success")
	if err == nil {
		t.Error("expected error from CollectRows")
	}

	// 0 rows path
	mock.ExpectQuery(`SELECT id, url, event FROM webhooks WHERE event = \$1`).
		WithArgs("update_empty").
		WillReturnRows(mock.NewRows([]string{"id", "url", "event"}))
	hooks, err := db.GetWebhooks(context.Background(), mock, "update_empty")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hooks == nil {
		t.Error("expected non-nil empty slice")
	}
}
