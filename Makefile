BINARY  := ssh-holdem
PORT    ?= 2222
GOFILES := $(shell find . -name '*.go' -not -path './.git/*')

# Where a `make install` deployment puts things.
PREFIX  ?= /usr/local
UNIT    := /etc/systemd/system/$(BINARY).service

.DEFAULT_GOAL := help

## help: list the targets
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## /  /' | sort

## build: compile the server
build: $(BINARY)

$(BINARY): $(GOFILES) go.mod go.sum
	go build -o $(BINARY) .

## run: build and deal on PORT (default 2222)
run: build
	./$(BINARY) -port $(PORT)

## test: run every test with the race detector
test:
	go test ./... -race

## test-short: run the tests without the race detector, for a quick loop
test-short:
	go test ./...

## cover: report coverage per package
cover:
	@go test ./... -cover

## cover-html: open a coverage report in the browser
cover-html:
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out

## soak: run the engine's fuzz gate on repeat, to shake out flakes
soak:
	go test ./game/ -run TestRandomTablePreservesChips -count=20 -race

## fmt: format every file
fmt:
	gofmt -w $(GOFILES)

## lint: check formatting and run go vet
lint:
	@unformatted=$$(gofmt -l . ); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt needed:"; echo "$$unformatted"; exit 1; \
	fi
	go vet ./...

## check: what CI runs, and what to run before committing
check: lint test

## tidy: prune and verify go.mod
tidy:
	go mod tidy
	go mod verify

## clean: remove build output, keeping the host key
clean:
	rm -f $(BINARY) coverage.out

## clean-hostkey: forget the server's ssh identity, so it regenerates
clean-hostkey:
	@echo "Every player will get a host key warning on their next connection."
	rm -rf .ssh

## install: install the binary to PREFIX/bin (default /usr/local)
install: build
	install -Dm755 $(BINARY) $(PREFIX)/bin/$(BINARY)

## uninstall: remove an installed binary
uninstall:
	rm -f $(PREFIX)/bin/$(BINARY)

## service: print a systemd unit for running this on a server
service:
	@printf '%s\n' \
		'[Unit]' \
		'Description=ssh-holdem' \
		'After=network-online.target' \
		'' \
		'[Service]' \
		'ExecStart=$(PREFIX)/bin/$(BINARY) -port $(PORT)' \
		'# The host key lives in this directory, so it has to survive' \
		'# restarts or every player gets a changed-key warning.' \
		'WorkingDirectory=/var/lib/$(BINARY)' \
		'StateDirectory=$(BINARY)' \
		'DynamicUser=yes' \
		'Restart=always' \
		'RestartSec=2' \
		'NoNewPrivileges=yes' \
		'PrivateTmp=yes' \
		'ProtectSystem=strict' \
		'ProtectHome=yes' \
		'' \
		'[Install]' \
		'WantedBy=multi-user.target'
	@echo
	@echo "# Write it with:  make service | sudo tee $(UNIT)"
	@echo "# Then:           sudo systemctl enable --now $(BINARY)"

.PHONY: help build run test test-short cover cover-html soak fmt lint check tidy \
	clean clean-hostkey install uninstall service
