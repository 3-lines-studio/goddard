ALTER TABLE axe.sessions RENAME COLUMN archived_at TO deleted_at;

DROP INDEX IF EXISTS axe.sessions_live;

CREATE UNIQUE INDEX IF NOT EXISTS sessions_live ON axe.sessions (scope) WHERE deleted_at IS NULL;

ALTER TABLE axe.sessions
    ADD COLUMN IF NOT EXISTS created_at BIGINT NOT NULL DEFAULT goddard.now();

ALTER TABLE axe.sessions ALTER COLUMN id SET DEFAULT goddard.ulid();

ALTER TABLE axe.entries
    ADD COLUMN IF NOT EXISTS id TEXT NOT NULL DEFAULT goddard.ulid(),
    ADD COLUMN IF NOT EXISTS created_at BIGINT NOT NULL DEFAULT goddard.now(),
    ADD COLUMN IF NOT EXISTS updated_at BIGINT NOT NULL DEFAULT goddard.now(),
    ADD COLUMN IF NOT EXISTS deleted_at BIGINT;

ALTER TABLE axe.entries DROP CONSTRAINT IF EXISTS entries_pkey;
ALTER TABLE axe.entries ADD PRIMARY KEY (id);
ALTER TABLE axe.entries ADD CONSTRAINT entries_natural UNIQUE (session_id, seq);

ALTER TABLE axe.resume
    ADD COLUMN IF NOT EXISTS id TEXT NOT NULL DEFAULT goddard.ulid(),
    ADD COLUMN IF NOT EXISTS created_at BIGINT NOT NULL DEFAULT goddard.now(),
    ADD COLUMN IF NOT EXISTS updated_at BIGINT NOT NULL DEFAULT goddard.now(),
    ADD COLUMN IF NOT EXISTS deleted_at BIGINT;

ALTER TABLE axe.resume DROP CONSTRAINT IF EXISTS resume_pkey;
ALTER TABLE axe.resume ADD PRIMARY KEY (id);
ALTER TABLE axe.resume ADD CONSTRAINT resume_natural UNIQUE (scope);

ALTER TABLE memo.facts
    ADD COLUMN IF NOT EXISTS id TEXT NOT NULL DEFAULT goddard.ulid(),
    ADD COLUMN IF NOT EXISTS created_at BIGINT NOT NULL DEFAULT goddard.now(),
    ADD COLUMN IF NOT EXISTS updated_at BIGINT NOT NULL DEFAULT goddard.now(),
    ADD COLUMN IF NOT EXISTS deleted_at BIGINT;

ALTER TABLE memo.facts DROP CONSTRAINT IF EXISTS facts_pkey;
ALTER TABLE memo.facts ADD PRIMARY KEY (id);
ALTER TABLE memo.facts ADD CONSTRAINT facts_natural UNIQUE (project, key);

ALTER TABLE memo.revisions
    ADD COLUMN IF NOT EXISTS id TEXT NOT NULL DEFAULT goddard.ulid(),
    ADD COLUMN IF NOT EXISTS created_at BIGINT NOT NULL DEFAULT goddard.now(),
    ADD COLUMN IF NOT EXISTS updated_at BIGINT NOT NULL DEFAULT goddard.now(),
    ADD COLUMN IF NOT EXISTS deleted_at BIGINT;

ALTER TABLE memo.revisions DROP CONSTRAINT IF EXISTS revisions_pkey;
ALTER TABLE memo.revisions ADD PRIMARY KEY (id);
ALTER TABLE memo.revisions ADD CONSTRAINT revisions_natural UNIQUE (project, key, rev);

ALTER TABLE skill.skills
    ADD COLUMN IF NOT EXISTS id TEXT NOT NULL DEFAULT goddard.ulid(),
    ADD COLUMN IF NOT EXISTS deleted_at BIGINT;

ALTER TABLE skill.skills DROP CONSTRAINT IF EXISTS skills_pkey;
ALTER TABLE skill.skills ADD PRIMARY KEY (id);
ALTER TABLE skill.skills ADD CONSTRAINT skills_natural UNIQUE (owner_kind, owner_id, name);

ALTER TABLE schedule.tasks
    ADD COLUMN IF NOT EXISTS id TEXT NOT NULL DEFAULT goddard.ulid(),
    ADD COLUMN IF NOT EXISTS created_at BIGINT NOT NULL DEFAULT goddard.now(),
    ADD COLUMN IF NOT EXISTS updated_at BIGINT NOT NULL DEFAULT goddard.now(),
    ADD COLUMN IF NOT EXISTS deleted_at BIGINT;

ALTER TABLE schedule.tasks DROP CONSTRAINT IF EXISTS tasks_pkey;
ALTER TABLE schedule.tasks ADD PRIMARY KEY (id);
ALTER TABLE schedule.tasks ADD CONSTRAINT tasks_natural UNIQUE (user_id, project, name);

ALTER TABLE schedule.runs ALTER COLUMN id DROP IDENTITY IF EXISTS;
ALTER TABLE schedule.runs ALTER COLUMN id TYPE TEXT USING id::text;
ALTER TABLE schedule.runs ALTER COLUMN id SET DEFAULT goddard.ulid();
ALTER TABLE schedule.runs
    ADD COLUMN IF NOT EXISTS seq BIGINT GENERATED ALWAYS AS IDENTITY,
    ADD COLUMN IF NOT EXISTS created_at BIGINT NOT NULL DEFAULT goddard.now(),
    ADD COLUMN IF NOT EXISTS updated_at BIGINT NOT NULL DEFAULT goddard.now(),
    ADD COLUMN IF NOT EXISTS deleted_at BIGINT;

DROP INDEX IF EXISTS schedule.runs_task;

CREATE INDEX IF NOT EXISTS runs_task ON schedule.runs (user_id, project, name, seq DESC);

ALTER TABLE heimdall.secrets
    ADD COLUMN IF NOT EXISTS id TEXT NOT NULL DEFAULT goddard.ulid(),
    ADD COLUMN IF NOT EXISTS created_at BIGINT NOT NULL DEFAULT goddard.now(),
    ADD COLUMN IF NOT EXISTS deleted_at BIGINT;

ALTER TABLE heimdall.secrets DROP CONSTRAINT IF EXISTS secrets_pkey;
ALTER TABLE heimdall.secrets ADD PRIMARY KEY (id);
ALTER TABLE heimdall.secrets ADD CONSTRAINT secrets_natural UNIQUE (project, env, name);

ALTER TABLE heimdall.tokens
    ADD COLUMN IF NOT EXISTS updated_at BIGINT NOT NULL DEFAULT goddard.now(),
    ADD COLUMN IF NOT EXISTS deleted_at BIGINT;

ALTER TABLE heimdall.tokens ALTER COLUMN id SET DEFAULT goddard.ulid();

ALTER TABLE heimdall.environments
    ADD COLUMN IF NOT EXISTS id TEXT NOT NULL DEFAULT goddard.ulid(),
    ADD COLUMN IF NOT EXISTS updated_at BIGINT NOT NULL DEFAULT goddard.now(),
    ADD COLUMN IF NOT EXISTS deleted_at BIGINT;

ALTER TABLE heimdall.environments DROP CONSTRAINT IF EXISTS environments_pkey;
ALTER TABLE heimdall.environments ADD PRIMARY KEY (id);
ALTER TABLE heimdall.environments ADD CONSTRAINT environments_natural UNIQUE (project, env);

ALTER TABLE heimdall.audit ALTER COLUMN id DROP DEFAULT;
ALTER TABLE heimdall.audit ALTER COLUMN id TYPE TEXT USING id::text;
ALTER TABLE heimdall.audit ALTER COLUMN id SET DEFAULT goddard.ulid();
ALTER TABLE heimdall.audit
    ADD COLUMN IF NOT EXISTS seq BIGINT GENERATED ALWAYS AS IDENTITY,
    ADD COLUMN IF NOT EXISTS created_at BIGINT NOT NULL DEFAULT goddard.now(),
    ADD COLUMN IF NOT EXISTS updated_at BIGINT NOT NULL DEFAULT goddard.now(),
    ADD COLUMN IF NOT EXISTS deleted_at BIGINT;

DROP SEQUENCE IF EXISTS heimdall.audit_id_seq;

ALTER TABLE heimdall.logins
    ADD COLUMN IF NOT EXISTS id TEXT NOT NULL DEFAULT goddard.ulid(),
    ADD COLUMN IF NOT EXISTS updated_at BIGINT NOT NULL DEFAULT goddard.now(),
    ADD COLUMN IF NOT EXISTS deleted_at BIGINT;

ALTER TABLE heimdall.logins DROP CONSTRAINT IF EXISTS logins_pkey;
ALTER TABLE heimdall.logins ADD PRIMARY KEY (id);
ALTER TABLE heimdall.logins ADD CONSTRAINT logins_natural UNIQUE (hash);

ALTER TABLE heimdall.sessions
    ADD COLUMN IF NOT EXISTS id TEXT NOT NULL DEFAULT goddard.ulid(),
    ADD COLUMN IF NOT EXISTS updated_at BIGINT NOT NULL DEFAULT goddard.now(),
    ADD COLUMN IF NOT EXISTS deleted_at BIGINT;

ALTER TABLE heimdall.sessions DROP CONSTRAINT IF EXISTS sessions_pkey;
ALTER TABLE heimdall.sessions ADD PRIMARY KEY (id);
ALTER TABLE heimdall.sessions ADD CONSTRAINT sessions_natural UNIQUE (hash);
