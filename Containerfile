FROM mirror.gcr.io/library/golang:alpine AS builder

RUN apk update && apk add --no-cache git

WORKDIR /build

COPY go.mod go.sum *.go .

RUN go get -d -v

RUN go build -ldflags="-w -s" -o /nerdctld


FROM ghcr.io/containerd/nerdctl:v1.7.7

COPY --from=builder /nerdctld /usr/local/bin/nerdctld

ENTRYPOINT ["/usr/local/bin/nerdctld"]
