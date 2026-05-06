package db_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
	"ubuntu-auto-update/backend/pkg/db"
)

func TestUpsertHost(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("error creating mock: %v", err)
	}
	defer mock.Close()

	now := time.Now()
	rows := mock.NewRows([]string{"id", "hostname", "ssh_user", "created_at", "updated_at", "last_seen", "update_output", "upgrade_output", "error"}).
		AddRow(int32(1), "test-host", "root", now, now, now, "out", "out", nil)

	mock.ExpectQuery(`INSERT INTO hosts`).
		WithArgs("test-host", "root", "out", "out", sql.NullString{}).
		WillReturnRows(rows)

	host, err := db.UpsertHost(context.Background(), mock, "test-host", "root", "out", "out", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if host.ID != 1 {
		t.Errorf("expected id 1, got %d", host.ID)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestListHosts(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("error creating mock: %v", err)
	}
	defer mock.Close()

	now := time.Now()
	rows := mock.NewRows([]string{"id", "hostname", "ssh_user", "created_at", "updated_at", "last_seen", "update_output", "upgrade_output", "error"}).
		AddRow(int32(1), "test-host", "root", now, now, now, "", "", nil)

	mock.ExpectQuery(`SELECT (.+) FROM hosts ORDER BY hostname`).
		WillReturnRows(rows)

	hosts, err := db.ListHosts(context.Background(), mock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(hosts) != 1 {
		t.Errorf("expected 1 host, got %d", len(hosts))
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestCreateHost(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("error creating mock: %v", err)
	}
	defer mock.Close()

	now := time.Now()
	rows := mock.NewRows([]string{"id", "hostname", "ssh_user", "created_at", "updated_at", "last_seen", "update_output", "upgrade_output", "error"}).
		AddRow(int32(1), "test-host", "root", now, now, now, "", "", nil)

	mock.ExpectQuery(`INSERT INTO hosts`).
		WithArgs("test-host", "root").
		WillReturnRows(rows)

	host, err := db.CreateHost(context.Background(), mock, "test-host", "root")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if host.ID != 1 {
		t.Errorf("expected id 1, got %d", host.ID)
	}

	// Test ErrDuplicateHostname
	mock.ExpectQuery(`INSERT INTO hosts`).
		WithArgs("test-host", "root").
		WillReturnError(&pgconn.PgError{Code: "23505"})

	_, err = db.CreateHost(context.Background(), mock, "test-host", "root")
	if err != db.ErrDuplicateHostname {
		t.Errorf("expected ErrDuplicateHostname, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestUpdateHostSSHUser(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("error creating mock: %v", err)
	}
	defer mock.Close()

	now := time.Now()
	rows := mock.NewRows([]string{"id", "hostname", "ssh_user", "created_at", "updated_at", "last_seen", "update_output", "upgrade_output", "error"}).
		AddRow(int32(1), "test-host", "ubuntu", now, now, now, "", "", nil)

	mock.ExpectQuery(`UPDATE hosts SET ssh_user = \$2 WHERE id = \$1`).
		WithArgs(int32(1), "ubuntu").
		WillReturnRows(rows)

	host, err := db.UpdateHostSSHUser(context.Background(), mock, 1, "ubuntu")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if host.SshUser != "ubuntu" {
		t.Errorf("expected ssh_user ubuntu, got %v", host.SshUser)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
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

	affected, err := db.DeleteHost(context.Background(), mock, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if affected != 1 {
		t.Errorf("expected affected 1, got %d", affected)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestGetSSHKey(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("error creating mock: %v", err)
	}
	defer mock.Close()

	// Need a valid encrypted key
	// crypto isn't easy to mock here so let's just make it return an empty string and fail the Decrypt step,
	// or return a valid string that fails decrypt. Wait, GetSSHKey uses crypto.Decrypt which fails if invalid.
	// But it returns the error. We can test that.
	rows := mock.NewRows([]string{"host_id", "private_key"}).
		AddRow(int32(1), "invalid-encrypted-key")

	mock.ExpectQuery(`SELECT host_id, private_key FROM ssh_keys WHERE host_id = \$1`).
		WithArgs(int32(1)).
		WillReturnRows(rows)

	_, err = db.GetSSHKey(context.Background(), mock, 1)
	if err == nil {
		t.Errorf("expected error decrypting invalid key")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestAddSSHKey(t *testing.T) {
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

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestSetSSHKeyAndUser(t *testing.T) {
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

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
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

	hooks, err := db.GetWebhooks(context.Background(), mock, "update_success")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(hooks) != 1 {
		t.Errorf("expected 1 webhook, got %d", len(hooks))
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}