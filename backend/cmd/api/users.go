package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"

	"ubuntu-auto-update/backend/pkg/audit"
	"ubuntu-auto-update/backend/pkg/middleware"
	"ubuntu-auto-update/backend/pkg/users"
)

// User-management endpoints. All require an authenticated admin (gated by
// middleware.RequireRole(session.RoleAdmin) in main.go's route registration).
// The audit log records every mutation so destructive ops are reversible
// from history.

func (app *Application) handleListUsers(w http.ResponseWriter, r *http.Request) {
	if app.DB == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "Database not configured")
		return
	}
	list, err := users.List(r.Context(), app.DB)
	if err != nil {
		log.Errorf("list users: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to list users")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

func (app *Application) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.Username == "" || req.Password == "" {
		writeJSONError(w, http.StatusBadRequest, "username and password are required")
		return
	}
	if req.Role == "" {
		req.Role = "viewer"
	}
	if !users.IsValidRole(req.Role) {
		writeJSONError(w, http.StatusBadRequest, "role must be viewer, operator, or admin")
		return
	}

	u, err := users.Create(r.Context(), app.DB, req.Username, req.Password, req.Role)
	if err != nil {
		switch {
		case errors.Is(err, users.ErrDuplicateUsername):
			writeJSONError(w, http.StatusConflict, "Username already exists")
		case errors.Is(err, users.ErrPasswordTooShort):
			writeJSONError(w, http.StatusBadRequest, "Password must be at least 12 characters")
		case errors.Is(err, users.ErrInvalidRole):
			writeJSONError(w, http.StatusBadRequest, "Invalid role")
		default:
			log.Errorf("create user: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "Failed to create user")
		}
		return
	}

	app.audit(r, audit.ActionUserCreate, "user", strconv.FormatInt(int64(u.ID), 10),
		map[string]interface{}{"username": u.Username, "role": u.Role})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(u)
}

func (app *Application) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	id, err := parseUserID(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid user id")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)

	var req struct {
		Role     *string `json:"role,omitempty"`
		Disabled *bool   `json:"disabled,omitempty"`
		Password *string `json:"password,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Role != nil {
		if err := users.SetRole(r.Context(), app.DB, id, *req.Role); err != nil {
			respondUserUpdateError(w, err)
			return
		}
		app.audit(r, audit.ActionUserUpdate, "user", strconv.FormatInt(int64(id), 10),
			map[string]interface{}{"role": *req.Role})
	}
	if req.Disabled != nil {
		if err := users.SetDisabled(r.Context(), app.DB, id, *req.Disabled); err != nil {
			respondUserUpdateError(w, err)
			return
		}
		action := audit.ActionUserEnable
		if *req.Disabled {
			action = audit.ActionUserDisable
		}
		app.audit(r, action, "user", strconv.FormatInt(int64(id), 10), nil)
	}
	if req.Password != nil {
		if err := users.SetPassword(r.Context(), app.DB, id, *req.Password); err != nil {
			respondUserUpdateError(w, err)
			return
		}
		app.audit(r, audit.ActionUserPassword, "user", strconv.FormatInt(int64(id), 10), nil)
	}

	w.WriteHeader(http.StatusNoContent)
}

func (app *Application) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := parseUserID(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid user id")
		return
	}

	// Prevent admins from deleting themselves out of the system.
	p := middleware.GetPrincipalFromContext(r)
	if p != nil && p.UserID == id {
		writeJSONError(w, http.StatusBadRequest, "Cannot delete your own account")
		return
	}

	if err := users.Delete(r.Context(), app.DB, id); err != nil {
		respondUserUpdateError(w, err)
		return
	}
	app.audit(r, audit.ActionUserDelete, "user", strconv.FormatInt(int64(id), 10), nil)
	w.WriteHeader(http.StatusNoContent)
}

func respondUserUpdateError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, users.ErrUserNotFound):
		writeJSONError(w, http.StatusNotFound, "User not found")
	case errors.Is(err, users.ErrInvalidRole):
		writeJSONError(w, http.StatusBadRequest, "Invalid role")
	case errors.Is(err, users.ErrPasswordTooShort):
		writeJSONError(w, http.StatusBadRequest, "Password must be at least 12 characters")
	default:
		log.Errorf("user update: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to update user")
	}
}

func parseUserID(r *http.Request) (int32, error) {
	idStr, ok := mux.Vars(r)["id"]
	if !ok {
		return 0, errors.New("missing user id")
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return 0, err
	}
	return int32(id), nil
}

// audit is a thin Application helper that captures principal + request
// context so handlers don't have to reassemble it. Best effort: a failed
// audit write logs but does not fail the original operation.
func (app *Application) audit(r *http.Request, action, targetType, targetID string, details map[string]interface{}) {
	p := middleware.GetPrincipalFromContext(r)
	ev := audit.Event{
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		Details:    details,
		IP:         middleware.ClientIP(r),
		UserAgent:  r.UserAgent(),
	}
	if p != nil {
		if p.UserID != 0 {
			id := p.UserID
			ev.ActorUserID = &id
		}
		ev.ActorLabel = p.Username
	}
	if app.DB == nil {
		// Non-fatal — likely a unit test path.
		return
	}
	if err := audit.Log(r.Context(), app.DB, ev); err != nil {
		log.Errorf("audit log: %v", err)
	}
}

// handleListAudit returns the most recent audit records, newest first.
// Filters: ?action= and ?target_type=&target_id=. Limit defaults to 100,
// hard cap 1000 (enforced inside audit.List).
func (app *Application) handleListAudit(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := 0
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	out, err := audit.List(r.Context(), app.DB, audit.ListOptions{
		Limit:      limit,
		Action:     q.Get("action"),
		TargetType: q.Get("target_type"),
		TargetID:   q.Get("target_id"),
	})
	if err != nil {
		log.Errorf("audit list: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to read audit log")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}
