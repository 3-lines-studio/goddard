import { useEffect, useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";

import type { Org, Sandbox, SandboxInput } from "./types";

const ROLES: Record<string, string> = {
  owner: "dueño",
  admin: "admin",
  member: "miembro",
};

// Space is the workspace of one owner: where its projects live and the machine
// that runs the tools over them. The key never comes back from the app, so the
// box starts empty, and an empty box keeps the one that is already there.
function Space({
  id,
  name,
  role,
  space,
  saving,
  removing,
  onSave,
  onRemove,
}: {
  id: string;
  name: string;
  role: string;
  space: Sandbox | undefined;
  saving: boolean;
  removing: boolean;
  onSave: (id: string, input: SandboxInput) => void;
  onRemove: (id: string) => void;
}) {
  const [path, setPath] = useState(space?.path ?? "");
  const [addr, setAddr] = useState(space?.addr ?? "");
  const [user, setUser] = useState(space?.user ?? "");
  const [key, setKey] = useState("");
  const writable = role === "owner" || role === "admin";

  useEffect(() => {
    if (!space) return;
    setPath(space.path);
    setAddr(space.addr);
    setUser(space.user);
    setKey("");
  }, [space]);

  return (
    <Card size="sm" data-space={id}>
      <CardHeader>
        <CardTitle className="font-normal">
          <span data-space-name={id}>{name}</span>
          {id ? <Badge variant="outline">{ROLES[role] ?? role}</Badge> : null}
        </CardTitle>
        <CardDescription>{space ? space.path : "leyendo…"}</CardDescription>
      </CardHeader>
      <CardContent>
        {!space ? null : writable ? (
          <form
            data-space-form={id}
            onSubmit={(event) => {
              event.preventDefault();
              onSave(id, { path, addr, user, key });
            }}
          >
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor={`space-path-${id}`}>volumen</FieldLabel>
                <Input
                  id={`space-path-${id}`}
                  name="path"
                  value={path}
                  data-space-path={id}
                  onChange={(event) => setPath(event.target.value)}
                />
                <FieldDescription>La carpeta donde viven los proyectos, montada en la máquina.</FieldDescription>
              </Field>
              <Field>
                <FieldLabel htmlFor={`space-addr-${id}`}>dirección</FieldLabel>
                <Input
                  id={`space-addr-${id}`}
                  name="addr"
                  value={addr}
                  data-space-addr={id}
                  placeholder="sandbox.local:22"
                  onChange={(event) => setAddr(event.target.value)}
                />
                <FieldDescription>La máquina que corre las herramientas, y el puerto del ssh.</FieldDescription>
              </Field>
              <Field>
                <FieldLabel htmlFor={`space-user-${id}`}>usuario</FieldLabel>
                <Input
                  id={`space-user-${id}`}
                  name="user"
                  value={user}
                  data-space-user={id}
                  placeholder="goddard"
                  onChange={(event) => setUser(event.target.value)}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor={`space-key-${id}`}>llave privada</FieldLabel>
                <Input
                  id={`space-key-${id}`}
                  name="key"
                  type="password"
                  value={key}
                  data-space-key={id}
                  placeholder={space.has_key ? "la que ya está" : "sin llave"}
                  onChange={(event) => setKey(event.target.value)}
                />
                <FieldDescription>
                  {space.has_key
                    ? "Ya hay una cargada. Escribí otra para reemplazarla, o dejá el casillero vacío."
                    : "El archivo de la llave, entero: la que entra a la máquina sin pedir contraseña."}
                </FieldDescription>
              </Field>
              <Field orientation="horizontal">
                <Button type="submit" size="sm" variant="outline" data-space-save={id}>
                  {saving ? "guardando…" : "Guardar"}
                </Button>
                {space.addr || space.has_key ? (
                  <Button
                    type="button"
                    size="sm"
                    variant="ghost"
                    data-space-remove={id}
                    className="text-muted-foreground"
                    onClick={() => onRemove(id)}
                  >
                    {removing ? "otra vez para sacarla" : "sacar la computadora"}
                  </Button>
                ) : null}
              </Field>
            </FieldGroup>
          </form>
        ) : (
          <p className="text-sm text-muted-foreground">
            Esto lo carga un dueño o un admin. La máquina es {space.addr || "—"} con el usuario {space.user || "—"}
            {space.has_key ? "" : ", y todavía no tiene llave"}.
          </p>
        )}
      </CardContent>
    </Card>
  );
}

export function Sandboxes({
  spaces,
  orgs,
  saving,
  removing,
  onSave,
  onRemove,
}: {
  spaces: Record<string, Sandbox>;
  orgs: Org[];
  saving: string;
  removing: string;
  onSave: (id: string, input: SandboxInput) => void;
  onRemove: (id: string) => void;
}) {
  const owners = [{ id: "", name: "Personal", role: "owner" }, ...orgs];
  return (
    <div className="mx-auto flex w-full max-w-3xl flex-col gap-6 p-4">
      <section className="flex flex-col gap-1">
        <h2 className="text-sm font-semibold">Computadoras</h2>
        <p className="text-sm text-muted-foreground">
          Dónde viven los proyectos y qué máquina corre las herramientas. Sin máquina no hay turno: el proyecto se
          queda sin lugar donde trabajar.
        </p>
      </section>
      {owners.map((owner) => (
        <Space
          key={owner.id || "personal"}
          id={owner.id}
          name={owner.name}
          role={owner.role}
          space={spaces[owner.id]}
          saving={saving === (owner.id || "personal")}
          removing={removing === (owner.id || "personal")}
          onSave={onSave}
          onRemove={onRemove}
        />
      ))}
    </div>
  );
}
