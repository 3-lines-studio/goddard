CREATE SCHEMA IF NOT EXISTS memo;

CREATE TABLE IF NOT EXISTS memo.facts (
    project TEXT NOT NULL,
    key TEXT NOT NULL,
    kind TEXT NOT NULL,
    body TEXT NOT NULL,
    fact_date BIGINT NOT NULL,
    PRIMARY KEY (project, key)
);

CREATE INDEX IF NOT EXISTS facts_project ON memo.facts (project, fact_date DESC, key);

CREATE TABLE IF NOT EXISTS memo.revisions (
    project TEXT NOT NULL,
    key TEXT NOT NULL,
    rev INTEGER NOT NULL,
    kind TEXT NOT NULL,
    body TEXT NOT NULL,
    last_seen BIGINT NOT NULL,
    PRIMARY KEY (project, key, rev)
);
