GO      ?= go
PREFIX  ?= $(HOME)/.local
BIN     ?= $(PREFIX)/bin
TAG     := $(shell git describe --tags --exact-match 2>/dev/null | sed 's/^v//')
VERSION := $(if $(TAG),$(TAG),$(shell sed -n 's/.*version   = "\(.*\)".*/\1/p' cmd/root.go | head -1))

.PHONY: all fmt vet lint test race build install release clean

all: lint test build

fmt:
	@test -z "$(shell gofmt -l .)" || { echo "not gofmt-clean:"; gofmt -l .; exit 1; }

vet:
	$(GO) vet ./...

lint: fmt vet

test:
	$(GO) test ./... -count=1

race:
	$(GO) test ./... -count=1 -race

build:
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "-s -w -X github.com/gaurav-dhakal/vibe-check/cmd.version=$(VERSION)" -o vibe-check .

install: build
	install -d $(BIN)
	install -m 0755 vibe-check $(BIN)/vibe-check
	ln -sf vibe-check $(BIN)/vcheck

release:
	VERSION=$(VERSION) ./scripts/build-release.sh

clean:
	rm -rf dist vibe-check vcheck *.test
