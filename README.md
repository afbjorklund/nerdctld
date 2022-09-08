# nerdctld

![nerd daemon](nerdctld.png)

This is a daemon offering a `nerdctl.sock` endpoint.

It can be used with `DOCKER_HOST=unix://nerdctl.sock`.

Normally the `nerdctl` tool is a CLI-only application.

A client for the `containerd` and `buildkitd` servers.

## Docker API

The Docker API (REST) is available at:

<https://docs.docker.com/engine/api/>

Docker version | API version
--- | ---
20.10 | 1.41
19.03 | 1.40
18.09 | 1.39
... | ...
17.03 | 1.26
1.13 | 1.25
1.12 | 1.24

## Debugging

You can use cURL for talking HTTP to a Unix socket:

`curl --unix-socket /var/run/docker.sock http://localhost:2375/_ping`

## Implementation

This program uses the "Gin" web framework for HTTP.

It and docs can be found at <https://gin-gonic.com/> with some nice [examples](https://github.com/gin-gonic/examples)

## Implemented commands

* version
* info (system info)
* images (image ls)
* load (image load)
* pull (image pull)
* ps (container ls)
* save (image save)
* build

Note: using "build" requires the `buildctl` client.

It also requires a running moby `buildkitd` server.
