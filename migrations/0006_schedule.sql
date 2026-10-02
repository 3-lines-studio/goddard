-- The agenda: the tasks of an owner and what they answered. A run is read
-- newest first and two of them can land in the same millisecond, so the
-- sequence is what orders them.
CREATE SCHEMA IF NOT EXISTS schedule;

CREATE TABLE IF NOT EXISTS schedule.tasks (
    id TEXT NOT NULL DEFAULT goddard.ulid(),
    owner_kind TEXT NOT NULL,
    owner_id TEXT NOT NULL,
    project TEXT NOT NULL,
    name TEXT NOT NULL,
    once_at TEXT NOT NULL DEFAULT '',
    daily_at TEXT NOT NULL DEFAULT '',
    every TEXT NOT NULL DEFAULT '',
    target TEXT NOT NULL DEFAULT '',
    prompt TEXT NOT NULL,
    silent BOOLEAN NOT NULL DEFAULT false,
    paused BOOLEAN NOT NULL DEFAULT false,
    claimed_until BIGINT,
    seen_ts BIGINT NOT NULL DEFAULT 0,
    created_at BIGINT NOT NULL DEFAULT goddard.now(),
    updated_at BIGINT NOT NULL DEFAULT goddard.now(),
    deleted_at BIGINT,
    PRIMARY KEY (id),
    CONSTRAINT tasks_natural UNIQUE (owner_kind, owner_id, project, name)
);

CREATE TABLE IF NOT EXISTS schedule.runs (
    id TEXT NOT NULL DEFAULT goddard.ulid(),
    owner_kind TEXT NOT NULL,
    owner_id TEXT NOT NULL,
    project TEXT NOT NULL,
    name TEXT NOT NULL,
    run_ts BIGINT NOT NULL,
    run_date TEXT NOT NULL,
    ms BIGINT NOT NULL,
    ok BOOLEAN NOT NULL,
    body TEXT NOT NULL,
    seq BIGINT GENERATED ALWAYS AS IDENTITY,
    created_at BIGINT NOT NULL DEFAULT goddard.now(),
    updated_at BIGINT NOT NULL DEFAULT goddard.now(),
    deleted_at BIGINT,
    PRIMARY KEY (id)
);

CREATE INDEX IF NOT EXISTS runs_task ON schedule.runs (owner_kind, owner_id, project, name, seq DESC);
