
GO ?= go

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

.PHONY: clean
clean:
	$(RM) nerdctld
