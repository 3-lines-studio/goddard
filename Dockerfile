FROM golang:1.27-bookworm AS build

COPY --from=oven/bun:1.4.2-debian /usr/local/bin/bun /usr/local/bin/bun

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY web/package.json web/bun.lock web/
RUN cd web && bun install --frozen-lockfile

COPY . .
RUN make -C web build

FROM debian:bookworm-slim

RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates \
 && rm -rf /var/lib/apt/lists/*

COPY --from=build /src/web/.bifrost/bifrost-app /usr/local/bin/goddard

EXPOSE 8080

CMD ["goddard"]
