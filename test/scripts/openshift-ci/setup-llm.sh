#!/usr/bin/env bash
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/common.sh"
source "$SCRIPT_DIR/version.sh"
PROJECT_ROOT="$(find_project_root "$SCRIPT_DIR")"

KSERVE_DEPLOY="true"
RHCL_DEPLOY="false"

show_usage() {
    echo "Usage: $0 [OPTIONS]"
    echo "Setup LLM environment on OpenShift"
    echo ""
    echo "Default behavior:"
    echo "  • KServe deployment: enabled"
    echo "  • Kuadrant deployment: disabled"
    echo ""
    echo "Options:"
    echo "  --skip-kserve         Skip KServe deployment"
    echo "  --deploy-kuadrant     Deploy Kuadrant"
    echo "  -h, --help            Show this help message"
    echo ""
    echo "Examples:"
    echo "  $0                           # Deploy KServe only (default)"
    echo "  $0 --skip-kserve             # Deploy neither"
    echo "  $0 --deploy-kuadrant         # Deploy both KServe and Kuadrant"
    echo "  $0 --skip-kserve --deploy-kuadrant  # Deploy Kuadrant only"
}

while [[ $# -gt 0 ]]; do
    case $1 in
        --skip-kserve)
            KSERVE_DEPLOY="false"
            shift
            ;;
        --deploy-kuadrant)
            RHCL_DEPLOY="true"
            shift
            ;;
        -h|--help)
            show_usage
            exit 0
            ;;
        *)
            echo "Error: Unknown option '$1'" >&2
            show_usage >&2
            exit 1
            ;;
    esac
done

echo "🔧 Configuration:"
echo "  KServe deployment: $([ "$KSERVE_DEPLOY" == "true" ] && echo "✅ enabled" || echo "❌ disabled")"
echo "  Kuadrant deployment: $([ "$RHCL_DEPLOY" == "true" ] && echo "✅ enabled" || echo "❌ disabled")"
echo ""

server_version=$(get_openshift_server_version)
echo "Checking OpenShift server version...($server_version)"

if version_compare "$server_version" "4.19.9"; then
  echo "🎯 Server version ($server_version) is 4.19.9 or higher - continue with the script"
else
  echo "🎯 Server version ($server_version) is not supported so stop the script"
  exit 1
fi

$SCRIPT_DIR/infra/deploy.cert-manager.sh
$SCRIPT_DIR/infra/deploy.lws.sh

$SCRIPT_DIR/infra/deploy.gateway.ingress.sh

if [ "${RHCL_DEPLOY}" == "true" ]; then
  $SCRIPT_DIR/infra/deploy.kuadrant.sh
fi

if [ "${KSERVE_DEPLOY}" == "true" ]; then
  kubectl create ns opendatahub || true

  kubectl kustomize config/crd/ | kubectl apply --server-side=true -f -
  wait_for_crd  llminferenceserviceconfigs.serving.kserve.io  90s

  kustomize build config/overlays/odh | kubectl apply  --server-side=true --force-conflicts -f -
  wait_for_pod_ready "opendatahub" "control-plane=kserve-controller-manager" 300s
fi

