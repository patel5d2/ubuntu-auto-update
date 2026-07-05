// Package updater runs apt-get update/upgrade against managed hosts. The
// single-host path lives in cmd/api/main.go for now; this package focuses on
// the bulk fan-out so multi-host orchestration can evolve independently of
// the WebSocket-bound single-host streamer.
package updater

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	log "github.com/sirupsen/logrus"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/sync/semaphore"

	"ubuntu-auto-update/backend/pkg/db"
	"ubuntu-auto-update/backend/pkg/models"
	"ubuntu-auto-update/backend/pkg/playbooks"
	sshpkg "ubuntu-auto-update/backend/pkg/ssh"
)

// DefaultConcurrency caps in-flight SSH sessions for a bulk run. Pulled from
// the planning doc — 5 is enough to be obviously parallel without saturating
// the SSH agent or the DB pool. Operators can override per request up to
// MaxConcurrency.
const (
	DefaultConcurrency = 5
	MaxConcurrency     = 20
)

// BulkRunOptions controls a fan-out update.
//
// Concurrency <= 0 means default; values above MaxConcurrency are clamped.
//
// Staged-rollout knobs:
//   - CanaryCount = 0 disables canarying; the whole fleet runs immediately
//     under the regular concurrency limit.
//   - CanaryCount > 0 reserves the first N hosts for an initial wave. After
//     the wave finishes, the coordinator sleeps CanaryWaitSeconds, then
//     either continues with the rest of the fleet (if no canary failed) or
//     aborts the remainder.
//   - AbortOnFailurePct: if a non-zero fraction of *completed* hosts has
//     failed, mark every remaining host failed without dialing it. 0 disables.
type BulkRunOptions struct {
	HostIDs           []int32
	Concurrency       int
	TriggeredBy       string
	CanaryCount       int
	CanaryWaitSeconds int
	AbortOnFailurePct int

	// Playbook fan-out. Zero values keep the apt-update path byte-identical:
	//   - Kind == "" is treated as RunKindUpdate.
	//   - Steps == nil/empty runs the single buildUpdateScript command.
	// Steps are RAW (uncompiled): the sudo prefix depends on each host's
	// ssh_user, known only inside runOne after ConnectToHost.
	Kind       models.RunKind
	Steps      []string
	UseSudo    bool
	PlaybookID *int32
}

// BulkResult is what we hand back to the API caller. RunIDs is parallel to
// HostIDs so the UI can subscribe to each per-host stream.
type BulkResult struct {
	GroupID string  `json:"group_id"`
	RunIDs  []int32 `json:"run_ids"`
	HostIDs []int32 `json:"host_ids"`
}

// Coordinator owns the dependencies the fan-out needs. Built once at app
// boot; safe for concurrent use.
type Coordinator struct {
	Pool   *pgxpool.Pool
	Dialer *sshpkg.Dialer
	// inFlightGroups remembers which UUIDs are currently active so the API
	// layer can rate-limit "one group per user" without a DB round trip.
	mu             sync.Mutex
	inFlightGroups map[string]struct{}
}

func New(pool *pgxpool.Pool, dialer *sshpkg.Dialer) *Coordinator {
	return &Coordinator{
		Pool:           pool,
		Dialer:         dialer,
		inFlightGroups: make(map[string]struct{}),
	}
}

// InFlightCount returns how many bulk groups are currently running. The HTTP
// layer uses this to enforce a per-user cap.
func (c *Coordinator) InFlightCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.inFlightGroups)
}

// Start kicks off a bulk run. It pre-creates one update_runs row per host so
// the UI sees them immediately, then returns. Actual SSH work happens on a
// detached background goroutine.
func (c *Coordinator) Start(ctx context.Context, opts BulkRunOptions) (BulkResult, error) {
	if len(opts.HostIDs) == 0 {
		return BulkResult{}, fmt.Errorf("no hosts selected")
	}
	conc := opts.Concurrency
	if conc <= 0 {
		conc = DefaultConcurrency
	}
	if conc > MaxConcurrency {
		conc = MaxConcurrency
	}

	groupID, err := newUUID()
	if err != nil {
		return BulkResult{}, fmt.Errorf("generate group id: %w", err)
	}

	// Pre-create runs so the UI can render the bulk view straight away.
	// Failure here is bad enough to bail entirely; we don't want a partial
	// fan-out where some hosts have rows and others don't.
	kind := opts.Kind
	if kind == "" {
		kind = models.RunKindUpdate
	}
	runIDs := make([]int32, len(opts.HostIDs))
	for i, hid := range opts.HostIDs {
		run, err := db.CreateRunFull(ctx, c.Pool, hid, opts.TriggeredBy, kind, groupID, opts.PlaybookID)
		if err != nil {
			return BulkResult{}, fmt.Errorf("create run for host %d: %w", hid, err)
		}
		runIDs[i] = run.ID
	}

	c.mu.Lock()
	c.inFlightGroups[groupID] = struct{}{}
	c.mu.Unlock()

	// Detach: the HTTP request is done as far as the caller is concerned.
	go c.run(opts, groupID, runIDs, conc)

	return BulkResult{GroupID: groupID, RunIDs: runIDs, HostIDs: opts.HostIDs}, nil
}

// run is the long-lived goroutine that actually performs the fan-out. It
// uses a weighted semaphore to keep concurrent SSH sessions bounded. When
// CanaryCount > 0 the host list is split into two waves with a configurable
// pause between them; an abort threshold can short-circuit the rest of the
// fleet.
func (c *Coordinator) run(opts BulkRunOptions, groupID string, runIDs []int32, conc int) {
	defer func() {
		c.mu.Lock()
		delete(c.inFlightGroups, groupID)
		c.mu.Unlock()
	}()

	// Fresh ctx — work isn't tied to the originating HTTP request, which has
	// long since returned. 30 min is generous for slow hosts.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	canary := opts.CanaryCount
	if canary < 0 {
		canary = 0
	}
	if canary > len(opts.HostIDs) {
		canary = len(opts.HostIDs)
	}

	// Wave 1: canary (or everything when CanaryCount == 0).
	end := canary
	if end == 0 {
		end = len(opts.HostIDs)
	}
	canaryFailures := c.runWave(ctx, opts, opts.HostIDs[:end], runIDs[:end], conc)

	// Stop early if the canary tripped the abort threshold.
	if canary > 0 {
		failPct := percent(canaryFailures, end)
		if shouldAbort(opts.AbortOnFailurePct, failPct) {
			log.Warnf("bulk %s: canary failed (%d/%d, %d%%) — aborting remainder",
				groupID, canaryFailures, end, failPct)
			c.skipRemaining(opts.HostIDs[end:], runIDs[end:],
				fmt.Sprintf("canary failure rate %d%% exceeded threshold %d%%", failPct, opts.AbortOnFailurePct))
			return
		}

		// Wait between waves. clamp to a reasonable ceiling so a typo can't
		// pin a goroutine for hours.
		if opts.CanaryWaitSeconds > 0 && end < len(opts.HostIDs) {
			wait := time.Duration(opts.CanaryWaitSeconds) * time.Second
			if wait > 10*time.Minute {
				wait = 10 * time.Minute
			}
			log.Infof("bulk %s: canary OK, sleeping %s before remainder", groupID, wait)
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
		}
	}

	// Wave 2: the rest, only when canary > 0 and there's something left.
	if canary > 0 && end < len(opts.HostIDs) {
		_ = c.runWave(ctx, opts, opts.HostIDs[end:], runIDs[end:], conc)
	}
}

// runWave executes one slice of hosts under the concurrency cap and returns
// the number of failed runs. The cap is applied within the wave so each wave
// is independently bounded.
func (c *Coordinator) runWave(ctx context.Context, opts BulkRunOptions, hostIDs, runIDs []int32, conc int) int {
	sem := semaphore.NewWeighted(int64(conc))
	var wg sync.WaitGroup
	var failures int64
	var mu sync.Mutex

	for i, hostID := range hostIDs {
		hostID := hostID
		runID := runIDs[i]

		if err := sem.Acquire(ctx, 1); err != nil {
			c.markFailed(runID, "bulk cancelled before start: "+err.Error())
			mu.Lock()
			failures++
			mu.Unlock()
			continue
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			defer sem.Release(1)
			if !c.runOne(ctx, opts, hostID, runID) {
				mu.Lock()
				failures++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return int(failures)
}

// skipRemaining marks every still-pending host failed without dialing it.
// Used when the canary trips the abort threshold.
func (c *Coordinator) skipRemaining(hostIDs, runIDs []int32, reason string) {
	for i := range hostIDs {
		c.markFailed(runIDs[i], "skipped: "+reason)
	}
}

func percent(part, total int) int {
	if total == 0 {
		return 0
	}
	return (part * 100) / total
}

func shouldAbort(thresholdPct, observedPct int) bool {
	if thresholdPct <= 0 || thresholdPct > 100 {
		return false
	}
	return observedPct >= thresholdPct
}

// runOne performs a single host's run. For the update path (opts.Steps empty)
// it runs the one buildUpdateScript command — byte-identical to before. For a
// playbook it compiles the raw steps for this host's ssh_user and runs them
// one SSH session per step, stopping at the first failure (mirrors
// runHostCommand). Output is captured to the pre-existing update_runs row.
// Returns true on success.
func (c *Coordinator) runOne(ctx context.Context, opts BulkRunOptions, hostID, runID int32) bool {
	finishStatus := models.RunStatusFailed
	finishExit := -1
	finishErr := ""

	defer func() {
		// Use a detached ctx so we still record terminal status even if the
		// parent ctx is cancelled.
		dbCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := db.FinishRun(dbCtx, c.Pool, runID, finishStatus, finishExit, finishErr); err != nil {
			log.Errorf("bulk: finish run %d: %v", runID, err)
		}
	}()

	client, host, err := c.Dialer.ConnectToHost(ctx, hostID)
	if err != nil {
		finishErr = "ssh connect: " + err.Error()
		_, _ = db.AppendRunOutput(ctx, c.Pool, runID, finishErr+"\n")
		return false
	}
	defer client.Close()

	cmds := []string{buildUpdateScript(host.SshUser)}
	if len(opts.Steps) > 0 {
		cmds = playbooks.CompileSteps(opts.Steps, host.SshUser, opts.UseSudo)
	}

	for _, cmd := range cmds {
		exit, cmdErr := c.runOneCommand(ctx, client, runID, cmd)
		if cmdErr != nil {
			finishExit = exit
			finishErr = cmdErr.Error()
			return false // stop-on-failure
		}
	}

	finishStatus = models.RunStatusSucceeded
	finishExit = 0
	return true
}

// runOneCommand runs a single shell line on an existing SSH client, tees its
// output to the run row, and returns the remote exit code (-1 on SSH-layer
// failure). Extracted from runOne so a playbook can loop it per step.
func (c *Coordinator) runOneCommand(ctx context.Context, client *gossh.Client, runID int32, cmd string) (int, error) {
	session, err := client.NewSession()
	if err != nil {
		return -1, fmt.Errorf("ssh session: %w", err)
	}
	defer session.Close()

	stdout, err := session.StdoutPipe()
	if err != nil {
		return -1, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		return -1, fmt.Errorf("stderr pipe: %w", err)
	}

	_, _ = db.AppendRunOutput(ctx, c.Pool, runID, "$ "+cmd+"\n")
	if err := session.Start(cmd); err != nil {
		return -1, fmt.Errorf("start command: %w", err)
	}

	var pumpWG sync.WaitGroup
	pumpWG.Add(2)
	go func() { defer pumpWG.Done(); pumpToRun(c.Pool, runID, stdout) }()
	go func() { defer pumpWG.Done(); pumpToRun(c.Pool, runID, stderr) }()
	pumpWG.Wait()

	err = session.Wait()
	if err == nil {
		return 0, nil
	}
	if exitErr, ok := err.(*gossh.ExitError); ok {
		return exitErr.ExitStatus(), fmt.Errorf("exit status %d", exitErr.ExitStatus())
	}
	return -1, err
}

func (c *Coordinator) markFailed(runID int32, msg string) {
	dbCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, _ = db.AppendRunOutput(dbCtx, c.Pool, runID, msg+"\n")
	if err := db.FinishRun(dbCtx, c.Pool, runID, models.RunStatusFailed, -1, msg); err != nil {
		log.Errorf("bulk: mark run %d failed: %v", runID, err)
	}
}

// pumpToRun copies an SSH reader straight to the run row. Bulk callers don't
// have a websocket; the row is the only audience.
func pumpToRun(pool *pgxpool.Pool, runID int32, src io.Reader) {
	buf := make([]byte, 4096)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_, _ = db.AppendRunOutput(ctx, pool, runID, string(buf[:n]))
			cancel()
		}
		if err != nil {
			return
		}
	}
}

// buildUpdateScript mirrors the same logic in cmd/api/main.go. Kept local so
// the updater package doesn't import cmd. The two should stay in sync; if
// they diverge, lift into a shared helper.
const aptNoninteractive = `DEBIAN_FRONTEND=noninteractive ` +
	`apt-get -o Dpkg::Options::="--force-confdef" -o Dpkg::Options::="--force-confold" -y `

func buildUpdateScript(sshUser string) string {
	prefix := ""
	if sshUser != "" && sshUser != "root" {
		prefix = "sudo -n "
	}
	return "set -o pipefail; " +
		"echo '== ubuntu-auto-update: bulk update =='; " +
		prefix + aptNoninteractive + "update && " +
		prefix + aptNoninteractive + "upgrade"
}

// newUUID returns a v4-style UUID string. Avoids a hard dep on
// github.com/google/uuid for one call site.
func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	hexStr := hex.EncodeToString(b[:])
	return hexStr[0:8] + "-" + hexStr[8:12] + "-" + hexStr[12:16] + "-" + hexStr[16:20] + "-" + hexStr[20:32], nil
}
