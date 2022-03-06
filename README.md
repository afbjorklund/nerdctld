# nerdctld

This is a daemon offering a `nerdctl.sock` endpoint.

It can be used with `DOCKER_HOST=unix://nerdctl.sock`.

## Docker API

The Docker API (REST) is available at:

<https://docs.docker.com/engine/api/>

## Debugging

You can use cURL for talking HTTP to a Unix socket:

`curl --unix-socket /var/run/docker.sock http://localhost:2375/_ping`

## Implemented commands

* version
* info (system info)
* images (image ls)
* ps (container ls)
