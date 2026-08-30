#!/bin/sh
# Entrypoint wrapper that restricts execution to /mcv and buildah only.
# buildah is needed because mcv re-executes itself via buildah's copier
# subprocess mechanism (buildah.InitReexec / unshare.MaybeReexecUsingUserNamespace).
set -eu

case "${1:-}" in
    /mcv|mcv|buildah|/usr/bin/buildah)
        exec "$@"
        ;;
    -*)
        # Flags passed directly — forward to /mcv (ENTRYPOINT default behavior)
        exec /mcv "$@"
        ;;
    "")
        exec /mcv
        ;;
    *)
        echo "error: command not allowed: $1" >&2
        echo "only /mcv and buildah are permitted" >&2
        exit 1
        ;;
esac