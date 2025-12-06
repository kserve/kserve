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
    local binary_name="kind-${os}-${arch}"
    local download_url="https://github.com/kubernetes-sigs/kind/releases/download/${KIND_VERSION}/${binary_name}"

    log_info "Installing Kind ${KIND_VERSION} for ${os}/${arch}..."

    if command -v kind &>/dev/null; then
        local current_version=$(kind version 2>/dev/null | grep -oP 'kind v[0-9.]+' | awk '{print $2}')
        if [[ -n "$current_version" ]] && version_gte "$current_version" "$KIND_VERSION"; then
            log_info "Kind ${current_version} is already installed (>= ${KIND_VERSION})"
            return 0
        fi
        [[ -n "$current_version" ]] && log_info "Upgrading Kind from ${current_version} to ${KIND_VERSION}..."
    fi

    local temp_dir=$(mktemp -d)
    local temp_file="${temp_dir}/kind"

    if command -v wget &>/dev/null; then
        wget -q "${download_url}" -O "${temp_file}"
    elif command -v curl &>/dev/null; then
        curl -sL "${download_url}" -o "${temp_file}"
    else
        log_error "Neither wget nor curl is available" >&2
        rm -rf "${temp_dir}"
        exit 1
    fi

    if [[ ! -f "${temp_file}" ]]; then
        log_error "kind binary not downloaded" >&2
        rm -rf "${temp_dir}"
        exit 1
    fi

    chmod +x "${temp_file}"

    if [[ -w "${BIN_DIR}" ]]; then
        mv "${temp_file}" "${BIN_DIR}/kind"
    else
        sudo mv "${temp_file}" "${BIN_DIR}/kind"
    fi

    rm -rf "${temp_dir}"

    log_success "Successfully installed Kind ${KIND_VERSION} to ${BIN_DIR}/kind"
    kind version
}

install
