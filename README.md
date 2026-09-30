# Goddard

Jimmy as a service: the agent, in Go, so bifrost can embed it.

## Packages

- `axe` — the agent harness: the loop, an OpenAI-compatible provider, the tools,
  the search and the page fetcher, project-scoped sessions, compaction, and the
  outbound secret sentinel. Port of [axe](https://github.com/3-lines-studio/axe),
  verified against it: `axe/testdata/` holds dumps of real Rust output that the
  tests replay byte for byte.

One dependency: `golang.org/x/net`, for the HTML parser behind `fetch`. The
article extraction is the one layer that is not byte-for-byte with the Rust,
which runs Readability and htmd.

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
```
