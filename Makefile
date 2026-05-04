BINARY := redscli
CMD := ./cmd/redscli
PKGS := ./...
GO ?= go
GOFLAGS ?= -buildvcs=false
PREFIX ?= /usr/local
DESTDIR ?=

.PHONY: help build test fmt vet check clean install run

help:
	@printf '%s\n' 'Targets:'
	@printf '  %-10s %s\n' 'build' 'Build ./redscli'
	@printf '  %-10s %s\n' 'test' 'Run Go tests'
	@printf '  %-10s %s\n' 'fmt' 'Format Go source'
	@printf '  %-10s %s\n' 'vet' 'Run go vet'
	@printf '  %-10s %s\n' 'check' 'Run fmt check, vet, and tests'
	@printf '  %-10s %s\n' 'clean' 'Remove build artifacts'
	@printf '  %-10s %s\n' 'install' 'Install binary into $(DESTDIR)$(PREFIX)/bin'
	@printf '  %-10s %s\n' 'run' 'Run the CLI, pass args with ARGS="..."'

build:
	$(GO) build $(GOFLAGS) -o $(BINARY) $(CMD)

test:
	$(GO) test $(PKGS)

fmt:
	$(GO) fmt $(PKGS)

vet:
	$(GO) vet $(PKGS)

check:
	@test -z "$$($(GO) fmt $(PKGS))" || (echo 'go fmt changed files; please review and rerun make check' >&2; exit 1)
	$(GO) vet $(PKGS)
	$(GO) test $(PKGS)

clean:
	rm -f $(BINARY)
	rm -rf bin dist
	rm -f coverage.out coverage.html *.coverprofile *.prof

install: build
	install -d "$(DESTDIR)$(PREFIX)/bin"
	install -m 0755 "$(BINARY)" "$(DESTDIR)$(PREFIX)/bin/$(BINARY)"

run:
	$(GO) run $(GOFLAGS) $(CMD) $(ARGS)
