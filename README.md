# nerdctld

This is a daemon offering a `nerdctl.sock` endpoint.

It can be used with `DOCKER_HOST=unix://nerdctl.sock`.

## Implemented commands

* version
* info (system info)
* images (image ls)
* ps (container ls)
