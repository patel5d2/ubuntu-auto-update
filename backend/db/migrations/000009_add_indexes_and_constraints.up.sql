-- Add proper indexes for performance
CREATE INDEX IF NOT EXISTS idx_hosts_hostname ON hosts(hostname);
CREATE INDEX IF NOT EXISTS idx_hosts_last_seen ON hosts(last_seen);
CREATE INDEX IF NOT EXISTS idx_hosts_created_at ON hosts(created_at);

-- Add index for SSH keys foreign key
CREATE INDEX IF NOT EXISTS idx_ssh_keys_host_id ON ssh_keys(host_id);

-- Add indexes for webhooks
CREATE INDEX IF NOT EXISTS idx_webhooks_event ON webhooks(event);

-- Add constraints for data integrity
ALTER TABLE hosts ADD CONSTRAINT IF NOT EXISTS check_hostname_not_empty 
    CHECK (length(trim(hostname)) > 0);

ALTER TABLE hosts ADD CONSTRAINT IF NOT EXISTS check_ssh_user_not_empty 
    CHECK (length(trim(ssh_user)) > 0);

-- Add constraint for webhook URLs
ALTER TABLE webhooks ADD CONSTRAINT IF NOT EXISTS check_webhook_url_format
    CHECK (url LIKE 'http%://_%');

-- Add constraint for webhook events
ALTER TABLE webhooks ADD CONSTRAINT IF NOT EXISTS check_webhook_event_valid
    CHECK (event IN ('update_success', 'update_failure', 'host_registered', 'host_offline'));

-- Add updated_at trigger for hosts table
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language plpgsql;

DROP TRIGGER IF EXISTS update_hosts_updated_at ON hosts;
CREATE TRIGGER update_hosts_updated_at 
    BEFORE UPDATE ON hosts 
    FOR EACH ROW 
    EXECUTE FUNCTION update_updated_at_column();

-- Add audit table for tracking changes
CREATE TABLE IF NOT EXISTS audit_logs (
    id SERIAL PRIMARY KEY,
    table_name TEXT NOT NULL,
    record_id INTEGER NOT NULL,
    action TEXT NOT NULL CHECK (action IN ('INSERT', 'UPDATE', 'DELETE')),
    old_values JSONB,
    new_values JSONB,
    changed_by TEXT,
    changed_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_table_name ON audit_logs(table_name);
CREATE INDEX IF NOT EXISTS idx_audit_logs_changed_at ON audit_logs(changed_at);
CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs(action);

-- Add user sessions table for better authentication tracking
CREATE TABLE IF NOT EXISTS user_sessions (
    id SERIAL PRIMARY KEY,
    user_id TEXT NOT NULL,
    username TEXT NOT NULL,
    session_token TEXT UNIQUE NOT NULL,
    ip_address INET,
    user_agent TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    last_accessed TIMESTAMPTZ DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    is_active BOOLEAN DEFAULT TRUE
);

CREATE INDEX IF NOT EXISTS idx_user_sessions_token ON user_sessions(session_token);
CREATE INDEX IF NOT EXISTS idx_user_sessions_user_id ON user_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_user_sessions_expires_at ON user_sessions(expires_at);

-- Add agent tokens table for better agent authentication
CREATE TABLE IF NOT EXISTS agent_tokens (
    id SERIAL PRIMARY KEY,
    host_id INTEGER NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    token_hash TEXT UNIQUE NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    last_used TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,
    is_active BOOLEAN DEFAULT TRUE,
    scopes TEXT[] DEFAULT ARRAY['update', 'report']
);

CREATE INDEX IF NOT EXISTS idx_agent_tokens_token_hash ON agent_tokens(token_hash);
CREATE INDEX IF NOT EXISTS idx_agent_tokens_host_id ON agent_tokens(host_id);
CREATE INDEX IF NOT EXISTS idx_agent_tokens_expires_at ON agent_tokens(expires_at);