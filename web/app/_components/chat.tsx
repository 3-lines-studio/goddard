import { useCallback, useEffect, useRef, useState } from "react";
import { MonitorIcon, XIcon } from "lucide-react";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty";
import { SidebarInset, SidebarProvider, SidebarTrigger } from "@/components/ui/sidebar";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

import { Agenda, keyOf } from "./agenda";
import { Login } from "./login";
import { Memory } from "./memory";
import { Models } from "./model";
import { Orgs } from "./orgs";
import { Rail } from "./rail";
import { Sandboxes } from "./sandbox";
import { Composer, Thread } from "./thread";
import type { Event, Fact, Member, Model, ModelInput, Org, Project, Sandbox, SandboxCheck, SandboxInput, Skill, Tab, Task, Upload } from "./types";

const TABS_KEY = "goddard-tabs";
const OPEN_KEY = "goddard-open-projects";
const THEME_KEY = "goddard-theme";

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

function tabFromKey(key: string, projects: Project[]): Tab | null {
  if (key === "agenda") return { key, kind: "agenda", title: "Agenda", slug: "" };
  if (key === "orgs") return { key, kind: "orgs", title: "Organizaciones", slug: "" };
  if (key === "sandbox") return { key, kind: "sandbox", title: "Computadoras", slug: "" };
  if (key === "model") return { key, kind: "model", title: "Modelo", slug: "" };
  const memory = key.match(/^memoria:(.+)$/);
  if (memory) {
    const project = projects.find((one) => one.slug === memory[1]);
    if (!project) return null;
    return { key, kind: "memoria", title: `memoria · ${project.name}`, slug: project.slug };
  }
  for (const project of projects) {
    const line = (project.conversations ?? []).find((one) => one.id === key);
    if (line) return { key, kind: "thread", title: line.title, slug: project.slug };
  }
  return null;
}

export function Chat() {
  const [projects, setProjects] = useState<Project[]>([]);
  const [orgs, setOrgs] = useState<Org[]>([]);
  const [members, setMembers] = useState<Record<string, Member[]>>({});
  const [spaces, setSpaces] = useState<Record<string, Sandbox>>({});
  const [models, setModels] = useState<Record<string, Model>>({});
  const [savingModel, setSavingModel] = useState("");
  const [removingModel, setRemovingModel] = useState("");
  const [checks, setChecks] = useState<Record<string, SandboxCheck>>({});
  const [checking, setChecking] = useState("");
  const [saving, setSaving] = useState("");
  const [removingSpace, setRemovingSpace] = useState("");
  const [keys, setKeys] = useState<string[]>([]);
  const [active, setActive] = useState("");
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());
  const [more, setMore] = useState<Set<string>>(new Set());
  const [lines, setLines] = useState<Event[]>([]);
  const [partial, setPartial] = useState("");
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [user, setUser] = useState("");
  const [editing, setEditing] = useState("");
  const [removing, setRemoving] = useState("");
  const [facts, setFacts] = useState<Fact[]>([]);
  const [skills, setSkills] = useState<Skill[]>([]);
  const [tasks, setTasks] = useState<Task[]>([]);
  const [openTasks, setOpenTasks] = useState<Set<string>>(new Set());
  const [removingTask, setRemovingTask] = useState("");
  const [pending, setPending] = useState<{ key: string; ts: number } | null>(null);
  const [uploads, setUploads] = useState<Upload[]>([]);
  const [theme, setTheme] = useState("dark");
  const [needLogin, setNeedLogin] = useState(false);
  const [sent, setSent] = useState("");
  const [link, setLink] = useState("");
  const restored = useRef(false);

  const tabs = keys.map((key) => tabFromKey(key, projects)).filter((one): one is Tab => one !== null);
  const tab = tabs.find((one) => one.key === active) ?? tabs[tabs.length - 1] ?? null;
  const panel = projects.find((one) => one.slug === tab?.slug) ?? null;
  const open = tab?.kind === "thread" ? tab.key : "";
  const unread = tasks.reduce((total, task) => total + task.unread, 0);

  useEffect(() => {
    setTheme(document.documentElement.classList.contains("dark") ? "dark" : "light");
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
    const projects: Project[] = data.projects ?? [];
    setProjects(projects);
    setOrgs(data.orgs ?? []);
    if (!restored.current) {
      restored.current = true;
      setCollapsed(new Set(readList(OPEN_KEY)));
      const wanted = readList(TABS_KEY).filter((key) => tabFromKey(key, projects) !== null);
      setKeys(wanted);
      setActive(wanted[wanted.length - 1] ?? "");
    }
  }, []);

  const loadMembers = useCallback(async (org: string) => {
    const response = await fetch(`/api/orgs/members?org=${encodeURIComponent(org)}`);
    if (!response.ok) {
      setError(await response.text());
      return;
    }
    const data = await response.json();
    setMembers((current) => ({ ...current, [org]: data.members ?? [] }));
  }, []);

  const loadSpace = useCallback(async (org: string) => {
    const target = org ? `/api/workspace?org=${encodeURIComponent(org)}` : "/api/workspace";
    const response = await fetch(target);
    if (!response.ok) {
      setError(await response.text());
      return;
    }
    const data: Sandbox = await response.json();
    setSpaces((current) => ({ ...current, [org]: data }));
  }, []);

  const loadModel = useCallback(async (org: string) => {
    const target = org ? `/api/model?org=${encodeURIComponent(org)}` : "/api/model";
    const response = await fetch(target);
    if (!response.ok) {
      setError(await response.text());
      return;
    }
    const data: Model = await response.json();
    setModels((current) => ({ ...current, [org]: data }));
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  useEffect(() => {
    if (tab?.kind !== "sandbox") return;
    if (!("" in spaces)) void loadSpace("");
    for (const org of orgs) if (!(org.id in spaces)) void loadSpace(org.id);
  }, [tab?.kind, orgs, spaces, loadSpace]);

  useEffect(() => {
    if (tab?.kind !== "model") return;
    if (!("" in models)) void loadModel("");
    for (const org of orgs) if (!(org.id in models)) void loadModel(org.id);
  }, [tab?.kind, orgs, models, loadModel]);

  useEffect(() => {
    if (tab?.kind !== "orgs") return;
    for (const org of orgs) {
      if (!(org.id in members)) void loadMembers(org.id);
    }
  }, [tab?.kind, orgs, members, loadMembers]);

  useEffect(() => {
    if (!open) return;
    setLines([]);
    setPartial("");
    setUploads([]);
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
    if (!user) return;
    const tick = setInterval(() => void load(), 15_000);
    return () => clearInterval(tick);
  }, [user, load]);

  async function send(event: React.FormEvent) {
    event.preventDefault();
    if (!text.trim() || busy) return;
    setBusy(true);
    setError("");
    const response = await fetch("/api/turns", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ conversation: open, text, uploads: uploads.map((one) => one.id) }),
    });
    setBusy(false);
    if (!response.ok) {
      setError(await response.text());
      return;
    }
    setText("");
    setUploads([]);
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
      setUploads((current) => [...current, upload]);
    }
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
    const fields = new FormData(form);
    const name = fields.get("name");
    const org = fields.get("org");
    if (typeof name !== "string" || !name.trim()) return;
    const response = await fetch("/api/projects", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ name, org: typeof org === "string" ? org : "" }),
    });
    if (!response.ok) {
      setError(await response.text());
      return;
    }
    form.reset();
    load();
  }

  async function saveSpace(org: string, input: SandboxInput) {
    setSaving(org || "personal");
    setError("");
    const response = await fetch("/api/workspace", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ org, ...input }),
    });
    setSaving("");
    if (!response.ok) {
      setError(await response.text());
      return;
    }
    await loadSpace(org);
    forgetCheck(org);
    load();
  }

  function askSpace(org: string) {
    if (removingSpace !== (org || "personal")) {
      setRemovingSpace(org || "personal");
      return;
    }
    void removeSpace(org);
  }

  async function removeSpace(org: string) {
    setRemovingSpace("");
    setError("");
    const target = org ? `/api/workspace?org=${encodeURIComponent(org)}` : "/api/workspace";
    const response = await fetch(target, { method: "DELETE" });
    if (!response.ok) {
      setError(await response.text());
      return;
    }
    await loadSpace(org);
    forgetCheck(org);
    load();
  }

  async function saveModel(org: string, input: ModelInput) {
    setSavingModel(org || "personal");
    setError("");
    const response = await fetch("/api/model", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ org, ...input }),
    });
    setSavingModel("");
    if (!response.ok) {
      setError(await response.text());
      return;
    }
    await loadModel(org);
  }

  function askModel(org: string) {
    if (removingModel !== (org || "personal")) {
      setRemovingModel(org || "personal");
      return;
    }
    void removeModel(org);
  }

  async function removeModel(org: string) {
    setRemovingModel("");
    setError("");
    const target = org ? `/api/model?org=${encodeURIComponent(org)}` : "/api/model";
    const response = await fetch(target, { method: "DELETE" });
    if (!response.ok) {
      setError(await response.text());
      return;
    }
    await loadModel(org);
  }

  function forgetCheck(org: string) {
    setChecks((current) => {
      const next = { ...current };
      delete next[org];
      return next;
    });
  }

  async function checkSpace(org: string) {
    setChecking(org || "personal");
    setError("");
    const response = await fetch("/api/workspace/check", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ org }),
    });
    setChecking("");
    if (!response.ok) {
      setError(await response.text());
      return;
    }
    const found: SandboxCheck = await response.json();
    setChecks((current) => ({ ...current, [org]: found }));
  }

  async function newOrg(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const name = new FormData(form).get("name");
    if (typeof name !== "string" || !name.trim()) return;
    const response = await fetch("/api/orgs", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ name }),
    });
    if (!response.ok) {
      setError(await response.text());
      return;
    }
    form.reset();
    await load();
  }

  async function invite(org: string, event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const body = new FormData(form);
    const email = body.get("email");
    if (typeof email !== "string" || !email.trim()) return;
    const response = await fetch("/api/orgs/members", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ org, email, role: String(body.get("role") ?? "member") }),
    });
    if (!response.ok) {
      setError(await response.text());
      return;
    }
    form.reset();
    await loadMembers(org);
  }

  async function setRole(org: string, person: string, role: string) {
    const response = await fetch("/api/orgs/members", {
      method: "PATCH",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ org, user: person, role }),
    });
    if (!response.ok) {
      setError(await response.text());
      return;
    }
    await loadMembers(org);
  }

  async function removeMember(org: string, person: string) {
    const response = await fetch(
      `/api/orgs/members?org=${encodeURIComponent(org)}&user=${encodeURIComponent(person)}`,
      { method: "DELETE" },
    );
    if (!response.ok) {
      setError(await response.text());
      return;
    }
    await loadMembers(org);
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

  function avatarState(project: Project) {
    if (collapsed.has(project.slug)) return "sleeping";
    if ((project.conversations ?? []).some((line) => line.running)) return "working";
    if (tab?.slug === project.slug && (tab?.kind === "thread" || tab?.kind === "memoria")) return "focused";
    return "idle";
  }

  function toggleTheme() {
    const next = theme === "dark" ? "light" : "dark";
    setTheme(next);
    document.documentElement.classList.toggle("dark", next === "dark");
    rememberTheme(next);
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
    const line = await response.json();
    await load();
    openTab(line.id);
  }

  const working =
    open !== "" && lines.length > 0 && !["done", "error", "stopped"].includes(lines[lines.length - 1].event);

  if (needLogin) {
    return <Login sent={sent} link={link} error={error} onSend={login} />;
  }

  return (
    <SidebarProvider className="h-dvh">
      <Rail
        projects={projects}
        orgs={orgs}
        tab={tab}
        unread={unread}
        user={user}
        theme={theme}
        collapsed={collapsed}
        more={more}
        editing={editing}
        removing={removing}
        avatarState={avatarState}
        onNewProject={newProject}
        onOpenProject={openProject}
        onNewConversation={(project) => void newConversation(project.id)}
        onOpenMemory={(project) => openTab(`memoria:${project.slug}`)}
        onToggleProject={toggleProject}
        onToggleMore={toggleMore}
        onEditing={setEditing}
        onAsk={ask}
        onRename={rename}
        onOpenTab={openTab}
        onOpenAgenda={() => openTab("agenda")}
        onOpenOrgs={() => openTab("orgs")}
        onOpenSandbox={() => openTab("sandbox")}
        onOpenModel={() => openTab("model")}
        onToggleTheme={toggleTheme}
        onLogout={logout}
      />
      <SidebarInset className="min-w-0 overflow-hidden">
        <header className="flex items-center gap-2 border-b p-2 md:hidden">
          <SidebarTrigger />
          <span className="truncate text-sm">{tab ? tab.title : "Goddard"}</span>
        </header>

        <Tabs
          value={tab?.key ?? ""}
          onValueChange={(value) => setActive(String(value))}
          className="min-h-0 flex-1"
        >
          <TabsList
            variant="line"
            className="h-auto w-full justify-start overflow-x-auto rounded-none border-b px-1.5 py-1"
          >
            {tabs.map((one) => (
              <span key={one.key} className="flex shrink-0 items-center">
                <TabsTrigger value={one.key} data-tab={one.key} className="max-w-48">
                  <span className="truncate">{one.title}</span>
                </TabsTrigger>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-xs"
                  data-close-tab={one.key}
                  aria-label={`Cerrar ${one.title}`}
                  onClick={() => closeTab(one.key)}
                  className="opacity-40 hover:opacity-100"
                >
                  <XIcon />
                </Button>
              </span>
            ))}
          </TabsList>

          {tab ? (
            <TabsContent value={tab.key} className="flex min-h-0 flex-1 flex-col">
              {tab.kind === "memoria" ? (
                <div className="min-h-0 flex-1 overflow-y-auto">
                  <Memory facts={facts} skills={skills} />
                </div>
              ) : tab.kind === "orgs" ? (
                <div className="min-h-0 flex-1 overflow-y-auto">
                  <Orgs
                    orgs={orgs}
                    members={members}
                    removing={removing}
                    onNew={newOrg}
                    onInvite={(org, event) => void invite(org, event)}
                    onRole={(org, person, role) => void setRole(org, person, role)}
                    onRemove={(org, person) => void removeMember(org, person)}
                    onAsk={ask}
                  />
                </div>
              ) : tab.kind === "sandbox" ? (
                <div className="min-h-0 flex-1 overflow-y-auto">
                  <Sandboxes
                    spaces={spaces}
                    orgs={orgs}
                    saving={saving}
                    removing={removingSpace}
                    checking={checking}
                    checks={checks}
                    onSave={(org, input) => void saveSpace(org, input)}
                    onRemove={(org) => askSpace(org)}
                    onCheck={(org) => void checkSpace(org)}
                  />
                </div>
              ) : tab.kind === "model" ? (
                <div className="min-h-0 flex-1 overflow-y-auto">
                  <Models
                    models={models}
                    orgs={orgs}
                    saving={savingModel}
                    removing={removingModel}
                    onSave={(org, input) => void saveModel(org, input)}
                    onRemove={(org) => askModel(org)}
                  />
                </div>
              ) : tab.kind === "agenda" ? (
                <div className="min-h-0 flex-1 overflow-y-auto">
                  <Agenda
                    tasks={tasks}
                    projects={projects}
                    open={openTasks}
                    pending={pending?.key ?? ""}
                    removing={removingTask}
                    unread={unread}
                    onToggle={readTask}
                    onRun={runTask}
                    onPause={pauseTask}
                    onAsk={askTask}
                    onReadAll={readAll}
                  />
                </div>
              ) : (
                <Thread lines={lines} partial={partial} workspace={panel?.workspace ?? ""} />
              )}
            </TabsContent>
          ) : (
            <TabsContent value="" className="flex min-h-0 flex-1 items-center justify-center">
              <Empty className="border-0">
                <EmptyHeader>
                  <EmptyMedia variant="icon">
                    <MonitorIcon />
                  </EmptyMedia>
                  <EmptyTitle>Goddard</EmptyTitle>
                  <EmptyDescription>Elegí una conversación o creá una nueva.</EmptyDescription>
                </EmptyHeader>
              </Empty>
            </TabsContent>
          )}
        </Tabs>

        {error ? (
          <Alert variant="destructive" className="rounded-none border-x-0">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        ) : null}

        {tab?.kind === "thread" ? (
          <Composer
            open={open}
            text={text}
            busy={busy}
            working={working}
            uploads={uploads}
            onText={setText}
            onSend={send}
            onStop={stop}
            onAttach={attach}
            onDetach={(id) => setUploads((current) => current.filter((one) => one.id !== id))}
          />
        ) : null}
      </SidebarInset>
    </SidebarProvider>
  );
}
