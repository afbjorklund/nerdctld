
GO ?= go
TAR ?= tar

PREFIX ?= /usr/local

all: binaries

VERSION = 0.6.0

nerdctld: main.go go.mod
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
	install -D -m 755 nerdctl.socket $(DESTDIR)$(PREFIX)/lib/systemd/system/nerdctl.socket
	install -D -m 755 nerdctl.service $(DESTDIR)$(PREFIX)/lib/systemd/user/nerdctl.service
	install -D -m 755 nerdctl.socket $(DESTDIR)$(PREFIX)/lib/systemd/user/nerdctl.socket

# "nerdctld"
.NOTPARALLEL:

.PHONY: artifacts
artifacts: artifacts-amd64 artifacts-arm64 artifacts-arm7 artifacts-riscv64 artifacts-s390x artifacts-ppc64le
artifacts-%:
	$(RM) nerdctld
	GOOS=linux GOARCH=$(subst arm7,arm GOARM=7,$*) \
	GO111MODULE=on CGO_ENABLED=0 $(MAKE) binaries \
	BUILDFLAGS="-ldflags '-s -w' -trimpath"
	GOOS=linux GOARCH=$* VERSION=$(VERSION) nfpm pkg --packager deb
	GOOS=linux GOARCH=$* VERSION=$(VERSION) nfpm pkg --packager rpm
	$(TAR) --owner=0 --group=0 -czvf nerdctld-$(VERSION)-linux-$(subst arm7,arm-v7,$*).tar.gz nerdctld docker.sh

.PHONY: clean
clean:
	$(RM) nerdctld
