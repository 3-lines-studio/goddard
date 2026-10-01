import { useCallback, useEffect, useRef, useState } from "react";

import { markdown } from "../_lib/markdown";
import { describeTool } from "../_lib/tool";

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
  when: string;
  at: string;
  every: string;
  prompt: string;
  paused: boolean;
  silent: boolean;
  target: string;
  last: { ts: number; ms: number; ok: boolean; text: string } | null;
};

type Event = {
  event: string;
  text?: string;
  name?: string;
  args?: string;
  id?: string;
  ms?: number;
  message?: string;
};

export function Chat() {
  const [projects, setProjects] = useState<Project[]>([]);
  const [open, setOpen] = useState("");
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
  const [agenda, setAgenda] = useState<Project | null>(null);
  const [tasks, setTasks] = useState<Task[]>([]);
  const [removingTask, setRemovingTask] = useState("");
  const [running, setRunning] = useState("");
  const [needLogin, setNeedLogin] = useState(false);
  const [sent, setSent] = useState("");
  const [link, setLink] = useState("");
  const bottom = useRef<HTMLDivElement>(null);

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
    setOpen((current) =>
      projects.some((project) => (project.conversations ?? []).some((line) => line.id === current))
        ? current
        : "",
    );
    setAgenda((current) => (projects.some((project) => project.id === current?.id) ? current : null));
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  useEffect(() => {
    if (!open) return;
    setLines([]);
    setPartial("");
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
      body: JSON.stringify({ conversation: open, text }),
    });
    setBusy(false);
    if (!response.ok) {
      setError(await response.text());
      return;
    }
    setText("");
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
    setOpen("");
    setLines([]);
    setUser("");
    setSent("");
    setLink("");
    setNeedLogin(true);
  }

  async function newProject(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const name = new FormData(event.currentTarget).get("name");
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
    event.currentTarget.reset();
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

  async function loadAgenda(slug: string) {
    const response = await fetch(`/api/agenda?project=${encodeURIComponent(slug)}`);
    if (!response.ok) {
      setError(await response.text());
      return;
    }
    const data = await response.json();
    setTasks(data.tasks ?? []);
  }

  async function openAgenda(project: Project) {
    setAgenda(project);
    setMenu(false);
    await loadAgenda(project.slug);
  }

  async function runTask(task: Task) {
    if (!agenda) return;
    setRunning(task.name);
    const response = await fetch("/api/agenda/run", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ project: agenda.slug, name: task.name }),
    });
    setRunning("");
    if (!response.ok) {
      setError(await response.text());
      return;
    }
    await loadAgenda(agenda.slug);
  }

  async function pauseTask(task: Task) {
    if (!agenda) return;
    const response = await fetch("/api/agenda", {
      method: "PATCH",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ project: agenda.slug, name: task.name, paused: !task.paused }),
    });
    if (!response.ok) {
      setError(await response.text());
      return;
    }
    await loadAgenda(agenda.slug);
  }

  function askTask(task: Task) {
    if (removingTask !== task.name) {
      setRemovingTask(task.name);
      return;
    }
    void removeTask(task);
  }

  async function removeTask(task: Task) {
    if (!agenda) return;
    setRemovingTask("");
    const query = `project=${encodeURIComponent(agenda.slug)}&name=${encodeURIComponent(task.name)}`;
    const response = await fetch(`/api/agenda?${query}`, { method: "DELETE" });
    if (!response.ok) {
      setError(await response.text());
      return;
    }
    await loadAgenda(agenda.slug);
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
    setAgenda(null);
    setOpen(line.id);
    setMenu(false);
  }

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
              className="rounded border border-neutral-700 bg-transparent px-3 py-2 text-sm"
            />
            <button type="submit" data-login-send="" className="rounded border border-neutral-700 px-3 py-2 text-sm">
              Mandame el link
            </button>
          </form>
        )}
        {error ? <p className="text-sm text-red-400">{error}</p> : null}
      </div>
    );
  }

  return (
    <div className="flex h-dvh">
      <aside className={`${menu ? "flex" : "hidden"} w-72 shrink-0 flex-col border-r border-neutral-800 md:flex`}>
        <form onSubmit={newProject} className="flex gap-2 border-b border-neutral-800 p-3">
          <input
            name="name"
            data-new-project=""
            placeholder="Nuevo proyecto"
            className="w-full rounded border border-neutral-700 bg-transparent px-2 py-1 text-sm"
          />
          <button type="submit" className="rounded border border-neutral-700 px-2 text-sm">
            +
          </button>
        </form>
        <nav className="flex-1 overflow-y-auto p-3">
          {projects.map((project) => (
            <div key={project.id} className="mb-4">
              <div className="flex items-center justify-between gap-1">
                {editing === project.id ? (
                  <Name
                    value={project.name}
                    save={(name) => rename("/api/projects", { id: project.id, name })}
                    cancel={() => setEditing("")}
                  />
                ) : (
                  <span className="truncate text-sm font-semibold">{project.name}</span>
                )}
                <span className="flex items-center gap-1">
                  <button
                    type="button"
                    onClick={() => setEditing(project.id)}
                    data-rename-project={project.id}
                    aria-label={`Renombrar ${project.name}`}
                    className="text-xs opacity-40 hover:opacity-100"
                  >
                    ✎
                  </button>
                  <button
                    type="button"
                    onClick={() => ask("/api/projects", project.id)}
                    data-delete-project={project.id}
                    aria-label={`Borrar ${project.name}`}
                    title={removing === project.id ? "otra vez para borrar" : `Borrar ${project.name}`}
                    className={`rounded px-1 text-xs ${
                      removing === project.id ? "bg-red-900 text-red-100" : "opacity-40 hover:opacity-100"
                    }`}
                  >
                    {removing === project.id ? "borrar" : "✕"}
                  </button>
                  <button
                    type="button"
                    onClick={() => newConversation(project.id)}
                    data-new-conversation={project.id}
                    className="rounded border border-neutral-700 px-1 text-xs"
                    aria-label={`Nueva conversación en ${project.name}`}
                  >
                    nueva
                  </button>
                  <button
                    type="button"
                    onClick={() => openAgenda(project)}
                    data-agenda={project.id}
                    aria-label={`Agenda de ${project.name}`}
                    className={`rounded border px-1 text-xs ${
                      agenda?.id === project.id ? "border-neutral-500" : "border-neutral-800 opacity-60"
                    }`}
                  >
                    agenda
                  </button>
                </span>
              </div>
              <ul className="mt-1">
                {(project.conversations ?? []).map((line) => (
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
                        onClick={() => {
                          setAgenda(null);
                          setOpen(line.id);
                          setMenu(false);
                        }}
                        data-conversation={line.id}
                        className={`min-w-0 flex-1 truncate rounded px-2 py-1 text-left text-sm ${
                          open === line.id ? "bg-neutral-800" : "hover:bg-neutral-900"
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
                      className="text-xs opacity-40 md:opacity-0 md:group-hover:opacity-40 hover:opacity-100"
                    >
                      ✎
                    </button>
                    <button
                      type="button"
                      onClick={() => ask("/api/conversations", line.id)}
                      data-delete-conversation={line.id}
                      aria-label={`Borrar ${line.title}`}
                      title={removing === line.id ? "otra vez para borrar" : `Borrar ${line.title}`}
                      className={`shrink-0 rounded px-1 text-xs ${
                        removing === line.id ? "bg-red-900 text-red-100" : "opacity-40 md:opacity-0 md:group-hover:opacity-40 hover:opacity-100"
                      }`}
                    >
                      {removing === line.id ? "borrar" : "✕"}
                    </button>
                  </li>
                ))}
              </ul>
            </div>
          ))}
          {projects.length === 0 ? <p className="text-sm opacity-60">Todavía no hay proyectos.</p> : null}
        </nav>
        <div className="flex items-center justify-between border-t border-neutral-800 p-3 text-xs">
          <span className="truncate opacity-60">{user}</span>
          <button type="button" onClick={logout} data-logout="" className="rounded border border-neutral-700 px-2 py-1">
            salir
          </button>
        </div>
      </aside>

      <main className="flex min-w-0 flex-1 flex-col">
        <header className="flex items-center gap-2 border-b border-neutral-800 p-3 md:hidden">
          <button type="button" onClick={() => setMenu(!menu)} className="rounded border border-neutral-700 px-2 text-sm">
            proyectos
          </button>
          <span className="truncate text-sm">{agenda ? `Agenda de ${agenda.name}` : titleOf(projects, open)}</span>
        </header>

        {agenda ? (
          <div className="flex-1 overflow-y-auto p-4">
            <div className="mx-auto flex max-w-3xl flex-col gap-3">
              <h2 className="text-sm font-semibold">Agenda de {agenda.name}</h2>
              {tasks.length === 0 ? <p className="text-sm opacity-60">No hay tareas en este proyecto.</p> : null}
              {tasks.map((task) => (
                <Task key={task.name} task={task} running={running} removing={removingTask}
                  run={() => runTask(task)} pause={() => pauseTask(task)} ask={() => askTask(task)} />
              ))}
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

        {error ? <p className="border-t border-red-900 px-4 py-2 text-sm text-red-400">{error}</p> : null}

        {agenda ? null : (
        <form onSubmit={send} className="border-t border-neutral-800 p-3">
          <div className="mx-auto flex max-w-3xl gap-2">
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
              className="w-full resize-none rounded border border-neutral-700 bg-transparent p-2 text-sm"
            />
            <button
              type="submit"
              data-send=""
              disabled={!open || busy || !text.trim()}
              className="self-end rounded border border-neutral-700 px-3 py-2 text-sm disabled:opacity-40"
            >
              Enviar
            </button>
          </div>
        </form>
        )}
      </main>
    </div>
  );
}

function Line({ line, workspace }: { line: Event; workspace: string }) {
  switch (line.event) {
    case "user":
      return (
        <div className="self-end rounded-lg bg-neutral-800 px-3 py-2 text-sm whitespace-pre-wrap">{line.text}</div>
      );
    case "assistant":
      return <Assistant text={line.text ?? ""} />;
    case "tool_start":
      return <Tool call={line} done={false} workspace={workspace} />;
    case "tool_result":
      return <Tool call={line} done={true} workspace={workspace} />;
    case "error":
      return <p className="rounded border border-red-900 px-3 py-2 text-sm text-red-400">{line.message}</p>;
    case "stopped":
      return <p className="text-xs opacity-60">frenado</p>;
    default:
      return null;
  }
}

function Task({
  task,
  running,
  removing,
  run,
  pause,
  ask,
}: {
  task: Task;
  running: string;
  removing: string;
  run: () => void;
  pause: () => void;
  ask: () => void;
}) {
  return (
    <div className="rounded border border-neutral-800 p-3 text-sm" data-task={task.name}>
      <div className="flex items-center justify-between gap-2">
        <span className="font-semibold">{task.name}</span>
        <span className="flex shrink-0 items-center gap-1 text-xs">
          <button
            type="button"
            onClick={run}
            disabled={running === task.name}
            data-run-task={task.name}
            className="rounded border border-neutral-700 px-2 py-0.5 disabled:opacity-40"
          >
            {running === task.name ? "corriendo…" : "correr"}
          </button>
          <button type="button" onClick={pause} data-pause-task={task.name} className="rounded border border-neutral-700 px-2 py-0.5">
            {task.paused ? "seguir" : "pausar"}
          </button>
          <button
            type="button"
            onClick={ask}
            data-remove-task={task.name}
            className={`rounded px-2 py-0.5 ${removing === task.name ? "bg-red-900 text-red-100" : "border border-neutral-700 opacity-60"}`}
          >
            {removing === task.name ? "borrar" : "✕"}
          </button>
        </span>
      </div>
      <p className="mt-1 text-xs opacity-60">
        {whenOf(task)}
        {task.paused ? " · pausada" : ""}
        {task.silent ? " · callada" : ""}
      </p>
      <p className="mt-2 whitespace-pre-wrap opacity-80">{task.prompt}</p>
      {task.last ? (
        <details className="mt-2 text-xs">
          <summary className="cursor-pointer opacity-60">
            última corrida {task.last.ok ? "ok" : "con error"} · {task.last.ms} ms · hace {ago(task.last.ts)}
          </summary>
          <pre className="mt-1 overflow-x-auto whitespace-pre-wrap opacity-80">{task.last.text}</pre>
        </details>
      ) : (
        <p className="mt-2 text-xs opacity-60">todavía no corrió</p>
      )}
    </div>
  );
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
      className="min-w-0 flex-1 rounded border border-neutral-700 bg-transparent px-1 text-sm"
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
        onClick={() => {
          void navigator.clipboard.writeText(text).then(() => setCopied(true));
        }}
        className="absolute top-0 right-0 rounded border border-neutral-800 bg-neutral-900 px-1 text-xs opacity-0 group-hover:opacity-60 hover:opacity-100"
      >
        {copied ? "copiado" : "copiar"}
      </button>
    </div>
  );
}

function Tool({ call, done, workspace }: { call: Event; done: boolean; workspace: string }) {
  const what = describeTool(call.name ?? "", call.args ?? "", workspace);
  return (
    <details className="rounded border border-neutral-800 text-xs">
      <summary className="cursor-pointer px-2 py-1">
        {call.name}
        <span className="ml-2 opacity-60">
          {what.dir ? what.dir + " · " : ""}
          {what.text}
        </span>
        {done ? <span className="ml-2 opacity-60">ok · {call.ms} ms</span> : null}
      </summary>
      <pre className="overflow-x-auto border-t border-neutral-800 px-2 py-1 opacity-80">
        {done ? call.text : call.args}
      </pre>
    </details>
  );
}

function titleOf(projects: Project[], open: string) {
  for (const project of projects) {
    for (const line of project.conversations ?? []) {
      if (line.id === open) return line.title;
    }
  }
  return "Goddard";
}
