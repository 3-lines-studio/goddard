import { useCallback, useEffect, useRef, useState } from "react";

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
    setProjects(data.projects ?? []);
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
              <div className="flex items-center justify-between">
                <span className="text-sm font-semibold">{project.name}</span>
                <button
                  type="button"
                  onClick={() => newConversation(project.id)}
                  data-new-conversation={project.id}
                  className="rounded border border-neutral-700 px-1 text-xs"
                  aria-label={`Nueva conversación en ${project.name}`}
                >
                  nueva
                </button>
              </div>
              <ul className="mt-1">
                {(project.conversations ?? []).map((line) => (
                  <li key={line.id}>
                    <button
                      type="button"
                      onClick={() => {
                        setOpen(line.id);
                        setMenu(false);
                      }}
                      data-conversation={line.id}
                      className={`w-full truncate rounded px-2 py-1 text-left text-sm ${
                        open === line.id ? "bg-neutral-800" : "hover:bg-neutral-900"
                      }`}
                    >
                      {line.title}
                      {line.running ? <span className="ml-1 text-xs opacity-60">·</span> : null}
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
          <span className="truncate text-sm">{titleOf(projects, open)}</span>
        </header>

        <div className="flex-1 overflow-y-auto p-4">
          {!open ? <p className="opacity-60">Elegí una conversación o creá una nueva.</p> : null}
          <div className="mx-auto flex max-w-3xl flex-col gap-3">
            {lines.map((line, index) => (
              <Line key={index} line={line} />
            ))}
            {partial ? <div className="text-sm whitespace-pre-wrap">{partial}</div> : null}
          </div>
          <div ref={bottom} />
        </div>

        {error ? <p className="border-t border-red-900 px-4 py-2 text-sm text-red-400">{error}</p> : null}

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
      </main>
    </div>
  );
}

function Line({ line }: { line: Event }) {
  switch (line.event) {
    case "user":
      return (
        <div className="self-end rounded-lg bg-neutral-800 px-3 py-2 text-sm whitespace-pre-wrap">{line.text}</div>
      );
    case "assistant":
      return <div className="text-sm whitespace-pre-wrap">{line.text}</div>;
    case "tool_start":
      return (
        <details className="rounded border border-neutral-800 text-xs">
          <summary className="cursor-pointer px-2 py-1">
            {line.name}
            <span className="ml-2 opacity-60">{summaryOf(line.args)}</span>
          </summary>
          <pre className="overflow-x-auto border-t border-neutral-800 px-2 py-1 opacity-80">{line.args}</pre>
        </details>
      );
    case "tool_result":
      return (
        <details className="rounded border border-neutral-800 text-xs">
          <summary className="cursor-pointer px-2 py-1">
            {line.name}
            <span className="ml-2 opacity-60">ok · {line.ms} ms</span>
          </summary>
          <pre className="overflow-x-auto border-t border-neutral-800 px-2 py-1 opacity-80">{line.text}</pre>
        </details>
      );
    case "error":
      return <p className="rounded border border-red-900 px-3 py-2 text-sm text-red-400">{line.message}</p>;
    case "stopped":
      return <p className="text-xs opacity-60">frenado</p>;
    default:
      return null;
  }
}

function summaryOf(args?: string) {
  if (!args) return "";
  return args.length > 80 ? args.slice(0, 80) + "…" : args;
}

function titleOf(projects: Project[], open: string) {
  for (const project of projects) {
    for (const line of project.conversations ?? []) {
      if (line.id === open) return line.title;
    }
  }
  return "Goddard";
}
