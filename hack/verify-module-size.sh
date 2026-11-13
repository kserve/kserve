#!/bin/bash
# Verify that the github.com/kserve/kserve module at HEAD stays under Go's
# 500 MiB module zip limit. Over it, `go get` and the module proxy fail with
# "module source tree too large" and the release cannot be used as a
# dependency (kserve#6302).
#
# The Go toolchain builds the zip itself, from the local checkout, the same way
# the proxy does: from the committed tree, without nested modules or ignored
# files. Uncommitted changes are not counted. No network access is needed.
set -eu -o pipefail

cd "$(dirname "$0")/.."

sha=$(git rev-parse HEAD)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

# Go serves the module from a throwaway repo with a single branch at HEAD,
# sharing the checkout's objects. Fetching from the checkout directly fails
# when it has no local branches, as on a CI detached-HEAD checkout: Go's
# history fetch only asks for refs/heads/* and exits 1 without a message.
git init -q --bare "$tmp/src"
echo "$(git rev-parse --path-format=absolute --git-common-dir)/objects" >"$tmp/src/objects/info/alternates"
git -C "$tmp/src" update-ref refs/heads/verify "$sha"

# Runs outside the module so that go.mod and go.sum are left alone.
# GOPRIVATE skips the checksum database, which has never seen this commit.
if ! (cd "$tmp" && env \
  GOPROXY=direct \
  GOPRIVATE=github.com/kserve/kserve \
  GOMODCACHE="$tmp/mod" \
  GOFLAGS=-modcacherw \
  GIT_CONFIG_COUNT=1 \
  GIT_CONFIG_KEY_0="url.file://$tmp/src.insteadOf" \
  GIT_CONFIG_VALUE_0=https://github.com/kserve/kserve \
  go mod download github.com/kserve/kserve@"$sha"); then
  echo "FAIL: github.com/kserve/kserve@$sha cannot be downloaded as a Go module."
  echo "If the module is too large, move the largest non-Go directory into its own"
  echo "module with a nested go.mod, as install/ is (kserve#6305)."
  exit 1
fi
