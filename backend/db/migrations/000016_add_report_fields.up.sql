-- Persist the rich data the Rust agent already sends in its report payload.
-- Before this, models.HostReport only decoded {hostname, update_output,
-- upgrade_output}, silently dropping reboot-required, package counts, and
-- OS/kernel info that the agent computes every run.
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS reboot_required   BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS packages_updated  INTEGER NOT NULL DEFAULT 0;
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS packages_available INTEGER NOT NULL DEFAULT 0;
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS os_version        TEXT NOT NULL DEFAULT '';
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS kernel_version    TEXT NOT NULL DEFAULT '';
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS agent_version     TEXT NOT NULL DEFAULT '';
