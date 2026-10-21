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
# digests of the base images, the daemon platform and this script. When master
# has published an image under the same key, build_image pulls it instead of
# building. Anything that cannot be keyed is built as before.
#
# Sourced by build-images.sh and build-server-runtimes.sh. The e2e-image-cache
# workflow runs it directly to fill the cache from master:
#
#   image-cache.sh plan             JSON list of images missing from the cache
#   image-cache.sh publish <image>  build and push one image unless present
#
# E2E_IMAGE_CACHE names the cache repository, e.g.
# ghcr.io/kserve/kserve-e2e-images. When unset, build_image only builds.

_IMAGE_CACHE_LIB="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/$(basename "${BASH_SOURCE[0]}")"
_IMAGE_CACHE_ROOT="$(cd "$(dirname "${_IMAGE_CACHE_LIB}")/../../.." && pwd)"

# Image name -> "<dockerfile> <context> [target]", relative to the repo root.
# Names match the cache tags, not the kserve-images.sh variables, so that
# overriding an image name locally cannot point at another image's cache.
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
    echo "image-cache: unknown image $1" >&2
    return 1
  fi
  IMAGE_TARGET=""
  read -r IMAGE_DOCKERFILE IMAGE_CONTEXT IMAGE_TARGET <<<"${spec}"
}

# Runs docker buildx build for the current image spec, passing extra arguments.
# Each base in IMAGE_BASES is pinned to the digest its key recorded, so a tag
# that moves after keying cannot be built into an image under the old key.
_build_spec() {
  local -a build_args=()
  local _ ref digest
  if [[ -n "${IMAGE_TARGET}" ]]; then
    build_args+=(--target "${IMAGE_TARGET}")
  fi
  while read -r _ ref digest; do
    if [[ -n "${ref}" && "${ref}" != *@* ]]; then
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
  local context=$1
  if [[ -z "${_IMAGE_CACHE_CONTEXTS[${context}]:-}" ]]; then
    local out hash
    out="$(mktemp -d)" || return 1
    if ! printf 'FROM scratch\nCOPY . /\n' |
      docker buildx build --quiet -f - -o "type=local,dest=${out}" "${_IMAGE_CACHE_ROOT}/${context}" >/dev/null; then
      rm -rf "${out}"
      return 1
    fi
    if ! hash="$(tar -C "${out}" --sort=name --format=gnu --mtime=@0 --owner=0 --group=0 --numeric-owner -cf - . |
      sha256sum)"; then
      rm -rf "${out}"
      return 1
    fi
    rm -rf "${out}"
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
    for attempt in 1 2 3 4; do
      if digest="$(docker buildx imagetools inspect --raw "${ref}" | sha256sum; exit "${PIPESTATUS[0]}")"; then
        _IMAGE_CACHE_DIGESTS[${ref}]="sha256:${digest%% *}"
        break
      fi
      [[ "${attempt}" == 4 ]] && return 1
      sleep $((attempt * attempt * 3))
    done
  fi
  REPLY="${_IMAGE_CACHE_DIGESTS[${ref}]}"
}

# Sets REPLY to one "base <ref> <digest>" line per external image the target
# stage (the last one without a target) needs: through FROM, COPY --from or
# RUN --mount from=, with global ARG defaults expanded. Stages the target does
# not reach are skipped, as BuildKit skips them. Anything that cannot be
# expanded is an error, not a guess.
_base_digests() {
  local IFS=$' \t\n'
  local dockerfile=$1 target=${2:-} line="" word="" name="" value="" image="" stage="" dep="" out="" ref=""
  local i=0 n=-1 seen_from=false
  local -A args=() index=() visited=()
  local -a deps=() words=() from=() todo=() refs=()
  while IFS= read -r line || [[ -n "${line}" ]]; do
    line="${line%$'\r'}"
    read -r -a words <<<"${line}"
    from=()
    if [[ "${seen_from}" == false && "${line}" =~ ^ARG[[:space:]] ]]; then
      for word in "${words[@]:1}"; do
        [[ "${word}" == *=* ]] || continue
        value="${word#*=}"
        value="${value#\"}"
        value="${value%\"}"
        for name in "${!args[@]}"; do
          value="${value//\$\{${name}\}/${args[${name}]}}"
        done
        args[${word%%=*}]="${value}"
      done
      continue
    elif [[ "${line}" =~ ^FROM[[:space:]] ]]; then
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
          --mount=*from=*)
            value="${word#*from=}"
            from+=("${value%%,*}")
            ;;
        esac
      done
    fi

    for image in "${from[@]}"; do
      for name in "${!args[@]}"; do
        image="${image//\$\{${name}\}/${args[${name}]}}"
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
    if ! _base_digest "${ref}"; then
      echo "image-cache: cannot resolve base image ${ref}" >&2
      return 1
    fi
    out+="base ${ref} ${REPLY}"$'\n'
  done < <(printf '%s\n' "${refs[@]}" | sort -u)
  REPLY="${out}"
}

# Sets IMAGE_KEY and IMAGE_BASES (the "base <ref> <digest>" lines it covers)
# for an image and logs the inputs, so a surprising miss can be diffed against
# the publisher's log.
image_key() {
  local name=$1 platform hash context_hash dockerfile_hash script_hash bases manifest
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
  _context_hash "${IMAGE_CONTEXT}" || return 1
  context_hash="${REPLY}"
  _base_digests "${IMAGE_DOCKERFILE}" "${IMAGE_TARGET}" || return 1
  bases="${REPLY}"

  manifest="script ${script_hash}
platform ${platform}
dockerfile ${IMAGE_DOCKERFILE} ${dockerfile_hash}
target ${IMAGE_TARGET:--}
context ${IMAGE_CONTEXT} ${context_hash}
${bases}"
  hash="$(sha256sum <<<"${manifest}")"
  IMAGE_KEY="${hash%% *}"
  IMAGE_BASES="${bases}"
  printf 'image-cache: %s key %s\n%s' "${name}" "${IMAGE_KEY}" "${manifest}" | sed '2,$s/^/  /' >&2
}

_image_published() {
  docker buildx imagetools inspect --raw "$1" >/dev/null 2>&1
}

# Writes image <ref> as a docker archive to <dest>. Pulls it from
# E2E_IMAGE_CACHE when master published the same key, otherwise builds it.
build_image() {
  local name=$1 ref=$2 dest=$3
  local start=${SECONDS} source=build key=- cached seconds
  IMAGE_BASES=""
  if [[ -n "${E2E_IMAGE_CACHE:-}" ]] && image_key "${name}"; then
    key="${IMAGE_KEY}"
    cached="${E2E_IMAGE_CACHE}:${name}-${key}"
    if _image_published "${cached}"; then
      if echo "FROM ${cached}" |
        docker buildx build -t "${ref}" -o "type=docker,dest=${dest},compression-level=0" -; then
        source=cache
      else
        echo "image-cache: pulling ${cached} failed, building instead" >&2
      fi
    fi
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

image_cache_plan() {
  local name missing=()
  while IFS= read -r name; do
    # An image that cannot be keyed here (e.g. a rate-limited base lookup) is
    # left to its publish job, which retries and fails loudly.
    if ! image_key "${name}" || ! _image_published "${E2E_IMAGE_CACHE}:${name}-${IMAGE_KEY}"; then
      missing+=("${name}")
    fi
  done < <(printf '%s\n' "${!IMAGE_SPECS[@]}" | sort)
  jq -cn '$ARGS.positional' --args "${missing[@]}"
}

image_cache_publish() {
  local name=$1 cached
  image_key "${name}" || return 1
  cached="${E2E_IMAGE_CACHE}:${name}-${IMAGE_KEY}"
  # A run queued behind another one for the same image finds it published.
  if _image_published "${cached}"; then
    echo "image-cache: ${cached} already published"
    return 0
  fi
  _build_spec -t "${cached}" --provenance=false --push
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  set -o errexit
  set -o nounset
  set -o pipefail
  : "${E2E_IMAGE_CACHE:?E2E_IMAGE_CACHE must name the cache repository}"
  case "${1:-}" in
    plan) image_cache_plan ;;
    publish) image_cache_publish "${2:?usage: $0 publish <image>}" ;;
    *)
      echo "usage: $0 plan | publish <image>" >&2
      exit 2
      ;;
  esac
fi
