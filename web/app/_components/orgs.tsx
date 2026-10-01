import { UsersIcon } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty";
import { Input } from "@/components/ui/input";
import { Item, ItemActions, ItemContent, ItemDescription, ItemGroup, ItemTitle } from "@/components/ui/item";

import type { Member, Org } from "./types";

const ROLES = [
  { value: "owner", label: "dueño" },
  { value: "admin", label: "admin" },
  { value: "member", label: "miembro" },
];

function roleName(role: string) {
  return ROLES.find((one) => one.value === role)?.label ?? role;
}

const roleSelect = "h-7 rounded-md border bg-transparent px-1 text-xs";

export function Orgs({
  orgs,
  members,
  removing,
  onNew,
  onInvite,
  onRole,
  onRemove,
  onAsk,
}: {
  orgs: Org[];
  members: Record<string, Member[]>;
  removing: string;
  onNew: (event: React.FormEvent<HTMLFormElement>) => void;
  onInvite: (org: string, event: React.FormEvent<HTMLFormElement>) => void;
  onRole: (org: string, user: string, role: string) => void;
  onRemove: (org: string, user: string) => void;
  onAsk: (path: string, id: string) => void;
}) {
  return (
    <div className="mx-auto flex w-full max-w-3xl flex-col gap-6 p-4">
      <section className="flex flex-col gap-3">
        <h2 className="text-sm font-semibold">Organizaciones</h2>
        <form onSubmit={onNew} className="flex gap-1.5">
          <Input name="name" data-new-org="" placeholder="Nueva organización" className="h-8" />
          <Button type="submit" size="sm" variant="outline">
            Crear
          </Button>
        </form>
        {orgs.length === 0 ? (
          <Empty className="border">
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <UsersIcon />
              </EmptyMedia>
              <EmptyTitle>No estás en ninguna</EmptyTitle>
              <EmptyDescription>
                Una organización es de varias personas: sus proyectos, su memoria y su agenda son del equipo.
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
        ) : null}
      </section>

      {orgs.map((org) => {
        const people = members[org.id];
        const canInvite = org.role === "owner" || org.role === "admin";
        return (
          <Card key={org.id} size="sm" data-org={org.id}>
            <CardHeader>
              <CardTitle className="font-normal">
                <span data-org-open={org.id}>{org.name}</span>
                <Badge variant="outline">{roleName(org.role)}</Badge>
              </CardTitle>
              <CardDescription>
                {org.slug}
                {org.role === "owner" ? (
                  <>
                    {" · "}
                    <button
                      type="button"
                      onClick={() => onAsk("/api/orgs", org.id)}
                      data-org-delete={org.id}
                      className={removing === org.id ? "text-destructive" : "underline"}
                    >
                      {removing === org.id ? "otra vez para borrar todo" : "borrar la organización"}
                    </button>
                  </>
                ) : null}
              </CardDescription>
            </CardHeader>
            <CardContent className="flex flex-col gap-3">
              <ItemGroup>
                {(people ?? []).map((member) => (
                  <Item key={member.user_id} size="sm" variant="outline" data-member={member.user_id}>
                    <ItemContent>
                      <ItemTitle>{member.name || member.email}</ItemTitle>
                      <ItemDescription>{member.email}</ItemDescription>
                    </ItemContent>
                    <ItemActions>
                      {org.role === "owner" ? (
                        <select
                          value={member.role}
                          data-member-role={member.user_id}
                          aria-label={`Rol de ${member.email}`}
                          onChange={(event) => onRole(org.id, member.user_id, event.target.value)}
                          className={roleSelect}
                        >
                          {ROLES.map((one) => (
                            <option key={one.value} value={one.value}>
                              {one.label}
                            </option>
                          ))}
                        </select>
                      ) : (
                        <Badge variant="outline">{roleName(member.role)}</Badge>
                      )}
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        data-member-remove={member.user_id}
                        onClick={() => onRemove(org.id, member.user_id)}
                      >
                        sacar
                      </Button>
                    </ItemActions>
                  </Item>
                ))}
              </ItemGroup>
              {canInvite ? (
                <form onSubmit={(event) => onInvite(org.id, event)} className="flex gap-1.5">
                  <Input name="email" data-invite={`${org.id}`} placeholder="mail de quien invitás" className="h-8" />
                  <select name="role" defaultValue="member" aria-label="Rol" className={`${roleSelect} h-8`}>
                    {ROLES.filter((one) => org.role === "owner" || one.value !== "owner").map((one) => (
                      <option key={one.value} value={one.value}>
                        {one.label}
                      </option>
                    ))}
                  </select>
                  <Button type="submit" size="sm" variant="outline">
                    Invitar
                  </Button>
                </form>
              ) : null}
            </CardContent>
          </Card>
        );
      })}
    </div>
  );
}
