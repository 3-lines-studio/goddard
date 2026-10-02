-- Who comes in. Links and sessions are kept as a hash and only while they last:
-- nothing here is a soft delete, an expired one is gone.
CREATE SCHEMA IF NOT EXISTS auth;

CREATE TABLE IF NOT EXISTS auth.users (
    id TEXT NOT NULL DEFAULT goddard.ulid(),
    email TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    created_at BIGINT NOT NULL DEFAULT goddard.now(),
    updated_at BIGINT NOT NULL DEFAULT goddard.now(),
    deleted_at BIGINT,
    PRIMARY KEY (id)
);

CREATE TABLE IF NOT EXISTS auth.logins (
    id TEXT NOT NULL DEFAULT goddard.ulid(),
    user_id TEXT NOT NULL REFERENCES auth.users (id) ON DELETE CASCADE,
    hash TEXT NOT NULL,
    expires_at BIGINT NOT NULL,
    created_at BIGINT NOT NULL DEFAULT goddard.now(),
    updated_at BIGINT NOT NULL DEFAULT goddard.now(),
    deleted_at BIGINT,
    PRIMARY KEY (id)
);

CREATE INDEX IF NOT EXISTS logins_hash ON auth.logins (hash);

CREATE TABLE IF NOT EXISTS auth.sessions (
    id TEXT NOT NULL DEFAULT goddard.ulid(),
    user_id TEXT NOT NULL REFERENCES auth.users (id) ON DELETE CASCADE,
    hash TEXT NOT NULL,
    expires_at BIGINT NOT NULL,
    created_at BIGINT NOT NULL DEFAULT goddard.now(),
    updated_at BIGINT NOT NULL DEFAULT goddard.now(),
    deleted_at BIGINT,
    PRIMARY KEY (id)
);

CREATE INDEX IF NOT EXISTS sessions_hash ON auth.sessions (hash);
