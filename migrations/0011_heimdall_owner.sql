ALTER TABLE heimdall.secrets ADD COLUMN owner_kind TEXT NOT NULL DEFAULT 'org';
ALTER TABLE heimdall.secrets ADD COLUMN owner_id TEXT NOT NULL DEFAULT 'default';
ALTER TABLE heimdall.secrets ALTER COLUMN owner_kind DROP DEFAULT;
ALTER TABLE heimdall.secrets ALTER COLUMN owner_id DROP DEFAULT;
ALTER TABLE heimdall.secrets DROP CONSTRAINT secrets_pkey;
ALTER TABLE heimdall.secrets ADD PRIMARY KEY (owner_kind, owner_id, project, env, name);
ALTER TABLE heimdall.secrets DROP CONSTRAINT secrets_natural;
ALTER TABLE heimdall.secrets ADD CONSTRAINT secrets_natural UNIQUE (owner_kind, owner_id, project, env, name);
ALTER TABLE heimdall.secrets ADD CONSTRAINT secrets_owner_kind CHECK (owner_kind IN ('org', 'user'));

ALTER TABLE heimdall.environments ADD COLUMN owner_kind TEXT NOT NULL DEFAULT 'org';
ALTER TABLE heimdall.environments ADD COLUMN owner_id TEXT NOT NULL DEFAULT 'default';
ALTER TABLE heimdall.environments ALTER COLUMN owner_kind DROP DEFAULT;
ALTER TABLE heimdall.environments ALTER COLUMN owner_id DROP DEFAULT;
ALTER TABLE heimdall.environments DROP CONSTRAINT environments_pkey;
ALTER TABLE heimdall.environments ADD PRIMARY KEY (owner_kind, owner_id, project, env);
ALTER TABLE heimdall.environments DROP CONSTRAINT environments_natural;
ALTER TABLE heimdall.environments ADD CONSTRAINT environments_natural UNIQUE (owner_kind, owner_id, project, env);
ALTER TABLE heimdall.environments ADD CONSTRAINT environments_owner_kind CHECK (owner_kind IN ('org', 'user'));

ALTER TABLE heimdall.tokens ADD COLUMN owner_kind TEXT NOT NULL DEFAULT 'org';
ALTER TABLE heimdall.tokens ADD COLUMN owner_id TEXT NOT NULL DEFAULT 'default';
ALTER TABLE heimdall.tokens ALTER COLUMN owner_kind DROP DEFAULT;
ALTER TABLE heimdall.tokens ALTER COLUMN owner_id DROP DEFAULT;
ALTER TABLE heimdall.tokens ADD CONSTRAINT tokens_owner_kind CHECK (owner_kind IN ('org', 'user'));

ALTER TABLE heimdall.audit ADD COLUMN owner_kind TEXT NOT NULL DEFAULT 'org';
ALTER TABLE heimdall.audit ADD COLUMN owner_id TEXT NOT NULL DEFAULT 'default';
ALTER TABLE heimdall.audit ALTER COLUMN owner_kind DROP DEFAULT;
ALTER TABLE heimdall.audit ALTER COLUMN owner_id DROP DEFAULT;
