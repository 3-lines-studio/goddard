import { MailIcon } from "lucide-react";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";

export function Login({
  sent,
  link,
  error,
  onSend,
}: {
  sent: string;
  link: string;
  error: string;
  onSend: (event: React.FormEvent<HTMLFormElement>) => void;
}) {
  return (
    <div className="flex min-h-dvh items-center justify-center p-6">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <CardTitle>Goddard</CardTitle>
          <CardDescription>Entrá con tu mail y te mando un link.</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          {sent ? (
            <div className="flex flex-col gap-2 text-sm">
              <p>Si el mail está en la lista, te llegó un link a {sent}.</p>
              {link ? (
                <a href={link} data-login-link="" className="break-all underline">
                  {link}
                </a>
              ) : null}
            </div>
          ) : (
            <form onSubmit={onSend} data-login-form="">
              <FieldGroup>
                <Field>
                  <FieldLabel htmlFor="email">Mail</FieldLabel>
                  <Input
                    id="email"
                    name="email"
                    type="email"
                    data-login-email=""
                    placeholder="tu@mail"
                    autoComplete="email"
                    required
                  />
                  <FieldDescription>El link dura un rato y sirve una sola vez.</FieldDescription>
                </Field>
                <Field>
                  <Button type="submit" data-login-send="">
                    <MailIcon data-icon="inline-start" />
                    Mandame el link
                  </Button>
                </Field>
              </FieldGroup>
            </form>
          )}
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
