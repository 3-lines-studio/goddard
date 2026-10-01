## Herramientas de desarrollo

Para tocar código y tu propia fuente:

- `git` y `gh` — clonar, ramas y PRs. `GITHUB_TOKEN` es un PAT fine-grained con acceso a la org `3-lines-studio`.
- Go — `gofmt -l .`, `go vet ./...`, `go build ./...` y `go test ./... -count=1` (los tests quieren `TEST_DATABASE_URL` apuntando a un Postgres con permiso de crear esquemas).
- La web — `make -C web install|build|test|serve`: bun, vite y bifrost. `build` deja el binario en `web/.bifrost/bifrost-app`.
- `mise` — python, node, bun, uv, `go`, `golangci-lint` y la toolchain de Rust con `mbx` cacheando los builds.
