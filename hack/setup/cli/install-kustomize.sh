#!/bin/bash

# Copyright 2025 The KServe Authors.
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
SCRIPT_DIR="$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")" && pwd)"
source "${SCRIPT_DIR}/../common.sh"
# INIT END

install() {
    local os=$(detect_os)
    local arch=$(detect_arch)
    local archive_name="kustomize_${KUSTOMIZE_VERSION}_${os}_${arch}.tar.gz"
    local download_url="https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%2F${KUSTOMIZE_VERSION}/${archive_name}"

    echo "Installing Kustomize ${KUSTOMIZE_VERSION} for ${os}/${arch}..."

    if command -v kustomize &>/dev/null; then
        local current_version=$(kustomize version --short 2>/dev/null | grep -oP 'v[0-9.]+' || echo "unknown")
        if [[ "$current_version" == "$KUSTOMIZE_VERSION" ]]; then
            echo "Kustomize ${KUSTOMIZE_VERSION} is already installed"
            return 0
        fi
        echo "Upgrading Kustomize from ${current_version} to ${KUSTOMIZE_VERSION}..."
    fi

    local temp_dir=$(mktemp -d)
    local temp_file="${temp_dir}/${archive_name}"

    if command -v wget &>/dev/null; then
        wget -q "${download_url}" -O "${temp_file}"
    elif command -v curl &>/dev/null; then
        curl -sL "${download_url}" -o "${temp_file}"
    else
        echo "Error: Neither wget nor curl is available" >&2
        rm -rf "${temp_dir}"
        exit 1
    fi

    tar -xzf "${temp_file}" -C "${temp_dir}"

    local binary_path="${temp_dir}/kustomize"

    if [[ ! -f "${binary_path}" ]]; then
        echo "Error: kustomize binary not found in archive" >&2
        rm -rf "${temp_dir}"
        exit 1
    fi

    chmod +x "${binary_path}"

    if [[ -w "${BIN_DIR}" ]]; then
        mv "${binary_path}" "${BIN_DIR}/kustomize"
    else
        sudo mv "${binary_path}" "${BIN_DIR}/kustomize"
    fi

    rm -rf "${temp_dir}"

    echo "Successfully installed Kustomize ${KUSTOMIZE_VERSION} to ${BIN_DIR}/kustomize"
    kustomize version
}

install
