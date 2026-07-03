# Research Plan — Ubuntu Auto-Update → a great OSS patch orchestrator

**Decided positioning (2026-07-02):**
- **Scope:** "Better patch orchestrator" — updates + scripts, done well, with
  scheduling, inventory/grouping, and a real dashboard. Compete with the *UX*
  of Ansible AWX / Semaphore, **not** with Ansible's config-management engine.
- **Goal:** Portfolio / open-source. Weight capability breadth, documentation,
  and clean architecture over monetization.

This is a research plan, not an implementation plan. Each phase lists the
**questions to answer**, **where to look**, and the **deliverable** that ends
the phase. Work top-to-bottom; Phase 1 is the foundation everything else cites.

---

## 0. What you already have (baseline — don't re-research this)

| Area | State | Notes |
|------|-------|-------|
| Agent (Rust) | ✅ solid | systemd timer, apt/snap/flatpak, maintenance window, enrollment, Prometheus textfile metrics |
| SSH push (Go) | ✅ solid | encrypted keys, host-key TOFU + DB store, test-connection w/ sudo probe |
| Bulk rollout | ✅ strong | **canary wave + wait + abort-on-failure %** — this is your best feature, most tools lack it |
| Real-time | ✅ | Postgres `LISTEN/NOTIFY` → single multiplexed WebSocket, snapshot-on-reconnect |
| History/audit | ✅ | `update_runs`, audit log, sessions, 3-role users |
| Webhooks | ✅ | event subscription + dispatcher |
| Observability | ✅ | Prometheus + Grafana dashboards, alerts |
| **Scheduling** | ❌ **missing** | agent has a local timer; the *backend* cannot schedule a run. Biggest functional gap. |
| **Inventory / groups** | ❌ **missing** | hosts are a flat list; no tags, groups, or saved selections |
| **Dashboard depth** | ⚠️ thin | Pico CSS; HostList / HostDetail / BulkUpdate / ExecuteScript / Login only. No fleet overview, no compliance view |
| Config management | ⛔ out of scope | by decision — not building this |

**Over-engineered for an OSS/portfolio goal (candidates to cut or shelve):**
`backend/pkg/licensing` (paid-feature JWT gating), the Expo `mobile/` app.
They imply "enterprise SaaS" and add maintenance for zero portfolio value.
→ *Decision item in Phase 1.*

---

## Phase 1 — Positioning & competitive teardown  *(highest value, do first)*

**Goal:** Know exactly which tools you're "like," what you'll copy, and what
your one-sentence pitch is.

**Questions to answer**
1. Who are the real comparators for *Ubuntu fleet patching* (not generic config mgmt)?
2. For each: what's their dashboard's information architecture (what screens, in what order)?
3. What is your **differentiator** in one sentence? (Hypothesis: *"the open-source patch orchestrator with real staged/canary rollouts and a live dashboard."*)
4. Does the licensing package / mobile app stay, or get cut for OSS focus?

**Where to look (teardown targets — screenshot their UIs & read their docs):**
- **Canonical Landscape** — the *direct* commercial equivalent (Ubuntu fleet patch mgmt). Study its dashboard, compliance/CVE view, profiles, scheduling. This is your #1 reference.
- **Ansible AWX / AAP** — job templates, schedules, inventories, "Topology"/host views, output streaming. Your UX north star.
- **Semaphore UI** — lightweight OSS AWX alternative; good "minimal but real" bar to clear.
- **Rundeck** — job scheduling + node filtering + ACLs; great model for "run command across filtered nodes."
- **Foreman / Katello, Uyuni (SUSE Manager)** — patch/errata dashboards; steal their "N hosts need M patches, X are security" framing.
- **unattended-upgrades / apticron** — the baseline you're replacing; know why a fleet operator wants more.

**Deliverable:** `docs/positioning.md` — a comparison table (tool × feature),
5–8 annotated dashboard screenshots you like, your one-sentence pitch, and a
keep/cut decision on licensing + mobile.

---

## Phase 2 — Feature gap analysis (prioritized backlog)

**Goal:** Turn "what am I missing" into a ranked, justified list.

**Questions to answer**
1. **Scheduling** (the top gap): should schedules live server-side (cron in the backend, fires SSH runs) or push a schedule down to the agent? What does a "run window", recurrence, and blackout look like? (Model after AWX schedules + Rundeck.)
2. **Inventory**: what's the minimum useful grouping? (tags on hosts → saved "smart groups" by tag/OS/last-seen). How do AWX inventories and Rundeck node filters work?
3. **Compliance / patch visibility**: can you show "hosts with pending *security* updates" fleet-wide? This needs the agent to report the `apt list --upgradable` breakdown (security vs regular) and CVE hints. Study `unattended-upgrades` origin filtering + Ubuntu USN/CVE feeds.
4. **Reboot orchestration**: you already detect `reboot-required`; what's the safe fleet reboot flow (drain, one-at-a-time, wait-for-back)?
5. **Notifications**: you have webhooks — what are table-stakes targets (Slack/Discord/email/PagerDuty) and is a generic webhook enough for OSS?
6. **Agent vs SSH story**: you have *both* a pull agent and SSH push. Clarify when each is used — is the agent optional? This confuses the pitch; resolve it.

**Where to look:** the Phase 1 tools' feature lists, plus:
- Ubuntu Security Notices (USN) JSON feed & `ubuntu-security-status` / `pro` tooling — for the compliance angle.
- AWX "workflow" + "schedule" docs; Rundeck job scheduling & node orchestration.

**Deliverable:** `docs/gaps.md` — a table: *feature · why it matters · effort (S/M/L) · comparator that has it · priority*. Recommend the top 3 to build next (hypothesis: **1) backend scheduler, 2) host tags/groups, 3) fleet overview + security-update visibility**).

---

## Phase 3 — Dashboard / UI research  *(your explicit ask)*

**Goal:** A concrete UI spec you can build against — screens, hierarchy, and a
stack decision. Current UI is Pico CSS + 5 pages; this phase decides how far to
push it.

**Questions to answer**
1. **Screen inventory** — what pages should exist? Proposed target set:
   - **Fleet Overview** (new) — the landing dashboard: total hosts, online/stale, hosts needing updates, security-critical count, recent run outcomes, reboot-required list. *This is the single highest-impact UI addition.*
   - **Hosts** — list w/ **filter by tag/group/status**, bulk-select → action.
   - **Host Detail** — keep tabs; add "pending updates (security vs other)" + "scheduled jobs for this host".
   - **Jobs / Runs** — a real job history across the fleet (you have `update_runs` + groups already; there's no dedicated screen).
   - **Schedules** (new) — CRUD recurring runs, next-fire times.
   - **Bulk / Rollout** — surface the canary/abort knobs you *already built* (they're invisible in the UI today — huge, cheap win).
2. **Stack decision** — stay Pico CSS (fast, portfolio-clean) vs move to Tailwind + a component lib (shadcn/ui, or Mantine) for a denser "ops console" look? Trade-off: polish vs. dependency weight. For a portfolio, a *clean, fast, consistent* dashboard beats a heavy one.
3. **Data-viz** — what charts actually help? (patch-status donut, updates-over-time, run success rate). Don't add charts for decoration; each must answer an operator question.
4. **Live output UX** — you stream logs over WS; study how AWX/Semaphore render streaming job output (virtualized log, auto-scroll, per-host tabs in bulk runs).
5. **Empty/error/loading states** — the thing that makes a portfolio UI look finished.

**Where to look:**
- Ansible AWX, Semaphore, Rundeck, Grafana, Coolify, Portainer — screenshot their fleet/overview and job-output screens.
- Design references: Tailwind UI dashboard patterns, shadcn/ui, tremor.so (dashboards/charts), Refactoring UI principles.
- Your own `observability/grafana/dashboards/backend-overview.json` — reuse those metrics in-app where sensible.

**Deliverable:** `docs/ui-spec.md` — a screen map (sitemap), 1 low-fi wireframe per new screen (ASCII or Figma), a stack decision with rationale, and a component inventory. Optionally a clickable Figma.

---

## Phase 4 — Architecture & security review (make it portfolio-grade)

**Goal:** The codebase should read as *deliberately simple and correct* — that's
what a portfolio reviewer judges.

**Questions to answer**
1. **Scheduler design** — new `pkg/scheduler` (backend cron → enqueues bulk runs via the existing Coordinator) vs a lighter approach. What survives a backend restart? (persist schedules in Postgres, recompute next-fire on boot). Keep it minimal — one goroutine + a `schedules` table, not a job-queue framework.
2. **SSH scale ceiling** — the Coordinator dials fresh SSH per host with a semaphore. At what host count does this break? What's the realistic supported fleet size to advertise?
3. **Secret handling** — SSH keys are encrypted at rest; review key rotation and the `ENCRYPTION_KEY_FILE` story for the README's threat model.
4. **Cut list** — confirm removal of `licensing` (and maybe `mobile`) per Phase 1. Fewer moving parts = stronger portfolio.
5. **Test & CI health** — you have good coverage already; identify the few integration tests that would most raise confidence (a real SSH-in-a-container update run end-to-end).

**Where to look:** your own `pkg/updater`, `pkg/ssh`, `pkg/events`; Go cron
libraries (`robfig/cron`) vs stdlib `time.Ticker` (prefer the lightest that
works); `golang-migrate` for the new `schedules` table.

**Deliverable:** `docs/architecture-decisions/` — 3–5 short ADRs (scheduler,
scale target, cut-list, secret model), plus an updated architecture diagram in
the README.

---

## Phase 5 — Docs, demo & positioning polish (portfolio payoff)

**Goal:** Someone lands on the repo and *gets it* in 60 seconds.

**Deliverables**
- **README rewrite** led by the one-sentence pitch + an animated GIF/screenshot of the dashboard doing a canary rollout.
- **A live demo** — a `docker compose` "demo mode" that spins up N fake SSH targets so the dashboard has data out of the box (reviewers won't wire up real hosts).
- **A short design write-up / blog** — "how the canary rollout coordinator works" is a genuinely interesting engineering story; that's the portfolio hook.
- **CONTRIBUTING + good first issues** — signals a real OSS project.

---

## Suggested order & rough time-box

| Phase | Effort | Why this order |
|-------|--------|----------------|
| 1 Positioning & teardown | 1–2 days | Everything cites it; cheapest, highest leverage |
| 2 Gap analysis | 1 day | Turns the teardown into a ranked backlog |
| 3 UI research | 2–3 days | Your explicit ask; unblocks the biggest visible wins |
| 4 Architecture review | 1–2 days | Needed before building the scheduler |
| 5 Docs & demo | ongoing | Do the README/demo *alongside* building, not after |

**If you only do three things:** (1) build the **backend scheduler**, (2) add
**host tags + a Fleet Overview page**, (3) **surface the canary/abort rollout
controls you already have** in the UI. Those three close the gap between "apt
runner" and "patch orchestrator" — and they're mostly wiring, not new engines.

---

## Open questions to resolve as you go
- Agent-pull vs SSH-push: pick a primary story or clearly document "agent optional, SSH is the control plane."
- Security-update visibility requires an agent/report change — worth it? (Yes, likely the #1 operator value-add.)
- Keep Pico CSS or invest in a denser dashboard stack — decide in Phase 3, don't drift.
