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
    local binary_name="yq_${os}_${arch}"
    local download_url="https://github.com/mikefarah/yq/releases/download/${YQ_VERSION}/${binary_name}"

    echo "Installing yq ${YQ_VERSION} for ${os}/${arch}..."

    if command -v yq &>/dev/null; then
        local current_version=$(yq --version 2>&1 | grep -oP 'version \K[v0-9.]+' || echo "unknown")
        if [[ "v$current_version" == "$YQ_VERSION" ]]; then
            echo "yq ${YQ_VERSION} is already installed"
            return 0
        fi
        echo "Upgrading yq from ${current_version} to ${YQ_VERSION}..."
    fi

    local temp_file=$(mktemp)

    if command -v wget &>/dev/null; then
        wget -q "${download_url}" -O "${temp_file}"
    elif command -v curl &>/dev/null; then
        curl -sL "${download_url}" -o "${temp_file}"
    else
        echo "Error: Neither wget nor curl is available" >&2
        rm -f "${temp_file}"
        exit 1
    fi

    chmod +x "${temp_file}"

    if [[ -w "${BIN_DIR}" ]]; then
        mv "${temp_file}" "${BIN_DIR}/yq"
    else
        sudo mv "${temp_file}" "${BIN_DIR}/yq"
    fi

    echo "Successfully installed yq ${YQ_VERSION} to ${BIN_DIR}/yq"
    yq --version
}

install
