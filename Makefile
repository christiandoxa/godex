GO ?= go
BINARY ?= bin/godex

.PHONY: build test race vet fmt format-check source-size tui-framework cross-build installer-check verify clean snapshot

build:
	mkdir -p $(dir $(BINARY))
	$(GO) build -trimpath -o $(BINARY) ./cmd/godex

test:
	$(GO) test -shuffle=on -count=1 ./...

race:
	$(GO) test -race -shuffle=on -count=1 ./...

vet:
	$(GO) vet ./...

fmt:
	gofmt -w cmd internal

format-check:
	./scripts/check-format.sh

source-size:
	./scripts/check-source-size.sh

tui-framework:
	./scripts/check-tui-framework.sh

cross-build:
	./scripts/check-cross-build.sh

installer-check:
	sh -n install.sh
	./scripts/test-installers.sh

verify: format-check source-size tui-framework cross-build installer-check vet race build

snapshot: verify
	goreleaser release --snapshot --clean

clean:
	rm -rf bin dist coverage.*
