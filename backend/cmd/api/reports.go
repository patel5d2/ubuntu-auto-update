package main

// Compliance report: one row per host answering "is this machine patched?" —
// pending updates, reboot flag, last successful update, last attempt outcome.
// JSON for the dashboard, ?format=csv for auditors and spreadsheets.

import (
	"encoding/csv"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	log "github.com/sirupsen/logrus"
)

type complianceRow struct {
	HostID            int32      `json:"host_id" db:"host_id"`
	Hostname          string     `json:"hostname" db:"hostname"`
	Tags              []string   `json:"tags" db:"tags"`
	OsVersion         string     `json:"os_version" db:"os_version"`
	PackagesAvailable int        `json:"packages_available" db:"packages_available"`
	RebootRequired    bool       `json:"reboot_required" db:"reboot_required"`
	LastSeen          time.Time  `json:"last_seen" db:"last_seen"`
	OfflineSince      *time.Time `json:"offline_since" db:"offline_since"`
	LastSuccessAt     *time.Time `json:"last_success_at" db:"last_success_at"`
	LastAttemptAt     *time.Time `json:"last_attempt_at" db:"last_attempt_at"`
	LastAttemptStatus *string    `json:"last_attempt_status" db:"last_attempt_status"`
}

const complianceQuery = `
	SELECT h.id AS host_id, h.hostname, h.tags, h.os_version,
	       h.packages_available, h.reboot_required, h.last_seen, h.offline_since,
	       ok.finished_at   AS last_success_at,
	       att.finished_at  AS last_attempt_at,
	       att.status::text AS last_attempt_status
	FROM hosts h
	LEFT JOIN LATERAL (
		SELECT finished_at FROM update_runs
		WHERE host_id = h.id AND kind = 'update' AND status = 'succeeded'
		ORDER BY started_at DESC LIMIT 1
	) ok ON true
	LEFT JOIN LATERAL (
		SELECT finished_at, status FROM update_runs
		WHERE host_id = h.id AND kind = 'update' AND status <> 'running'
		ORDER BY started_at DESC LIMIT 1
	) att ON true
	ORDER BY h.hostname`

func (app *Application) handleComplianceReport(w http.ResponseWriter, r *http.Request) {
	rows, err := app.DB.Query(r.Context(), complianceQuery)
	if err != nil {
		log.Errorf("compliance report: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to build report")
		return
	}
	report, err := pgx.CollectRows(rows, pgx.RowToStructByName[complianceRow])
	if err != nil {
		log.Errorf("compliance report collect: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to build report")
		return
	}
	if report == nil {
		report = []complianceRow{}
	}

	if r.URL.Query().Get("format") != "csv" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(report)
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition",
		`attachment; filename="compliance-`+time.Now().UTC().Format("2006-01-02")+`.csv"`)
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"hostname", "tags", "os_version", "pending_updates",
		"reboot_required", "online", "last_seen", "last_successful_update",
		"last_attempt", "last_attempt_status"})
	fmtTime := func(t *time.Time) string {
		if t == nil {
			return ""
		}
		return t.UTC().Format(time.RFC3339)
	}
	// Hostnames/tags/OS are agent-supplied; a value like =HYPERLINK(...)
	// executes as a formula when the CSV opens in Excel/Sheets. Neutralize.
	safe := func(v string) string {
		if v != "" && strings.ContainsRune("=+-@", rune(v[0])) {
			return "'" + v
		}
		return v
	}
	for _, row := range report {
		status := ""
		if row.LastAttemptStatus != nil {
			status = *row.LastAttemptStatus
		}
		_ = cw.Write([]string{
			safe(row.Hostname),
			safe(strings.Join(row.Tags, " ")),
			safe(row.OsVersion),
			strconv.Itoa(row.PackagesAvailable),
			strconv.FormatBool(row.RebootRequired),
			strconv.FormatBool(row.OfflineSince == nil),
			row.LastSeen.UTC().Format(time.RFC3339),
			fmtTime(row.LastSuccessAt),
			fmtTime(row.LastAttemptAt),
			status,
		})
	}
	cw.Flush()
}
