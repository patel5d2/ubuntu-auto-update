package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestComplianceReportJSONAndCSV(t *testing.T) {
	app, mock := testAppWithDB(t)
	defer mock.Close()

	now := time.Now()
	ok := now.Add(-time.Hour)
	status := "succeeded"
	for _, format := range []string{"", "csv"} {
		mock.ExpectQuery(`SELECT h.id AS host_id`).
			WillReturnRows(mock.NewRows([]string{"host_id", "hostname", "tags", "os_version",
				"packages_available", "reboot_required", "last_seen", "offline_since",
				"last_success_at", "last_attempt_at", "last_attempt_status"}).
				AddRow(int32(1), "web-1", []string{"prod"}, "Ubuntu 24.04", 3, true, now, nil, &ok, &ok, &status))

		url := "/api/v1/reports/compliance"
		if format != "" {
			url += "?format=" + format
		}
		req := httptest.NewRequest(http.MethodGet, url, nil)
		rr := httptest.NewRecorder()
		app.handleComplianceReport(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("format=%q: status %d: %s", format, rr.Code, rr.Body.String())
		}
		body := rr.Body.String()
		if format == "csv" {
			if ct := rr.Header().Get("Content-Type"); ct != "text/csv" {
				t.Errorf("csv content type = %q", ct)
			}
			if !strings.Contains(body, "web-1") || !strings.Contains(body, "hostname,tags") {
				t.Errorf("csv body missing expected content:\n%s", body)
			}
		} else {
			if !strings.Contains(body, `"hostname":"web-1"`) || !strings.Contains(body, `"packages_available":3`) {
				t.Errorf("json body missing expected content:\n%s", body)
			}
		}
	}
}
