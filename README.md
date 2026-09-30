# Goddard

Jimmy as a service: the agent, in Go, so bifrost can embed it.

## Packages

- `axe` — the agent harness: the loop, an OpenAI-compatible provider, the
  tools, project-scoped sessions, compaction, and the outbound secret
  sentinel. Port of [axe](https://github.com/3-lines-studio/axe), with no
  external dependencies.

## Check

```
go vet ./...
go test ./...
```
