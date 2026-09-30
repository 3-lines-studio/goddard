# Goddard

Jimmy as a service: the agent, in Go, so bifrost can embed it.

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
  directory the embedder puts first overrides the one that ships with the
  binary, and a fragment a language does not have falls back to the default
  one, `es-AR`. The fragments we ship — jimmy's prompts, as they are — are the
  default language and travel embedded in the binary. The Rust harness that
  compiles jimmy's `prompt.rs` on its own left
  `prompt/testdata/paridad-rust.txt`, and the tests replay it byte for byte.
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
  page and the magic link, which bifrost brings.

Five dependencies: `golang.org/x/net` for the HTML parser behind `fetch` (the
article extraction is the one layer of `axe` that is not byte-for-byte with the
Rust, which runs Readability and htmd), `golang.org/x/crypto` and
`github.com/zeebo/blake3` for the sealed boxes and the key derived per
environment, `github.com/jackc/pgx/v5` for the store, and `modernc.org/sqlite`,
pure Go, to read the old store once and bring it across. `prompt` does not add
one: fragments and variables are the standard library and nothing else.

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
`axe.NewPgStore(db, scope)` — the pool the service already has for heimdall,
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

The scope is whatever the embedder says a conversation set is — a chat, a
project, a user — and two services over the same database and scope see the
same history. There is no store on files: the port of the Rust `FsStore` is
gone, because a cloud axe has no volume to mount.

## Check

```
go vet ./...
go test ./...
TEST_DATABASE_URL=postgres://… go test ./...   # suma el store y las migraciones
```
