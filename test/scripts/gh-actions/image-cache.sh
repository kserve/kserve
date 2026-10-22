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

# Content-hash keyed reuse of the e2e images.
#
# An image's key covers everything its build can read: the Dockerfile, the
# build target, the build context as BuildKit sees it after .dockerignore, the
# digests of the base images, the daemon platform, this script and the publish
# workflow - and the ISO week, so that what a build fetched unpinned (see the
# workflow) is reused for a week at most. When master has published and
# attested an image under the same key, build_image pulls it instead of
# building. Anything that cannot be keyed or verified is built as before.
# Editing this script or the workflow rekeys every image, which is also how to
# drop the whole cache at once.
#
# Who can push a tag doesn't matter: a hit needs a provenance attestation that
# the publish job in .github/workflows/e2e-image-cache.yml signed on master for
# exactly that tag and digest (see _image_verified).
#
# Sourced by build-images.sh and build-server-runtimes.sh. The e2e-image-cache
# workflow runs it directly to fill the cache from master:
#
#   image-cache.sh plan               publish jobs for the images e2e runs
#                                     cannot use yet, as JSON
#   image-cache.sh publish <image>... build and push images unless attested
#   image-cache.sh prune [days]       delete versions no current key uses, older
#                                     than days (default 7)
#
# E2E_IMAGE_CACHE names the cache repository, e.g.
# ghcr.io/kserve/kserve-e2e-images. When unset, build_image only builds.
# Verifying attestations needs gh with a token (GH_TOKEN or GITHUB_TOKEN);
# E2E_IMAGE_CACHE_SIGNER overrides the repository whose publish job must have
# signed them, E2E_IMAGE_CACHE_SUBJECTS is where publish lists what to attest,
# E2E_IMAGE_CACHE_EPOCH replaces the week (tests).

_IMAGE_CACHE_LIB="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/$(basename "${BASH_SOURCE[0]}")"
_IMAGE_CACHE_ROOT="$(cd "$(dirname "${_IMAGE_CACHE_LIB}")/../../.." && pwd)"
# Its BuildKit and build setup produce the published images.
_IMAGE_CACHE_PUBLISHER=.github/workflows/e2e-image-cache.yml

# Image name -> "<dockerfile> <context> [target]", relative to the repo root.
# Names match the cache tags, not the kserve-images.sh variables, so that
# overriding an image name locally cannot point at another image's cache.
#
# A new e2e image needs a line here and a build_image call in build-images.sh
# or build-server-runtimes.sh; image-cache-test.sh fails when the two disagree.
# The publisher, its job groups and pruning all read this table.
declare -gA IMAGE_SPECS=(
  [kserve-controller]="Dockerfile ."
  [kserve-localmodel-controller]="localmodel.Dockerfile ."
  [kserve-localmodelnode-agent]="localmodel-agent.Dockerfile ."
  [kserve-kernelcachenode-agent]="kernelcache-agent.Dockerfile ."
  [kserve-mcv-minimal]="kernelcache/mcv/mcv.Dockerfile . mcv-minimal"
  [agent]="agent.Dockerfile ."
  [router]="router.Dockerfile ."
  [llmisvc-controller]="llmisvc-controller.Dockerfile ."
  [storage-initializer]="python/storage-initializer.Dockerfile python"
  [sklearnserver]="python/sklearn.Dockerfile python"
  [xgbserver]="python/xgb.Dockerfile python"
  [lgbserver]="python/lgb.Dockerfile python"
  [pmmlserver]="python/pmml.Dockerfile python"
  [paddleserver]="python/paddle.Dockerfile python"
  [predictiveserver]="python/predictiveserver.Dockerfile python"
  [autogluonserver]="python/autogluon.Dockerfile python"
  [huggingfaceserver]="python/huggingface_server_cpu.Dockerfile python"
  [custom-model-grpc]="python/custom_model_grpc.Dockerfile python"
  [custom-image-transformer-grpc]="python/custom_transformer_grpc.Dockerfile python"
  [image-transformer]="python/custom_transformer.Dockerfile python"
  [art-explainer]="python/artexplainer.Dockerfile python"
)

# Memoized per process: the key functions return through REPLY and globals
# rather than stdout so that they never run in a subshell.
declare -gA _IMAGE_CACHE_CONTEXTS=()
declare -gA _IMAGE_CACHE_DIGESTS=()

# Sets IMAGE_DOCKERFILE, IMAGE_CONTEXT (repo-relative) and IMAGE_TARGET.
image_spec() {
  local IFS=$' \t\n'
  local spec="${IMAGE_SPECS[$1]:-}"
  if [[ -z "${spec}" ]]; then
    echo "image-cache: unknown image $1, add it to IMAGE_SPECS in ${_IMAGE_CACHE_LIB#"${_IMAGE_CACHE_ROOT}"/}" >&2
    return 1
  fi
  read -r IMAGE_DOCKERFILE IMAGE_CONTEXT IMAGE_TARGET <<<"${spec}"
}

# Prints the image names in a stable order.
_image_names() {
  printf '%s\n' "${!IMAGE_SPECS[@]}" | sort
}

# Runs docker buildx build for the current image spec, passing extra arguments.
# Each base and frontend in IMAGE_BASES is pinned to the digest its key
# recorded, so a tag that moves after keying cannot be built into an image
# under the old key.
_build_spec() {
  local IFS=$' \t\n'
  local -a build_args=()
  local kind ref digest
  if [[ -n "${IMAGE_TARGET}" ]]; then
    build_args+=(--target "${IMAGE_TARGET}")
  fi
  while read -r kind ref digest; do
    if [[ -z "${ref}" || "${ref}" == *@* ]]; then
      continue
    elif [[ "${kind}" == syntax ]]; then
      build_args+=(--build-arg "BUILDKIT_SYNTAX=${ref}@${digest}")
    else
      build_args+=(--build-context "${ref}=docker-image://${ref}@${digest}")
    fi
  done <<<"${IMAGE_BASES:-}"
  docker buildx build -f "${_IMAGE_CACHE_ROOT}/${IMAGE_DOCKERFILE}" "${build_args[@]}" "$@" \
    "${_IMAGE_CACHE_ROOT}/${IMAGE_CONTEXT}"
}

# Sets REPLY to the hash of a build context as BuildKit transfers it. The probe
# build copies the whole context through .dockerignore, so the hash covers a
# superset of what any Dockerfile using that context can COPY.
_context_hash() {
  local context=$1 out hash=""
  if [[ -z "${_IMAGE_CACHE_CONTEXTS[${context}]:-}" ]]; then
    out="$(mktemp -d)" || return 1
    if printf 'FROM scratch\nCOPY . /\n' |
      docker buildx build --quiet -f - -o "type=local,dest=${out}" "${_IMAGE_CACHE_ROOT}/${context}" >/dev/null; then
      hash="$(tar -C "${out}" --sort=name --format=gnu --mtime=@0 --owner=0 --group=0 --numeric-owner -cf - . |
        sha256sum; exit "${PIPESTATUS[0]}")" || hash=""
    fi
    rm -rf "${out}"
    [[ -n "${hash}" ]] || return 1
    _IMAGE_CACHE_CONTEXTS[${context}]="${hash%% *}"
  fi
  REPLY="${_IMAGE_CACHE_CONTEXTS[${context}]}"
}

# Sets REPLY to the digest a base image reference resolves to right now. Hashes
# the raw top-level manifest: one request, where --format walks every platform
# manifest and trips registry rate limits.
_base_digest() {
  local ref=$1 digest attempt
  if [[ "${ref}" == *@sha256:* ]]; then
    REPLY="${ref#*@}"
    return 0
  fi
  if [[ -z "${_IMAGE_CACHE_DIGESTS[${ref}]:-}" ]]; then
    # Shared runner IPs hit registry rate limits; back off rather than miss.
    # A failure is remembered too, so images sharing the base don't retry.
    _IMAGE_CACHE_DIGESTS[${ref}]=-
    for attempt in 1 2 3 4; do
      if digest="$(timeout 60 docker buildx imagetools inspect --raw "${ref}" | sha256sum; exit "${PIPESTATUS[0]}")"; then
        _IMAGE_CACHE_DIGESTS[${ref}]="sha256:${digest%% *}"
        break
      fi
      ((attempt == 4)) || sleep $((attempt * attempt * 3))
    done
  fi
  [[ "${_IMAGE_CACHE_DIGESTS[${ref}]}" != - ]] || return 1
  REPLY="${_IMAGE_CACHE_DIGESTS[${ref}]}"
}

# Sets REPLY to one "base <ref> <digest>" line per external image the target
# stage (the last one without a target) needs - through FROM, COPY --from or
# RUN --mount from=, with global ARG defaults expanded - plus a "syntax <ref>
# <digest>" line for a # syntax= frontend. Stages the target does not reach
# are skipped, as BuildKit skips them. Anything it cannot read the way
# BuildKit does (other parser directives, heredocs, unresolved ARGs, a base
# named like a stage) is an error, not a guess.
_base_digests() {
  local IFS=$' \t\n'
  local dockerfile=$1 target=${2:-} line="" logical="" instr="" word="" name="" value="" image="" stage=""
  # BuildKit takes any word of <<, an optional -, and a name as a heredoc,
  # whatever the name starts with. Anything that could be one is refused, here
  # strings included; refusing too much only costs the cache.
  local dep="" out="" ref="" syntax="" opt="" heredoc='<<-?[^[:space:]<]'
  local i=0 n=-1 seen_from=false header=true first=true
  local -A args=() index=() visited=()
  local -a deps=() words=() from=() todo=() refs=() opts=()
  while IFS= read -r line || [[ -n "${line}" ]]; do
    line="${line%$'\r'}"
    if [[ "${first}" == true ]]; then
      first=false
      # BuildKit drops a byte order mark and a shebang line before directives.
      line="${line#$'\xef\xbb\xbf'}"
      [[ "${line}" != '#!'* ]] || continue
    fi
    # Parser directives are comments before anything else, indented or not.
    if [[ "${header}" == true && "${line}" =~ ^[[:space:]]*#[[:space:]]*([A-Za-z]+)[[:space:]]*=[[:space:]]*([^[:space:]]*) ]]; then
      case "${BASH_REMATCH[1],,}" in
        syntax) syntax="${BASH_REMATCH[2]}" ;;
        check) ;;
        *)
          echo "image-cache: unsupported parser directive '${line}' in ${dockerfile}" >&2
          return 1
          ;;
      esac
      continue
    fi
    header=false
    # Comments and blank lines, also inside a continuation.
    [[ ! "${line}" =~ ^[[:space:]]*(#|$) ]] || continue
    # A line continues when it ends in an escape that is not itself escaped,
    # trailing blanks allowed; the next line is appended as is, like BuildKit.
    logical+="${line}"
    if [[ "${logical}" =~ ^(.*[^\\])\\[[:blank:]]*$ ]]; then
      logical="${BASH_REMATCH[1]}"
      continue
    fi
    line="${logical}"
    logical=""
    if [[ "${line}" =~ ${heredoc} ]]; then
      echo "image-cache: heredocs are not supported in ${dockerfile}" >&2
      return 1
    fi
    read -r -a words <<<"${line}"
    instr="${words[0]^^}"
    from=()
    if [[ "${seen_from}" == false && "${instr}" == ARG ]]; then
      for word in "${words[@]:1}"; do
        [[ "${word}" == *=* ]] || continue
        value="${word#*=}"
        value="${value#\"}"
        value="${value%\"}"
        for name in "${!args[@]}"; do
          value="${value//\$\{${name}\}/"${args[${name}]}"}"
        done
        # A reference to a later or unknown ARG: BuildKit reads it as empty.
        [[ "${value}" != *\$* ]] || value="\${unresolved}"
        args[${word%%=*}]="${value}"
      done
      continue
    elif [[ "${instr}" == FROM ]]; then
      seen_from=true
      n=$((n + 1))
      deps[n]=""
      image=""
      stage=""
      for ((i = 1; i < ${#words[@]}; i++)); do
        if [[ "${words[i]}" == --* ]]; then
          continue
        elif [[ -z "${image}" ]]; then
          image="${words[i]}"
        elif [[ "${words[i]^^}" == AS ]]; then
          stage="${words[i + 1]:-}"
          break
        fi
      done
      from=("${image}")
    elif ((n >= 0)); then
      for word in "${words[@]}"; do
        case "${word}" in
          --from=*) from+=("${word#--from=}") ;;
          --mount=*)
            # BuildKit lowercases mount option names.
            IFS=, read -r -a opts <<<"${word#--mount=}"
            for opt in "${opts[@]}"; do
              [[ "${opt,,}" != from=* ]] || from+=("${opt#*=}")
            done
            ;;
        esac
      done
    fi

    for image in "${from[@]}"; do
      for name in "${!args[@]}"; do
        image="${image//\$\{${name}\}/"${args[${name}]}"}"
      done
      if [[ -z "${image}" || "${image}" == *\$* ]]; then
        echo "image-cache: cannot resolve '${line}' in ${dockerfile}" >&2
        return 1
      fi
      if [[ "${image}" =~ ^[0-9]+$ ]]; then
        deps[n]+="s:${image} "
      elif [[ -n "${index[${image,,}]:-}" ]]; then
        deps[n]+="s:${index[${image,,}]} "
      elif [[ "${image}" != scratch ]]; then
        deps[n]+="r:${image} "
      fi
    done
    if [[ -n "${stage}" ]]; then
      index[${stage,,}]=${n}
      stage=""
    fi
  done <"${_IMAGE_CACHE_ROOT}/${dockerfile}"
  # BuildKit runs an instruction left open at the end of the file.
  if [[ -n "${logical}" ]]; then
    echo "image-cache: ${dockerfile} ends in a line continuation" >&2
    return 1
  fi
  if ((n < 0)); then
    echo "image-cache: no FROM in ${dockerfile}" >&2
    return 1
  fi

  todo=("${n}")
  if [[ -n "${target}" ]]; then
    if [[ -z "${index[${target,,}]:-}" ]]; then
      echo "image-cache: no stage ${target} in ${dockerfile}" >&2
      return 1
    fi
    todo=("${index[${target,,}]}")
  fi
  while ((${#todo[@]})); do
    i=${todo[-1]}
    unset 'todo[-1]'
    [[ -z "${visited[${i}]:-}" ]] || continue
    visited[${i}]=1
    read -r -a words <<<"${deps[i]:-}"
    for dep in "${words[@]}"; do
      case "${dep}" in
        s:*) todo+=("${dep#s:}") ;;
        r:*) refs+=("${dep#r:}") ;;
      esac
    done
  done

  while IFS= read -r ref; do
    [[ -n "${ref}" ]] || continue
    # A pin is a named build context, which would also replace a stage of
    # that name.
    if [[ -n "${index[${ref,,}]:-}" ]]; then
      echo "image-cache: base ${ref} has the name of a stage in ${dockerfile}" >&2
      return 1
    fi
    if ! _base_digest "${ref}"; then
      echo "image-cache: cannot resolve base image ${ref}" >&2
      return 1
    fi
    out+="base ${ref} ${REPLY}"$'\n'
  done < <(printf '%s\n' "${refs[@]}" | sort -u)
  if [[ -n "${syntax}" ]]; then
    if ! _base_digest "${syntax}"; then
      echo "image-cache: cannot resolve frontend ${syntax}" >&2
      return 1
    fi
    out+="syntax ${syntax} ${REPLY}"$'\n'
  fi
  REPLY="${out}"
}

# Sets IMAGE_KEY and IMAGE_BASES (the base and syntax lines it covers) for an
# image and logs the inputs, so a surprising miss can be diffed against the
# publisher's log. Both are empty when the image cannot be keyed.
image_key() {
  local name=$1 platform hash context_hash dockerfile_hash script_hash publisher_hash bases manifest
  local epoch="${E2E_IMAGE_CACHE_EPOCH:-$(date -u +%G-W%V)}"
  IMAGE_KEY=""
  IMAGE_BASES=""
  image_spec "${name}" || return 1
  if [[ -e "${_IMAGE_CACHE_ROOT}/${IMAGE_DOCKERFILE}.dockerignore" ]]; then
    # The context probe only reads <context>/.dockerignore.
    echo "image-cache: ${IMAGE_DOCKERFILE}.dockerignore is not supported" >&2
    return 1
  fi
  platform="$(docker version -f '{{.Server.Os}}/{{.Server.Arch}}')" || return 1
  hash="$(sha256sum <"${_IMAGE_CACHE_ROOT}/${IMAGE_DOCKERFILE}")" || return 1
  dockerfile_hash="${hash%% *}"
  hash="$(sha256sum <"${_IMAGE_CACHE_LIB}")" || return 1
  script_hash="${hash%% *}"
  hash="$(sha256sum <"${_IMAGE_CACHE_ROOT}/${_IMAGE_CACHE_PUBLISHER}")" || return 1
  publisher_hash="${hash%% *}"
  _context_hash "${IMAGE_CONTEXT}" || return 1
  context_hash="${REPLY}"
  _base_digests "${IMAGE_DOCKERFILE}" "${IMAGE_TARGET}" || return 1
  bases="${REPLY}"

  manifest="epoch ${epoch}
script ${script_hash}
publisher ${publisher_hash}
platform ${platform}
dockerfile ${IMAGE_DOCKERFILE} ${dockerfile_hash}
target ${IMAGE_TARGET:--}
context ${IMAGE_CONTEXT} ${context_hash}
${bases}"
  hash="$(sha256sum <<<"${manifest}")"
  IMAGE_KEY="${hash%% *}"
  [[ "${IMAGE_KEY}" =~ ^[0-9a-f]{64}$ ]] || { IMAGE_KEY=""; return 1; }
  IMAGE_BASES="${bases}"
  printf 'image-cache: %s key %s\n%s' "${name}" "${IMAGE_KEY}" "${manifest}" | sed '2,$s/^/  /' >&2
}

# A registry that stalls once tends to stall again: after the first timeout
# the rest of the job builds without the cache, so a stall costs one timeout
# per job rather than one per image, and the job still has time to build.
_image_cache_stalled() {
  _IMAGE_CACHE_STALLED=1
  echo "image-cache: $1 timed out, building the remaining images without the cache" >&2
}

# Sets REPLY to the digest a reference points to now. A stalled registry must
# not hang a build; a timeout reads as not published.
_image_digest() {
  local digest rc=0
  digest="$(timeout 60 docker buildx imagetools inspect --raw "$1" 2>/dev/null | sha256sum
    exit "${PIPESTATUS[0]}")" || rc=$?
  ((rc != 124)) || _image_cache_stalled "looking up $1"
  ((rc == 0)) || return 1
  REPLY="sha256:${digest%% *}"
}

_image_published() {
  _image_digest "$1"
}

# Sets REPLY to <repository>@<digest> when the tag <ref> points to an image the
# publish job of E2E_IMAGE_CACHE_SIGNER (owner/repo, default: the current
# repository) attested on master under exactly that tag and digest. Anyone with
# a write token can push a tag, including a re-tag of an older attested image;
# only that job can attest one. Use the returned reference, not the tag, so a
# tag moved after the check is not what gets pulled. Errors go to $2.
_image_verified() {
  local ref=$1 signer="${E2E_IMAGE_CACHE_SIGNER:-${GITHUB_REPOSITORY:-}}" digest pinned workflow errors rc=1
  [[ -n "${signer}" ]] || return 1
  _image_digest "${ref}" || return 1
  digest="${REPLY}"
  pinned="${ref%:*}@${digest}"
  workflow="${signer}/${_IMAGE_CACHE_PUBLISHER}@refs/heads/master"
  errors="$(mktemp)" || return 1
  # The attestations API answers 5xx now and then; a retry is cheaper than the
  # build a miss costs.
  for _ in 1 2; do
    if _attested "${pinned}" "${ref}" "${signer}" "${workflow}" 2>"${errors}"; then
      rc=0
      break
    fi
    grep -q 'HTTP 5[0-9][0-9]' "${errors}" || break
    sleep 5
  done
  cat "${errors}" >>"${2:-/dev/null}"
  rm -f "${errors}"
  ((rc == 0)) || return 1
  REPLY="${pinned}"
}

# gh enforces the identity, but how strictly (case, anchoring) has changed
# between releases, so the certificate is checked again here: the publish
# workflow on master, on a GitHub-hosted runner, from one of its own triggers,
# and a subject of exactly this tag and digest. Anyone with a write token can
# attest a digest too; the limit keeps their attestations from crowding out
# the publisher's.
_attested() {
  local pinned=$1 ref=$2 signer=$3 workflow=$4
  timeout -k 10 120 gh attestation verify "oci://${pinned}" --repo "${signer}" --signer-workflow "${workflow}" \
    --source-ref refs/heads/master --deny-self-hosted-runners --limit 100 --format json |
    jq -e --arg name "${ref}" --arg digest "${pinned##*@sha256:}" \
      --arg san "https://github.com/${workflow}" --arg repo "https://github.com/${signer}" '
      any(.[].verificationResult
        | select(.signature.certificate as $c
          | $c.subjectAlternativeName == $san and $c.sourceRepositoryRef == "refs/heads/master"
            and $c.sourceRepositoryURI == $repo and $c.runnerEnvironment == "github-hosted"
            and ($c.githubWorkflowTrigger | IN("push", "schedule", "workflow_dispatch")))
        | .statement.subject[];
        .name == $name and .digest.sha256 == $digest)' >/dev/null
}

# Writes image <ref> as a docker archive to <dest>. Pulls it from
# E2E_IMAGE_CACHE when master published the same key, otherwise builds it.
build_image() {
  local name=$1 ref=$2 dest=$3
  local start=${SECONDS} source=build key=- cached seconds errors="" rc
  IMAGE_BASES=""
  if [[ -n "${E2E_IMAGE_CACHE:-}" && -z "${_IMAGE_CACHE_STALLED:-}" ]] && image_key "${name}"; then
    key="${IMAGE_KEY}"
    cached="${E2E_IMAGE_CACHE,,}:${name}-${key}"
    errors="$(mktemp 2>/dev/null)" || errors=""
    if _image_verified "${cached}" "${errors:-/dev/null}"; then
      # Several times a normal pull, and short enough that the job's
      # timeout-minutes still leaves time to build instead.
      rc=0
      echo "FROM ${REPLY}" |
        timeout -k 30 300 docker buildx build -t "${ref}" -o "type=docker,dest=${dest},compression-level=0" - || rc=$?
      if ((rc == 0)); then
        source=cache
      elif ((rc == 124 || rc == 137)); then
        _image_cache_stalled "pulling ${REPLY}"
      else
        echo "image-cache: pulling ${REPLY} failed, building instead" >&2
      fi
    elif [[ -z "${_IMAGE_CACHE_STALLED:-}" ]] && _image_published "${cached}"; then
      echo "image-cache: ${cached} did not verify as the publisher's, building instead" >&2
      [[ -z "${errors}" ]] || sed 's/^/image-cache:   /' "${errors}" >&2
    fi
    [[ -z "${errors}" ]] || rm -f "${errors}"
  fi

  if [[ "${source}" == build ]]; then
    image_spec "${name}" || return 1
    _build_spec -t "${ref}" -o "type=docker,dest=${dest},compression-level=0" || return 1
  fi

  seconds=$((SECONDS - start))
  echo "image-cache: ${name} from ${source} in ${seconds}s"
  if [[ -n "${GITHUB_STEP_SUMMARY:-}" ]]; then
    if [[ -z "${_IMAGE_CACHE_SUMMARY:-}" ]]; then
      printf '| image | key | source | seconds |\n|---|---|---|---|\n' >>"${GITHUB_STEP_SUMMARY}"
      _IMAGE_CACHE_SUMMARY=1
    fi
    printf '| %s | %s | %s | %s |\n' "${name}" "${key:0:12}" "${source}" "${seconds}" >>"${GITHUB_STEP_SUMMARY}"
  fi
}

# Succeeds when a reference is readable without credentials, the way e2e runs
# read the cache; errors go to $2 (default: discarded). BUILDX_BUILDER names a
# builder kept in the real DOCKER_CONFIG, so the check uses the default one.
_image_public() {
  local config rc=0
  config="$(mktemp -d)" || return 1
  env -u BUILDX_BUILDER DOCKER_CONFIG="${config}" \
    timeout 60 docker buildx imagetools inspect --raw "$1" >/dev/null 2>"${2:-/dev/null}" || rc=$?
  rm -rf "${config}"
  return "${rc}"
}

# Prints the publish job an image belongs to. Images sharing a build context
# build in one job, so per-job setup and shared stages are paid once;
# huggingfaceserver gets its own because it takes as long as the rest together.
_image_group() {
  if [[ "$1" == huggingfaceserver ]]; then
    echo "$1"
    return 0
  fi
  image_spec "$1" || return 1
  if [[ "${IMAGE_CONTEXT}" == . ]]; then echo go; else echo "${IMAGE_CONTEXT}"; fi
}

# Prints the publish jobs as a JSON list of {group, images}, for the images
# e2e runs cannot read yet. Looks anonymously, as they do, so a private package
# shows up as missing and its publish job fails loudly.
image_cache_plan() {
  local name group lines=""
  while IFS= read -r name; do
    # An image that cannot be keyed here (e.g. a rate-limited base lookup) is
    # left to its publish job, which retries and fails loudly.
    if ! image_key "${name}" || ! _image_public "${E2E_IMAGE_CACHE}:${name}-${IMAGE_KEY}" ||
      ! _image_verified "${E2E_IMAGE_CACHE}:${name}-${IMAGE_KEY}"; then
      group="$(_image_group "${name}")" || return 1
      lines+="${group} ${name}"$'\n'
    fi
  done < <(_image_names)
  jq -cRn '[inputs | select(length > 0) | split(" ")]
    | group_by(.[0]) | map({group: .[0][0], images: (map(.[1]) | join(" "))})' <<<"${lines}"
}

# Publishes one image under <cached>, its bases pinned to <bases> as keyed,
# and appends "<digest> <cached>" to <subjects> for the attestation step.
_publish_one() {
  local name=$1 cached=$2 subjects=$4 meta digest
  IMAGE_BASES=$3
  image_spec "${name}" || return 1
  # A job queued behind another one for the same image finds it attested. A
  # tag without the attestation (pushed by anyone else, or by a run whose
  # attestation step failed) is built and pushed over.
  if _image_verified "${cached}"; then
    echo "image-cache: ${cached} already published"
  else
    meta="$(mktemp)" || return 1
    if ! _build_spec -t "${cached}" --provenance=false --push --metadata-file "${meta}"; then
      rm -f "${meta}"
      return 1
    fi
    digest="$(jq -r '."containerimage.digest" // empty' "${meta}")"
    rm -f "${meta}"
    [[ "${digest}" == sha256:* ]] || return 1
    echo "${digest#sha256:}  ${cached}" >"${subjects}"
  fi
  if ! _image_public "${cached}" /dev/stderr; then
    echo "image-cache: ${cached} is not readable without credentials, so every e2e lookup misses. Make the package public."
    return 1
  fi
}

# Builds and pushes the given images that are not attested yet, up to
# IMAGE_CACHE_JOBS (default 4) at a time in one builder, then checks each is
# readable without credentials. Lists "<digest>  <tag>" per pushed image in
# E2E_IMAGE_CACHE_SUBJECTS for the workflow to attest, and nothing else: the
# workflow attests whatever the file holds. Output streams live, prefixed per
# image.
image_cache_publish() {
  local name failed=0 status
  local -a keyed=()
  local -A keys=() key_bases=()
  [[ -z "${E2E_IMAGE_CACHE_SUBJECTS:-}" ]] || : >"${E2E_IMAGE_CACHE_SUBJECTS}"
  # Keys first, in this shell: they memoize, the builds run in subshells. An
  # image that cannot be keyed fails the job without holding up the others.
  for name in "$@"; do
    if ! image_key "${name}"; then
      echo "::error::image-cache: ${name} cannot be keyed, not publishing it"
      failed=1
      continue
    fi
    keyed+=("${name}")
    keys[${name}]="${IMAGE_KEY}"
    key_bases[${name}]="${IMAGE_BASES}"
  done
  status="$(mktemp -d)" || return 1
  for name in "${keyed[@]}"; do
    while (($(jobs -rp | wc -l) >= ${IMAGE_CACHE_JOBS:-4})); do
      sleep 2
    done
    {
      rc=0
      _publish_one "${name}" "${E2E_IMAGE_CACHE}:${name}-${keys[${name}]}" "${key_bases[${name}]}" \
        "${status}/${name}.subject" 2>&1 || rc=$?
      echo "${rc}" >"${status}/${name}"
    } | sed -u "s/^/[${name}] /" &
  done
  wait
  for name in "${keyed[@]}"; do
    if [[ "$(cat "${status}/${name}" 2>/dev/null)" != 0 ]]; then
      echo "::error::image-cache: publishing ${name} failed, see the [${name}] lines above"
      failed=1
    fi
    # Every pushed image gets attested, also when a later check failed.
    if [[ -s "${status}/${name}.subject" ]]; then
      if [[ -n "${E2E_IMAGE_CACHE_SUBJECTS:-}" ]]; then
        cat "${status}/${name}.subject" >>"${E2E_IMAGE_CACHE_SUBJECTS}"
      else
        echo "::warning::image-cache: E2E_IMAGE_CACHE_SUBJECTS is unset, ${name} will not be attested and never hit"
      fi
    fi
  done
  rm -rf "${status}"
  return "${failed}"
}

# Deletes cache versions that no current key uses, unless they were created in
# the last <days>. PR runs merge the latest master, so only current keys can
# hit; the recent ones stay for runs that merged a slightly older master.
# Deletes nothing unless every image could be keyed, since a missing key would
# make its live version look stale.
image_cache_prune() {
  local days=${1:-7} name owner package scope live=() stale="" id tags deleted=0 failed=0
  owner="${E2E_IMAGE_CACHE}"
  owner="${owner#*/}"
  package="${owner#*/}"
  owner="${owner%%/*}"
  # A plain loop, not a process substitution, so no failure goes unseen here.
  for name in "${!IMAGE_SPECS[@]}"; do
    if ! image_key "${name}"; then
      echo "image-cache: ${name} has no key, not pruning" >&2
      return 1
    fi
    live+=("${name}-${IMAGE_KEY}")
  done

  scope="users/${owner}"
  if [[ "$(gh api "users/${owner}" --jq .type)" == Organization ]]; then
    scope="orgs/${owner}"
  fi
  scope+="/packages/container/${package//\//%2F}/versions"

  stale="$(gh api --paginate "${scope}" --jq '.[]' | jq -rs \
    --argjson live "$(jq -cn '$ARGS.positional | map({(.): true}) | add // {}' --args "${live[@]}")" \
    --argjson days "${days}" '
      .[]
      | select((.created_at | fromdateiso8601) < now - $days * 86400)
      | select(any(.metadata.container.tags[]?; $live[.]) | not)
      | "\(.id) \(.metadata.container.tags | join(","))"')" || return 1

  while read -r id tags; do
    [[ -n "${id}" ]] || continue
    # Public versions with over 5,000 downloads cannot be deleted.
    if gh api -X DELETE "${scope}/${id}" >/dev/null; then
      echo "image-cache: deleted ${id} ${tags:-untagged}"
      deleted=$((deleted + 1))
    else
      echo "::warning::image-cache: could not delete version ${id} ${tags:-untagged}"
      failed=$((failed + 1))
    fi
  done <<<"${stale}"
  echo "image-cache: pruned ${deleted}, kept ${failed} undeletable, ${#live[@]} live keys"
  # Every deletion failing points at permissions rather than single versions.
  ((deleted > 0 || failed == 0))
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  set -o errexit
  set -o nounset
  set -o pipefail
  : "${E2E_IMAGE_CACHE:?E2E_IMAGE_CACHE must name the cache repository}"
  # Image references are lowercase; repository owners need not be.
  E2E_IMAGE_CACHE="${E2E_IMAGE_CACHE,,}"
  case "${1:-}" in
    plan) image_cache_plan ;;
    publish)
      shift
      image_cache_publish "${@:?usage: $0 publish <image>...}"
      ;;
    prune) image_cache_prune "${@:2}" ;;
    *)
      echo "usage: $0 plan | publish <image>... | prune [days]" >&2
      exit 2
      ;;
  esac
fi
