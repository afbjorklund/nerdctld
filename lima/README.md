# Lima examples

Default: [`nerdctl.yaml`](./nerdctl.yaml) (Ubuntu)

Distro:
- [`nerdctl-alpine.yaml`](./nerdctl-alpine.yaml): Alpine Linux
- [`nerdctl-fedora.yaml`](./nerdctl-fedora.yaml): Fedora

Container engines:
- [`nerdctl.yaml`](./nerdctl.yaml): Nerdctl [containerd/buildkitd]
- [`nerdctl-rootful.yaml`](./nerdctl-rootful.yaml): Nerdctl (rootful)

## Usage
Run `limactl start nerdctl.yaml` to create a Lima instance named "nerdctl".

To open a shell, run `limactl shell nerdctl` or `LIMA_INSTANCE=nerdctl lima`.
