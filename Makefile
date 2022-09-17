
GO ?= go

PREFIX ?= /usr/local

all: binaries

nerdctld:
	$(GO) build -o $@ $(BUILDFLAGS)

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

.PHONY: artifacts
artifacts:
	$(RM) nerdctld
	GOOS=linux GOARCH=amd64 \
	GO111MODULE=on CGO_ENABLED=0 $(MAKE) binaries \
	BUILDFLAGS="-ldflags '-s -w'"
	tar --owner=0 --group=0 -czvf nerdctld-linux-amd64.tar.gz nerdctld docker.sh
	$(RM) nerdctld
	GOOS=linux GOARCH=arm64 \
	GO111MODULE=on CGO_ENABLED=0 $(MAKE) binaries \
	BUILDFLAGS="-ldflags '-s -w'"
	tar --owner=0 --group=0 -czvf nerdctld-linux-arm64.tar.gz nerdctld docker.sh
	$(RM) nerdctld

.PHONY: clean
clean:
	$(RM) nerdctld
