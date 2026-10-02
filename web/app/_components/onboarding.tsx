import { useEffect, useState } from "react";
import { LaptopIcon, SparklesIcon } from "lucide-react";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";

import type { SandboxCheck, Setup } from "./types";

const STEPS = ["vos", "el modelo", "la computadora"];

// Onboarding is the first setup: what goddard calls somebody, the model it
// answers with and the machine its tools run in. The name is a courtesy and
// the other two are not — without a machine and a model there is no turn — and
// what it writes is what the panels write, so the same person can come back and
// change its mind from there.
export function Onboarding() {
  const [setup, setSetup] = useState<Setup | null>(null);
  const [step, setStep] = useState(0);
  const [name, setName] = useState("");
  const [base, setBase] = useState("");
  const [modelName, setModelName] = useState("");
  const [modelKey, setModelKey] = useState("");
  const [path, setPath] = useState("");
  const [addr, setAddr] = useState("");
  const [user, setUser] = useState("");
  const [key, setKey] = useState("");
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [answer, setAnswer] = useState("");

  useEffect(() => {
    void load();
  }, []);

  async function load() {
    const response = await fetch("/api/onboarding");
    if (!response.ok) {
      setError(await response.text());
      return;
    }
    const found: Setup = await response.json();
    setSetup(found);
    setName(found.user.name);
    setBase(found.model.base);
    setModelName(found.model.name);
    setPath(found.compute.path);
    setAddr(found.compute.addr);
    setUser(found.compute.user);
  }

  async function call(target: string, method: string, body?: unknown) {
    setBusy(target + method);
    setError("");
    const response = await fetch(target, {
      method,
      headers: { "content-type": "application/json" },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    setBusy("");
    if (!response.ok) {
      setError(await response.text());
      return null;
    }
    return response;
  }

  async function saveName(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (await call("/api/me", "PATCH", { name })) setStep(1);
  }

  async function useHouse() {
    if (await call("/api/model", "DELETE")) setStep(2);
  }

  async function saveModel(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!(await call("/api/model", "POST", { base, name: modelName, key: modelKey }))) return;
    setModelKey("");
    setStep(2);
  }

  async function saveMachine(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setAnswer("");
    if (!(await call("/api/workspace", "POST", { path, addr, user, key }))) return;
    const response = await call("/api/workspace/check", "POST", { org: "" });
    if (!response) return;
    const found: SandboxCheck = await response.json();
    if (!found.ok) {
      setAnswer(found.message);
      return;
    }
    window.location.href = "/";
  }

  return (
    <div className="flex min-h-dvh items-center justify-center p-6" data-onboarding="">
      <Card className="w-full max-w-lg">
        <CardHeader>
          <CardTitle>Goddard</CardTitle>
          <CardDescription>
            {step + 1} de {STEPS.length}: {STEPS[step]}.
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          {setup === null ? (
            <p className="text-sm text-muted-foreground">Leyendo…</p>
          ) : step === 0 ? (
            <form onSubmit={saveName} data-onboarding-form="vos">
              <FieldGroup>
                <Field>
                  <FieldLabel htmlFor="name">¿Cómo te llamamos?</FieldLabel>
                  <Input
                    id="name"
                    name="name"
                    value={name}
                    data-onboarding-name=""
                    autoComplete="name"
                    onChange={(event) => setName(event.target.value)}
                  />
                  <FieldDescription>Es el nombre con el que te habla el bot. El mail es {setup.user.email}.</FieldDescription>
                </Field>
                <Field orientation="horizontal">
                  <Button type="submit" data-onboarding-next="vos" disabled={busy !== ""}>
                    {busy ? "guardando…" : "Seguir"}
                  </Button>
                  <Button type="button" variant="ghost" data-onboarding-skip="" onClick={() => setStep(1)}>
                    Saltear
                  </Button>
                </Field>
              </FieldGroup>
            </form>
          ) : step === 1 ? (
            <FieldGroup data-onboarding-form="modelo">
              <Field>
                <Card size="sm" data-onboarding-house="">
                  <CardHeader>
                    <CardTitle className="font-normal">
                      El de la casa <Badge variant="outline">incluido</Badge>
                    </CardTitle>
                    <CardDescription>
                      {setup.model.house}, el que ya viene con goddard. No hay que cargar nada.
                    </CardDescription>
                  </CardHeader>
                  <CardContent>
                    <Button type="button" variant="outline" data-onboarding-use-house="" onClick={useHouse} disabled={busy !== ""}>
                      <SparklesIcon data-icon="inline-start" />
                      Usar el de la casa
                    </Button>
                  </CardContent>
                </Card>
              </Field>
              <Field>
                <Card size="sm" data-onboarding-own="">
                  <CardHeader>
                    <CardTitle className="font-normal">El mío</CardTitle>
                    <CardDescription>Tu proveedor y tu clave: lo que gaste lo pagás vos.</CardDescription>
                  </CardHeader>
                  <CardContent>
                    <form onSubmit={saveModel} data-onboarding-form="modelo-propio">
                      <FieldGroup>
                        <Field>
                          <FieldLabel htmlFor="base">dirección</FieldLabel>
                          <Input
                            id="base"
                            name="base"
                            value={base}
                            data-onboarding-base=""
                            placeholder="https://api.deepseek.com"
                            onChange={(event) => setBase(event.target.value)}
                          />
                        </Field>
                        <Field>
                          <FieldLabel htmlFor="model">modelo</FieldLabel>
                          <Input
                            id="model"
                            name="model"
                            value={modelName}
                            data-onboarding-model=""
                            placeholder="deepseek-flash"
                            onChange={(event) => setModelName(event.target.value)}
                          />
                        </Field>
                        <Field>
                          <FieldLabel htmlFor="model-key">clave</FieldLabel>
                          <Textarea
                            id="model-key"
                            name="key"
                            rows={2}
                            value={modelKey}
                            data-onboarding-key=""
                            className="font-mono text-xs"
                            placeholder={setup.model.has_key ? "la que ya está" : "sk-…"}
                            onChange={(event) => setModelKey(event.target.value)}
                          />
                        </Field>
                        <Field>
                          <Button type="submit" variant="outline" data-onboarding-save-model="" disabled={busy !== ""}>
                            {busy ? "guardando…" : "Usar el mío"}
                          </Button>
                        </Field>
                      </FieldGroup>
                    </form>
                  </CardContent>
                </Card>
              </Field>
              <Field>
                <Button type="button" variant="ghost" data-onboarding-skip-model="" onClick={() => setStep(2)}>
                  Seguir con el de la casa
                </Button>
              </Field>
            </FieldGroup>
          ) : (
            <form onSubmit={saveMachine} data-onboarding-form="computadora">
              <FieldGroup>
                <Field>
                  <FieldLabel htmlFor="path">volumen</FieldLabel>
                  <Input
                    id="path"
                    name="path"
                    value={path}
                    data-onboarding-path=""
                    onChange={(event) => setPath(event.target.value)}
                  />
                  <FieldDescription>
                    La carpeta donde viven los proyectos, ya montada en la máquina: goddard la usa tal cual y no la
                    crea.
                  </FieldDescription>
                </Field>
                <Field>
                  <FieldLabel htmlFor="addr">dirección</FieldLabel>
                  <Input
                    id="addr"
                    name="addr"
                    value={addr}
                    data-onboarding-addr=""
                    placeholder="sandbox.local:22"
                    onChange={(event) => setAddr(event.target.value)}
                  />
                  <FieldDescription>La máquina que corre las herramientas, y el puerto del ssh.</FieldDescription>
                </Field>
                <Field>
                  <FieldLabel htmlFor="user">usuario</FieldLabel>
                  <Input
                    id="user"
                    name="user"
                    value={user}
                    data-onboarding-user=""
                    placeholder="goddard"
                    onChange={(event) => setUser(event.target.value)}
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor="ssh-key">llave privada</FieldLabel>
                  <Textarea
                    id="ssh-key"
                    name="key"
                    rows={4}
                    value={key}
                    data-onboarding-ssh-key=""
                    className="font-mono text-xs"
                    placeholder={setup.compute.has_key ? "la que ya está" : "el archivo entero"}
                    onChange={(event) => setKey(event.target.value)}
                  />
                  <FieldDescription>La que entra a la máquina sin pedir contraseña.</FieldDescription>
                </Field>
                <Field>
                  <Button type="submit" data-onboarding-check="" disabled={busy !== ""}>
                    <LaptopIcon data-icon="inline-start" />
                    {busy ? "probando…" : "Guardar y probar"}
                  </Button>
                </Field>
              </FieldGroup>
            </form>
          )}
          {answer ? (
            <Alert variant="destructive">
              <AlertDescription data-onboarding-answer="">{answer}</AlertDescription>
            </Alert>
          ) : null}
          {error ? (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          ) : null}
        </CardContent>
      </Card>
    </div>
  );
}
