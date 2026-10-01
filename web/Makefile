APP := .

.PHONY: install build dev serve clean

install:
	bun install

build:
	go run github.com/3-lines-studio/bifrost/cmd/bifrost build $(APP)

dev:
	go run github.com/3-lines-studio/bifrost/cmd/bifrost dev $(APP)

serve: build
	./.bifrost/bifrost-app

clean:
	rm -rf .bifrost
