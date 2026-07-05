package main

// Reboot orchestration: issue a reboot over SSH and verify the host comes
// back (boot_id change, with a went-down-and-returned fallback). One engine
// for any host count — a single-host reboot is a bulk run of one, so it
// shares the coordinator's concurrency, run history, and webhook dispatch.

import (
	"encoding/json"
	"net/http"

	log "github.com/sirupsen/logrus"

	"ubuntu-auto-update/backend/pkg/audit"
	"ubuntu-auto-update/backend/pkg/middleware"
	"ubuntu-auto-update/backend/pkg/models"
	"ubuntu-auto-update/backend/pkg/updater"
)

func (app *Application) handleBulkReboot(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	var req struct {
		HostIDs     []int32 `json:"host_ids"`
		Concurrency int     `json:"concurrency,omitempty"`
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

	triggeredBy := "unknown"
	if user := middleware.GetUserFromContext(r); user != nil {
		triggeredBy = user.Username
	}

	result, err := app.BulkUpdater.Start(r.Context(), updater.BulkRunOptions{
		HostIDs:     req.HostIDs,
		Concurrency: req.Concurrency,
		TriggeredBy: triggeredBy,
		Kind:        models.RunKindReboot,
		Reboot:      true,
	})
	if err != nil {
		log.Errorf("bulk reboot start failed: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to start reboot: "+err.Error())
		return
	}

	log.Infof("Reboot (%s) triggered by %s across %d hosts", result.GroupID, triggeredBy, len(req.HostIDs))
	app.audit(r, audit.ActionRunBulkReboot, "run_group", result.GroupID,
		map[string]interface{}{"host_count": len(req.HostIDs)})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(result)
}
