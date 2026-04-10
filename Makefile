.PHONY: build run test install

build:
  go build -ldflags="-s -w" -o bin/workspace-portal ./cmd/portal

run:
  go run ./cmd/portal --config config.yaml

test:
  go test -v ./...

install: build
  cp bin/workspace-portal /usr/local/bin/workspace-portal