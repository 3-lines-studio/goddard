import { useEffect, useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";

import type { Model, ModelInput, Org } from "./types";

const ROLES: Record<string, string> = {
  owner: "dueño",
  admin: "admin",
  member: "miembro",
};

// Choice is the model of one owner: the one it brought, or the one of the
// house. The key never comes back from the app, so the box starts empty, and an
// empty box keeps the one that is already there.
function Choice({
  id,
  name,
  role,
  model,
  saving,
  removing,
  onSave,
  onRemove,
}: {
  id: string;
  name: string;
  role: string;
  model: Model | undefined;
  saving: boolean;
  removing: boolean;
  onSave: (id: string, input: ModelInput) => void;
  onRemove: (id: string) => void;
}) {
  const [base, setBase] = useState(model?.base ?? "");
  const [modelName, setModelName] = useState(model?.name ?? "");
  const [key, setKey] = useState("");
  const writable = role === "owner" || role === "admin";

  useEffect(() => {
    if (!model) return;
    setBase(model.base);
    setModelName(model.name);
    setKey("");
  }, [model]);

  return (
    <Card size="sm" data-model={id}>
      <CardHeader>
        <CardTitle className="font-normal">
          <span data-model-owner={id}>{name}</span>
          {id ? <Badge variant="outline">{ROLES[role] ?? role}</Badge> : null}
        </CardTitle>
        <CardDescription>
          {!model ? "leyendo…" : model.own ? model.name : `el de la casa: ${model.house}`}
        </CardDescription>
      </CardHeader>
      <CardContent>
        {!model ? null : writable ? (
          <form
            data-model-form={id}
            onSubmit={(event) => {
              event.preventDefault();
              onSave(id, { base, name: modelName, key });
            }}
          >
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor={`model-base-${id}`}>dirección</FieldLabel>
                <Input
                  id={`model-base-${id}`}
                  name="base"
                  value={base}
                  data-model-base={id}
                  placeholder="https://api.deepseek.com"
                  onChange={(event) => setBase(event.target.value)}
                />
                <FieldDescription>La dirección que habla el protocolo de OpenAI.</FieldDescription>
              </Field>
              <Field>
                <FieldLabel htmlFor={`model-name-${id}`}>modelo</FieldLabel>
                <Input
                  id={`model-name-${id}`}
                  name="name"
                  value={modelName}
                  data-model-name={id}
                  placeholder={model.house}
                  onChange={(event) => setModelName(event.target.value)}
                />
                <FieldDescription>El nombre que se le pide, tal como lo conoce esa dirección.</FieldDescription>
              </Field>
              <Field>
                <FieldLabel htmlFor={`model-key-${id}`}>clave</FieldLabel>
                <Textarea
                  id={`model-key-${id}`}
                  name="key"
                  rows={2}
                  value={key}
                  data-model-key={id}
                  placeholder={model.has_key ? "la que ya está" : "sin clave"}
                  className="font-mono text-xs"
                  onChange={(event) => setKey(event.target.value)}
                />
                <FieldDescription>
                  {model.has_key
                    ? "Ya hay una cargada. Escribí otra para reemplazarla, o dejá el casillero vacío."
                    : "La clave con la que esa dirección deja pasar."}
                </FieldDescription>
              </Field>
              <Field orientation="horizontal">
                <Button type="submit" size="sm" variant="outline" data-model-save={id}>
                  {saving ? "guardando…" : "Guardar"}
                </Button>
                {model.own ? (
                  <Button
                    type="button"
                    size="sm"
                    variant="ghost"
                    data-model-remove={id}
                    className="text-muted-foreground"
                    onClick={() => onRemove(id)}
                  >
                    {removing ? "otra vez para volver" : "volver al de la casa"}
                  </Button>
                ) : null}
              </Field>
            </FieldGroup>
          </form>
        ) : (
          <p className="text-sm text-muted-foreground">
            Esto lo carga un dueño o un admin. Ahora contesta {model.own ? model.name : `el de la casa: ${model.house}`}
            {model.own && !model.has_key ? ", y todavía no tiene clave" : ""}.
          </p>
        )}
      </CardContent>
    </Card>
  );
}

export function Models({
  models,
  orgs,
  saving,
  removing,
  onSave,
  onRemove,
}: {
  models: Record<string, Model>;
  orgs: Org[];
  saving: string;
  removing: string;
  onSave: (id: string, input: ModelInput) => void;
  onRemove: (id: string) => void;
}) {
  const owners = [{ id: "", name: "Personal", role: "owner" }, ...orgs];
  return (
    <div className="mx-auto flex w-full max-w-3xl flex-col gap-6 p-4">
      <section className="flex flex-col gap-1">
        <h2 className="text-sm font-semibold">Modelo</h2>
        <p className="text-sm text-muted-foreground">
          Con qué contesta el agente. Sin cargar nada contesta el de la casa; si traés el tuyo, sus turnos van por ahí y
          la clave queda guardada con el resto de los secretos.
        </p>
      </section>
      {owners.map((owner) => (
        <Choice
          key={owner.id || "personal"}
          id={owner.id}
          name={owner.name}
          role={owner.role}
          model={models[owner.id]}
          saving={saving === (owner.id || "personal")}
          removing={removing === (owner.id || "personal")}
          onSave={onSave}
          onRemove={onRemove}
        />
      ))}
    </div>
  );
}
