#!/bin/sh
# Entrypoint wrapper for capture orchestration and direct MCV commands.
set -eu

if [ "${MCV_CAPTURE_MODE:-false}" = "true" ]; then
    exec python3 /capture-entrypoint.py
fi

case "${1:-}" in
    /mcv|mcv|buildah|/usr/bin/buildah)
        exec "$@"
        ;;
    -*)
        # Flags passed directly -- forward to /mcv (ENTRYPOINT default behavior)
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
