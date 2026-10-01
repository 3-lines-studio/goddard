APP := .

.PHONY: install build dev serve test clean

install:
	bun install

build:
	go run github.com/3-lines-studio/bifrost/cmd/bifrost build $(APP)

dev:
	go run github.com/3-lines-studio/bifrost/cmd/bifrost dev $(APP)

test:
	bun test

serve: build
	./.bifrost/bifrost-app

clean:
	rm -rf .bifrost
