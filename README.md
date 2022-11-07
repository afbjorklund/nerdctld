# nerdctld

![nerd daemon](nerdctld.png)

This is a daemon offering a `nerdctl.sock` endpoint.

It can be used with `DOCKER_HOST=unix://nerdctl.sock`.

Normally the `nerdctl` tool is a CLI-only application.

A client for the `containerd` and `buildkitd` servers.

<https://github.com/containerd/nerdctl>

<https://github.com/docker/cli> `docker`

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

## Diagram

![diagram](diagram.png)

* `docker.sock`
* `nerdctl.sock`
* `containerd.sock`
* `buildkit.sock`

## Debugging

You can use cURL for talking HTTP to a Unix socket:

`curl --unix-socket /var/run/docker.sock http://localhost:2375/_ping`

## Running daemon

### user containerd

```console
$ nerdctl version
```

`systemctl --user start nerdctl`

```shell
DOCKER_HOST=unix://$XDG_RUNTIME_DIR/nerdctl.sock docker version
```

### system containerd

```console
$ sudo nerdctl version
```

`sudo systemctl --system start nerdctl`

```shell
sudo DOCKER_HOST=unix:///var/run/nerdctl.sock docker version
```

If you wanted to grant all users in the group "nerdctl" access:

/etc/systemd/system/nerdctl.service.d/10-group.conf

```
[Service]
UMask=0007
Group=nerdctl
```

This would allow access without using `sudo` (or being `root`).

**Warning**

This `nerdctl` group grants privileges equivalent to the `root` user.

Compare with the `docker` group for the docker daemon (`dockerd`):

<https://docs.docker.com/engine/security/#docker-daemon-attack-surface>

To run nerdctl without root privileges, see rootless (user) mode above.

## Kubernetes

In order to see the Kubernetes containers and images,
you will need to change the containerd "namespace":


/etc/systemd/system/nerdctl.service.d/10-namespace.conf

```
[Service]
Environment=CONTAINERD_NAMESPACE=k8s.io
```

You could also use the Kubernetes tool called `crictl`,
and configure it to talk to the "containerd" runtime:

/etc/crictl.yaml

```
runtime-endpoint: unix:///run/containerd/containerd.sock
```

<https://github.com/kubernetes-sigs/cri-tools>

One difference is that nerdctl shows the repo digest as ID,
whereas `crictl images` shows the image ID as the image ID:

```
REPOSITORY                                 TAG        IMAGE ID        CREATED              PLATFORM       SIZE         BLOB SIZE
registry.k8s.io/pause                      3.8        900118502363    About an hour ago    linux/amd64    700.0 KiB    304.0 KiB
registry.k8s.io/pause                      <none>     900118502363    About an hour ago    linux/amd64    700.0 KiB    304.0 KiB
registry.k8s.io/kube-apiserver             v1.25.1    4f261d18da8d    About an hour ago    linux/amd64    125.5 MiB    32.6 MiB
registry.k8s.io/kube-apiserver             <none>     4f261d18da8d    About an hour ago    linux/amd64    125.5 MiB    32.6 MiB
```

```
IMAGE                                     TAG                 IMAGE ID            SIZE
registry.k8s.io/pause                     3.8                 4873874c08efc       311kB
registry.k8s.io/kube-apiserver            v1.25.1             b09a3dc327be2       34.2MB
```

Another difference is that nerdctl shows "pause" containers,
but these sandbox images are hidden in `crictl ps` output:

```
CONTAINER ID    IMAGE                                              COMMAND                   CREATED              STATUS    PORTS    NAMES
669b48948f93    registry.k8s.io/pause:3.8                          "/pause"                  About an hour ago    Up                 k8s://kube-system/kube-apiserver-lima-k8s
8b960604e6d2    registry.k8s.io/kube-apiserver:v1.25.1             "kube-apiserver --ad…"    About an hour ago    Up                 k8s://kube-system/kube-apiserver-lima-k8s/kube-apiserver
```

```
CONTAINER           IMAGE               CREATED             STATE               NAME                      ATTEMPT             POD ID              POD
8b960604e6d29       b09a3dc327be2       About an hour ago   Running             kube-apiserver            0                   669b48948f931       kube-apiserver-lima-k8s
```

## Remote socket

Calling the socket over `ssh:` requires a program:

`docker system dial-stdio`

It is possible to replace it with a small wrapper:

`socat - nerdctl.sock`

But the feature is **not** available in `nerdctl` (yet):

```
FATA[0000] unknown subcommand "dial-stdio" for "system"
```

And the ssh command has been hardcoded to call "docker":

```go
sp.Args("docker", "system", "dial-stdio")
```

Included is a small `nerdctl` shell wrapper for `docker`.

It will forward `docker`, to `nerdctl` or `nerdctl.sock`.

## Implementation

This program uses the "Gin" web framework for HTTP.

It and docs can be found at <https://gin-gonic.com/> with some nice [examples](https://github.com/gin-gonic/examples)

## Implemented commands

* version
* info (system info)
* images (image ls)
* inspect (image inspect)
* history (image history)
* load (image load)
* pull (image pull)
* ps (container ls)
* inspect (container inspect)
* save (image save)
* build

Note: using "build" requires the `buildctl` client.

It also requires a running moby `buildkitd` server.

* <https://github.com/containerd/containerd>

* <https://github.com/moby/buildkit>
