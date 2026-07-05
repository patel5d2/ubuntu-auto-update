package playbooks_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v4"

	"ubuntu-auto-update/backend/pkg/playbooks"
)

func cols(mock pgxmock.PgxPoolIface) *pgxmock.Rows {
	return mock.NewRows([]string{"id", "name", "description", "steps", "use_sudo", "created_by", "created_at", "updated_at"})
}

func TestCompileSteps(t *testing.T) {
	cases := []struct {
		name    string
		steps   []string
		sshUser string
		useSudo bool
		want    []string
	}{
		{"root passthrough", []string{"apt-get update"}, "root", true, []string{"apt-get update"}},
		{"useSudo false passthrough", []string{"whoami"}, "nginx", false, []string{"whoami"}},
		{"empty user passthrough", []string{"whoami"}, "", true, []string{"whoami"}},
		{
			"non-root wraps",
			[]string{"apt-get update"},
			"nginx", true,
			[]string{"sudo -n sh -c 'apt-get update'"},
		},
		{
			"compound step escalates whole",
			[]string{"cd /x && echo hi"},
			"nginx", true,
			[]string{"sudo -n sh -c 'cd /x && echo hi'"},
		},
		{
			"single quotes escaped",
			[]string{`echo 'hi there'`},
			"nginx", true,
			[]string{`sudo -n sh -c 'echo '\''hi there'\'''`},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := playbooks.CompileSteps(c.steps, c.sshUser, c.useSudo)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("CompileSteps(%q,%q,%v) = %q, want %q", c.steps, c.sshUser, c.useSudo, got, c.want)
			}
		})
	}
}

// CompileSteps must not alias/mutate the caller's slice.
func TestCompileStepsCopiesPassthrough(t *testing.T) {
	orig := []string{"a", "b"}
	out := playbooks.CompileSteps(orig, "root", true)
	out[0] = "mutated"
	if orig[0] != "a" {
		t.Fatalf("CompileSteps aliased the input slice")
	}
}

func TestCreateAndGet(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer mock.Close()
	now := time.Now()

	mock.ExpectQuery(`INSERT INTO playbooks`).
		WithArgs("harden", "desc", []string{"echo 1"}, true, "admin").
		WillReturnRows(cols(mock).AddRow(int32(1), "harden", "desc", []string{"echo 1"}, true, "admin", now, now))

	pb, err := playbooks.Create(context.Background(), mock, "harden", "desc", []string{"echo 1"}, true, "admin")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if pb.Name != "harden" || len(pb.Steps) != 1 || !pb.UseSudo {
		t.Errorf("unexpected playbook: %+v", pb)
	}

	mock.ExpectQuery(`SELECT (.+) FROM playbooks WHERE id = \$1`).
		WithArgs(int32(1)).
		WillReturnRows(cols(mock).AddRow(int32(1), "harden", "desc", []string{"echo 1"}, true, "admin", now, now))
	got, err := playbooks.Get(context.Background(), mock, 1)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != 1 {
		t.Errorf("Get ID = %d", got.ID)
	}
}

func TestListAndUpdateAndDelete(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer mock.Close()
	now := time.Now()

	mock.ExpectQuery(`SELECT (.+) FROM playbooks ORDER BY name`).
		WillReturnRows(cols(mock).AddRow(int32(1), "a", "", []string{"x"}, true, "admin", now, now))
	list, err := playbooks.List(context.Background(), mock)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v (n=%d)", err, len(list))
	}

	mock.ExpectQuery(`UPDATE playbooks`).
		WithArgs(int32(1), "a2", "d2", []string{"y"}, false).
		WillReturnRows(cols(mock).AddRow(int32(1), "a2", "d2", []string{"y"}, false, "admin", now, now))
	up, err := playbooks.Update(context.Background(), mock, 1, "a2", "d2", []string{"y"}, false)
	if err != nil || up.Name != "a2" || up.UseSudo {
		t.Fatalf("update: %v %+v", err, up)
	}

	mock.ExpectExec(`DELETE FROM playbooks WHERE id = \$1`).
		WithArgs(int32(1)).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))
	n, err := playbooks.Delete(context.Background(), mock, 1)
	if err != nil || n != 1 {
		t.Fatalf("delete: %v n=%d", err, n)
	}
}
