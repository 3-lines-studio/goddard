import { CalendarClockIcon, ChevronRightIcon, XIcon } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
} from "@/components/ui/card";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty";
import { cn } from "@/lib/utils";

import { markdown } from "../_lib/markdown";
import type { Project, Task } from "./types";

export function Agenda({
  tasks,
  projects,
  open,
  pending,
  removing,
  unread,
  onToggle,
  onRun,
  onPause,
  onAsk,
  onReadAll,
}: {
  tasks: Task[];
  projects: Project[];
  open: Set<string>;
  pending: string;
  removing: string;
  unread: number;
  onToggle: (task: Task) => void;
  onRun: (task: Task) => void;
  onPause: (task: Task) => void;
  onAsk: (task: Task) => void;
  onReadAll: () => void;
}) {
  if (tasks.length === 0) {
    return (
      <Empty className="mx-auto max-w-3xl border">
        <EmptyHeader>
          <EmptyMedia variant="icon">
            <CalendarClockIcon />
          </EmptyMedia>
          <EmptyTitle>No hay tareas agendadas</EmptyTitle>
          <EmptyDescription>Las que agendes van a aparecer acá.</EmptyDescription>
        </EmptyHeader>
      </Empty>
    );
  }
  return (
    <div className="mx-auto flex w-full max-w-3xl flex-col gap-3 p-4">
      {tasks.map((task) => (
        <TaskCard
          key={keyOf(task)}
          task={task}
          project={nameOfProject(projects, task.project)}
          open={open.has(keyOf(task))}
          running={pending === keyOf(task)}
          removing={removing === keyOf(task)}
          onToggle={() => onToggle(task)}
          onRun={() => onRun(task)}
          onPause={() => onPause(task)}
          onAsk={() => onAsk(task)}
        />
      ))}
      {unread > 0 ? (
        <Button type="button" variant="outline" size="sm" onClick={onReadAll} data-read-all="" className="self-start">
          marcar todo leído
        </Button>
      ) : null}
    </div>
  );
}

function TaskCard({
  task,
  project,
  open,
  running,
  removing,
  onToggle,
  onRun,
  onPause,
  onAsk,
}: {
  task: Task;
  project: string;
  open: boolean;
  running: boolean;
  removing: boolean;
  onToggle: () => void;
  onRun: () => void;
  onPause: () => void;
  onAsk: () => void;
}) {
  const key = keyOf(task);
  const last = task.runs[0];
  const state = running ? "running" : !last ? "" : last.ok ? "done" : "error";
  return (
    <Collapsible render={<Card size="sm" data-task={key} className="group/task" />}>
      <CardHeader>
        <CollapsibleTrigger
          render={
            <Button
              type="button"
              variant="ghost"
              size="sm"
              data-task-toggle={key}
              className="h-auto w-full justify-start gap-2 px-1 font-normal"
            />
          }
        >
          <span
            data-task-state={state}
            className={cn(
              "size-2 shrink-0 rounded-full",
              state === "running" && "bg-foreground",
              state === "done" && "bg-ok",
              state === "error" && "bg-destructive",
              state === "" && "border border-muted-foreground",
            )}
          />
          <span className="font-semibold">{task.name}</span>
          <span className="truncate text-xs text-muted-foreground">
            {project}
            {" · "}
            {whenOf(task)}
          </span>
          {task.unread > 0 ? (
            <Badge variant="secondary" data-task-unread={key}>
              {task.unread}
            </Badge>
          ) : null}
          {task.paused ? <Badge variant="outline">pausada</Badge> : null}
          {task.silent ? <Badge variant="outline">callada</Badge> : null}
          <ChevronRightIcon className="ms-1 shrink-0 text-muted-foreground transition-transform group-data-open/task:rotate-90" />
        </CollapsibleTrigger>
        <CardAction className="flex items-center gap-1">
          <Button type="button" variant="outline" size="xs" onClick={onRun} disabled={running} data-run-task={key}>
            {running ? "corriendo…" : "correr"}
          </Button>
          <Button type="button" variant="outline" size="xs" onClick={onPause} data-pause-task={key}>
            {task.paused ? "seguir" : "pausar"}
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="icon-xs"
            onClick={onAsk}
            data-remove-task={key}
            aria-label={`Quitar ${task.name}`}
            className={cn(removing && "text-destructive")}
          >
            <XIcon />
          </Button>
        </CardAction>
        <CardDescription className="px-1">{task.prompt}</CardDescription>
      </CardHeader>
      {open ? (
        <CollapsibleContent>
          <CardContent className="border-t pt-3">
            {task.runs.length === 0 ? (
              <p className="text-xs text-muted-foreground">Todavía no corrió.</p>
            ) : null}
            {task.runs.map((one) => (
              <div key={one.ts} className="mt-2 first:mt-0">
                <span className="text-xs text-muted-foreground" title={`hace ${ago(one.ts)} · ${one.ms} ms`}>
                  {moment(one.date, one.ts)}
                  {one.ok ? "" : " · con error"}
                </span>
                <div
                  className={cn("md text-sm", one.ok ? "text-muted-foreground" : "text-destructive")}
                  dangerouslySetInnerHTML={{ __html: markdown(one.text.trim() || "sin novedades") }}
                />
              </div>
            ))}
          </CardContent>
        </CollapsibleContent>
      ) : null}
    </Collapsible>
  );
}

export function keyOf(task: Task) {
  return `${task.project}/${task.name}`;
}

function nameOfProject(projects: Project[], slug: string) {
  return projects.find((one) => one.slug === slug)?.name ?? slug;
}

function moment(date: string, ts: number) {
  const at = new Date(ts * 1000);
  const clock = `${String(at.getHours()).padStart(2, "0")}:${String(at.getMinutes()).padStart(2, "0")}`;
  const today = new Date();
  const day = (one: Date) =>
    `${one.getFullYear()}-${String(one.getMonth() + 1).padStart(2, "0")}-${String(one.getDate()).padStart(2, "0")}`;
  if (date === day(today)) return `hoy ${clock}`;
  if (date === day(new Date(today.getTime() - 86_400_000))) return `ayer ${clock}`;
  return `${date} ${clock}`;
}

function whenOf(task: Task) {
  if (task.every) return `cada ${task.every}`;
  if (task.at) return `todos los días a las ${task.at}`;
  if (task.when) return `una vez el ${task.when.replace("T", " a las ")}`;
  return "sin horario";
}

function ago(ts: number) {
  const seconds = Math.max(0, Math.floor(Date.now() / 1000) - ts);
  if (seconds < 60) return `${seconds} s`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)} min`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)} h`;
  return `${Math.floor(seconds / 86400)} d`;
}
