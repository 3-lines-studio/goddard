import { Fragment, useState } from "react";
import {
  BrainIcon,
  CalendarClockIcon,
  ChevronDownIcon,
  CpuIcon,
  EllipsisIcon,
  HardDriveIcon,
  LogOutIcon,
  MessageSquarePlusIcon,
  MoonIcon,
  PencilIcon,
  PlusIcon,
  SunIcon,
  TrashIcon,
  UsersIcon,
  XIcon,
} from "lucide-react";

import { Avatar as Frame, AvatarFallback } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuAction,
  SidebarMenuBadge,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar";
import { cn } from "@/lib/utils";

import { machinePercent, machineRatio, machineSize } from "../_lib/machine";
import { Avatar } from "./avatar";
import type { Machine, Org, Project, Tab } from "./types";
import { VISIBLE } from "./types";

export function Rail({
  projects,
  orgs,
  tab,
  unread,
  user,
  machine,
  workspace,
  theme,
  collapsed,
  more,
  editing,
  removing,
  avatarState,
  onNewProject,
  onOpenProject,
  onNewConversation,
  onOpenMemory,
  onToggleProject,
  onToggleMore,
  onEditing,
  onAsk,
  onRename,
  onOpenTab,
  onOpenAgenda,
  onOpenOrgs,
  onToggleTheme,
  onLogout,
}: {
  projects: Project[];
  orgs: Org[];
  tab: Tab | null;
  unread: number;
  user: string;
  machine: Machine | null;
  workspace: string;
  theme: string;
  collapsed: Set<string>;
  more: Set<string>;
  editing: string;
  removing: string;
  avatarState: (project: Project) => string;
  onNewProject: (event: React.FormEvent<HTMLFormElement>) => void;
  onOpenProject: (project: Project) => void;
  onNewConversation: (project: Project) => void;
  onOpenMemory: (project: Project) => void;
  onToggleProject: (slug: string) => void;
  onToggleMore: (slug: string) => void;
  onEditing: (id: string) => void;
  onAsk: (path: string, id: string) => void;
  onRename: (path: string, body: Record<string, string>) => void;
  onOpenTab: (key: string) => void;
  onOpenAgenda: () => void;
  onOpenOrgs: () => void;
  onToggleTheme: () => void;
  onLogout: () => void;
}) {
  const groups = [
    { id: "", name: "Personal", projects: projects.filter((one) => one.owner.kind !== "org") },
    ...orgs.map((org) => ({
      id: org.id,
      name: org.name,
      projects: projects.filter((one) => one.owner.kind === "org" && one.owner.id === org.id),
    })),
  ].filter((group) => group.projects.length > 0);

  return (
    <Sidebar>
      <SidebarHeader>
        <form onSubmit={onNewProject} className="flex gap-1.5">
          <Input name="name" data-new-project="" placeholder="Nuevo proyecto" className="h-8" />
          <select
            name="org"
            data-new-project-org=""
            aria-label="De quién es el proyecto"
            className="h-8 max-w-24 rounded-md border bg-transparent px-1 text-xs"
          >
            <option value="">Personal</option>
            {orgs
              .filter((org) => org.role === "owner")
              .map((org) => (
                <option key={org.id} value={org.id}>
                  {org.name}
                </option>
              ))}
          </select>
          <Button type="submit" variant="outline" size="icon" aria-label="Agregar proyecto">
            <PlusIcon />
          </Button>
        </form>
      </SidebarHeader>

      <SidebarContent>
        {groups.map((group) => (
          <Fragment key={group.id || "personal"}>
            <SidebarGroupLabel data-owner={group.id}>{group.name}</SidebarGroupLabel>
            {group.projects.map((project) => {
          const threads = project.conversations ?? [];
          const shut = collapsed.has(project.slug);
          const shown = more.has(project.slug) ? threads : threads.slice(0, VISIBLE);
          return (
            <SidebarGroup key={project.id}>
              <div className="flex items-center gap-1.5 px-1">
                <Frame size="sm">
                  <AvatarFallback className="bg-transparent">
                    <Avatar name={project.slug} state={avatarState(project)} size={22} />
                  </AvatarFallback>
                </Frame>
                {editing === project.id ? (
                  <Name
                    value={project.name}
                    save={(name) => onRename("/api/projects", { id: project.id, name })}
                    cancel={() => onEditing("")}
                  />
                ) : (
                  <Button
                    type="button"
                    variant="ghost"
                    size="xs"
                    onClick={() => onOpenProject(project)}
                    data-project={project.id}
                    className="min-w-0 flex-1 justify-start px-1 font-semibold"
                  >
                    <span className="truncate">{project.name}</span>
                  </Button>
                )}
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-xs"
                  onClick={() => onToggleProject(project.slug)}
                  data-toggle-project={project.id}
                  aria-label={shut ? `Mostrar los hilos de ${project.name}` : `Ocultar los hilos de ${project.name}`}
                >
                  <ChevronDownIcon className={cn("transition-transform", shut && "-rotate-90")} />
                </Button>
                <DropdownMenu>
                  <DropdownMenuTrigger
                    render={
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon-xs"
                        data-project-menu={project.id}
                        aria-label={`Opciones de ${project.name}`}
                      />
                    }
                  >
                    <EllipsisIcon />
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="start" className="w-48">
                    <DropdownMenuGroup>
                      <DropdownMenuItem
                        data-new-conversation={project.id}
                        onClick={() => onNewConversation(project)}
                      >
                        <MessageSquarePlusIcon />
                        nueva conversación
                      </DropdownMenuItem>
                      <DropdownMenuItem data-memory={project.id} onClick={() => onOpenMemory(project)}>
                        <BrainIcon />
                        memoria
                      </DropdownMenuItem>
                      <DropdownMenuItem data-rename-project={project.id} onClick={() => onEditing(project.id)}>
                        <PencilIcon />
                        renombrar
                      </DropdownMenuItem>
                    </DropdownMenuGroup>
                    <DropdownMenuSeparator />
                    <DropdownMenuGroup>
                      <DropdownMenuItem
                        variant="destructive"
                        data-delete-project={project.id}
                        onClick={() => onAsk("/api/projects", project.id)}
                      >
                        <TrashIcon />
                        {removing === project.id ? "otra vez para borrar" : "quitar"}
                      </DropdownMenuItem>
                    </DropdownMenuGroup>
                  </DropdownMenuContent>
                </DropdownMenu>
              </div>

              {shut ? null : (
                <SidebarGroupContent>
                  <SidebarMenu>
                    {shown.map((line) => (
                      <SidebarMenuItem key={line.id}>
                        {editing === line.id ? (
                          <Name
                            value={line.title}
                            save={(title) => onRename("/api/conversations", { id: line.id, title })}
                            cancel={() => onEditing("")}
                          />
                        ) : (
                          <SidebarMenuButton
                            size="sm"
                            isActive={tab?.key === line.id}
                            onClick={() => onOpenTab(line.id)}
                            data-conversation={line.id}
                            tooltip={line.title}
                            className="group-has-data-[sidebar=menu-action]/menu-item:pr-12"
                          >
                            <span className="truncate">
                              {line.title}
                              {line.running ? <span className="ml-1 opacity-60">·</span> : null}
                            </span>
                          </SidebarMenuButton>
                        )}
                        <SidebarMenuAction
                          showOnHover
                          aria-label={`Renombrar ${line.title}`}
                          data-rename-conversation={line.id}
                          onClick={() => onEditing(line.id)}
                          className="right-7"
                        >
                          <PencilIcon />
                        </SidebarMenuAction>
                        <SidebarMenuAction
                          showOnHover
                          aria-label={`Borrar ${line.title}`}
                          title={removing === line.id ? "otra vez para borrar" : `Borrar ${line.title}`}
                          data-delete-conversation={line.id}
                          onClick={() => onAsk("/api/conversations", line.id)}
                          className={cn(removing === line.id && "text-destructive")}
                        >
                          <XIcon />
                        </SidebarMenuAction>
                      </SidebarMenuItem>
                    ))}
                  </SidebarMenu>
                  {threads.length > VISIBLE ? (
                    <Button
                      type="button"
                      variant="ghost"
                      size="xs"
                      onClick={() => onToggleMore(project.slug)}
                      data-more-threads={project.slug}
                      className="mt-1 text-muted-foreground"
                    >
                      {more.has(project.slug) ? "ver menos" : `ver más (${threads.length - VISIBLE})`}
                    </Button>
                  ) : null}
                </SidebarGroupContent>
              )}
            </SidebarGroup>
            );
          })}
          </Fragment>
        ))}
        {projects.length === 0 ? (
          <p className="px-3 py-2 text-sm text-muted-foreground">Todavía no hay proyectos.</p>
        ) : null}
      </SidebarContent>

      <SidebarFooter>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton isActive={tab?.kind === "agenda"} onClick={onOpenAgenda} data-agenda="" tooltip="Agenda">
              <CalendarClockIcon />
              <span>Agenda</span>
            </SidebarMenuButton>
            {unread > 0 ? <SidebarMenuBadge data-agenda-badge="">{unread}</SidebarMenuBadge> : null}
          </SidebarMenuItem>
          <SidebarMenuItem>
            <SidebarMenuButton isActive={tab?.kind === "orgs"} onClick={onOpenOrgs} data-orgs="" tooltip="Organizaciones">
              <UsersIcon />
              <span>Organizaciones</span>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>

        {machine ? (
          <div className="flex items-center justify-between gap-2 px-2 text-xs" data-machine="">
            <span
              data-machine-memory=""
              className="flex items-center gap-1 text-muted-foreground"
              title={`${machineRatio(machine.memory.used, machine.memory.total)} · la memoria del contenedor, cache y kernel incluidos (anónima ${machineSize(
                machine.memory.anon,
              )}, cache ${machineSize(machine.memory.cache)}, kernel ${machineSize(machine.memory.kernel)})`}
            >
              <CpuIcon />
              {machineSize(machine.memory.used)}
            </span>
            {machine.disk.total > 0 ? (
              <span
                data-machine-disk=""
                className="flex items-center gap-1 text-muted-foreground"
                title={`${machineRatio(machine.disk.used, machine.disk.total)} · el volumen ${workspace}`}
              >
                <HardDriveIcon />
                {machinePercent(machine.disk.used, machine.disk.total)}
              </span>
            ) : null}
          </div>
        ) : null}

        <div className="flex items-center justify-between gap-2 px-2 text-xs">
          <span className="truncate text-muted-foreground">{user}</span>
          <span className="flex shrink-0 items-center gap-1">
            <Button
              type="button"
              variant="ghost"
              size="icon-xs"
              onClick={onToggleTheme}
              data-theme-toggle=""
              aria-label={theme === "dark" ? "Tema claro" : "Tema oscuro"}
            >
              {theme === "dark" ? <SunIcon /> : <MoonIcon />}
            </Button>
            <Button type="button" variant="ghost" size="icon-xs" onClick={onLogout} data-logout="" aria-label="Salir">
              <LogOutIcon />
            </Button>
          </span>
        </div>
      </SidebarFooter>
    </Sidebar>
  );
}

function Name({ value, save, cancel }: { value: string; save: (text: string) => void; cancel: () => void }) {
  const [draft, setDraft] = useState(value);
  return (
    <Input
      autoFocus
      value={draft}
      data-name=""
      onChange={(event) => setDraft(event.target.value)}
      onKeyDown={(event) => {
        if (event.key === "Enter" && draft.trim()) save(draft.trim());
        if (event.key === "Escape") cancel();
      }}
      onBlur={() => (draft.trim() && draft.trim() !== value ? save(draft.trim()) : cancel())}
      className="h-7 min-w-0 flex-1 px-1 text-sm"
    />
  );
}
