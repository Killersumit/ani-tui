APP := ani-tui
PREFIX ?= /usr/local
BINDIR ?= $(PREFIX)/bin
GO ?= go
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || printf dev)
COMMIT ?= $(shell git rev-parse --short=12 HEAD 2>/dev/null || printf unknown)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(BUILD_DATE)

.PHONY: all build test vet check install uninstall clean release

all: build

build:
	$(GO) build -trimpath -ldflags="$(LDFLAGS)" -o $(APP) .

test:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

check: test vet

# For an unprivileged user install, run: make install PREFIX="$$HOME/.local"
install: build
	install -d "$(DESTDIR)$(BINDIR)"
	install -m 0755 $(APP) "$(DESTDIR)$(BINDIR)/$(APP)"

# Removing ani-tui never removes ani-cli history or user configuration.
uninstall:
	rm -f "$(DESTDIR)$(BINDIR)/$(APP)"

release:
	VERSION=$(VERSION) sh ./scripts/build-release.sh

clean:
	rm -f $(APP)
	rm -rf release
