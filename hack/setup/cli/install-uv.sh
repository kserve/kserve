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

    # uv uses different naming convention
    local uv_os="${os}"
    local uv_arch="${arch}"

    # Map architecture names
    if [[ "${arch}" == "amd64" ]]; then
        uv_arch="x86_64"
    elif [[ "${arch}" == "arm64" ]]; then
        uv_arch="aarch64"
    fi

    # Map OS names
    if [[ "${os}" == "darwin" ]]; then
        uv_os="apple-darwin"
    elif [[ "${os}" == "linux" ]]; then
        uv_os="unknown-linux-gnu"
    fi

    local archive_name="uv-${uv_arch}-${uv_os}.tar.gz"
    local download_url="https://github.com/astral-sh/uv/releases/download/${UV_VERSION}/${archive_name}"

    echo "Installing uv ${UV_VERSION} for ${os}/${arch}..."

    if command -v uv &>/dev/null; then
        local current_version=$(uv --version 2>/dev/null | awk '{print $2}')
        if [[ -n "$current_version" ]] && version_gte "$current_version" "$UV_VERSION"; then
            echo "uv ${current_version} is already installed (>= ${UV_VERSION})"
            return 0
        fi
        [[ -n "$current_version" ]] && echo "Upgrading uv from ${current_version} to ${UV_VERSION}..."
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

    local binary_path="${temp_dir}/uv-${uv_arch}-${uv_os}/uv"

    if [[ ! -f "${binary_path}" ]]; then
        echo "Error: uv binary not found in archive" >&2
        rm -rf "${temp_dir}"
        exit 1
    fi

    chmod +x "${binary_path}"

    if [[ -w "${BIN_DIR}" ]]; then
        mv "${binary_path}" "${BIN_DIR}/uv"
    else
        sudo mv "${binary_path}" "${BIN_DIR}/uv"
    fi

    rm -rf "${temp_dir}"

    echo "Successfully installed uv ${UV_VERSION} to ${BIN_DIR}/uv"
    uv version
}

install
