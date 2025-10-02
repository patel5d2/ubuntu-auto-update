CREATE TABLE ssh_credentials (
    host_id INTEGER PRIMARY KEY REFERENCES hosts(id) ON DELETE CASCADE,
    username TEXT NOT NULL,
    password TEXT NOT NULL
);