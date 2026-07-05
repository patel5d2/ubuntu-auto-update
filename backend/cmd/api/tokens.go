package main

// API-token (PAT) management, admin-only. The raw token appears once in the
// create response and never again; the DB stores only its SHA-256.

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"

	"ubuntu-auto-update/backend/pkg/apitokens"
	"ubuntu-auto-update/backend/pkg/audit"
	"ubuntu-auto-update/backend/pkg/middleware"
)

func (app *Application) handleListAPITokens(w http.ResponseWriter, r *http.Request) {
	toks, err := apitokens.List(r.Context(), app.DB)
	if err != nil {
		log.Errorf("list api tokens: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to list tokens")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(toks)
}

func (app *Application) handleCreateAPIToken(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	var req struct {
		Name string `json:"name"`
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeJSONError(w, http.StatusBadRequest, "name is required")
		return
	}
	switch req.Role {
	case "viewer", "operator", "admin":
	default:
		writeJSONError(w, http.StatusBadRequest, "role must be viewer, operator, or admin")
		return
	}

	createdBy := "unknown"
	if user := middleware.GetUserFromContext(r); user != nil {
		createdBy = user.Username
	}

	tok, raw, err := apitokens.Create(r.Context(), app.DB, req.Name, req.Role, createdBy)
	if err != nil {
		log.Errorf("create api token: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to create token")
		return
	}
	app.audit(r, audit.ActionTokenCreate, "api_token", strconv.FormatInt(int64(tok.ID), 10),
		map[string]interface{}{"name": tok.Name, "role": tok.Role})

	// The raw secret rides along exactly once — the standard PAT pattern;
	// only the SHA-256 is stored.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(struct { // #nosec G117 -- intentional one-time secret disclosure at mint
		apitokens.Token
		Secret string `json:"secret"`
	}{tok, raw})
}

func (app *Application) handleDeleteAPIToken(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 32)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid token ID")
		return
	}
	rows, err := apitokens.Delete(r.Context(), app.DB, int32(id))
	if err != nil {
		log.Errorf("delete api token: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to delete token")
		return
	}
	if rows == 0 {
		writeJSONError(w, http.StatusNotFound, "Token not found")
		return
	}
	app.audit(r, audit.ActionTokenDelete, "api_token", strconv.FormatInt(id, 10), nil)
	w.WriteHeader(http.StatusNoContent)
}
