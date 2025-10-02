DROP TABLE ssh_credentials;

CREATE TABLE ssh_keys (
    host_id INTEGER PRIMARY KEY REFERENCES hosts(id) ON DELETE CASCADE,
    private_key TEXT NOT NULL
);