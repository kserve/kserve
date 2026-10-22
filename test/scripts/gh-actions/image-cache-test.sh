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
# Temp dirs of child shells (context probes) go away with WORK, also when a
# run is interrupted.
export TMPDIR="${WORK}"
# Keys include the ISO week; a run crossing midnight on Sunday must not see
# every key move.
export E2E_IMAGE_CACHE_EPOCH=test

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
git -C "${WORK}/tree" add -A --force
git -C "${WORK}/tree" -c user.name=test -c user.email=test@example.com commit -q -m baseline

# Prints "<image> <key>" for every image of a tree, "<image> FAIL" when it
# cannot be keyed; the reasons go to ${WORK}/keys.err. MOVED names a base image
# whose stubbed digest differs from the baseline; PLATFORM overrides the daemon
# platform.
keys() {
  MOVED="${MOVED:-}" PLATFORM="${PLATFORM:-}" bash -c '
    set -o errexit -o nounset -o pipefail
    source "$1/'"${LIB}"'"
    _base_digest() { REPLY="sha256:stub-$1"; [[ "$1" != "${MOVED}" ]] || REPLY+="-moved"; }
    if [[ -n "${PLATFORM}" ]]; then
      docker() { if [[ "$1" == version ]]; then echo "${PLATFORM}"; else command docker "$@"; fi; }
    fi
    while IFS= read -r name; do
      if image_key "${name}" 2>>"$2"; then echo "${name} ${IMAGE_KEY}"; else echo "${name} FAIL"; fi
    done < <(_image_names)
  ' _ "$1" "${WORK}/keys.err"
}

# Prints "<image> <ref> <base|syntax>" for every keyed base image and frontend.
bases() {
  bash -c '
    source "$1/'"${LIB}"'"
    _base_digest() { REPLY="sha256:stub"; }
    for name in "${!IMAGE_SPECS[@]}"; do
      image_key "${name}" 2>"$2" >/dev/null
      sed -n "s/^  \(base\|syntax\) \([^ ]*\) .*/${name} \2 \1/p" "$2"
    done
  ' _ "$1" "${WORK}/bases.err"
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
  cat "${WORK}/keys.err" >&2
  exit 1
fi
GO="$(group .)"
PYTHON="$(group python)"

failures=0
pass() {
  echo "ok   $1"
}
fail() {
  echo "FAIL $1"
  failures=$((failures + 1))
}

# check <name> <expected images> <keys> [reference keys, default BASELINE]
# An image that cannot be keyed on either side never counts as expected: the
# cache would be off for it.
check() {
  local name=$1 expected got unkeyed reference="${4:-${BASELINE}}"
  expected="$(xargs -n1 <<<"$2" | sort | xargs)"
  got="$(join <(echo "${reference}") <(echo "$3") | awk '$2 != $3 {print $1}' | sort | xargs)"
  unkeyed="$(printf '%s\n%s\n' "${reference}" "$3" | awk '$2 == "FAIL" {print $1}' | sort -u | xargs)"
  if [[ -n "${unkeyed}" ]]; then
    got="(unkeyed: ${unkeyed}) ${got}"
  fi
  if [[ "$(wc -l <<<"$3")" != "$(wc -l <<<"${reference}")" ]]; then
    got="(incomplete) ${got}"
  fi
  if [[ "${got}" == "${expected}" ]]; then
    pass "${name}"
  else
    fail "${name}: expected [${expected}] got [${got}]"
  fi
}

restore() {
  git -C "${WORK}/tree" reset -q --hard
  git -C "${WORK}/tree" clean -q -f -d -x
}

# edit <name> <expected images> <snippet>: runs the snippet in the tree and
# checks which keys moved against the baseline.
edit() {
  (cd "${WORK}/tree" && eval "$3")
  check "$1" "$2" "$(keys "${WORK}/tree")"
  restore
}

# moved <name> <expected images> <ref> <snippet>: after the snippet, moving
# <ref> rekeys exactly the expected images.
moved() {
  (cd "${WORK}/tree" && eval "$4")
  check "$1" "$2" "$(MOVED="$3" keys "${WORK}/tree")" "$(keys "${WORK}/tree")"
  restore
}

# fails <name> <image> <snippet>: after the snippet, the image has no key, so
# it builds as if there were no cache.
fails() {
  local got
  (cd "${WORK}/tree" && eval "$3")
  # A key function that crashes instead of failing is a failure of this case.
  got="$(keys "${WORK}/tree" | awk -v image="$2" '$1 == image {print $2}')" || got="(crashed)"
  if [[ "${got}" == FAIL ]]; then pass "$1"; else fail "$1: $2 not reported unkeyable, got [${got}]"; fi
  restore
}

first() {
  find "$1" -name "$2" -type f | sort | sed -n 1p
}

# Adds a stage in front of router.Dockerfile and reads from it in the final
# stage: $1 is the stage line.
with_stage='{ echo "$1"; cat router.Dockerfile; echo "COPY --from=extra /etc/hostname /extra"; } >r.new && mv r.new router.Dockerfile'

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
# Its BuildKit and build setup produce what gets published.
edit "publish workflow" "${GO} ${PYTHON}" 'echo "# x" >>.github/workflows/e2e-image-cache.yml'
edit "outside every context" "" \
  'for d in test/e2e charts config hack docs install; do mkdir -p "${d}"; echo x >"${d}/new-file.txt"; done'
edit "dockerignored python tests" "" 'echo x >python/kserve/test/new_test.py'

# A moved base digest or frontend rekeys exactly the images built on it.
BASES="$(bases "${WORK}/baseline")"
while read -r ref; do
  expected="$(awk -v ref="${ref}" '$2 == ref {print $1}' <<<"${BASES}" | sort -u | xargs)"
  check "base ${ref} moved" "${expected}" "$(MOVED="${ref}" keys "${WORK}/baseline")"
done < <(awk '{print $2}' <<<"${BASES}" | sort -u)

# A # syntax= frontend is read by the build, so it is keyed; expected from the
# Dockerfiles, not from the key's own manifest.
expected="$(bash -c '
  source "$1/'"${LIB}"'"
  for name in "${!IMAGE_SPECS[@]}"; do
    read -r dockerfile _ <<<"${IMAGE_SPECS[${name}]}"
    if head -1 "$1/${dockerfile}" | grep -qiE "^#[[:space:]]*syntax[[:space:]]*="; then echo "${name}"; fi
  done
' _ "${WORK}/baseline" | sort | xargs)"
got="$(awk '$3 == "syntax" {print $1}' <<<"${BASES}" | sort | xargs)"
if [[ -n "${expected}" && "${got}" == "${expected}" ]]; then
  pass "frontend keyed"
else
  fail "frontend keyed: expected [${expected}] got [${got}]"
fi

check "other daemon platform" "${GO} ${PYTHON}" "$(PLATFORM=linux/arm64 keys "${WORK}/baseline")"
check "next week" "${GO} ${PYTHON}" "$(E2E_IMAGE_CACHE_EPOCH=next keys "${WORK}/baseline")"

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

# Instructions are read the way BuildKit reads them.
moved "base reached only by COPY --from" "router" busybox:1.36 \
  'echo "COPY --from=busybox:1.36 /bin/true /true" >>router.Dockerfile'
moved "lowercase from" "router" alpine:3.20 "set -- 'from alpine:3.20 as extra'; ${with_stage}"
moved "indented FROM" "router" alpine:3.20 "set -- '  FROM alpine:3.20 AS extra'; ${with_stage}"
moved "continuation line starting with from" "" helper \
  'printf "RUN echo \\\\\n  from helper import x\n" >>router.Dockerfile'
moved "blanks after a continuation" "" helper \
  'printf "RUN echo \\\\   \n  from helper import x\n" >>router.Dockerfile'
# An escaped escape does not continue the line, so the FROM after it counts.
moved "line ending in an escaped backslash" "router" alpine:3.21 \
  '{ echo "FROM busybox:1.36 AS pre"; echo "RUN echo one \\\\"; echo "FROM alpine:3.21 AS extra"; cat router.Dockerfile
     echo "COPY --from=extra /etc/hostname /extra"; } >r.new && mv r.new router.Dockerfile'
# BuildKit appends the next line as is: an option split over two lines is one.
moved "mount option continued on the next line" "router" busybox:1.36 \
  'printf "RUN --mount=type=bind,\\\\\nfrom=busybox:1.36,target=/m true\n" >>router.Dockerfile'
# A stage the target does not reach is not built, so its base is no input.
moved "unreachable stage base" "" busybox:1.36 \
  '{ echo "FROM busybox:1.36 AS unused"; cat router.Dockerfile; } >r.new && mv r.new router.Dockerfile'

# Anything the key cannot read the way BuildKit does leaves the image unkeyed.
fails "heredoc" router 'printf "RUN <<EOF\necho x\nEOF\n" >>router.Dockerfile'
fails "heredoc with a numeric delimiter" router 'printf "RUN cat <<0\nFROM busybox:1.36\n0\n" >>router.Dockerfile'
fails "ARG forward reference" router \
  "{ echo 'ARG B=golang\${A}'; echo 'ARG A=:1.26.8'; } >a.new && cat router.Dockerfile >>a.new && mv a.new router.Dockerfile &&
   set -- 'FROM \${B} AS extra'; ${with_stage}"
fails "base named like a stage" router "set -- 'FROM alpine AS alpine'; ${with_stage/--from=extra/--from=alpine}"
fails "unsupported parser directive" router \
  '{ echo "# escape=\`"; cat router.Dockerfile; } >r.new && mv r.new router.Dockerfile'
fails "open continuation at the end" router 'printf "COPY --from=busybox:1.36 /bin/true /true \\\\\n" >>router.Dockerfile'
fails "no FROM" router 'grep -viE "^[[:space:]]*FROM[[:space:]]" router.Dockerfile >r.new; mv r.new router.Dockerfile'

# BuildKit drops a byte order mark and a shebang line before reading the file.
moved "FROM after a byte order mark" "router" alpine:3.20 \
  "set -- \$'\\xef\\xbb\\xbfFROM alpine:3.20 AS extra'; ${with_stage}"
moved "syntax directive after a shebang" "router" docker/dockerfile:1.7 \
  '{ echo "#!/usr/bin/env -S docker build . --file"; echo "# syntax=docker/dockerfile:1.7"; cat router.Dockerfile; } >r.new &&
   mv r.new router.Dockerfile'

# Builds pin every base and frontend a key covers to the keyed digest, so a tag
# moving between keying and building cannot put another base under that key.
# Prints "<image> <ref|BUILDKIT_SYNTAX> <pin>" from the build arguments.
pin_args='
  printf "%s\n" "$@" | awk '"'"'(prev == "--build-context") || (prev == "--build-arg" && /^BUILDKIT_SYNTAX=/) {print "pin " $0} {prev = $0}'"'"''
expected="$(awk '$3 == "base" {print $1, $2, "docker-image://" $2 "@sha256:stub-" $2}
  $3 == "syntax" {print $1, "BUILDKIT_SYNTAX", $2 "@sha256:stub-" $2}' <<<"${BASES}" | sort -u)"
pins_check() {
  local got
  got="$(awk '
    match($0, /^\[[^]]*\] pin /) {
      name = substr($0, 2, index($0, "]") - 2)
      pin = substr($0, RLENGTH + 1)
      eq = index(pin, "=")
      print name, substr(pin, 1, eq - 1), substr(pin, eq + 1)
    }' <<<"$2" | sort -u)"
  if [[ "${got}" == "${expected}" ]]; then
    pass "$1"
  else
    fail "$1:"
    diff <(echo "${expected}") <(echo "${got}") || true
  fi
}
# The real publish path, docker stubbed for the push builds only; each push
# reports a digest made from its tag, as buildx would in --metadata-file.
# A line left from an earlier run must not be attested again.
echo "0000  example.invalid/cache:stale" >"${WORK}/subjects"
pins_check "publish pins every keyed base" "$(bash -c '
  set -o errexit -o nounset -o pipefail
  source "$1/'"${LIB}"'"
  _base_digest() { REPLY="sha256:stub-$1"; }
  _image_verified() { return 1; }
  _image_public() { return 0; }
  docker() {
    local IFS=" " meta="" tag="" prev=""
    if [[ " $* " != *" --push "* ]]; then command docker "$@"; return; fi
    for arg in "$@"; do
      [[ "${prev}" != --metadata-file ]] || meta="${arg}"
      [[ "${prev}" != -t ]] || tag="${arg}"
      prev="${arg}"
    done
    printf "{\"containerimage.digest\": \"sha256:%s\"}" "$(sha256sum <<<"${tag}" | cut -d" " -f1)" >"${meta}"
    '"${pin_args}"'
  }
  mapfile -t names < <(_image_names)
  E2E_IMAGE_CACHE=example.invalid/cache E2E_IMAGE_CACHE_SUBJECTS="$2" image_cache_publish "${names[@]}" 2>/dev/null
' _ "${WORK}/baseline" "${WORK}/subjects")"
# Every pushed image is listed for the attestation step, under its exact tag
# and the digest the push reported.
expected="$(while read -r name key; do
  tag="example.invalid/cache:${name}-${key}"
  echo "$(sha256sum <<<"${tag}" | cut -d" " -f1)  ${tag}"
done <<<"${BASELINE}" | sort)"
if [[ "$(sort "${WORK}/subjects")" == "${expected}" ]]; then
  pass "publish lists every pushed image for attestation"
else
  fail "publish lists every pushed image for attestation:"
  diff <(echo "${expected}") <(sort "${WORK}/subjects") || true
fi
expected="$(awk '$3 == "base" {print $1, $2, "docker-image://" $2 "@sha256:stub-" $2}
  $3 == "syntax" {print $1, "BUILDKIT_SYNTAX", $2 "@sha256:stub-" $2}' <<<"${BASES}" | sort -u)"
# The PR-side build path, under the IFS=, that build-server-runtimes.sh sets.
pins_check "PR builds pin every keyed base under IFS=," "$(bash -c '
  set -o errexit -o nounset -o pipefail
  source "$1/'"${LIB}"'"
  _base_digest() { REPLY="sha256:stub-$1"; }
  _image_verified() { return 1; }
  _image_published() { return 1; }
  # build_image keeps the image in its local "name".
  docker() {
    local IFS=" "
    if [[ " $* " != *" type=docker,dest="* ]]; then command docker "$@"; return; fi
    '"${pin_args}"' | sed "s/^/[${name}] /"
  }
  mapfile -t names < <(_image_names)
  IFS=,
  for name in "${names[@]}"; do
    E2E_IMAGE_CACHE=example.invalid/cache build_image "${name}" "test/${name}:t" /dev/null 2>/dev/null
  done
' _ "${WORK}/baseline")"

# A cached image is used only with an attestation for exactly its tag and
# digest, from the publish workflow on master. gh is stubbed: it records its
# arguments and answers with the subjects in SUBJECTS ("<name> <digest>"
# lines), signed by the certificate of master's publish job for owner/repo
# with the fields in CERT (a JSON object) changed. The first GH_5XX calls fail
# the way the attestations API does when it is down; gh.calls counts calls.
verified() {
  rm -f "${WORK}/gh.calls"
  SUBJECTS="$1" GH_RC="${2:-0}" SIGNER="${3-owner/repo}" CERT="${4:-{\}}" GH_5XX="${GH_5XX:-0}" bash -c '
    set -o errexit -o nounset -o pipefail
    source "$1/'"${LIB}"'"
    args_file="$2"
    # timeout runs a program, not a function; let it reach the gh stub.
    timeout() { while [[ "$1" == -* ]]; do shift 2; done; shift; "$@"; }
    sleep() { :; }
    _image_digest() { REPLY="sha256:aaaa"; }
    gh() {
      printf "%s\n" "$@" >"${args_file}"
      echo x >>"${args_file%/*}/gh.calls"
      if (($(wc -l <"${args_file%/*}/gh.calls") <= GH_5XX)); then
        echo "HTTP 502: Server Error (https://api.github.com/repos/owner/repo/attestations/sha256:aaaa)" >&2
        return 1
      fi
      ((GH_RC == 0)) || return "${GH_RC}"
      # Parenthesised: jq 1.7 (ubuntu-24.04) binds "as" tighter than "+".
      jq -Rn --argjson cert "${CERT}" "
        ({subjectAlternativeName: \"https://github.com/owner/repo/.github/workflows/e2e-image-cache.yml@refs/heads/master\",
         sourceRepositoryRef: \"refs/heads/master\", sourceRepositoryURI: \"https://github.com/owner/repo\",
         runnerEnvironment: \"github-hosted\", githubWorkflowTrigger: \"push\"} + \$cert) as \$c
        | [{verificationResult: {signature: {certificate: \$c},
            statement: {subject: [inputs | select(length > 0) | split(\" \") | {name: .[0], digest: {sha256: .[1]}}]}}}]" <<<"${SUBJECTS}"
    }
    if E2E_IMAGE_CACHE_SIGNER="${SIGNER}" GITHUB_REPOSITORY= _image_verified example.invalid/cache:router-k; then
      echo "verified ${REPLY}"
    else
      echo unverified
    fi
  ' _ "${WORK}/baseline" "${WORK}/gh.args"
}
attested() {
  local got
  got="$(verified "${@:3}")"
  if [[ "${got}" == "$2" ]]; then pass "$1"; else fail "$1: expected [$2] got [${got}]"; fi
}
ok_subject="example.invalid/cache:router-k aaaa"
attested "attested tag and digest" "verified example.invalid/cache@sha256:aaaa" "${ok_subject}"
attested "attested tag, other digest (re-tag)" unverified "example.invalid/cache:router-k bbbb
example.invalid/cache:agent-k aaaa"
attested "attested digest, other tag" unverified "example.invalid/cache:agent-k aaaa"
attested "verification fails" unverified "${ok_subject}" 1
if [[ "$(wc -l <"${WORK}/gh.calls")" == 1 ]]; then
  pass "a failed verification is not retried"
else
  fail "a failed verification is not retried: $(wc -l <"${WORK}/gh.calls") calls"
fi
GH_5XX=1 attested "attestations API fails once with 5xx" "verified example.invalid/cache@sha256:aaaa" "${ok_subject}"
GH_5XX=2 attested "attestations API keeps failing with 5xx" unverified "${ok_subject}"
attested "no signer repository" unverified "${ok_subject}" 0 ""
# The certificate is checked here too, not only by gh, whose matching has
# changed between releases (case-insensitive refs, prefix workflow paths).
attested "signed from a MASTER branch" unverified "${ok_subject}" 0 owner/repo \
  '{"sourceRepositoryRef": "refs/heads/MASTER", "subjectAlternativeName": "https://github.com/owner/repo/.github/workflows/e2e-image-cache.yml@refs/heads/MASTER"}'
attested "signed by another workflow" unverified "${ok_subject}" 0 owner/repo \
  '{"subjectAlternativeName": "https://github.com/owner/repo/.github/workflows/e2e-image-cache.yml.evil@refs/heads/master"}'
attested "signed by another repository" unverified "${ok_subject}" 0 owner/repo \
  '{"sourceRepositoryURI": "https://github.com/owner/other"}'
attested "signed on a self-hosted runner" unverified "${ok_subject}" 0 owner/repo '{"runnerEnvironment": "self-hosted"}'
attested "signed from pull_request_target" unverified "${ok_subject}" 0 owner/repo \
  '{"githubWorkflowTrigger": "pull_request_target"}'
attested "signed from a schedule" "verified example.invalid/cache@sha256:aaaa" "${ok_subject}" 0 owner/repo \
  '{"githubWorkflowTrigger": "schedule"}'
verified "${ok_subject}" >/dev/null
args=" $(tr '\n' ' ' <"${WORK}/gh.args") "
missing=""
for want in "oci://example.invalid/cache@sha256:aaaa" "--repo owner/repo" \
  "--signer-workflow owner/repo/.github/workflows/e2e-image-cache.yml@refs/heads/master" \
  "--source-ref refs/heads/master" "--deny-self-hosted-runners" "--limit 100"; do
  [[ "${args}" == *" ${want} "* ]] || missing+=" [${want}]"
done
if [[ -z "${missing}" ]]; then
  pass "verifies by digest, signer workflow and master"
else
  fail "verifies by digest, signer workflow and master: missing${missing}"
fi

# Only a verified tag is pulled, by digest; a published one without its
# attestation builds. In publish it is pushed over and listed for attestation.
source_of() {
  VERIFIED="$1" bash -c '
    set -o errexit -o nounset -o pipefail
    source "$1/'"${LIB}"'"
    _base_digest() { REPLY="sha256:stub-$1"; }
    _image_verified() { [[ "${VERIFIED}" == yes ]] && REPLY="example.invalid/cache@sha256:aaaa"; }
    _image_published() { return 0; }
    timeout() { while [[ "$1" == -* ]]; do shift 2; done; shift; "$@"; }
    docker() {
      local IFS=" "
      if [[ " $* " != *" type=docker,dest="* ]]; then command docker "$@"; return; fi
      # The pull reads its Dockerfile from stdin; it must name the digest.
      if [[ " $* " == *" - "* ]]; then [[ "$(cat)" == "FROM example.invalid/cache@sha256:aaaa" ]]; fi
    }
    E2E_IMAGE_CACHE=example.invalid/cache build_image router test/router:t /dev/null 2>/dev/null | awk "{print \$4}"
  ' _ "${WORK}/baseline"
}
if [[ "$(source_of yes)" == cache ]]; then pass "verified tag is pulled by digest"; else fail "verified tag is pulled by digest"; fi
if [[ "$(source_of no)" == build ]]; then pass "unattested tag is built"; else fail "unattested tag is built"; fi
republished="$(bash -c '
  set -o errexit -o nounset -o pipefail
  source "$1/'"${LIB}"'"
  _base_digest() { REPLY="sha256:stub-$1"; }
  _image_verified() { return 1; }
  _image_published() { return 0; }
  _image_public() { return 0; }
  docker() {
    local IFS=" " prev="" meta=""
    if [[ " $* " != *" --push "* ]]; then command docker "$@"; return; fi
    for arg in "$@"; do [[ "${prev}" != --metadata-file ]] || meta="${arg}"; prev="${arg}"; done
    echo "{\"containerimage.digest\": \"sha256:cccc\"}" >"${meta}"
    echo pushed
  }
  E2E_IMAGE_CACHE=example.invalid/cache E2E_IMAGE_CACHE_SUBJECTS="$2" image_cache_publish router 2>/dev/null
' _ "${WORK}/baseline" "${WORK}/republished")"
if grep -q "pushed" <<<"${republished}" && grep -q "^cccc  example.invalid/cache:router-" "${WORK}/republished"; then
  pass "publish pushes over an unattested tag and attests it"
else
  fail "publish pushes over an unattested tag and attests it"
fi

# The specs and the build scripts agree: every image built has a spec, every
# spec is built, and nothing bypasses build_image (it would never be cached).
scripts=("${PROJECT_ROOT}/test/scripts/gh-actions/build-images.sh" "${PROJECT_ROOT}/test/scripts/gh-actions/build-server-runtimes.sh")
built="$(grep -ohE '^[[:space:]]*build_image [a-z0-9-]+' "${scripts[@]}" | awk '{print $2}' | sort -u | xargs)"
specs="$(bash -c 'source "$1/'"${LIB}"'"; _image_names' _ "${WORK}/baseline" | xargs)"
if [[ "${built}" == "${specs}" ]] && ! grep -q 'docker buildx build' "${scripts[@]}"; then
  pass "build scripts and specs agree"
else
  fail "build scripts and specs disagree: built [${built}] specs [${specs}], raw builds: $(grep -c 'docker buildx build' "${scripts[@]}" | xargs)"
fi

# A workflow that reads the cache runs PR code with its token, so the token is
# read-only. In the publisher only publish signs, and only publish and prune
# write packages. Prints "<job> <scope> <level>", (workflow) for the top level.
permissions_of() {
  yq e '
    [{"job": "(workflow)", "p": .permissions}] + [.jobs | to_entries[] | {"job": .key, "p": .value.permissions}]
    | .[] | select(.p != null) | .job as $job
    | (select(.p | tag == "!!map") | .p | to_entries[] | $job + " " + .key + " " + .value),
      (select(.p | tag != "!!map") | $job + " (all) " + .p)' "$1"
}
publisher_writes="prune packages
publish attestations
publish id-token
publish packages"
bad=""
for wf in "${PROJECT_ROOT}"/.github/workflows/*.y*ml; do
  grep -q 'E2E_IMAGE_CACHE:' "${wf}" || continue
  perms="$(permissions_of "${wf}")"
  writes="$(awk '$3 != "read" && $3 != "none" {print $1, $2}' <<<"${perms}" | sort)"
  want=""
  [[ "${wf##*/}" != e2e-image-cache.yml ]] || want="${publisher_writes}"
  if ! grep -q '^(workflow) ' <<<"${perms}" || [[ "${writes}" != "${want}" ]]; then
    bad+=" ${wf##*/}: [$(xargs <<<"${writes}")]"
  fi
done
if [[ -z "${bad}" ]]; then
  pass "workflows reading the cache hold read-only tokens"
else
  fail "workflows reading the cache hold write tokens or no permissions block:${bad}"
fi

echo "${failures} failed"
((failures == 0))
