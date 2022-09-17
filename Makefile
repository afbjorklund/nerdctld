
GO ?= go

PREFIX ?= /usr/local

all: binaries

nerdctld:
	$(GO) build -o $@

.PHONY: binaries
binaries: nerdctld

.PHONY: lint
lint:
	golangci-lint run

.PHONY: fix
fix:
	golangci-lint run --fix

.PHONY: install
install: nerdctld
	install -D -m 755 nerdctld $(DESTDIR)$(PREFIX)/bin/nerdctld
	install -D -m 755 nerdctl.service $(DESTDIR)$(PREFIX)/lib/systemd/system/nerdctl.service
	install -D -m 755 nerdctl.service $(DESTDIR)$(PREFIX)/lib/systemd/user/nerdctl.service

.PHONY: clean
clean:
	$(RM) nerdctld
