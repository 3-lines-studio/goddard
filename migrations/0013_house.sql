-- La máquina que goddard le da a un dueño que no tiene la suya. Es una fila
-- más del workspace: el path es el del volumen dentro de esa máquina y los
-- secretos para entrar siguen en heimdall, así que un sandbox de la casa y uno
-- propio son lo mismo para el resto de goddard. Lo que hace falta saber de la
-- casa es quién la provee y cuál es su id, para poder despertarla y pararla.
ALTER TABLE workspace.workspaces
    ADD COLUMN IF NOT EXISTS provider TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS sandbox_id TEXT NOT NULL DEFAULT '';
