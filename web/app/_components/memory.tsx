import { BookIcon, PuzzleIcon } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty";
import { Item, ItemContent, ItemDescription, ItemGroup, ItemMedia, ItemTitle } from "@/components/ui/item";

import type { Fact, Skill } from "./types";

export function Memory({ facts, skills }: { facts: Fact[]; skills: Skill[] }) {
  return (
    <div className="mx-auto flex w-full max-w-3xl flex-col gap-6 p-4">
      <section className="flex flex-col gap-3">
        <h2 className="text-sm font-semibold">Skills instaladas</h2>
        {skills.length === 0 ? (
          <Empty className="border">
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <PuzzleIcon />
              </EmptyMedia>
              <EmptyTitle>No hay ninguna instalada</EmptyTitle>
              <EmptyDescription>Las skills son instrucciones que el agente carga cuando la tarea las pide.</EmptyDescription>
            </EmptyHeader>
          </Empty>
        ) : (
          <ItemGroup>
            {skills.map((skill) => (
              <Item key={skill.name} variant="outline" size="sm">
                <ItemMedia variant="icon">
                  <PuzzleIcon />
                </ItemMedia>
                <ItemContent>
                  <ItemTitle>
                    {skill.name}
                    <Badge variant="outline">
                      {skill.owner}
                      {skill.role ? ` · ${skill.role}` : ""}
                    </Badge>
                  </ItemTitle>
                  <ItemDescription>{skill.description}</ItemDescription>
                </ItemContent>
              </Item>
            ))}
          </ItemGroup>
        )}
      </section>

      <section className="flex flex-col gap-3">
        <h2 className="text-sm font-semibold">Memoria</h2>
        {facts.length === 0 ? (
          <Empty className="border">
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <BookIcon />
              </EmptyMedia>
              <EmptyTitle>No hay hechos</EmptyTitle>
              <EmptyDescription>Lo que valga la pena recordar va a quedar acá.</EmptyDescription>
            </EmptyHeader>
          </Empty>
        ) : (
          facts.map((fact) => (
            <Card key={fact.key} size="sm" data-fact={fact.key}>
              <CardHeader>
                <CardTitle className="font-normal">{fact.key}</CardTitle>
                <CardDescription>
                  <Badge variant="outline">{fact.kind}</Badge> {day(fact.date)}
                </CardDescription>
              </CardHeader>
              <CardContent className="whitespace-pre-wrap">{fact.body}</CardContent>
            </Card>
          ))
        )}
      </section>
    </div>
  );
}

function day(days: number) {
  return new Date(days * 86400000).toISOString().slice(0, 10);
}
