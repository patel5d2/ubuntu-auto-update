package main

// Playbook CRUD + run handlers. Runs reuse the existing streaming engines
// (runHostCommandOpts for single host, the bulk Coordinator for fan-out); this
// file only loads/validates playbooks and wires them in.

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	log "github.com/sirupsen/logrus"

	"ubuntu-auto-update/backend/pkg/audit"
	"ubuntu-auto-update/backend/pkg/db"
	"ubuntu-auto-update/backend/pkg/middleware"
	"ubuntu-auto-update/backend/pkg/models"
	"ubuntu-auto-update/backend/pkg/playbooks"
	"ubuntu-auto-update/backend/pkg/updater"
)

const (
	maxPlaybookSteps    = 50
	maxPlaybookStepSize = 4096
)

// cleanSteps trims each step, drops blanks, and enforces the size caps. Returns
// an error string (empty when valid) so handlers can 400 with a clear message.
func cleanSteps(raw []string) ([]string, string) {
	out := make([]string, 0, len(raw))
	for _, s := range raw {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if len(s) > maxPlaybookStepSize {
			return nil, "each step must be at most 4096 characters"
		}
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil, "at least one non-empty step is required"
	}
	if len(out) > maxPlaybookSteps {
		return nil, "at most 50 steps per playbook"
	}
	return out, ""
}

func parsePlaybookID(r *http.Request) (int32, error) {
	id, err := strconv.Atoi(mux.Vars(r)["id"])
	return int32(id), err
}

// ---- CRUD ----

func (app *Application) handleListPlaybooks(w http.ResponseWriter, r *http.Request) {
	pbs, err := playbooks.List(r.Context(), app.DB)
	if err != nil {
		log.Errorf("list playbooks: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to list playbooks")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pbs)
}

func (app *Application) handleGetPlaybook(w http.ResponseWriter, r *http.Request) {
	id, err := parsePlaybookID(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid playbook ID")
		return
	}
	pb, err := playbooks.Get(r.Context(), app.DB, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSONError(w, http.StatusNotFound, "Playbook not found")
			return
		}
		log.Errorf("get playbook: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to get playbook")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pb)
}

type playbookRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Steps       []string `json:"steps"`
	UseSudo     *bool    `json:"use_sudo"` // pointer so omitted == default true
}

func (app *Application) handleCreatePlaybook(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	var req playbookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeJSONError(w, http.StatusBadRequest, "name is required")
		return
	}
	steps, msg := cleanSteps(req.Steps)
	if msg != "" {
		writeJSONError(w, http.StatusBadRequest, msg)
		return
	}
	useSudo := req.UseSudo == nil || *req.UseSudo

	createdBy := "unknown"
	if user := middleware.GetUserFromContext(r); user != nil {
		createdBy = user.Username
	}

	pb, err := playbooks.Create(r.Context(), app.DB, name, strings.TrimSpace(req.Description), steps, useSudo, createdBy)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			writeJSONError(w, http.StatusConflict, "A playbook with that name already exists")
			return
		}
		log.Errorf("create playbook: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to create playbook")
		return
	}
	app.audit(r, audit.ActionPlaybookCreate, "playbook", strconv.FormatInt(int64(pb.ID), 10),
		map[string]interface{}{"name": pb.Name, "step_count": len(pb.Steps)})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(pb)
}

func (app *Application) handleUpdatePlaybook(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	id, err := parsePlaybookID(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid playbook ID")
		return
	}
	var req playbookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeJSONError(w, http.StatusBadRequest, "name is required")
		return
	}
	steps, msg := cleanSteps(req.Steps)
	if msg != "" {
		writeJSONError(w, http.StatusBadRequest, msg)
		return
	}
	useSudo := req.UseSudo == nil || *req.UseSudo

	pb, err := playbooks.Update(r.Context(), app.DB, id, name, strings.TrimSpace(req.Description), steps, useSudo)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSONError(w, http.StatusNotFound, "Playbook not found")
			return
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			writeJSONError(w, http.StatusConflict, "A playbook with that name already exists")
			return
		}
		log.Errorf("update playbook: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to update playbook")
		return
	}
	app.audit(r, audit.ActionPlaybookUpdate, "playbook", strconv.FormatInt(int64(pb.ID), 10),
		map[string]interface{}{"name": pb.Name, "step_count": len(pb.Steps)})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pb)
}

func (app *Application) handleDeletePlaybook(w http.ResponseWriter, r *http.Request) {
	id, err := parsePlaybookID(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid playbook ID")
		return
	}
	// Friendly precheck: the FK is ON DELETE RESTRICT, but a bare FK violation
	// is an opaque 500. Tell the operator which schedules block the delete.
	var count int
	if err := app.DB.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM schedules WHERE playbook_id = $1`, id).Scan(&count); err != nil {
		log.Errorf("delete playbook precheck: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to delete playbook")
		return
	}
	if count > 0 {
		writeJSONError(w, http.StatusConflict,
			"playbook is used by "+strconv.Itoa(count)+" schedule(s); delete or change them first")
		return
	}

	rows, err := playbooks.Delete(r.Context(), app.DB, id)
	if err != nil {
		log.Errorf("delete playbook: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to delete playbook")
		return
	}
	if rows == 0 {
		writeJSONError(w, http.StatusNotFound, "Playbook not found")
		return
	}
	app.audit(r, audit.ActionPlaybookDelete, "playbook", strconv.FormatInt(int64(id), 10), nil)
	w.WriteHeader(http.StatusNoContent)
}

// ---- Runs ----

// handleRunPlaybook streams a playbook over SSH to one host, reusing the same
// engine as run-update. WS auth (?token=) is handled by the op subrouter.
func (app *Application) handleRunPlaybook(w http.ResponseWriter, r *http.Request) {
	id, err := parseHostID(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid host ID")
		return
	}
	pbID, err := strconv.Atoi(r.URL.Query().Get("playbook_id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "playbook_id query param is required")
		return
	}
	host, err := db.GetHost(r.Context(), app.DB, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSONError(w, http.StatusNotFound, "Host not found")
			return
		}
		log.Errorf("run playbook get host: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to retrieve host")
		return
	}
	pb, err := playbooks.Get(r.Context(), app.DB, int32(pbID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSONError(w, http.StatusNotFound, "Playbook not found")
			return
		}
		log.Errorf("run playbook get: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to retrieve playbook")
		return
	}
	// Audit before the WS upgrade (parity with handleExecuteScript).
	app.audit(r, audit.ActionRunPlaybook, "host", strconv.FormatInt(int64(id), 10),
		map[string]interface{}{"playbook_id": pb.ID, "playbook_name": pb.Name, "step_count": len(pb.Steps)})

	steps := playbooks.CompileSteps(pb.Steps, host.SshUser, pb.UseSudo)
	app.runHostCommandOpts(w, r, id, models.RunKindPlaybook, steps, &pb.ID)
}

// handleBulkRunPlaybook fans a playbook across many hosts via the bulk
// coordinator. Mirrors handleBulkRunUpdate's guards.
func (app *Application) handleBulkRunPlaybook(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	var req struct {
		HostIDs           []int32 `json:"host_ids"`
		PlaybookID        int32   `json:"playbook_id"`
		Concurrency       int     `json:"concurrency,omitempty"`
		CanaryCount       int     `json:"canary_count,omitempty"`
		CanaryWaitSeconds int     `json:"canary_wait_seconds,omitempty"`
		AbortOnFailurePct int     `json:"abort_on_failure_pct,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if len(req.HostIDs) == 0 {
		writeJSONError(w, http.StatusBadRequest, "host_ids must not be empty")
		return
	}
	if len(req.HostIDs) > 200 {
		writeJSONError(w, http.StatusBadRequest, "host_ids capped at 200 per request")
		return
	}
	if app.BulkUpdater.InFlightCount() >= 1 {
		writeJSONError(w, http.StatusConflict, "Another bulk run is already running. Try again when it finishes.")
		return
	}

	pb, err := playbooks.Get(r.Context(), app.DB, req.PlaybookID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSONError(w, http.StatusNotFound, "Playbook not found")
			return
		}
		log.Errorf("bulk playbook get: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to retrieve playbook")
		return
	}

	user := middleware.GetUserFromContext(r)
	triggeredBy := "unknown"
	if user != nil {
		triggeredBy = user.Username
	}

	pbID := pb.ID
	result, err := app.BulkUpdater.Start(r.Context(), updater.BulkRunOptions{
		HostIDs:           req.HostIDs,
		Concurrency:       req.Concurrency,
		TriggeredBy:       triggeredBy,
		CanaryCount:       req.CanaryCount,
		CanaryWaitSeconds: req.CanaryWaitSeconds,
		AbortOnFailurePct: req.AbortOnFailurePct,
		Kind:              models.RunKindPlaybook,
		Steps:             pb.Steps,
		UseSudo:           pb.UseSudo,
		PlaybookID:        &pbID,
	})
	if err != nil {
		log.Errorf("bulk playbook start failed: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to start bulk playbook: "+err.Error())
		return
	}

	log.Infof("Bulk playbook %q (%s) triggered by %s across %d hosts", pb.Name, result.GroupID, triggeredBy, len(req.HostIDs))
	app.audit(r, audit.ActionRunBulkPlaybook, "run_group", result.GroupID,
		map[string]interface{}{
			"playbook_id":          pb.ID,
			"host_count":           len(req.HostIDs),
			"canary_count":         req.CanaryCount,
			"canary_wait_seconds":  req.CanaryWaitSeconds,
			"abort_on_failure_pct": req.AbortOnFailurePct,
		})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(result)
}
