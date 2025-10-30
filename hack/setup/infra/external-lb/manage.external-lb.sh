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

# Setup External LoadBalancer for local Kubernetes clusters
# Usage: manage.external-lb.sh [--reinstall|--uninstall]
#   or:  REINSTALL=true manage.external-lb.sh
#   or:  UNINSTALL=true manage.external-lb.sh
#
# Supported platforms:
#   - kind: Uses cloud-provider-kind
#   - minikube: Uses MetalLB addon
#
# Environment variables:
#   METALLB_IP_RANGE_START - MetalLB IP range start (default: <minikube-ip>.200)
#   METALLB_IP_RANGE_END   - MetalLB IP range end (default: <minikube-ip>.235)

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

# VARIABLES
PLATFORM="${PLATFORM:-$(detect_platform)}"
TEMPLATE_DIR="${SCRIPT_DIR}/templates"
# VARIABLES END

uninstall() {
    log_info "Uninstalling External LoadBalancer for platform: ${PLATFORM}"

    case "${PLATFORM}" in
        kind)
            if pgrep -f cloud-provider-kind > /dev/null; then
                log_info "Stopping cloud-provider-kind..."
                pkill -f cloud-provider-kind || true
                log_success "cloud-provider-kind stopped"
            else
                log_info "cloud-provider-kind is not running"
            fi
            ;;

        minikube)
            log_info "Disabling MetalLB addon..."
            minikube addons disable metallb 2>/dev/null || true
            log_success "MetalLB disabled"
            ;;

        openshift|kubernetes)
            log_info "Platform ${PLATFORM} does not require external LB teardown. Skipping."
            ;;
    esac

    log_success "External LoadBalancer uninstalled for ${PLATFORM}!"
}

install() {
    if [ "$REINSTALL" = true ]; then
        log_info "Reinstalling External LoadBalancer..."
        uninstall
    fi

    log_info "Setting up External LoadBalancer for platform: ${PLATFORM}"

    case "${PLATFORM}" in
        kind)
            log_info "Installing cloud-provider-kind for KIND cluster..."

            if ! command_exists cloud-provider-kind; then
                log_info "Installing cloud-provider-kind..."
                go install sigs.k8s.io/cloud-provider-kind@latest

                if ! command_exists cloud-provider-kind; then
                    log_error "Failed to install cloud-provider-kind. Make sure GOPATH/bin is in your PATH."
                    exit 1
                fi
            fi

            if pgrep -f cloud-provider-kind > /dev/null; then
                log_info "cloud-provider-kind is already running"
            else
                log_info "Starting cloud-provider-kind..."
                cloud-provider-kind > /dev/null 2>&1 &
                sleep 2

                if pgrep -f cloud-provider-kind > /dev/null; then
                    log_success "cloud-provider-kind started successfully"
                else
                    log_error "Failed to start cloud-provider-kind"
                    exit 1
                fi
            fi
            ;;

        minikube)
            log_info "Setting up MetalLB for Minikube cluster..."

            log_info "Enabling MetalLB addon..."
            minikube addons enable metallb

            MINIKUBE_IP=$(minikube ip)
            if [[ -z "${MINIKUBE_IP}" ]]; then
                log_error "Failed to get minikube IP"
                exit 1
            fi

            log_info "Minikube IP: ${MINIKUBE_IP}"

            PREFIX=${MINIKUBE_IP%.*}
            START=${METALLB_IP_RANGE_START:-${PREFIX}.200}
            END=${METALLB_IP_RANGE_END:-${PREFIX}.235}

            log_info "Configuring MetalLB IP range: ${START}-${END}"

            sed -e "s/{{START}}/${START}/g" -e "s/{{END}}/${END}/g" \
                "${TEMPLATE_DIR}/metallb-config.yaml.tmpl" | kubectl apply -f -

            log_success "MetalLB configured successfully with IP range: ${START}-${END}"
            ;;

        openshift|kubernetes)
            log_info "Platform ${PLATFORM} does not require external LB setup. Skipping."
            exit 0
            ;;

        *)
            log_error "Unknown platform: ${PLATFORM}"
            exit 1
            ;;
    esac

    log_success "External LoadBalancer setup completed for ${PLATFORM}!"
}

if [ "$UNINSTALL" = true ]; then
    uninstall
    exit 0
fi

install
