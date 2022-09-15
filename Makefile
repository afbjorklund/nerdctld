
GO ?= go

all: binaries

nerdctld:
	$(GO) build -o $@

binaries: nerdctld

.PHONY: lint
lint:
	golangci-lint run

.PHONY: lint
fix:
	golangci-lint run --fix

.PHONY: clean
clean:
	$(RM) nerdctld