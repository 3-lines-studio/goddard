-- What the agent remembers of a project. `facts` is what is in context and
-- `revisions` is every version it went through, which is the history that never
-- gets deleted.
CREATE SCHEMA IF NOT EXISTS memo;

CREATE TABLE IF NOT EXISTS memo.facts (
    id TEXT NOT NULL DEFAULT goddard.ulid(),
    owner_kind TEXT NOT NULL,
    owner_id TEXT NOT NULL,
    project TEXT NOT NULL,
    key TEXT NOT NULL,
    kind TEXT NOT NULL,
    body TEXT NOT NULL,
    fact_date BIGINT NOT NULL,
    created_at BIGINT NOT NULL DEFAULT goddard.now(),
    updated_at BIGINT NOT NULL DEFAULT goddard.now(),
    deleted_at BIGINT,
    PRIMARY KEY (id),
    CONSTRAINT facts_natural UNIQUE (owner_kind, owner_id, project, key)
);

CREATE INDEX IF NOT EXISTS facts_project ON memo.facts (owner_kind, owner_id, project, fact_date DESC, key);

CREATE TABLE IF NOT EXISTS memo.revisions (
    id TEXT NOT NULL DEFAULT goddard.ulid(),
    owner_kind TEXT NOT NULL,
    owner_id TEXT NOT NULL,
    project TEXT NOT NULL,
    key TEXT NOT NULL,
    rev INTEGER NOT NULL,
    kind TEXT NOT NULL,
    body TEXT NOT NULL,
    last_seen BIGINT NOT NULL,
    created_at BIGINT NOT NULL DEFAULT goddard.now(),
    updated_at BIGINT NOT NULL DEFAULT goddard.now(),
    deleted_at BIGINT,
    PRIMARY KEY (id),
    CONSTRAINT revisions_natural UNIQUE (owner_kind, owner_id, project, key, rev)
);
