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

# This script sets up the kserve-ci-e2e-test namespace for E2E testing.
# It is idempotent - it will delete and recreate the namespace if it already exists.
set -o errexit
set -o nounset
set -o pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"

source "$SCRIPT_DIR/common.sh"

# Get deployment type from first argument, default to empty string
DEPLOYMENT_TYPE="${1:-}"

# Image variables with defaults (will use environment variables if set)
: "${SKLEARN_IMAGE:=kserve/sklearnserver:latest}"
: "${STORAGE_INITIALIZER_IMAGE:=quay.io/opendatahub/kserve-storage-initializer:latest}"

NAMESPACE="kserve-ci-e2e-test"

echo "Setting up CI namespace: $NAMESPACE"

# Delete namespace if it exists for idempotency
"$SCRIPT_DIR/teardown-ci-namespace.sh" "$DEPLOYMENT_TYPE" "$NAMESPACE"

# Create namespace
echo "Creating namespace $NAMESPACE"
cat <<EOF | oc apply -f -
apiVersion: v1
kind: Namespace
metadata:
  name: $NAMESPACE
EOF

# Apply S3 artifact secret
echo "Applying S3 artifact secret"
oc apply -f "$PROJECT_ROOT/config/overlays/test/s3-local-backend/mlpipeline-s3-artifact-secret.yaml" -n "$NAMESPACE"

# Apply storage-config secret (used by TLS and storagespec tests)
echo "Applying storage-config secret"
oc apply -f "$PROJECT_ROOT/config/overlays/test/s3-local-backend/storage-config-secret.yaml" -n "$NAMESPACE"

# Create empty odh-trusted-ca-bundle configmap (used by S3 TLS tests).
# Created here rather than in a pytest fixture to avoid race conditions
# when pytest-xdist distributes tests across multiple workers.
echo "Creating odh-trusted-ca-bundle configmap"
cat <<EOF | oc apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: odh-trusted-ca-bundle
  namespace: $NAMESPACE
EOF

# Build and apply ServingRuntimes
echo "Installing ServingRuntimes"
kustomize build "$PROJECT_ROOT/config/overlays/test/clusterresources" |
  sed 's/ClusterServingRuntime/ServingRuntime/' |
  sed '/runAsUser:/d' | # remove runAs from existing servingRuntimes
  sed "s|kserve/sklearnserver:latest|${SKLEARN_IMAGE}|" |
  sed "s|kserve/storage-initializer:latest|${STORAGE_INITIALIZER_IMAGE}|" |
  oc apply -n "$NAMESPACE" -f -

echo "CI namespace setup complete"

