
GO ?= go
TAR ?= tar

PREFIX ?= /usr/local

all: binaries

VERSION = 0.7.0

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
artifacts: artifacts-linux-amd64 artifacts-linux-arm64
artifacts: artifacts-darwin-amd64 artifacts-darwin-arm64
artifacts: artifacts-linux-arm7 artifacts-linux-riscv64
artifacts: artifacts-linux-s390x artifacts-linux-ppc64le
artifacts-%:
	$(RM) nerdctld
	GOOS=$(firstword $(subst -, ,$*)) GOARCH=$(subst arm7,arm GOARM=7,$(lastword $(subst -, ,$*))) \
	GO111MODULE=on CGO_ENABLED=0 $(MAKE) binaries \
	BUILDFLAGS="-ldflags '-s -w' -trimpath"
	test "$(firstword $(subst -, ,$*))" != "linux" || \
	GOOS=$(firstword $(subst -, ,$*)) GOARCH=$(lastword $(subst -, ,$*)) VERSION=$(VERSION) nfpm pkg --packager deb
	test "$(firstword $(subst -, ,$*))" != "linux" || \
	GOOS=$(firstword $(subst -, ,$*)) GOARCH=$(lastword $(subst -, ,$*)) VERSION=$(VERSION) nfpm pkg --packager rpm
	test "$(firstword $(subst -, ,$*))" != "linux" || script="docker.sh"; \
	$(TAR) --owner=0 --group=0 -czvf nerdctld-$(VERSION)-$(firstword $(subst -, ,$*))-$(subst arm7,arm-v7,$(lastword $(subst -, ,$*))).tar.gz nerdctld $$script

.PHONY: clean
clean:
	$(RM) nerdctld
