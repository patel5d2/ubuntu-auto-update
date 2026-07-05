package main

// Schedules CRUD + the fleet overview endpoint. Both exist to feed the
// dashboard; neither touches SSH directly.

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/jackc/pgx/v5"
	log "github.com/sirupsen/logrus"

	"ubuntu-auto-update/backend/pkg/middleware"
	"ubuntu-auto-update/backend/pkg/playbooks"
	"ubuntu-auto-update/backend/pkg/scheduler"
)

func (app *Application) handleListSchedules(w http.ResponseWriter, r *http.Request) {
	scheds, err := scheduler.List(r.Context(), app.DB)
	if err != nil {
		log.Errorf("list schedules: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to list schedules")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(scheds)
}

func (app *Application) handleCreateSchedule(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)

	var req struct {
		Name            string    `json:"name"`
		HostIDs         []int32   `json:"host_ids"`
		IntervalMinutes int32     `json:"interval_minutes"`
		StartAt         time.Time `json:"start_at,omitempty"`
		PlaybookID      *int32    `json:"playbook_id,omitempty"` // nil ⇒ apt-update schedule
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	switch {
	case req.Name == "":
		writeJSONError(w, http.StatusBadRequest, "name is required")
		return
	case len(req.HostIDs) == 0:
		writeJSONError(w, http.StatusBadRequest, "host_ids must not be empty")
		return
	case req.IntervalMinutes < 5:
		writeJSONError(w, http.StatusBadRequest, "interval_minutes must be at least 5")
		return
	}

	// Validate the playbook exists up front so the schedule can't reference a
	// missing one (the FK would also reject it, but this gives a clean 404).
	if req.PlaybookID != nil {
		if _, err := playbooks.Get(r.Context(), app.DB, *req.PlaybookID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeJSONError(w, http.StatusNotFound, "Playbook not found")
				return
			}
			log.Errorf("create schedule playbook lookup: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "Failed to validate playbook")
			return
		}
	}

	createdBy := "unknown"
	if user := middleware.GetUserFromContext(r); user != nil {
		createdBy = user.Username
	}

	sched, err := scheduler.Create(r.Context(), app.DB, req.Name, req.HostIDs, req.IntervalMinutes, req.StartAt, createdBy, req.PlaybookID)
	if err != nil {
		log.Errorf("create schedule: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to create schedule")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(sched)
}

func (app *Application) handleUpdateSchedule(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)

	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 32)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid schedule ID")
		return
	}
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Enabled == nil {
		writeJSONError(w, http.StatusBadRequest, "Body must include enabled: true|false")
		return
	}

	sched, err := scheduler.SetEnabled(r.Context(), app.DB, int32(id), *req.Enabled)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSONError(w, http.StatusNotFound, "Schedule not found")
			return
		}
		log.Errorf("update schedule: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to update schedule")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sched)
}

func (app *Application) handleDeleteSchedule(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 32)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid schedule ID")
		return
	}
	rows, err := scheduler.Delete(r.Context(), app.DB, int32(id))
	if err != nil {
		log.Errorf("delete schedule: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to delete schedule")
		return
	}
	if rows == 0 {
		writeJSONError(w, http.StatusNotFound, "Schedule not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleOverview returns fleet-level stats for the dashboard landing page in
// one round trip per table.
func (app *Application) handleOverview(w http.ResponseWriter, r *http.Request) {
	var out struct {
		TotalHosts  int64 `json:"total_hosts"`
		OnlineHosts int64 `json:"online_hosts"` // last_seen within 24h
		ErrorHosts  int64 `json:"error_hosts"`
		RebootHosts int64 `json:"reboot_hosts"`
		Runs7d      int64 `json:"runs_7d"`
		Failed7d    int64 `json:"failed_7d"`
		RunningNow  int64 `json:"running_now"`
	}

	err := app.DB.QueryRow(r.Context(), `
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE last_seen > NOW() - INTERVAL '24 hours'),
		       COUNT(*) FILTER (WHERE error IS NOT NULL AND error <> ''),
		       COUNT(*) FILTER (WHERE reboot_required)
		FROM hosts`).Scan(&out.TotalHosts, &out.OnlineHosts, &out.ErrorHosts, &out.RebootHosts)
	if err != nil {
		log.Errorf("overview hosts: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to compute overview")
		return
	}

	err = app.DB.QueryRow(r.Context(), `
		SELECT COUNT(*) FILTER (WHERE started_at > NOW() - INTERVAL '7 days'),
		       COUNT(*) FILTER (WHERE started_at > NOW() - INTERVAL '7 days' AND status = 'failed'),
		       COUNT(*) FILTER (WHERE status = 'running')
		FROM update_runs`).Scan(&out.Runs7d, &out.Failed7d, &out.RunningNow)
	if err != nil {
		log.Errorf("overview runs: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to compute overview")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}
