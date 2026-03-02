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

# This is a helper script to remove the E2E in local test execution environments.
# Only works for raw deployment mode.
set -o errexit
set -o nounset
set -o pipefail

: "${SKLEARN_IMAGE:=kserve/sklearnserver:latest}"
: "${KSERVE_CONTROLLER_IMAGE:=quay.io/opendatahub/kserve-controller:latest}"
: "${KSERVE_AGENT_IMAGE:=quay.io/opendatahub/kserve-agent:latest}"
: "${KSERVE_ROUTER_IMAGE:=quay.io/opendatahub/kserve-router:latest}"
: "${STORAGE_INITIALIZER_IMAGE:=quay.io/opendatahub/kserve-storage-initializer:latest}"
: "${ODH_MODEL_CONTROLLER_IMAGE:=quay.io/opendatahub/odh-model-controller:fast}"
: "${ERROR_404_ISVC_IMAGE:=error-404-isvc:latest}"
: "${SUCCESS_200_ISVC_IMAGE:=success-200-isvc:latest}"

echo "SKLEARN_IMAGE=$SKLEARN_IMAGE"
echo "KSERVE_CONTROLLER_IMAGE=$KSERVE_CONTROLLER_IMAGE"
echo "KSERVE_AGENT_IMAGE=$KSERVE_AGENT_IMAGE"
echo "KSERVE_ROUTER_IMAGE=$KSERVE_ROUTER_IMAGE"
echo "STORAGE_INITIALIZER_IMAGE=$STORAGE_INITIALIZER_IMAGE"
echo "ERROR_404_ISVC_IMAGE=$ERROR_404_ISVC_IMAGE"
echo "SUCCESS_200_ISVC_IMAGE=$SUCCESS_200_ISVC_IMAGE"

# Create directory for installing tooling
# It is assumed that $HOME/.local/bin is in the $PATH
mkdir -p $HOME/.local/bin
MY_PATH=$(dirname "$0")
PROJECT_ROOT=$MY_PATH/../../../

echo "Deleting KServe with Minio"
kustomize build $PROJECT_ROOT/config/overlays/test |
  sed "s|kserve/storage-initializer:latest|${STORAGE_INITIALIZER_IMAGE}|" |
  sed "s|kserve/agent:latest|${KSERVE_AGENT_IMAGE}|" |
  sed "s|kserve/router:latest|${KSERVE_ROUTER_IMAGE}|" |
  sed "s|kserve/kserve-controller:latest|${KSERVE_CONTROLLER_IMAGE}|" |
  oc delete -f - --ignore-not-found || true

if [[ "${1:-}" =~ kserve_on_openshift ]]; then
  echo "Deleting TLS MinIO resources and generated certificates"
  kustomize build $PROJECT_ROOT/test/overlays/openshift-ci |
    oc delete -n kserve -f - --ignore-not-found || true
  oc delete secret minio-tls-custom -n kserve --ignore-not-found || true
  oc delete secret minio-tls-serving -n kserve --ignore-not-found || true
  # Clean up storage-config secret entries for TLS MinIO
  if oc get secret storage-config -n kserve-ci-e2e-test > /dev/null 2>&1; then
    oc patch secret storage-config -n kserve-ci-e2e-test --type=json \
      -p='[{"op": "remove", "path": "/data/localTLSMinIOServing"}, {"op": "remove", "path": "/data/localTLSMinIOCustom"}]' 2>/dev/null || true
  fi
  rm -rf $PROJECT_ROOT/test/scripts/openshift-ci/tls/certs
fi
# Install DSC/DSCI for test. (sometimes there is timing issue when it is under the same kustomization so it is separated)
oc delete -f config/overlays/test/dsci.yaml --ignore-not-found || true
oc delete -f config/overlays/test/dsc.yaml --ignore-not-found || true


echo "Deleting ODH Model Controller"
kustomize build $PROJECT_ROOT/test/scripts/openshift-ci |
    sed "s|quay.io/opendatahub/odh-model-controller:fast|${ODH_MODEL_CONTROLLER_IMAGE}|" |
    oc delete -f - --ignore-not-found || true
  oc wait --for=delete pod -l app=odh-model-controller -n kserve --timeout=30s 2>/dev/null || true


echo "Delete CI namespace and ServingRuntimes"
# Tear down the CI namespace (Kubernetes will automatically clean up all resources including ServiceMeshMember)
"$MY_PATH/teardown-ci-namespace.sh" "${1:-}" "kserve-ci-e2e-test"

oc delete -f $PROJECT_ROOT/config/overlays/test/minio/minio-user-secret.yaml -n kserve-ci-e2e-test --ignore-not-found || true

kustomize build $PROJECT_ROOT/config/overlays/test/clusterresources |
  sed 's/ClusterServingRuntime/ServingRuntime/' |
  sed "s|kserve/sklearnserver:latest|${SKLEARN_IMAGE}|" |
  sed "s|kserve/storage-initializer:latest|${STORAGE_INITIALIZER_IMAGE}|" |
  oc delete -n kserve-ci-e2e-test -f - --ignore-not-found || true


cat <<EOF | oc delete -f - --ignore-not-found || true
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-all
  namespace: kserve
spec:
  podSelector: {}
  ingress:
  - {}
  egress:
  - {}
  policyTypes:
  - Ingress
  - Egress
EOF

echo "Delete CMA / KEDA operator"
oc delete kedacontroller -n openshift-keda keda --ignore-not-found || true
oc delete subscription -n openshift-keda openshift-custom-metrics-autoscaler-operator --ignore-not-found || true
oc delete namespace openshift-keda --ignore-not-found || true

echo "Teardown complete"
