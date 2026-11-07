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

# INIT
set -o errexit
set -o nounset
set -o pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=/dev/null
source "${SCRIPT_DIR}/../../../kserve-deps.env"
BIN_DIR="${BIN_DIR:-${SCRIPT_DIR}/../../../bin}"
# INIT END

if [[ -x "${BIN_DIR}/lychee" ]] && \
    [[ "$("${BIN_DIR}/lychee" --version)" == "lychee ${LYCHEE_VERSION#v}" ]]; then
    exit 0
fi

case "$(uname -m)" in
    x86_64 | amd64) arch=x86_64 ;;
    aarch64 | arm64) arch=aarch64 ;;
    *) echo "Unsupported Lychee architecture: $(uname -m)" >&2; exit 1 ;;
esac
case "$(uname -s)" in
    Linux) target="${arch}-unknown-linux-musl" ;;
    Darwin) target="${arch}-apple-darwin" ;;
    *) echo "Unsupported operating system: $(uname -s)" >&2; exit 1 ;;
esac

archive="lychee-${target}.tar.gz"
expected=$(awk -v archive="$archive" '$2 == archive {print $1}' "${SCRIPT_DIR}/lychee-checksums.txt")
[[ "$expected" =~ ^[0-9a-f]{64}$ ]] || { echo "Missing pinned checksum for $archive" >&2; exit 1; }
download_dir=$(mktemp -d)
trap 'rm -rf "${download_dir}"' EXIT
url="https://github.com/lycheeverse/lychee/releases/download/lychee-${LYCHEE_VERSION}/${archive}"
curl --fail --silent --show-error --location --retry 3 "${url}" --output "${download_dir}/${archive}"
if command -v sha256sum >/dev/null; then
    printf '%s  %s\n' "$expected" "${download_dir}/${archive}" | sha256sum --check -
else
    printf '%s  %s\n' "$expected" "${download_dir}/${archive}" | shasum -a 256 --check -
fi
tar -xzf "${download_dir}/${archive}" -C "${download_dir}"
binary="${download_dir}/lychee-${target}/lychee"
[[ "$("$binary" --version)" == "lychee ${LYCHEE_VERSION#v}" ]] || { echo "Unexpected Lychee version" >&2; exit 1; }
mkdir -p "${BIN_DIR}"
install -m 755 "$binary" "${BIN_DIR}/lychee"
"${BIN_DIR}/lychee" --version
