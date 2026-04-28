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
}

export interface Webhook {
  id: number;
  url: string;
  event: string;
}

export type RunKind = 'preview' | 'update';
export type RunStatus = 'running' | 'succeeded' | 'failed' | 'cancelled';

export interface UpdateRun {
  id: number;
  host_id: number;
  triggered_by: string;
  kind: RunKind;
  status: RunStatus;
  exit_code: number | null;
  started_at: string;
  finished_at: string | null;
  output: string;
  error: string | null;
}
