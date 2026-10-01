import { useCallback, useEffect, useRef, useState } from "react";

import { markdown } from "../_lib/markdown";
import { describeTool } from "../_lib/tool";
import { Avatar } from "./avatar";
import { Icon } from "./icon";

type Line = {
  id: string;
  title: string;
  source: string;
  running: boolean;
};

type Project = {
  id: string;
  slug: string;
  name: string;
  conversations: Line[];
};

type Task = {
  name: string;
  project: string;
  when: string;
  at: string;
  every: string;
  prompt: string;
  paused: boolean;
  silent: boolean;
  target: string;
  unread: number;
  runs: { ts: number; date: string; ms: number; ok: boolean; text: string }[];
};

type Fact = {
  key: string;
  kind: string;
  body: string;
  project: string;
  date: number;
};

type Skill = {
  name: string;
  description: string;
  owner: string;
  role: string;
  updated: number;
};

type Tab = {
  key: string;
  kind: "thread" | "agenda" | "memoria";
  title: string;
  slug: string;
};

type Event = {
  event: string;
  text?: string;
  name?: string;
  mime?: string;
  caption?: string;
  args?: string;
  id?: string;
  ms?: number;
  message?: string;
};

const TABS_KEY = "goddard-tabs";
const OPEN_KEY = "goddard-open-projects";
const THEME_KEY = "goddard-theme";
const VISIBLE = 10;

const TOOL_ICON: Record<string, string> = {
  read: "file",
  write: "file-plus",
  edit: "pencil",
  bash: "terminal",
  search: "search",
  fetch: "download",
  browse: "globe",
};

function readList(key: string): string[] {
  try {
    const value = JSON.parse(localStorage.getItem(key) || "[]");
    return Array.isArray(value) ? value.filter((one) => typeof one === "string") : [];
  } catch {
    return [];
  }
}

function writeList(key: string, values: string[]) {
  try {
    localStorage.setItem(key, JSON.stringify(values));
  } catch {
    return;
  }
}

function rememberTheme(name: string) {
  try {
    localStorage.setItem(THEME_KEY, name);
  } catch {
    return;
  }
}

function keyOf(task: Task) {
  return `${task.project}/${task.name}`;
}

function tabFromKey(key: string, projects: Project[]): Tab | null {
  if (key === "agenda") return { key, kind: "agenda", slug: "", title: "Agenda" };
  for (const view of ["memoria"] as const) {
    if (!key.startsWith(view + ":")) continue;
    const slug = key.slice(view.length + 1);
    const project = projects.find((one) => one.slug === slug);
    if (!project) return null;
    const name = view === "agenda" ? "Agenda" : "Memoria";
    return { key, kind: view, slug, title: `${name} · ${project.name}` };
  }
  for (const project of projects) {
    for (const line of project.conversations ?? []) {
      if (line.id === key) return { key, kind: "thread", slug: project.slug, title: line.title };
    }
  }
  return null;
}

export function Chat() {
  const [projects, setProjects] = useState<Project[]>([]);
  const [keys, setKeys] = useState<string[]>([]);
  const [active, setActive] = useState("");
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());
  const [more, setMore] = useState<Set<string>>(new Set());
  const [menuProject, setMenuProject] = useState("");
  const [lines, setLines] = useState<Event[]>([]);
  const [partial, setPartial] = useState("");
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [menu, setMenu] = useState(false);
  const [user, setUser] = useState("");
  const [workspace, setWorkspace] = useState("");
  const [editing, setEditing] = useState("");
  const [removing, setRemoving] = useState("");
  const [facts, setFacts] = useState<Fact[]>([]);
  const [skills, setSkills] = useState<Skill[]>([]);
  const [tasks, setTasks] = useState<Task[]>([]);
  const [openTasks, setOpenTasks] = useState<Set<string>>(new Set());
  const [removingTask, setRemovingTask] = useState("");
  const [pending, setPending] = useState<{ key: string; ts: number } | null>(null);
  const [attachments, setAttachments] = useState<{ id: string; name: string; mime: string }[]>([]);
  const files = useRef<HTMLInputElement>(null);
  const [theme, setTheme] = useState("dark");
  const [needLogin, setNeedLogin] = useState(false);
  const [sent, setSent] = useState("");
  const [link, setLink] = useState("");
  const bottom = useRef<HTMLDivElement>(null);
  const restored = useRef(false);

  const tabs = keys.map((key) => tabFromKey(key, projects)).filter((one): one is Tab => one !== null);
  const tab = tabs.find((one) => one.key === active) ?? tabs[tabs.length - 1] ?? null;
  const panel = projects.find((one) => one.slug === tab?.slug) ?? null;
  const open = tab?.kind === "thread" ? tab.key : "";
  const unread = tasks.reduce((total, task) => total + task.unread, 0);

  useEffect(() => {
    setTheme(document.documentElement.dataset.theme === "light" ? "light" : "dark");
  }, []);

  useEffect(() => {
    if (restored.current) writeList(TABS_KEY, keys);
  }, [keys]);

  useEffect(() => {
    if (restored.current) writeList(OPEN_KEY, [...collapsed]);
  }, [collapsed]);

  const load = useCallback(async () => {
    const response = await fetch("/api/state");
    if (response.status === 401) {
      setNeedLogin(true);
      return;
    }
    if (!response.ok) {
      setError(await response.text());
      return;
    }
    const data = await response.json();
    setNeedLogin(false);
    setUser(data.user ?? "");
    setWorkspace(data.workspace ?? "");
    const projects: Project[] = data.projects ?? [];
    setProjects(projects);
    if (!restored.current) {
      restored.current = true;
      setCollapsed(new Set(readList(OPEN_KEY)));
      const wanted = readList(TABS_KEY).filter((key) => tabFromKey(key, projects) !== null);
      setKeys(wanted);
      setActive(wanted[wanted.length - 1] ?? "");
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  useEffect(() => {
    if (!open) return;
    setLines([]);
    setPartial("");
    setAttachments([]);
    const stream = new EventSource(`/api/stream?conversation=${encodeURIComponent(open)}`);
    stream.onmessage = (message) => {
      const line: Event = JSON.parse(message.data);
      if (line.event === "delta") {
        setPartial((current) => current + (line.text ?? ""));
        return;
      }
      if (line.event === "assistant") setPartial("");
      setLines((current) => [...current, line]);
      if (line.event === "done" || line.event === "error") load();
    };
    return () => stream.close();
  }, [open, load]);

  useEffect(() => {
    if (tab?.kind === "agenda") void loadTasks();
    if (tab?.kind === "memoria") void loadMemory(tab.slug);
  }, [tab?.kind, tab?.slug]);

  useEffect(() => {
    if (!user) return;
    void loadTasks();
    const tick = setInterval(() => void loadTasks(), 60_000);
    return () => clearInterval(tick);
  }, [user]);

  useEffect(() => {
    if (!pending) return;
    const tick = setInterval(() => void loadTasks(), 3_000);
    return () => clearInterval(tick);
  }, [pending]);

  useEffect(() => {
    bottom.current?.scrollIntoView({ block: "end" });
  }, [lines, partial]);

  async function send(event: React.FormEvent) {
    event.preventDefault();
    if (!text.trim() || busy) return;
    setBusy(true);
    setError("");
    const response = await fetch("/api/turns", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ conversation: open, text, uploads: attachments.map((one) => one.id) }),
    });
    setBusy(false);
    if (!response.ok) {
      setError(await response.text());
      return;
    }
    setText("");
    setAttachments([]);
  }

  async function stop() {
    const response = await fetch("/api/turns/stop", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ conversation: open }),
    });
    if (!response.ok) {
      setError(await response.text());
      return;
    }
    load();
  }

  async function attach(chosen: FileList | null) {
    if (!chosen || !open) return;
    for (const file of Array.from(chosen)) {
      const body = new FormData();
      body.append("conversation", open);
      body.append("file", file);
      const response = await fetch("/api/uploads", { method: "POST", body });
      if (!response.ok) {
        setError(await response.text());
        return;
      }
      const upload = await response.json();
      setAttachments((current) => [...current, upload]);
    }
    if (files.current) files.current.value = "";
  }

  async function login(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const email = new FormData(event.currentTarget).get("email");
    if (typeof email !== "string") return;
    setError("");
    const response = await fetch("/api/login", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ email }),
    });
    if (!response.ok) {
      setError(await response.text());
      return;
    }
    const data = await response.json();
    setSent(email);
    setLink(data.link ?? "");
  }

  async function logout() {
    await fetch("/api/logout", { method: "POST" });
    setProjects([]);
    setKeys([]);
    setActive("");
    setLines([]);
    setUser("");
    setSent("");
    setLink("");
    setNeedLogin(true);
  }

  async function newProject(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const name = new FormData(form).get("name");
    if (typeof name !== "string" || !name.trim()) return;
    const response = await fetch("/api/projects", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ name }),
    });
    if (!response.ok) {
      setError(await response.text());
      return;
    }
    form.reset();
    load();
  }

  async function rename(path: string, body: Record<string, string>) {
    const response = await fetch(path, {
      method: "PATCH",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(body),
    });
    setEditing("");
    if (!response.ok) {
      setError(await response.text());
      return;
    }
    load();
  }

  function ask(path: string, id: string) {
    if (removing !== id) {
      setRemoving(id);
      return;
    }
    void remove(path, id);
  }

  async function remove(path: string, id: string) {
    setRemoving("");
    const response = await fetch(`${path}?id=${encodeURIComponent(id)}`, { method: "DELETE" });
    if (!response.ok) {
      setError(await response.text());
      return;
    }
    load();
  }

  async function loadTasks() {
    const response = await fetch("/api/agenda");
    if (!response.ok) {
      setError(await response.text());
      return;
    }
    const data = await response.json();
    const tasks: Task[] = data.tasks ?? [];
    setTasks(tasks);
    setPending((current) => {
      if (!current) return current;
      const task = tasks.find((one) => keyOf(one) === current.key);
      if (!task) return null;
      return (task.runs[0]?.ts ?? 0) === current.ts ? current : null;
    });
  }

  async function loadMemory(slug: string) {
    const memory = await fetch(`/api/memo?project=${encodeURIComponent(slug)}`);
    if (!memory.ok) {
      setError(await memory.text());
      return;
    }
    setFacts((await memory.json()).facts ?? []);
    const installed = await fetch("/api/skills");
    if (!installed.ok) {
      setError(await installed.text());
      return;
    }
    setSkills((await installed.json()).skills ?? []);
  }

  function openTab(key: string) {
    setKeys((current) => (current.includes(key) ? current : [...current, key]));
    setActive(key);
    setMenu(false);
    setMenuProject("");
  }

  function closeTab(key: string) {
    setKeys((current) => current.filter((one) => one !== key));
    setActive((current) => (current === key ? "" : current));
  }

  function toggleProject(slug: string) {
    setCollapsed((current) => {
      const next = new Set(current);
      if (next.has(slug)) next.delete(slug);
      else next.add(slug);
      return next;
    });
  }

  function toggleMore(slug: string) {
    setMore((current) => {
      const next = new Set(current);
      if (next.has(slug)) next.delete(slug);
      else next.add(slug);
      return next;
    });
  }

  function openAgenda() {
    openTab("agenda");
  }

  function avatarState(project: Project) {
    if (collapsed.has(project.slug)) return "sleeping";
    if ((project.conversations ?? []).some((line) => line.running)) return "working";
    if (tab?.slug === project.slug && (tab?.kind === "thread" || tab?.kind === "memoria")) return "focused";
    return "idle";
  }

  function toggleTheme() {
    const next = theme === "dark" ? "light" : "dark";
    setTheme(next);
    document.documentElement.dataset.theme = next;
    rememberTheme(next);
  }

  function openMemory(project: Project) {
    openTab(`memoria:${project.slug}`);
  }

  function openProject(project: Project) {
    const newest = (project.conversations ?? [])[0];
    if (!newest) {
      void newConversation(project.id);
      return;
    }
    openTab(newest.id);
  }

  async function runTask(task: Task) {
    setPending({ key: keyOf(task), ts: task.runs[0]?.ts ?? 0 });
    const response = await fetch("/api/agenda/run", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ project: task.project, name: task.name }),
    });
    if (!response.ok) {
      setPending(null);
      setError(await response.text());
    }
  }

  async function pauseTask(task: Task) {
    const response = await fetch("/api/agenda", {
      method: "PATCH",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ project: task.project, name: task.name, paused: !task.paused }),
    });
    if (!response.ok) {
      setError(await response.text());
      return;
    }
    await loadTasks();
  }

  async function readTask(task: Task) {
    const key = keyOf(task);
    setOpenTasks((current) => {
      const next = new Set(current);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
    if (task.unread === 0) return;
    setTasks((current) => current.map((one) => (keyOf(one) === key ? { ...one, unread: 0 } : one)));
    const response = await fetch("/api/agenda/read", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ project: task.project, name: task.name }),
    });
    if (!response.ok) setError(await response.text());
  }

  async function readAll() {
    const response = await fetch("/api/agenda/read", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({}),
    });
    if (!response.ok) {
      setError(await response.text());
      return;
    }
    await loadTasks();
  }

  function askTask(task: Task) {
    if (removingTask !== keyOf(task)) {
      setRemovingTask(keyOf(task));
      return;
    }
    void removeTask(task);
  }

  async function removeTask(task: Task) {
    setRemovingTask("");
    const query = `project=${encodeURIComponent(task.project)}&name=${encodeURIComponent(task.name)}`;
    const response = await fetch(`/api/agenda?${query}`, { method: "DELETE" });
    if (!response.ok) {
      setError(await response.text());
      return;
    }
    await loadTasks();
  }

  async function newConversation(project: string) {
    const response = await fetch("/api/conversations", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ project }),
    });
    if (!response.ok) {
      setError(await response.text());
      return;
    }
    const line: Line = await response.json();
    await load();
    openTab(line.id);
  }

  const working =
    open !== "" && lines.length > 0 && !["done", "error", "stopped"].includes(lines[lines.length - 1].event);

  if (needLogin) {
    return (
      <div className="mx-auto flex h-dvh max-w-sm flex-col justify-center gap-4 p-6">
        <h1 className="text-2xl font-semibold">Goddard</h1>
        {sent ? (
          <>
            <p className="text-sm">Si el mail está en la lista, te llegó un link a {sent}.</p>
            {link ? (
              <p className="text-sm break-all">
                <a href={link} data-login-link="" className="underline">
                  {link}
                </a>
              </p>
            ) : null}
          </>
        ) : (
          <form onSubmit={login} className="flex flex-col gap-2">
            <input
              name="email"
              type="email"
              data-login-email=""
              placeholder="tu@mail"
              className="rounded border border-line bg-transparent px-3 py-2 text-sm"
            />
            <button type="submit" data-login-send="" className="rounded border border-line px-3 py-2 text-sm">
              Mandame el link
            </button>
          </form>
        )}
        {error ? <p className="text-sm text-err">{error}</p> : null}
      </div>
    );
  }

  return (
    <div className="flex h-dvh">
      <aside className={`${menu ? "flex" : "hidden"} w-72 shrink-0 flex-col border-r border-line bg-panel md:flex`}>
        <form onSubmit={newProject} className="flex gap-2 border-b border-line p-3">
          <input
            name="name"
            data-new-project=""
            placeholder="Nuevo proyecto"
            className="w-full rounded border border-line bg-transparent px-2 py-1 text-sm"
          />
          <button
            type="submit"
            aria-label="Agregar proyecto"
            className="rounded border border-line px-1.5 py-1 opacity-70 hover:opacity-100"
          >
            <Icon name="plus" />
          </button>
        </form>
        <nav className="flex-1 overflow-y-auto p-2">
          {projects.map((project) => {
            const threads = project.conversations ?? [];
            const shut = collapsed.has(project.slug);
            const shown = more.has(project.slug) ? threads : threads.slice(0, VISIBLE);
            return (
              <div key={project.id} className="mb-3">
                <div className="flex items-center gap-1.5">
                  <Avatar name={project.slug} state={avatarState(project)} />
                  {editing === project.id ? (
                    <Name
                      value={project.name}
                      save={(name) => rename("/api/projects", { id: project.id, name })}
                      cancel={() => setEditing("")}
                    />
                  ) : (
                    <button
                      type="button"
                      onClick={() => openProject(project)}
                      data-project={project.id}
                      className="min-w-0 flex-1 truncate rounded px-1 py-1 text-left text-sm font-semibold hover:bg-hover"
                    >
                      {project.name}
                    </button>
                  )}
                  <button
                    type="button"
                    onClick={() => toggleProject(project.slug)}
                    data-toggle-project={project.id}
                    aria-label={shut ? `Mostrar los hilos de ${project.name}` : `Ocultar los hilos de ${project.name}`}
                    className="shrink-0 rounded p-1 opacity-60 hover:opacity-100"
                  >
                    <Icon name="down" size={14} className={`transition-transform ${shut ? "-rotate-90" : ""}`} />
                  </button>
                  <button
                    type="button"
                    onClick={() => setMenuProject(menuProject === project.slug ? "" : project.slug)}
                    data-project-menu={project.id}
                    aria-label={`Opciones de ${project.name}`}
                    className={`shrink-0 rounded p-1 ${
                      menuProject === project.slug ? "bg-hover" : "opacity-60 hover:opacity-100"
                    }`}
                  >
                    <Icon name="dots" size={14} />
                  </button>
                </div>
                {menuProject === project.slug ? (
                  <div className="my-1 flex flex-col rounded border border-line p-1 text-sm">
                    <button
                      type="button"
                      onClick={() => newConversation(project.id)}
                      data-new-conversation={project.id}
                      className="rounded px-2 py-1 text-left hover:bg-hover"
                    >
                      nueva conversación
                    </button>
                    <button
                      type="button"
                      onClick={() => openMemory(project)}
                      data-memory={project.id}
                      className="rounded px-2 py-1 text-left hover:bg-hover"
                    >
                      memoria
                    </button>
                    <button
                      type="button"
                      onClick={() => {
                        setMenuProject("");
                        setEditing(project.id);
                      }}
                      data-rename-project={project.id}
                      className="rounded px-2 py-1 text-left hover:bg-hover"
                    >
                      renombrar
                    </button>
                    <button
                      type="button"
                      onClick={() => ask("/api/projects", project.id)}
                      data-delete-project={project.id}
                      title={removing === project.id ? "otra vez para borrar" : `Borrar ${project.name}`}
                      className={`rounded px-2 py-1 text-left hover:bg-hover ${
                        removing === project.id ? "bg-err text-bg" : "text-err"
                      }`}
                    >
                      {removing === project.id ? "otra vez para borrar" : "quitar"}
                    </button>
                  </div>
                ) : null}
                {shut ? null : (
                  <ul className="mt-1">
                    {shown.map((line) => (
                      <li key={line.id} className="group flex items-center gap-1">
                        {editing === line.id ? (
                          <Name
                            value={line.title}
                            save={(title) => rename("/api/conversations", { id: line.id, title })}
                            cancel={() => setEditing("")}
                          />
                        ) : (
                          <button
                            type="button"
                            onClick={() => openTab(line.id)}
                            data-conversation={line.id}
                            className={`min-w-0 flex-1 truncate rounded px-2 py-1 text-left text-sm ${
                              tab?.key === line.id ? "bg-raised" : "hover:bg-hover"
                            }`}
                          >
                            {line.title}
                            {line.running ? <span className="ml-1 text-xs opacity-60">·</span> : null}
                          </button>
                        )}
                        <button
                          type="button"
                          onClick={() => setEditing(line.id)}
                          data-rename-conversation={line.id}
                          aria-label={`Renombrar ${line.title}`}
                          className="rounded p-1 opacity-40 md:opacity-0 md:group-hover:opacity-40 hover:opacity-100"
                        >
                          <Icon name="pencil" size={13} />
                        </button>
                        <button
                          type="button"
                          onClick={() => ask("/api/conversations", line.id)}
                          data-delete-conversation={line.id}
                          aria-label={`Borrar ${line.title}`}
                          title={removing === line.id ? "otra vez para borrar" : `Borrar ${line.title}`}
                          className={`shrink-0 rounded ${
                            removing === line.id
                              ? "bg-err px-1 text-xs text-bg"
                              : "p-1 opacity-40 md:opacity-0 md:group-hover:opacity-40 hover:opacity-100"
                          }`}
                        >
                          {removing === line.id ? "borrar" : <Icon name="close" size={13} />}
                        </button>
                      </li>
                    ))}
                  </ul>
                )}
                {!shut && threads.length > VISIBLE ? (
                  <button
                    type="button"
                    onClick={() => toggleMore(project.slug)}
                    data-more-threads={project.slug}
                    className="mt-1 px-2 text-xs opacity-60 hover:opacity-100"
                  >
                    {more.has(project.slug) ? "ver menos" : `ver más (${threads.length - VISIBLE})`}
                  </button>
                ) : null}
              </div>
            );
          })}
          {projects.length === 0 ? <p className="p-2 text-sm opacity-60">Todavía no hay proyectos.</p> : null}
        </nav>
        <div className="border-t border-line p-2">
          <button
            type="button"
            onClick={openAgenda}
            data-agenda=""
            className={`flex w-full items-center gap-2 rounded px-2 py-1 text-sm ${
              tab?.kind === "agenda" ? "bg-raised" : "hover:bg-hover"
            }`}
          >
            <Icon name="clock" size={15} />
            Agenda
            {unread > 0 ? (
              <span data-agenda-badge="" className="rounded bg-raised px-1 text-xs">
                {unread}
              </span>
            ) : null}
          </button>
        </div>
        <div className="flex items-center justify-between gap-2 border-t border-line p-2 text-xs">
          <span className="truncate opacity-60">{user}</span>
          <span className="flex shrink-0 items-center gap-1">
            <button
              type="button"
              onClick={toggleTheme}
              data-theme-toggle=""
              aria-label={theme === "dark" ? "Tema claro" : "Tema oscuro"}
              className="rounded p-1 opacity-60 hover:opacity-100"
            >
              <Icon name={theme === "dark" ? "sun" : "moon"} />
            </button>
            <button
              type="button"
              onClick={logout}
              data-logout=""
              aria-label="Salir"
              className="rounded p-1 opacity-60 hover:opacity-100"
            >
              <Icon name="logout" />
            </button>
          </span>
        </div>
      </aside>

      <main className="flex min-w-0 flex-1 flex-col">
        <header className="flex items-center gap-2 border-b border-line p-2 md:hidden">
          <button type="button" onClick={() => setMenu(!menu)} className="rounded border border-line px-2 text-sm">
            proyectos
          </button>
          <span className="truncate text-sm">{tab ? tab.title : "Goddard"}</span>
        </header>

        <nav className="flex items-center gap-1 overflow-x-auto border-b border-line p-1">
          {tabs.map((one) => (
            <span
              key={one.key}
              className={`flex shrink-0 items-center gap-1 rounded px-2 py-1 text-sm ${
                one.key === tab?.key ? "bg-raised" : "hover:bg-hover"
              }`}
            >
              <button type="button" onClick={() => setActive(one.key)} data-tab={one.key} className="max-w-48 truncate">
                {one.title}
              </button>
              <button
                type="button"
                onClick={() => closeTab(one.key)}
                data-close-tab={one.key}
                aria-label={`Cerrar ${one.title}`}
                className="rounded p-0.5 opacity-40 hover:opacity-100"
              >
                <Icon name="close" size={12} />
              </button>
            </span>
          ))}
        </nav>

        {tab?.kind === "memoria" ? (
          <div className="flex-1 overflow-y-auto p-4">
            <div className="mx-auto flex max-w-3xl flex-col gap-4">
              <h2 className="text-sm font-semibold">Skills instaladas</h2>
              {skills.length === 0 ? <p className="text-sm opacity-60">No hay ninguna instalada.</p> : null}
              <ul className="flex flex-col gap-2">
                {skills.map((skill) => (
                  <li key={skill.name} className="rounded border border-line p-2 text-sm">
                    <span className="font-semibold">{skill.name}</span>
                    <span className="ml-2 text-xs opacity-60">
                      {skill.owner}
                      {skill.role ? ` · ${skill.role}` : ""}
                    </span>
                    <p className="mt-1 text-xs opacity-70">{skill.description}</p>
                  </li>
                ))}
              </ul>
              <h2 className="text-sm font-semibold">Memoria de {panel?.name}</h2>
              {facts.length === 0 ? <p className="text-sm opacity-60">No hay hechos.</p> : null}
              {facts.map((fact) => (
                <article key={fact.key} className="rounded border border-line p-2 text-sm" data-fact={fact.key}>
                  <p className="text-xs opacity-60">
                    {fact.key} · {fact.kind} · {day(fact.date)}
                  </p>
                  <p className="mt-1 whitespace-pre-wrap">{fact.body}</p>
                </article>
              ))}
            </div>
          </div>
        ) : tab?.kind === "agenda" ? (
          <div className="flex-1 overflow-y-auto p-4">
            <div className="mx-auto flex max-w-3xl flex-col gap-3">
              {tasks.length === 0 ? <p className="text-sm opacity-60">No hay tareas agendadas.</p> : null}
              {tasks.map((task) => (
                <Task
                  key={keyOf(task)}
                  task={task}
                  project={nameOfProject(projects, task.project)}
                  open={openTasks.has(keyOf(task))}
                  running={pending?.key === keyOf(task)}
                  removing={removingTask === keyOf(task)}
                  toggle={() => readTask(task)}
                  run={() => runTask(task)}
                  pause={() => pauseTask(task)}
                  ask={() => askTask(task)}
                />
              ))}
              {unread > 0 ? (
                <button
                  type="button"
                  onClick={readAll}
                  data-read-all=""
                  className="self-start rounded border border-line px-2 py-1 text-xs"
                >
                  marcar todo leído
                </button>
              ) : null}
            </div>
          </div>
        ) : (
        <div className="flex-1 overflow-y-auto p-4">
          {!open ? <p className="opacity-60">Elegí una conversación o creá una nueva.</p> : null}
          <div className="mx-auto flex max-w-3xl flex-col gap-3">
            {lines.map((line, index) => (
              <Line key={index} line={line} workspace={workspace} />
            ))}
            {partial ? <div className="text-sm whitespace-pre-wrap">{partial}</div> : null}
          </div>
          <div ref={bottom} />
        </div>
        )}

        {error ? <p className="border-t border-err px-4 py-2 text-sm text-err">{error}</p> : null}

        {tab?.kind === "thread" ? (
        <form onSubmit={send} className="border-t border-line p-3">
          <div className="mx-auto flex max-w-3xl flex-col gap-2">
            {attachments.length > 0 ? (
              <div className="flex flex-wrap gap-2 text-xs">
                {attachments.map((one) => (
                  <span key={one.id} className="flex items-center gap-1 rounded border border-line px-2 py-1">
                    {one.name}
                    <button
                      type="button"
                      data-detach={one.id}
                      onClick={() => setAttachments((current) => current.filter((other) => other.id !== one.id))}
                      className="rounded p-0.5 opacity-60 hover:opacity-100"
                    >
                      <Icon name="close" size={12} />
                    </button>
                  </span>
                ))}
              </div>
            ) : null}
            <div className="flex gap-2">
            <input ref={files} type="file" multiple data-files="" className="sr-only" onChange={(event) => void attach(event.target.files)} />
            <button
              type="button"
              data-attach=""
              onClick={() => files.current?.click()}
              disabled={!open}
              className="self-end rounded border border-line px-3 py-2 text-sm disabled:opacity-40"
            >
              adjuntar
            </button>
            <textarea
              data-composer=""
              value={text}
              onChange={(event) => setText(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === "Enter" && !event.shiftKey) {
                  event.preventDefault();
                  event.currentTarget.form?.requestSubmit();
                }
              }}
              rows={2}
              placeholder={open ? "Escribí un mensaje…" : "Elegí una conversación primero"}
              disabled={!open}
              className="w-full resize-none rounded border border-line bg-transparent p-2 text-sm"
            />
            <button
              type="submit"
              data-send=""
              disabled={!open || busy || working || (!text.trim() && attachments.length === 0)}
              className="self-end rounded border border-line px-3 py-2 text-sm disabled:opacity-40"
            >
              Enviar
            </button>
            {working ? (
              <button
                type="button"
                data-stop=""
                onClick={() => void stop()}
                className="self-end rounded border border-line px-3 py-2 text-sm"
              >
                detener
              </button>
            ) : null}
            </div>
          </div>
        </form>
        ) : null}
      </main>
    </div>
  );
}

function Line({ line, workspace }: { line: Event; workspace: string }) {
  switch (line.event) {
    case "file":
      return <File line={line} />;
    case "user":
      return (
        <div className="self-end rounded-lg bg-raised px-3 py-2 text-sm whitespace-pre-wrap">{line.text}</div>
      );
    case "assistant":
      return <Assistant text={line.text ?? ""} />;
    case "tool_start":
      return <Tool call={line} done={false} workspace={workspace} />;
    case "tool_result":
      return <Tool call={line} done={true} workspace={workspace} />;
    case "error":
      return <p className="rounded border border-err px-3 py-2 text-sm text-err">{line.message}</p>;
    case "stopped":
      return <p className="text-xs opacity-60">frenado</p>;
    default:
      return null;
  }
}

function File({ line }: { line: Event }) {
  const href = `/api/uploads?id=${line.id}`;
  const caption = line.caption ? <p className="mt-1 text-xs opacity-60">{line.caption}</p> : null;
  if ((line.mime ?? "").startsWith("image/")) {
    return (
      <figure className="self-start">
        <a href={href} target="_blank">
          <img src={href} alt={line.name ?? ""} className="max-h-72 rounded border border-line" />
        </a>
        {caption}
      </figure>
    );
  }
  return (
    <div className="self-start">
      <a
        href={href}
        target="_blank"
        data-file={line.id}
        className="inline-block rounded border border-line px-3 py-2 text-sm underline"
      >
        {line.name}
      </a>
      {caption}
    </div>
  );
}

function Task({
  task,
  project,
  open,
  running,
  removing,
  toggle,
  run,
  pause,
  ask,
}: {
  task: Task;
  project: string;
  open: boolean;
  running: boolean;
  removing: boolean;
  toggle: () => void;
  run: () => void;
  pause: () => void;
  ask: () => void;
}) {
  const key = keyOf(task);
  const last = task.runs[0];
  const state = running ? "running" : !last ? "" : last.ok ? "done" : "error";
  return (
    <article className="rounded border border-line text-sm" data-task={key}>
      <div className="flex items-center gap-2 p-2">
        <button
          type="button"
          onClick={toggle}
          data-task-toggle={key}
          className="flex min-w-0 flex-1 items-center gap-2 text-left"
        >
          <span
            data-task-state={state}
            className={`h-2 w-2 shrink-0 rounded-full ${
              state === "running"
                ? "bg-text"
                : state === "done"
                  ? "bg-ok"
                  : state === "error"
                    ? "bg-err"
                    : "border border-strong"
            }`}
          />
          <span className="font-semibold">{task.name}</span>
          <span className="truncate text-xs opacity-60">
            {project}
            {" · "}
            {whenOf(task)}
          </span>
          {task.unread > 0 ? (
            <span data-task-unread={key} className="rounded bg-raised px-1 text-xs">
              {task.unread}
            </span>
          ) : null}
          {task.paused ? (
            <span className="rounded border border-line px-1 text-xs opacity-60">pausada</span>
          ) : null}
          {task.silent ? (
            <span className="rounded border border-line px-1 text-xs opacity-60">callada</span>
          ) : null}
        </button>
        <span className="flex shrink-0 items-center gap-1 text-xs">
          <button
            type="button"
            onClick={run}
            disabled={running}
            data-run-task={key}
            className="rounded border border-line px-2 py-0.5 disabled:opacity-40"
          >
            {running ? "corriendo…" : "correr"}
          </button>
          <button type="button" onClick={pause} data-pause-task={key} className="rounded border border-line px-2 py-0.5">
            {task.paused ? "seguir" : "pausar"}
          </button>
          <button
            type="button"
            onClick={ask}
            data-remove-task={key}
            className={`rounded px-2 py-0.5 ${removing ? "bg-err text-bg" : "border border-line opacity-60"}`}
          >
            {removing ? "borrar" : "✕"}
          </button>
        </span>
      </div>
      <p className="px-2 pb-2 text-xs whitespace-pre-wrap opacity-60">{task.prompt}</p>
      {open ? (
        <div className="border-t border-line p-2">
          {task.runs.length === 0 ? <p className="text-xs opacity-60">Todavía no corrió.</p> : null}
          {task.runs.map((one) => (
            <div key={one.ts} className="mt-1 first:mt-0">
              <span className="text-xs opacity-60" title={`hace ${ago(one.ts)} · ${one.ms} ms`}>
                {moment(one.date, one.ts)}
                {one.ok ? "" : " · con error"}
              </span>
              <div
                className={`md text-sm ${one.ok ? "opacity-80" : "text-err"}`}
                dangerouslySetInnerHTML={{ __html: markdown(one.text.trim() || "sin novedades") }}
              />
            </div>
          ))}
        </div>
      ) : null}
    </article>
  );
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

function day(days: number) {
  return new Date(days * 86400000).toISOString().slice(0, 10);
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

function Name({ value, save, cancel }: { value: string; save: (text: string) => void; cancel: () => void }) {
  const [draft, setDraft] = useState(value);
  return (
    <input
      autoFocus
      value={draft}
      data-name=""
      onChange={(event) => setDraft(event.target.value)}
      onKeyDown={(event) => {
        if (event.key === "Enter" && draft.trim()) save(draft.trim());
        if (event.key === "Escape") cancel();
      }}
      onBlur={() => (draft.trim() && draft.trim() !== value ? save(draft.trim()) : cancel())}
      className="min-w-0 flex-1 rounded border border-line bg-transparent px-1 text-sm"
    />
  );
}

function Assistant({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);
  if (!text) return null;
  return (
    <div className="group relative self-stretch">
      <div className="md text-sm" dangerouslySetInnerHTML={{ __html: markdown(text) }} />
      <button
        type="button"
        data-copy=""
        aria-label={copied ? "copiado" : "copiar"}
        onClick={() => {
          void navigator.clipboard.writeText(text).then(() => setCopied(true));
        }}
        className="absolute top-0 right-0 rounded border border-line bg-panel p-1 opacity-0 group-hover:opacity-60 hover:opacity-100"
      >
        <Icon name={copied ? "check" : "copy"} size={13} />
      </button>
    </div>
  );
}

function Tool({ call, done, workspace }: { call: Event; done: boolean; workspace: string }) {
  const what = describeTool(call.name ?? "", call.args ?? "", workspace);
  return (
    <details className="rounded border border-line text-xs">
      <summary className="flex cursor-pointer items-center gap-1.5 px-2 py-1">
        <Icon name={TOOL_ICON[call.name ?? ""] ?? "dot"} size={13} />
        {call.name}
        <span className="ml-2 opacity-60">
          {what.dir ? what.dir + " · " : ""}
          {what.text}
        </span>
        {done ? <span className="ml-2 opacity-60">ok · {call.ms} ms</span> : null}
      </summary>
      <pre className="overflow-x-auto border-t border-line px-2 py-1 opacity-80">
        {done ? call.text : call.args}
      </pre>
    </details>
  );
}
