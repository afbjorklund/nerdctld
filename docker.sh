#!/bin/sh
# wrapper to call containerd (nerdctl) instead of docker
if [ "$1" = "system" ] && [ "$2" = "dial-stdio" ]; then
  exec socat - "${XDG_RUNTIME_DIR:-/var/run}/nerdctl.sock"
fi
nerdctl "$@"
