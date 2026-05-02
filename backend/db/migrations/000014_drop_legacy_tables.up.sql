-- Drop legacy tables that were replaced by migration 000013

-- Replaced by 'sessions' which stores SHA-256 hashes instead of plaintext tokens
DROP TABLE IF EXISTS user_sessions CASCADE;

-- Replaced by 'sessions' with agent_label
DROP TABLE IF EXISTS agent_tokens CASCADE;

-- Replaced by 'audit_log' with better actor attribution
DROP TABLE IF EXISTS audit_logs CASCADE;
