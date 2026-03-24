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

# This script tears down the CI namespace for E2E testing.
# It deletes the namespace, and Kubernetes will automatically clean up all resources within it.
set -o errexit
set -o nounset
set -o pipefail

# Deployment type (if "raw", skip ServiceMeshMember deletion)
DEPLOYMENT_TYPE="${1:-}"

# Namespace to tear down (default: kserve-ci-e2e-test)
NAMESPACE="${2:-kserve-ci-e2e-test}"

echo "Tearing down CI namespace: $NAMESPACE"

# Delete namespace if it exists
if oc get namespace "$NAMESPACE" >/dev/null 2>&1; then
    # Delete ServiceMeshMember in $NAMESPACE BEFORE deleting the namespace (if not raw deployment)
    if [ "$DEPLOYMENT_TYPE" != "raw" ]; then
        if oc get servicemeshmember default -n "$NAMESPACE" 2>/dev/null; then
            echo "Deleting ServiceMeshMember default in $NAMESPACE (before namespace deletion)"
            # Use timeout to prevent indefinite wait when finalizers are stuck
            if ! timeout 60s oc delete servicemeshmember default -n "$NAMESPACE" --ignore-not-found 2>/dev/null; then
                echo "WARNING: ServiceMeshMember default in $NAMESPACE did not delete within 60s, removing finalizers"
                oc patch servicemeshmember default -n "$NAMESPACE" --type=json -p='[{"op": "remove", "path": "/metadata/finalizers"}]' 2>/dev/null || true
                oc delete servicemeshmember default -n "$NAMESPACE" --ignore-not-found || true
            fi
        fi
    fi
  echo "Deleting namespace $NAMESPACE..."
  oc delete namespace "$NAMESPACE" --ignore-not-found --wait=true --timeout=120s || true
  # Wait for namespace to be fully deleted
  echo "Waiting for namespace to be fully deleted..."
  while oc get namespace "$NAMESPACE" >/dev/null 2>&1; do
    sleep 2
  done
  echo "Namespace $NAMESPACE has been deleted"
else
  echo "Namespace $NAMESPACE does not exist, skipping deletion"
fi

echo "CI namespace teardown complete"

