.PHONY: build deploy run test install tailscale-prune

build:
	go build -ldflags="-s -w" -o bin/workspace-portal ./cmd/portal

deploy: build
	cp bin/workspace-portal /usr/local/bin/portal

run:
	go run ./cmd/portal --config config.yaml

test:
	go test -v ./...

tailscale-prune:
	./bin/tailscale-prune

install: deploy
	./deploy/launchd/install.sh
