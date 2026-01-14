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

# Install Gateway API CRDs
# Usage: manage.gateway-api-crd-only.sh [--reinstall|--uninstall]
#   or:  REINSTALL=true manage.gateway-api-crd-only.sh
#   or:  UNINSTALL=true manage.gateway-api-crd-only.sh

# INIT
SCRIPT_DIR="$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")" && pwd)"

source "${SCRIPT_DIR}/../../common.sh"

REINSTALL="${REINSTALL:-false}"
UNINSTALL="${UNINSTALL:-false}"

if [[ "$*" == *"--uninstall"* ]]; then
    UNINSTALL=true
elif [[ "$*" == *"--reinstall"* ]]; then
    REINSTALL=true
fi
# INIT END

# Resolve the Gateway API version to an available GitHub release.
# If the version is a pseudo-version (e.g., v1.3.1-0.20251106052652-079e4774d76b),
# find the next available release version that is >= the base version.
resolve_gateway_api_version() {
    local version="$1"
    local base_version

    # Check if version is a pseudo-version (contains timestamp like -0.YYYYMMDD)
    if [[ "$version" =~ ^v([0-9]+\.[0-9]+\.[0-9]+)-0\.[0-9]{14}- ]]; then
        base_version="${BASH_REMATCH[1]}"
        log_info "Detected pseudo-version ${version}, base version is ${base_version}"

        # Fetch available releases from GitHub and find versions matching vX.Y.Z pattern
        local releases
        releases=$(curl -s "https://api.github.com/repos/kubernetes-sigs/gateway-api/releases" | \
            grep -oE '"tag_name":\s*"v[0-9]+\.[0-9]+\.[0-9]+"' | \
            grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | \
            sort -V)

        if [ -z "$releases" ]; then
            log_warning "Failed to fetch releases from GitHub, trying version as-is"
            echo "$version"
            return
        fi

        # Find the smallest version >= base_version
        local next_version=""
        for release in $releases; do
            local release_num="${release#v}"
            if version_gte "$release_num" "$base_version"; then
                next_version="$release"
                break
            fi
        done

        if [ -n "$next_version" ]; then
            log_info "Using next available release: ${next_version}"
            echo "$next_version"
            return
        fi

        # Fallback to latest version if no suitable release found
        local latest_version
        latest_version=$(echo "$releases" | tail -1)
        log_warning "No suitable release found >= v${base_version}, using latest: ${latest_version}"
        echo "$latest_version"
    else
        # Version is a regular release tag, use as-is
        echo "$version"
    fi
}

uninstall() {
    log_info "Uninstalling Gateway API CRDs..."
    local resolved_version
    resolved_version=$(resolve_gateway_api_version "$GATEWAY_API_VERSION")
    kubectl delete -f "https://github.com/kubernetes-sigs/gateway-api/releases/download/${resolved_version}/standard-install.yaml" --ignore-not-found=true 2>/dev/null || true
    log_success "Gateway API CRDs uninstalled"
}

install() {
    if kubectl get crd gateways.gateway.networking.k8s.io &>/dev/null; then
        if [ "$REINSTALL" = false ]; then
            log_info "Gateway API CRDs are already installed. Use --reinstall to reinstall."
            return 0
        else
            log_info "Reinstalling Gateway API CRDs..."
            uninstall
        fi
    fi

    local resolved_version
    resolved_version=$(resolve_gateway_api_version "$GATEWAY_API_VERSION")

    log_info "Installing Gateway API CRDs ${resolved_version}..."
    kubectl apply -f "https://github.com/kubernetes-sigs/gateway-api/releases/download/${resolved_version}/standard-install.yaml"

    log_success "Successfully installed Gateway API CRDs ${resolved_version}"

    wait_for_crds "60s" \
        "gateways.gateway.networking.k8s.io" \
        "gatewayclasses.gateway.networking.k8s.io"

    log_success "Gateway API CRDs are ready!"
}

if [ "$UNINSTALL" = true ]; then
    uninstall
    exit 0
fi

install
