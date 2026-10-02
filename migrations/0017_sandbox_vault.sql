-- The sandbox of an owner is not part of the workspace: where it is, who to be
-- there and the key to get in are three secrets of the owner in heimdall,
-- under the project `sandbox` and the environment `default`. Keeping them in a
-- row too would be two places for one string, and the vault is the one that
-- holds what must not be read in clear.

ALTER TABLE workspace.workspaces
    DROP COLUMN IF EXISTS sandbox_kind,
    DROP COLUMN IF EXISTS sandbox_addr,
    DROP COLUMN IF EXISTS sandbox_user;
