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
    local archive_name="helm-${HELM_VERSION}-${os}-${arch}.tar.gz"
    local download_url="https://get.helm.sh/${archive_name}"

    log_info "Installing Helm ${HELM_VERSION} for ${os}/${arch}..."

    if command -v helm &>/dev/null; then
        local current_version=$(helm version --template='{{.Version}}' 2>/dev/null)
        if [[ -n "$current_version" ]] && version_gte "$current_version" "$HELM_VERSION"; then
            log_info "Helm ${current_version} is already installed (>= ${HELM_VERSION})"
            return 0
        fi
        [[ -n "$current_version" ]] && log_info "Upgrading Helm from ${current_version} to ${HELM_VERSION}..."
    fi

    local temp_dir=$(mktemp -d)
    local temp_file="${temp_dir}/${archive_name}"

    if command -v wget &>/dev/null; then
        wget -q "${download_url}" -O "${temp_file}"
    elif command -v curl &>/dev/null; then
        curl -sL "${download_url}" -o "${temp_file}"
    else
        log_error "Neither wget nor curl is available" >&2
        rm -rf "${temp_dir}"
        exit 1
    fi

    tar -xzf "${temp_file}" -C "${temp_dir}"

    local binary_path="${temp_dir}/${os}-${arch}/helm"

    if [[ ! -f "${binary_path}" ]]; then
        log_error "helm binary not found in archive" >&2
        rm -rf "${temp_dir}"
        exit 1
    fi

    chmod +x "${binary_path}"

    if [[ -w "${BIN_DIR}" ]]; then
        mv "${binary_path}" "${BIN_DIR}/helm"
    else
        sudo mv "${binary_path}" "${BIN_DIR}/helm"
    fi

    rm -rf "${temp_dir}"

    log_success "Successfully installed Helm ${HELM_VERSION} to ${BIN_DIR}/helm"
    helm version
}

install
