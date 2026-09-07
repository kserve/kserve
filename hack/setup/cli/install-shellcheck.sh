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
SCRIPT_DIR="$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")" && pwd)"
source "${SCRIPT_DIR}/../common.sh"
# INIT END

# ShellCheck release archives are named with uname-style architectures
# (x86_64 / aarch64) rather than the Go-style names detect_arch returns.
detect_shellcheck_arch() {
    case "$(uname -m)" in
        x86_64 | amd64) echo "x86_64" ;;
        aarch64 | arm64) echo "aarch64" ;;
        *)
            log_error "Unsupported architecture for ShellCheck: $(uname -m)"
            exit 1
            ;;
    esac
}

install() {
    local os arch archive_name download_url
    os=$(detect_os)
    arch=$(detect_shellcheck_arch)
    # .tar.gz is published from v0.11.0 onwards and avoids depending on xz;
    # releases older than that ship .tar.xz only.
    archive_name="shellcheck-${SHELLCHECK_VERSION}.${os}.${arch}.tar.gz"
    download_url="https://github.com/koalaman/shellcheck/releases/download/${SHELLCHECK_VERSION}/${archive_name}"

    log_info "Installing ShellCheck ${SHELLCHECK_VERSION} for ${os}/${arch}..."

    # ShellCheck findings change between releases, so require the exact pinned
    # version rather than "at least" the pinned version.
    if [[ -x "${BIN_DIR}/shellcheck" ]]; then
        local current_version
        current_version=$("${BIN_DIR}/shellcheck" --version 2>/dev/null | awk '/^version:/ {print "v" $2}')
        if [[ "${current_version}" == "${SHELLCHECK_VERSION}" ]]; then
            log_info "ShellCheck ${current_version} is already installed in ${BIN_DIR}"
            return 0
        fi
        [[ -n "${current_version}" ]] && log_info "Replacing ShellCheck ${current_version} with ${SHELLCHECK_VERSION} in ${BIN_DIR}..."
    fi

    local temp_dir temp_file
    temp_dir=$(mktemp -d)
    temp_file="${temp_dir}/${archive_name}"

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

    local binary_path="${temp_dir}/shellcheck-${SHELLCHECK_VERSION}/shellcheck"

    if [[ ! -f "${binary_path}" ]]; then
        log_error "shellcheck binary not found in archive" >&2
        rm -rf "${temp_dir}"
        exit 1
    fi

    chmod +x "${binary_path}"

    if [[ -w "${BIN_DIR}" ]]; then
        mv "${binary_path}" "${BIN_DIR}/shellcheck"
    else
        sudo mv "${binary_path}" "${BIN_DIR}/shellcheck"
    fi

    rm -rf "${temp_dir}"

    log_success "Successfully installed ShellCheck ${SHELLCHECK_VERSION} to ${BIN_DIR}/shellcheck"
    "${BIN_DIR}/shellcheck" --version
}

install
