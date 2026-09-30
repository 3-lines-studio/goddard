# Goddard

Jimmy as a service: the agent, in Go, so bifrost can embed it.

## Packages

- `axe` — the agent harness: the loop, an OpenAI-compatible provider, the tools,
  the search and the page fetcher, project-scoped sessions, compaction, and the
  outbound secret sentinel. Port of [axe](https://github.com/3-lines-studio/axe),
  verified against it: `axe/testdata/` holds dumps of real Rust output that the
  tests replay byte for byte.
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
pure Go, to read the old store once and bring it across.

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

## Check

```
go vet ./...
go test ./...
TEST_DATABASE_URL=postgres://… go test ./...   # suma el store y las migraciones
```
