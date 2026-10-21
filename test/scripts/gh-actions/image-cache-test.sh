#!/bin/bash

# Copyright 2026 The KServe Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Checks which image-cache.sh keys an edit changes. A key that misses an input
# lets a PR reuse an image built from other code and pass on it, so every case
# asserts the exact set of images whose key moves. Base digests are stubbed:
# the test needs git and docker buildx, but no registry.

# The edit snippets are single-quoted on purpose: they expand inside the tree.
# shellcheck disable=SC2016

set -o errexit
set -o nounset
set -o pipefail
export LC_ALL=C

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"
LIB=test/scripts/gh-actions/image-cache.sh
WORK="$(mktemp -d)"
trap 'rm -rf "${WORK}"' EXIT

# Two copies of the committed tree at different paths, with the working copy
# of the cache script. install/ and docs/ are large and outside every context.
copy() {
  mkdir -p "$1"
  git -C "${PROJECT_ROOT}" archive HEAD -- . ':!install' ':!docs' | tar -x -C "$1"
  cp "${PROJECT_ROOT}/${LIB}" "$1/${LIB}"
}
copy "${WORK}/baseline"
copy "${WORK}/tree"
git -C "${WORK}/tree" init -q
git -C "${WORK}/tree" add -A
git -C "${WORK}/tree" -c user.name=test -c user.email=test@example.com commit -q -m baseline

# Prints "<image> <key>" for every image of a tree. MOVED names a base image
# whose stubbed digest differs from the baseline; PLATFORM overrides the
# daemon platform.
keys() {
  MOVED="${MOVED:-}" PLATFORM="${PLATFORM:-}" bash -c '
    set -o errexit -o nounset -o pipefail
    source "$1/'"${LIB}"'"
    _base_digest() { REPLY="sha256:stub-$1"; [[ "$1" != "${MOVED}" ]] || REPLY+="-moved"; }
    if [[ -n "${PLATFORM}" ]]; then
      docker() { if [[ "$1" == version ]]; then echo "${PLATFORM}"; else command docker "$@"; fi; }
    fi
    for name in $(printf "%s\n" "${!IMAGE_SPECS[@]}" | sort); do
      if image_key "${name}" 2>/dev/null; then echo "${name} ${IMAGE_KEY}"; else echo "${name} FAIL"; fi
    done
  ' _ "$1"
}

# Prints "<image> <base ref>" for every base image of every image.
bases() {
  bash -c '
    source "$1/'"${LIB}"'"
    _base_digest() { REPLY="sha256:stub"; }
    for name in "${!IMAGE_SPECS[@]}"; do
      image_key "${name}" 2>&1 >/dev/null | sed -n "s/^  base \([^ ]*\) .*/${name} \1/p"
    done
  ' _ "$1"
}

# Prints the images whose build context is $1.
group() {
  bash -c '
    source "$1/'"${LIB}"'"
    for name in "${!IMAGE_SPECS[@]}"; do
      read -r _ context _ <<<"${IMAGE_SPECS[${name}]}"
      [[ "${context}" != "$2" ]] || echo "${name}"
    done
  ' _ "${WORK}/baseline" "$1" | sort | xargs
}

BASELINE="$(keys "${WORK}/baseline")"
if grep -q FAIL <<<"${BASELINE}"; then
  echo "baseline keys failed:" >&2
  grep FAIL <<<"${BASELINE}" >&2
  exit 1
fi
GO="$(group .)"
PYTHON="$(group python)"

failures=0
# check <name> <expected images> <keys> [reference keys, default BASELINE]
check() {
  local name=$1 expected got reference="${4:-${BASELINE}}"
  expected="$(xargs -n1 <<<"$2" | sort | xargs)"
  got="$(join <(echo "${reference}") <(echo "$3") | awk '$2 != $3 {print $1}' | sort | xargs)"
  if [[ "$(wc -l <<<"$3")" != "$(wc -l <<<"${reference}")" ]]; then
    got="(incomplete) ${got}"
  fi
  if [[ "${got}" == "${expected}" ]]; then
    echo "ok   ${name}"
  else
    echo "FAIL ${name}: expected [${expected}] got [${got}]"
    failures=$((failures + 1))
  fi
}

# Runs a shell snippet in the tree, checks the keys, then restores the tree.
edit() {
  (cd "${WORK}/tree" && eval "$3")
  check "$1" "$2" "$(keys "${WORK}/tree")"
  git -C "${WORK}/tree" reset -q --hard
  git -C "${WORK}/tree" clean -q -f -d -x
}

first() {
  find "$1" -name "$2" -type f | sort | head -1
}

check "same tree at another path" "" "$(keys "${WORK}/tree")"

edit "python source" "${PYTHON}" 'echo "# x" >>"$(first python/kserve/kserve "*.py")"'
edit "python file mode" "${PYTHON}" 'chmod +x "$(first python/kserve/kserve "*.py")"'
edit "new file in python/" "${PYTHON}" 'echo x >python/new-file.txt'
# python Dockerfiles are inside the python/ context.
edit "python Dockerfile" "${PYTHON}" 'echo "# x" >>python/sklearn.Dockerfile'
edit "go source" "${GO}" 'echo "// x" >>"$(first pkg "*.go")"'
edit "go.mod" "${GO}" 'echo "// x" >>go.mod'
edit "LICENSE" "${GO}" 'echo x >>LICENSE'
# kernelcache/ is in the root context, mcv.Dockerfile with it.
edit "kernelcache/" "${GO}" 'echo x >kernelcache/new-file.txt'
edit "root Dockerfile" "router" 'echo "# x" >>router.Dockerfile'
edit "outside every context" "" \
  'for d in test/e2e charts config hack docs install; do mkdir -p "${d}"; echo x >"${d}/new-file.txt"; done'
edit "dockerignored python tests" "" 'echo x >python/kserve/test/new_test.py'

# A moved base digest rekeys exactly the images built on it.
BASES="$(bases "${WORK}/baseline")"
while read -r ref; do
  expected="$(awk -v ref="${ref}" '$2 == ref {print $1}' <<<"${BASES}" | sort -u | xargs)"
  check "base ${ref} moved" "${expected}" "$(MOVED="${ref}" keys "${WORK}/baseline")"
done < <(awk '{print $2}' <<<"${BASES}" | sort -u)

check "other daemon platform" "${GO} ${PYTHON}" "$(PLATFORM=linux/arm64 keys "${WORK}/baseline")"

# No Dockerfile reads an external image through COPY --from today; one that
# starts to must still rekey when that image moves.
(cd "${WORK}/tree" && echo "COPY --from=busybox:1.36 /bin/true /true" >>router.Dockerfile)
check "base reached only by COPY --from moved" "router" \
  "$(MOVED=busybox:1.36 keys "${WORK}/tree")" "$(keys "${WORK}/tree")"
git -C "${WORK}/tree" reset -q --hard

# The Go images reach their golang base only through COPY --from a builder
# stage; expected from the Dockerfiles, not from the key's own manifest.
ref="$(awk '/^FROM /{print $2; exit}' "${WORK}/baseline/router.Dockerfile")"
expected="$(bash -c '
  source "$1/'"${LIB}"'"
  for name in "${!IMAGE_SPECS[@]}"; do
    read -r dockerfile _ <<<"${IMAGE_SPECS[${name}]}"
    if grep -qE "^FROM $2( |\$)" "$1/${dockerfile}"; then echo "${name}"; fi
  done
' _ "${WORK}/baseline" "${ref}")"
check "base reached through a COPY --from stage moved" "${expected}" "$(MOVED="${ref}" keys "${WORK}/baseline")"

# A stage the target does not reach is not built, so its base is no input.
(cd "${WORK}/tree" && { echo "FROM busybox:1.36 AS unused"; cat router.Dockerfile; } >router.Dockerfile.new &&
  mv router.Dockerfile.new router.Dockerfile)
check "unreachable stage base moved" "" \
  "$(MOVED=busybox:1.36 keys "${WORK}/tree")" "$(keys "${WORK}/tree")"
git -C "${WORK}/tree" reset -q --hard

echo "${failures} failed"
((failures == 0))
