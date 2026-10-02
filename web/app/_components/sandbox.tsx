import { useEffect, useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Spinner } from "@/components/ui/spinner";
import { Textarea } from "@/components/ui/textarea";

import type { Org, Sandbox, SandboxCheck, SandboxInput } from "./types";

const ROLES: Record<string, string> = {
  owner: "dueño",
  admin: "admin",
  member: "miembro",
};

// loaded says whether that owner has a machine loaded at all. The path that
// comes with no machine is the volume of the house, and showing it as if it
// were somebody's own is what makes a machine of one's own look like it
// belongs in /volumes.
function loaded(space: Sandbox) {
  return space.addr !== "" || space.has_key;
}

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
  checking,
  answer,
  onSave,
  onRemove,
  onCheck,
}: {
  id: string;
  name: string;
  role: string;
  space: Sandbox | undefined;
  saving: boolean;
  removing: boolean;
  checking: boolean;
  answer: SandboxCheck | undefined;
  onSave: (id: string, input: SandboxInput) => void;
  onRemove: (id: string) => void;
  onCheck: (id: string) => void;
}) {
  const [path, setPath] = useState(space?.path ?? "");
  const [addr, setAddr] = useState(space?.addr ?? "");
  const [user, setUser] = useState(space?.user ?? "");
  const [key, setKey] = useState("");
  const [passphrase, setPassphrase] = useState("");
  const writable = role === "owner" || role === "admin";

  useEffect(() => {
    if (!space) return;
    setPath(loaded(space) ? space.path : "");
    setAddr(space.addr);
    setUser(space.user);
    setKey("");
    setPassphrase("");
  }, [space]);

  return (
    <Card size="sm" data-space={id}>
      <CardHeader>
        <CardTitle className="font-normal">
          <span data-space-name={id}>{name}</span>
          {id ? <Badge variant="outline">{ROLES[role] ?? role}</Badge> : null}
        </CardTitle>
        <CardDescription>{space ? space.path : <Skeleton className="h-4 w-40" />}</CardDescription>
      </CardHeader>
      <CardContent>
        {!space ? null : writable ? (
          <form
            data-space-form={id}
            onSubmit={(event) => {
              event.preventDefault();
              onSave(id, { path, addr, user, key, passphrase });
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
                  placeholder="/home/tu-usuario/goddard"
                  onChange={(event) => setPath(event.target.value)}
                />
                <FieldDescription>La carpeta donde viven los proyectos, montada en la máquina. Tiene que existir: goddard no la crea.</FieldDescription>
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
                <Textarea
                  id={`space-key-${id}`}
                  name="key"
                  rows={4}
                  value={key}
                  data-space-key={id}
                  placeholder={space.has_key ? "la que ya está" : "sin llave"}
                  className="font-mono text-xs"
                  onChange={(event) => setKey(event.target.value)}
                />
                <FieldDescription>
                  {space.has_key
                    ? "Ya hay una cargada. Escribí otra para reemplazarla, o dejá el casillero vacío."
                    : "El archivo de la llave, entero: la que entra a la máquina sin pedir contraseña."}
                </FieldDescription>
              </Field>
              <Field>
                <FieldLabel htmlFor={`space-passphrase-${id}`}>passphrase</FieldLabel>
                <Input
                  id={`space-passphrase-${id}`}
                  name="passphrase"
                  value={passphrase}
                  data-space-passphrase={id}
                  placeholder={space.has_passphrase ? "la que ya está" : "sin passphrase"}
                  onChange={(event) => setPassphrase(event.target.value)}
                />
                <FieldDescription>Sólo si la llave se guardó con una palabra.</FieldDescription>
              </Field>
              <Field orientation="horizontal">
                <Button type="submit" size="sm" variant="outline" data-space-save={id} disabled={saving}>
                  {saving ? <Spinner data-icon="inline-start" /> : null}
                  {saving ? "guardando…" : "Guardar"}
                </Button>
                <Button
                  type="button"
                  size="sm"
                  variant="ghost"
                  data-space-check={id}
                  disabled={checking}
                  onClick={() => onCheck(id)}
                >
                  {checking ? <Spinner data-icon="inline-start" /> : null}
                  {checking ? "probando…" : "probar"}
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
              {answer ? (
                <p
                  data-space-answer={id}
                  className={answer.ok ? "text-sm text-muted-foreground" : "text-sm text-destructive"}
                >
                  {answer.ok ? `anda: la máquina contesta y ve ${space.path}` : answer.message}
                </p>
              ) : null}
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
  checking,
  checks,
  onSave,
  onRemove,
  onCheck,
}: {
  spaces: Record<string, Sandbox>;
  orgs: Org[];
  saving: string;
  removing: string;
  checking: string;
  checks: Record<string, SandboxCheck>;
  onSave: (id: string, input: SandboxInput) => void;
  onRemove: (id: string) => void;
  onCheck: (id: string) => void;
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
          checking={checking === (owner.id || "personal")}
          answer={checks[owner.id]}
          onSave={onSave}
          onRemove={onRemove}
          onCheck={onCheck}
        />
      ))}
    </div>
  );
}
