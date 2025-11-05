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

# Create Istio IngressClass resource
# Usage: manage.istio-ingress-class.sh [--reinstall|--uninstall]
#   or:  REINSTALL=true manage.istio-ingress-class.sh
#   or:  UNINSTALL=true manage.istio-ingress-class.sh

# INIT
SCRIPT_DIR="$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")" && pwd)"

source "${SCRIPT_DIR}/../common.sh"

REINSTALL="${REINSTALL:-false}"
UNINSTALL="${UNINSTALL:-false}"

if [[ "$*" == *"--uninstall"* ]]; then
    UNINSTALL=true
elif [[ "$*" == *"--reinstall"* ]]; then
    REINSTALL=true
fi
# INIT END

uninstall() {
    log_info "Deleting Istio IngressClass 'istio'..."
    kubectl delete ingressclass "istio" --ignore-not-found=true --force --grace-period=0 2>/dev/null || true
    log_success "Istio IngressClass 'istio' deleted"
}

install() {
    if kubectl get ingressclass "istio" &>/dev/null; then
        if [ "$REINSTALL" = false ]; then
            log_info "Istio IngressClass 'istio' already exists. Use --reinstall to recreate."
            exit 0
        else
            log_info "Recreating Istio IngressClass 'istio'..."
            uninstall
        fi
    fi

    log_info "Creating Istio IngressClass 'istio'..."
    cat <<EOF | kubectl apply -f -
apiVersion: networking.k8s.io/v1
kind: IngressClass
metadata:
  name: istio
spec:
  controller: istio.io/ingress-controller
EOF

    log_success "Istio IngressClass 'istio' created successfully!"
}

if [ "$UNINSTALL" = true ]; then
    uninstall
    exit 0
fi

install
