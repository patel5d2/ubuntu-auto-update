package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	log "github.com/sirupsen/logrus"

	"ubuntu-auto-update/backend/pkg/audit"
	"ubuntu-auto-update/backend/pkg/db"
	sshpkg "ubuntu-auto-update/backend/pkg/ssh"
)

// handleBulkEnroll lets an operator enroll many hosts in one request.
//
// Request body:
//   { "hosts": [ {"hostname": "...", "ssh_user": "...", "password": "..."},
//                 ... ],
//     "concurrency": 4,           // optional, default 4, max 8
//     "sudo_scope":  "apt|full" } // optional, default apt
//
// Each host runs the same one-shot bootstrap flow handleCreateHost uses
// (password SSH → install key → configure sudo → store encrypted key). Per-host
// failures don't abort the rest of the batch — the response contains a
// per-host status so the operator can fix and retry the failures only.
func (app *Application) handleBulkEnroll(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)

	var req struct {
		Hosts []struct {
			Hostname string `json:"hostname"`
			SshUser  string `json:"ssh_user"`
			Password string `json:"password"`
		} `json:"hosts"`
		Concurrency int    `json:"concurrency"`
		SudoScope   string `json:"sudo_scope"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if len(req.Hosts) == 0 {
		writeJSONError(w, http.StatusBadRequest, "hosts must not be empty")
		return
	}
	if len(req.Hosts) > 100 {
		writeJSONError(w, http.StatusBadRequest, "hosts capped at 100 per request")
		return
	}
	conc := req.Concurrency
	if conc <= 0 {
		conc = 4
	}
	if conc > 8 {
		conc = 8
	}
	scope := req.SudoScope
	if scope != "full" {
		scope = "apt"
	}

	// We don't tie work to r.Context(): the request returns once we have
	// per-host results, but we want each host to be able to run a few seconds
	// even if an operator clicks away.
	type result struct {
		Hostname       string `json:"hostname"`
		OK             bool   `json:"ok"`
		HostID         int32  `json:"host_id,omitempty"`
		Error          string `json:"error,omitempty"`
		SudoConfigured bool   `json:"sudo_configured,omitempty"`
		Fingerprint    string `json:"fingerprint,omitempty"`
	}
	results := make([]result, len(req.Hosts))

	sem := make(chan struct{}, conc)
	var wg sync.WaitGroup
	for i := range req.Hosts {
		i := i
		h := req.Hosts[i]
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			res := result{Hostname: h.Hostname}

			hostname := strings.TrimSpace(h.Hostname)
			sshUser := strings.TrimSpace(h.SshUser)
			if sshUser == "" {
				sshUser = "root"
			}
			if hostname == "" || h.Password == "" {
				res.Error = "hostname and password are required"
				results[i] = res
				return
			}

			// Per-host budget: 90 s for the SSH dance, generous and bounded.
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()

			created, err := db.CreateHost(ctx, app.DB, hostname, sshUser)
			if err != nil {
				if errors.Is(err, db.ErrDuplicateHostname) {
					res.Error = "hostname already exists"
				} else {
					res.Error = "create host: " + err.Error()
				}
				results[i] = res
				return
			}

			boot, err := app.SSHDialer.BootstrapOpts(ctx, hostname, sshUser, h.Password,
				sshpkg.BootstrapOptions{SudoScope: scope})
			if err != nil {
				_, _ = db.DeleteHost(ctx, app.DB, created.ID)
				res.Error = err.Error()
				results[i] = res
				return
			}

			if err := db.AddSSHKey(ctx, app.DB, created.ID, boot.PrivateKeyPEM); err != nil {
				_, _ = db.DeleteHost(ctx, app.DB, created.ID)
				res.Error = "store key: " + err.Error()
				results[i] = res
				return
			}
			if err := app.SSHDialer.AppendKnownHost(hostname, boot.HostKey); err != nil {
				log.Errorf("bulk-enroll: append host key for %s: %v", hostname, err)
			}

			res.OK = true
			res.HostID = created.ID
			res.SudoConfigured = boot.SudoConfigured
			res.Fingerprint = boot.HostKeyFingerprint
			results[i] = res

			app.audit(r, audit.ActionHostBootstrap, "host", strconv.FormatInt(int64(created.ID), 10),
				map[string]interface{}{
					"hostname":    hostname,
					"ssh_user":    sshUser,
					"sudo_scope":  scope,
					"fingerprint": boot.HostKeyFingerprint,
					"source":      "bulk_enroll",
				})
		}()
	}
	wg.Wait()

	// Quick aggregate so the UI can summarize at a glance.
	var success, failure int
	for _, r := range results {
		if r.OK {
			success++
		} else {
			failure++
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if failure == 0 {
		w.WriteHeader(http.StatusCreated)
	} else if success == 0 {
		w.WriteHeader(http.StatusBadGateway)
	} else {
		w.WriteHeader(http.StatusMultiStatus)
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"results":         results,
		"success_count":   success,
		"failure_count":   failure,
	})
}

// handleRotateKey generates a fresh keypair on the host using the existing
// key, stores the new private key encrypted, and revokes the old key from
// authorized_keys. Idempotent: re-running just installs a new key.
func (app *Application) handleRotateKey(w http.ResponseWriter, r *http.Request) {
	id, err := parseHostID(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid host ID")
		return
	}

	host, err := db.GetHost(r.Context(), app.DB, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSONError(w, http.StatusNotFound, "Host not found")
			return
		}
		log.Errorf("rotate-key: get host %d: %v", id, err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to read host")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	rotated, rotErr := app.SSHDialer.RotateKey(ctx, id)
	// RotateKey returns a partial result + error if the new key works but old
	// key revocation failed; persist the new key in that case so future dials
	// use it. The error is returned to the operator so they can investigate.
	if rotErr != nil && rotated.PrivateKeyPEM == "" {
		log.Warnf("rotate-key for %s (id=%d): %v", host.Hostname, id, rotErr)
		writeJSONError(w, http.StatusBadGateway, "Key rotation failed: "+rotErr.Error())
		return
	}

	if err := db.AddSSHKey(ctx, app.DB, id, rotated.PrivateKeyPEM); err != nil {
		log.Errorf("rotate-key: store key for host %d: %v", id, err)
		writeJSONError(w, http.StatusInternalServerError, "Rotation succeeded but persisting key failed; please retry")
		return
	}

	app.audit(r, audit.ActionHostKeyRotate, "host", strconv.FormatInt(int64(id), 10),
		map[string]interface{}{"hostname": host.Hostname})

	w.Header().Set("Content-Type", "application/json")
	resp := map[string]interface{}{"ok": true}
	if rotErr != nil {
		resp["warning"] = rotErr.Error()
	}
	json.NewEncoder(w).Encode(resp)
}
