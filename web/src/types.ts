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
