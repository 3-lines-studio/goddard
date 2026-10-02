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
  one, `es-AR`. The fragments we ship travel embedded in the binary as the
  default language, and they are goddard's: they came from jimmy and say what
  goddard is, not what jimmy was. The
  Rust harness that compiles jimmy's `prompt.rs` on its own left
  `prompt/testdata/paridad-rust.txt`, and the tests replay it byte for byte.
- `auth` — who can come in: the users, the one-shot links that let them in
  and the sessions that keep them in, in the `auth` schema. It is the app's and
  not heimdall's: the vault keeps the tokens of the machines that ask it for
  secrets, and this keeps the people. A link and a session are stored as their
  hash and nothing else, and an expired one is dropped, not marked.
- `org` — the organizations: who is in one and what they can do, in the `org`
  schema of the same Postgres. An organization owns projects, has a workspace
  of its own and shares its memory with its members, so being in one is how
  several people work on the same thing. Every organization has at least one
  owner, and the roles are three: an owner does everything and decides who else
  gets in, an admin brings people in and takes them out, and a member works
  inside. Somebody comes in by the email they already signed in with, so there
  is no invitation to accept yet.
- `naming` — the slug: a name turned into what a directory and the tables can
  hold, lowercase, without accents and with single dashes. The projects and the
  organizations share it: the slug of a project names its directory, and both
  are the names the tables of goddard keep.
- `chat` — the projects and the conversations, in the `chat` schema of the
  same Postgres. A project belongs to a user or to an organization, and the
  organizations' are what several people share. Its slug is the string the rest
  of goddard already keeps in its own tables: memo, schedule and heimdall name
  the project that way. A conversation is a thread inside it,
  and its id is the scope an axe session uses, so the history of the thread is
  the history of that session and nobody keeps the two in step. The log of the
  thread (`events`) is what the stream reads with `since`, and the attachments
  (`uploads`) live in the database and not on a disk: a file in one instance's
  filesystem is a file the others cannot show. Renaming a project keeps its
  slug, and both are marked as gone instead of dropped: what happened keeps
  pointing at them.
- `heimdall` — the secrets store: an owner — an organization or a user — and
  under it the projects, the environments and the tokens scoped to one
  environment and, if you want, to a list of key names, so an agent gets test
  credentials with no path to production. Port of
  [heimdall](https://github.com/3-lines-studio/heimdall), on the same Postgres
  as the rest of goddard and in its own `heimdall` schema, with the schema as a
  migration like everything else (see `migrations/`). Nothing of the store that
  came before crosses: goddard starts with the owner in the schema and inside
  the sealed box, so there is no migration to write and nothing to be
  compatible with. The command line keeps the `doppler` dialect,
  `heimdall run --preserve-env -- npm test`. Outside: the HTTP server, the web
  page and the magic link, which bifrost and `auth` bring. It is where goddard
  keeps what it has to seal: the sandbox of an owner — where it is, who to be
  there and the key to get in — under the `sandbox` project and the `default`
  environment, sealed with `HEIMDALL_MASTER_KEY`.
- `workspace` — where the projects of an owner live, in the `workspace` schema
  of the same Postgres: the path of the volume that holds them, `/volumes/<owner
  id>` when nobody says otherwise, and the project inside it by its slug. It is
  the owner's — a person, or the organization the projects are shared with —
  and the path is stable on purpose, because a project that moves leaves every
  file behind. How to reach the machine that runs the tools over them is not
  here: it is the sandbox of `heimdall`, and where the tools of a turn run.
- `skill` — the skills: named instructions the agent loads into its context
  when the task calls for them, in the `skill` schema of the same Postgres.
  Port of jimmy's `src/skill.rs`, with the tree of directories replaced by
  rows. A skill belongs to one owner — the system, an organization or a user —
  and names the role of that owner it is for, empty meaning all of them. A
  viewer sees the system's skills, its organization's and its own, and the
  closest owner wins when the name repeats: that is jimmy's
  local-shadows-builtin, through ownership instead of directory order. The
  system's are read only and the package never writes them. It is the tool the
  harness gives the agent, with the store and the viewer already in: `list`
  renders what that viewer can see, `load` reads one, and `save` and `remove`
  write and drop the ones of the viewer's own user — a skill the agent learned
  is a skill the next turn's index already carries.
- `memo` — the memory: the durable facts the agent keeps about itself and about
  the project it is working on, in the `memo` schema of the same Postgres. Port
  of jimmy's `src/memo.rs`, with the tree of files replaced by rows. A fact is a
  key, a kind from a short list, a body and the day it was last touched, and the
  key says where it belongs: `usuario` is a general fact and `jimmy/telemetria`
  one of the project `jimmy`. Who it belongs to is the other half: the general
  facts are the person's and the ones of a project are the project's, so an
  organization shares its memory with its members and nobody reads the general
  memory of somebody else. What the prompt gets is `Render` — every general
  fact plus the newest of the project in hand — and the rest stays in the store
  until the topic comes back. `Add` is one transaction that writes the fact and
  the revision behind it, so jimmy's `sync` is gone: the store already knows
  whether it created, updated, reasserted or left a fact alone, and says so. The
  tool the harness gets is the command line jimmy had, `add`, `show` and `list`,
  and the dump jimmy's binary left in `memo/testdata/paridad-rust.txt` is
  replayed byte for byte.
- `schedule` — the agenda: the tasks the app runs on their own, in the
  `schedule` schema of the same Postgres. Port of jimmy's `src/schedule.rs`,
  with the directory of TOML files replaced by rows. A task belongs to an owner
  — the user or the organization that owns the project it lives in — and an
  owner of nobody is the project's own task, the one everybody who sees the
  project shares. What a viewer sees is their own tasks, the ones of their
  organizations and the ones of nobody, which is the same list the web shows
  and the tool hands the agent. A task says when it runs in one of three ways:
  once, every day at an hour, or every so often since its last run. The last
  twenty runs are kept, and that log is also the state of the task: the last
  one is what the interval measures from. A run of a task of an organization
  runs as the organization: the skills of the team, the memory of the project
  and the name of the organization in the prompt. What makes the agenda hold in
  the cloud is the claim: a pass takes what is due with
  `FOR UPDATE SKIP LOCKED` and holds it with a lease while it runs, so two
  instances never run the same task and one that dies mid-run leaves its task
  for the next pass. `Runner` and `Notifier` are what the app plugs in: how a
  prompt is answered, and where a copy goes when the task names a target.

Four dependencies: `golang.org/x/net` for the HTML parser behind `fetch` (the
article extraction is the one layer of `axe` that is not byte-for-byte with the
Rust, which runs Readability and htmd), `golang.org/x/crypto` and
`github.com/zeebo/blake3` for the sealed boxes and the key derived per
owner, project and environment, and `github.com/jackc/pgx/v5` for the store.
`prompt`, `skill` and `schedule` do not add one: fragments, variables, skills and
rows are the standard library and nothing else.

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
make -C web test      # bun test: the frontend's own
```

`web/app/_lib/` holds what the frontend does on its own: the markdown renderer
and the line that describes a tool call, both ported from jimmy along with the
tests that came with them.

What the routes answer today:

```
GET  /api/health                        the database answers
GET  /api/state                         the projects and their conversations
POST /api/projects                      {name, org}: opens one for whoever asks, or for their organization
PATCH  /api/projects                    {id, name}: renames it, slug untouched
DELETE /api/projects?id=                takes it out of the list
POST /api/conversations                 {project, title}
PATCH  /api/conversations               {id, title}: renames it
DELETE /api/conversations?id=           takes the thread out of the list
POST /api/turns                         {conversation, text, uploads}: 202, and the turn runs
POST /api/turns/stop                    {conversation}: cuts the turn short
POST /api/uploads                       multipart {conversation, file}: one attachment
GET  /api/uploads?id=                   the bytes of one, as they were sent
GET  /api/events?conversation=&since=   the log of the thread
GET  /api/stream?conversation=&since=   the same log, live: text/event-stream
GET    /api/agenda?project=             the tasks of that project, with their last run
PATCH  /api/agenda                      {project, name, paused}: pauses it or lets it go
DELETE /api/agenda?project=&name=       takes it out of the agenda
POST   /api/agenda/run                  {project, name}: runs it now, and answers what it said
POST /api/orgs                          {name}: opens one, and whoever asks is its owner
GET  /api/orgs                          the organizations of whoever is asking, with their role
GET    /api/orgs/members?org=           who is in one, with the role of each
POST   /api/orgs/members                {org, email, role}: brings somebody in by their mail
PATCH  /api/orgs/members                {org, user, role}: changes what they can do
DELETE /api/orgs/members?org=&user=      takes somebody out
GET  /api/workspace?org=                where the projects of an owner live and how to reach its sandbox
POST /api/workspace                     {org, path, addr, user, key}: loads it; what comes empty stays as it is
DELETE /api/workspace?org=              unloads it: the volume by default, and a turn does not run
POST /api/login                         {email}: mails a one-shot link, or hands it back
GET  /auth?token=                       burns the link, sets the cookie and goes home
POST /api/logout                        signs the session out
```

Serving also starts the agenda: every minute `schedule.Service` looks for what
is due and runs it with the same agent a turn uses, in a conversation the task
owns inside its project — made the first time the task fires and named after
it. The run is written down either way, in `schedule.runs`, and its thread is
where the answer is read: a task that runs alone leaves the same trail as one
asked for by hand. The local hour is UTC plus `GODDARD_TZ_OFFSET` hours, zero
by default. A turn can be cut short: `POST /api/turns/stop` cancels the context the turn
runs with, and the log gets a `stopped` and then its `done`. It stops between
steps — the model's next token, the next tool — so a command already running
finishes on its own, the way it does in axe.

Every project has a directory of its own in the workspace, under the owner of
the project and its slug — `<owner>/<slug>`, inside `/volumes`, with the id of
the owner in front and not its name — and that is where the tools of a turn
run. Two people name their projects the same way, so the owner comes first; and
two projects are two trees, so one never reads the files of the other. That
directory is not in the container goddard runs in: it hangs from the volume the
sandbox mounts, and the tools run in the sandbox, over ssh, with the three
secrets the owner loaded in heimdall. Without a sandbox the turn does not run
at all.

A message may carry files. They are kept in the database — any instance serves
any conversation — and written into that workspace when the turn runs, under
`files/<conversation>/`, because that is where the agent's tools look: what it
reads is the same file the thread is showing. `POST /api/uploads` takes one and
names it, `POST /api/turns` puts it in the message, and the log keeps a `file`
event so the thread shows it. The way in is not only the web: the agent has a
`send` tool that takes a file from the workspace and does the same three
things, which is how a screenshot it took ends up in the thread.

The `memoria` button next to it is what the agent knows: the skills this
viewer can see and the facts about the project, which is the same index and the
same memory the prompt hands the model.

The `agenda` button in the header of a project is that list: what
each task is, when it runs, what it answered last, and the three buttons that
run it now, pause it and take it out. A member of the organization sees the
tasks of the projects of the team in the same list, and can pause or take out
the ones of the team.

The `computadoras` button at the bottom of the rail is where the sandbox of
whoever the panel is for is loaded: the volume where the projects live and the
address, the user and the key of the machine that runs the tools. There is one
card per owner — the person, and each organization they are in — and the key is
asked for and never shown: what comes back is whether there is one. The key of
an organization is loaded by its owners and its admins. `sacar la computadora`
unloads it: the three secrets go and the projects go back to the volume by
default, which is how somebody says that machine is not theirs anymore.

Everything under `/api/` except the health check wants the session cookie, and
answers 401 without it. Who may ask for a link is `GODDARD_ALLOWED_EMAILS`, a
comma separated list; empty means anybody, which is a goddard of one.

Who a turn is for comes from the session and not from the environment: the id
the database minted is what the skills, the agenda and the memory keep, and the
email is only how somebody comes in and can change.

To run it: `DATABASE_URL`, `OPENAI_API_KEY` and `HEIMDALL_MASTER_KEY`, plus
`GODDARD_BASE`, `GODDARD_MODEL` and `GODDARD_ALLOWED_EMAILS` when the defaults
do not fit. The master key is 32 bytes in hex, the same in every instance of
the same goddard, and nothing opens again if it changes. The
link goes out through Resend with `RESEND_API_KEY` and `GODDARD_WEB_FROM`;
without a key the link comes back in the response instead of in an email, which
is how it is used in development.

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
TEST_DATABASE_URL=postgres://… go test ./...   # suma el store, las migraciones y las rutas
```

Every route has its test, and they go through the route and not around it: the
test starts the app with `apptest.Route` — the same `app.Start` the binary calls
— and then calls the `Post`, `Get`, `Patch` or `Delete` of the package, the
functions the router reaches. No browser and no binary in the middle.
