# Goddard

Jimmy as a service: the agent, in Go, as a bifrost app over the packages
below.

## Packages

- `axe` — the agent harness: the loop, an OpenAI-compatible provider, the tools,
  the search and the page fetcher, sessions, compaction, and the outbound
  secret sentinel. Port of [axe](https://github.com/3-lines-studio/axe),
  verified against it: `axe/testdata/` holds dumps of real Rust output that the
  tests replay byte for byte. The history lives in Postgres, in the `axe`
  schema of the same database as the rest of goddard, so an axe in the cloud
  resumes a conversation from any instance and needs no volume.
- `prompt` — the system prompt as fragments: a spec names them in order,
  each one is a markdown file with `{{variables}}`, and the language picks
  which file answers. Port of jimmy's assembler with the languages in it: a
  fragment is `<language>/<name>.md` inside every `fs.FS`, in order, so a
  directory the app puts first overrides the one that ships with the
  binary, and a fragment a language does not have falls back to the default
  one, `es-AR`. The fragments we ship are jimmy's, adapted as the port
  reaches them, and travel embedded in the binary as the default language. The
  Rust harness that compiles jimmy's `prompt.rs` on its own left
  `prompt/testdata/paridad-rust.txt`, and the tests replay it byte for byte.
- `auth` — who can come in: the users, the one-shot links that let them in
  and the sessions that keep them in, in the `auth` schema. It is the app's and
  not heimdall's: the vault keeps the tokens of the machines that ask it for
  secrets, and this keeps the people. A link and a session are stored as their
  hash and nothing else, and an expired one is dropped, not marked.
- `chat` — the projects and the conversations, in the `chat` schema of the
  same Postgres. A project is a place where work lives and its slug is the
  string the rest of goddard already keeps in its own tables: memo, schedule
  and heimdall name the project that way. A conversation is a thread inside it,
  and its id is the scope an axe session uses, so the history of the thread is
  the history of that session and nobody keeps the two in step. The log of the
  thread (`events`) is what the stream reads with `since`, and the attachments
  (`uploads`) live in the database and not on a disk: a file in one instance's
  filesystem is a file the others cannot show. Renaming a project keeps its
  slug, and both are marked as gone instead of dropped: what happened keeps
  pointing at them.
- `heimdall` — the secrets store: projects, environments and tokens scoped to
  one environment and, if you want, to a list of key names, so an agent gets
  test credentials with no path to production. Port of
  [heimdall](https://github.com/3-lines-studio/heimdall), on the same Postgres
  as the rest of goddard and in its own `heimdall` schema, with the schema as a
  migration like everything else (see `migrations/`). Verified both ways: what
  the Rust crate left in its SQLite file imports into this store with the
  sealed values intact, and the Rust server serves a store this one wrote. The
  command line keeps the `doppler` dialect,
  `heimdall run --preserve-env -- npm test`. Outside: the HTTP server, the web
  page and the magic link, which bifrost and `auth` bring.
- `skill` — the skills: named instructions the agent loads into its context
  when the task calls for them, in the `skill` schema of the same Postgres.
  Port of jimmy's `src/skill.rs`, with the tree of directories replaced by
  rows. A skill belongs to one owner — the system, an organization or a user —
  and names the role of that owner it is for, empty meaning all of them. A
  viewer sees the system's skills, its organization's and its own, and the
  closest owner wins when the name repeats: that is jimmy's
  local-shadows-builtin, through ownership instead of directory order. The
  system's are read only and nobody installs them through this package. It is
  the tool the harness gives the agent, with the store and the viewer already
  in: `list` renders what that viewer can see and `load` reads one.
- `memo` — the memory: the durable facts the agent keeps about itself and about
  the project it is working on, in the `memo` schema of the same Postgres. Port
  of jimmy's `src/memo.rs`, with the tree of files replaced by rows. A fact is a
  key, a kind from a short list, a body and the day it was last touched, and the
  key says where it belongs: `usuario` is a general fact and `jimmy/telemetria`
  one of the project `jimmy`. What the prompt gets is `Render` — every general
  fact plus the newest of the project in hand — and the rest stays in the store
  until the topic comes back. `Add` is one transaction that writes the fact and
  the revision behind it, so jimmy's `sync` is gone: the store already knows
  whether it created, updated, reasserted or left a fact alone, and says so. The
  tool the harness gets is the command line jimmy had, `add`, `show` and `list`,
  and the dump jimmy's binary left in `memo/testdata/paridad-rust.txt` is
  replayed byte for byte.
- `schedule` — the agenda: the tasks the app runs on their own, in the
  `schedule` schema of the same Postgres. Port of jimmy's `src/schedule.rs`,
  with the directory of TOML files replaced by rows. A task belongs to a user
  inside a project — an empty user is the project's own task, the one everybody
  shares — and says when it runs in one of three ways: once, every day at an
  hour, or every so often since its last run. The last twenty runs are kept,
  and that log is also the state of the task: the last one is what the interval
  measures from. What makes the agenda hold in the cloud is the claim: a pass
  takes what is due with `FOR UPDATE SKIP LOCKED` and holds it with a lease
  while it runs, so two instances never run the same task and one that dies
  mid-run leaves its task for the next pass. `Runner` and `Notifier` are what
  the app plugs in: how a prompt is answered, and where a copy goes when
  the task names a target.

Five dependencies: `golang.org/x/net` for the HTML parser behind `fetch` (the
article extraction is the one layer of `axe` that is not byte-for-byte with the
Rust, which runs Readability and htmd), `golang.org/x/crypto` and
`github.com/zeebo/blake3` for the sealed boxes and the key derived per
environment, `github.com/jackc/pgx/v5` for the store, and `modernc.org/sqlite`,
pure Go, to read the old store once and bring it across. `prompt`, `skill` and
`schedule` do not add one: fragments, variables, skills and rows are the
standard library and nothing else.

## Embed

```go
provider := axe.NewOpenAI("https://api.openai.com/v1", apiKey)
tools := axe.BuildTools("/path/to/project")
options := &axe.RunOptions{
    Model:    "gpt-4.1-mini",
    System:   axe.SystemPrompt(tools),
    Tools:    tools,
    MaxTurns: math.MaxInt,
}
sink := &axe.SinkBase{}
end := axe.RunStream(ctx, provider, options, history, sink)
```

`end.Messages` is the grown transcript and `end.Outcome` says why the run
stopped (`OutcomeDone`, `OutcomeMaxTurns`, `OutcomeCancelled`,
`OutcomeCompact`, `OutcomeFailed`).

The transcript is saved through a `Store`. The one that ships is
`axe.NewPgStore(db, scope)` — the pool the app already has for heimdall,
with the migrations applied first and the driver registered by whoever opens
it (`_ "github.com/jackc/pgx/v5/stdlib"`):

```go
db, _ := sql.Open("pgx", os.Getenv("DATABASE_URL"))
migrations.Apply(ctx, db)
store := axe.NewPgStore(db, "chat-1")

entries := make([]axe.Entry, 0, len(end.Messages))
for _, message := range end.Messages {
    entries = append(entries, axe.MessageEntry(message))
}
store.Append(ctx, entries)
```

The scope is whatever the app says a conversation set is — a chat, a
project, a user — and two instances over the same database and scope see the
same history. There is no store on files: the port of the Rust `FsStore` is
gone, because a cloud axe has no volume to mount.

## Web

`web/` is the app: a bifrost tree whose `app/server.go` opens the database,
applies the migrations and then serves. The page and the API routes live in
`web/app/`, and the frontend is built by Vite through bifrost.

```
make -C web install   # bun install
make -C web build     # bifrost build, leaves .bifrost/bifrost-app
make -C web serve     # runs it, DATABASE_URL in the environment
```

What the routes answer today:

```
GET  /api/health                        the database answers
GET  /api/state                         the projects and their conversations
POST /api/projects                      {name}
POST /api/conversations                 {project, title}
POST /api/turns                         {conversation, text}: 202, and the turn runs
GET  /api/events?conversation=&since=   the log of the thread
```

To run it: `DATABASE_URL` and `OPENAI_API_KEY`, plus `GODDARD_BASE`,
`GODDARD_MODEL`, `GODDARD_WORKSPACE`, `GODDARD_USER` and `GODDARD_ORG` when the
defaults do not fit.

## Migrate

The schema is not applied by the binary the agent travels in: it is
`cmd/migrate`, for whoever operates the app. The external one does not
carry it, so asking for a skill cannot change the database.

```
migrate            aplica lo que falte
migrate status     qué corrió y qué no
```

## Check

```
go vet ./...
go test ./...
TEST_DATABASE_URL=postgres://… go test ./...   # suma el store y las migraciones
```
