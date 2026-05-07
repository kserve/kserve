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
    local binary_name="operator-sdk_${os}_${arch}"
    local download_url="https://github.com/operator-framework/operator-sdk/releases/download/${OPERATOR_SDK_VERSION}/${binary_name}"

    log_info "Installing operator-sdk ${OPERATOR_SDK_VERSION} for ${os}/${arch}..."

    if [[ -x "${BIN_DIR}/operator-sdk" ]]; then
        local current_version=$("${BIN_DIR}/operator-sdk" version 2>&1 | grep -oP 'v[0-9.]+' | head -1)
        if [[ -n "$current_version" ]] && version_gte "$current_version" "$OPERATOR_SDK_VERSION"; then
            log_info "operator-sdk ${current_version} is already installed in ${BIN_DIR} (>= ${OPERATOR_SDK_VERSION})"
            return 0
        fi
        [[ -n "$current_version" ]] && log_info "Upgrading operator-sdk from ${current_version} to ${OPERATOR_SDK_VERSION}..."
    fi

    local temp_file=$(mktemp)

    if command -v wget &>/dev/null; then
        wget -q "${download_url}" -O "${temp_file}"
    elif command -v curl &>/dev/null; then
        curl -sL "${download_url}" -o "${temp_file}"
    else
        log_error "Neither wget nor curl is available" >&2
        rm -f "${temp_file}"
        exit 1
    fi

    chmod +x "${temp_file}"

    if [[ -w "${BIN_DIR}" ]]; then
        mv "${temp_file}" "${BIN_DIR}/operator-sdk"
    else
        sudo mv "${temp_file}" "${BIN_DIR}/operator-sdk"
    fi

    log_success "Successfully installed operator-sdk ${OPERATOR_SDK_VERSION} to ${BIN_DIR}/operator-sdk"
    "${BIN_DIR}/operator-sdk" version
}

install