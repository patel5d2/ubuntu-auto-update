CREATE TABLE webhooks (
    id SERIAL PRIMARY KEY,
    url TEXT NOT NULL,
    event TEXT NOT NULL
);