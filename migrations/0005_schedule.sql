CREATE SCHEMA IF NOT EXISTS schedule;

CREATE TABLE IF NOT EXISTS schedule.tasks (
    user_id TEXT NOT NULL,
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
    PRIMARY KEY (user_id, project, name)
);

CREATE TABLE IF NOT EXISTS schedule.runs (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id TEXT NOT NULL,
    project TEXT NOT NULL,
    name TEXT NOT NULL,
    run_ts BIGINT NOT NULL,
    run_date TEXT NOT NULL,
    ms BIGINT NOT NULL,
    ok BOOLEAN NOT NULL,
    body TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS runs_task ON schedule.runs (user_id, project, name, id DESC);
