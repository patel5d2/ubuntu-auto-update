export interface Host {
  id: number;
  hostname: string;
  ssh_user: string;
  created_at: string;
  updated_at: string;
  last_seen: string;
  update_output: string;
  upgrade_output: string;
  error: string | null;
  tags: string[];
  reboot_required: boolean;
  packages_updated: number;
  packages_available: number;
  os_version: string;
  kernel_version: string;
  agent_version: string;
  offline_since: string | null;
}

export interface Schedule {
  id: number;
  name: string;
  host_ids: number[];
  interval_minutes: number;
  next_run_at: string;
  enabled: boolean;
  created_by: string;
  created_at: string;
  playbook_id: number | null;
  concurrency: number;
  canary_count: number;
  canary_wait_seconds: number;
  abort_on_failure_pct: number;
  window_start_minute: number | null;
  window_end_minute: number | null;
  window_days: number;
  security_only: boolean;
}

export interface Overview {
  total_hosts: number;
  online_hosts: number;
  error_hosts: number;
  reboot_hosts: number;
  runs_7d: number;
  failed_7d: number;
  running_now: number;
}

export interface ApiToken {
  id: number;
  name: string;
  role: Role;
  created_by: string;
  created_at: string;
  last_used_at: string | null;
}

export interface Webhook {
  id: number;
  url: string;
  event: string;
}

export type RunKind = 'preview' | 'update' | 'playbook' | 'reboot';
export type RunStatus = 'running' | 'succeeded' | 'failed' | 'cancelled';

export interface Playbook {
  id: number;
  name: string;
  description: string;
  steps: string[];
  use_sudo: boolean;
  created_by: string;
  created_at: string;
  updated_at: string;
}

export interface UpdateRun {
  id: number;
  host_id: number;
  run_group_id: string | null;
  triggered_by: string;
  kind: RunKind;
  status: RunStatus;
  exit_code: number | null;
  started_at: string;
  finished_at: string | null;
  output: string;
  error: string | null;
  playbook_id: number | null;
}

export interface BulkRunResult {
  group_id: string;
  run_ids: number[];
  host_ids: number[];
}

export interface TestConnectionResult {
  ok: boolean;
  latency_ms: number;
  sudo_state: 'root' | 'available' | 'unavailable' | 'n/a';
  greeting: string;
  error?: string;
}

export type Role = 'viewer' | 'operator' | 'admin';

export interface User {
  id: number;
  username: string;
  role: Role;
  disabled_at?: string | null;
  created_at: string;
  updated_at: string;
  last_login_at?: string | null;
  failed_logins: number;
  locked_until?: string | null;
}

export interface AuditRecord {
  id: number;
  occurred_at: string;
  actor_user_id?: number;
  actor_label?: string;
  action: string;
  target_type?: string;
  target_id?: string;
  request_id?: string;
  ip?: string;
  user_agent?: string;
  details?: Record<string, unknown>;
}

export interface BulkEnrollHostResult {
  hostname: string;
  ok: boolean;
  host_id?: number;
  error?: string;
  sudo_configured?: boolean;
  fingerprint?: string;
}

export interface BulkEnrollResult {
  results: BulkEnrollHostResult[];
  success_count: number;
  failure_count: number;
}
