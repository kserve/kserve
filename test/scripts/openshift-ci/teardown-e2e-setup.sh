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

# This is a helper script to run E2E tests on the openshift-ci operator.
# This script assumes to be run inside a container/machine that has
# python pre-installed and the `oc` command available. Additional tooling,
# like kustomize and the mc client are installed by the script if not available.
# The oc CLI is assumed to be configured with the credentials of the
# target cluster. The target cluster is assumed to be a clean cluster.
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

# Install KServe stack
if [ "$1" != "raw" ]; then
  echo "Deleting OSSM"
  oc delete servicemeshcontrolplane basic -n istio-system --ignore-not-found
  SMO_VERSION=$(oc get subscription servicemeshoperator -n openshift-operators -o jsonpath='{.status.currentCSV}')
  oc delete subscription -n openshift-operators servicemeshoperator --ignore-not-found
  oc delete csv -n openshift-operators $SMO_VERSION --ignore-not-found
  oc delete project istio-system --ignore-not-found
  echo "Deleting Serverless"
  SERVERLESS_VERSION=$(oc get subscription serverless-operator -n openshift-serverless -o jsonpath='{.status.currentCSV}')
  oc delete service knative-local-gateway -n istio-system --ignore-not-found
  oc delete gateway knative-ingress-gateway -n knative-serving --ignore-not-found
  oc delete gateway knative-local-gateway -n knative-serving --ignore-not-found
  oc delete KnativeServing knative-serving -n knative-serving --ignore-not-found
  oc delete ServiceMeshMember default -n knative-serving --ignore-not-found
  oc delete project knative-serving --ignore-not-found
  oc delete subscription serverless-operator -n openshift-serverless --ignore-not-found
  oc delete operatorgroup serverless-operators -n openshift-serverless --ignore-not-found
  oc delete csv $SERVERLESS_VERSION -n openshift-serverless --ignore-not-found
  oc delete namespace openshift-serverless --ignore-not-found
fi

echo "Installing KServe with Minio"
kustomize build $PROJECT_ROOT/config/overlays/test |
  sed "s|kserve/storage-initializer:latest|${STORAGE_INITIALIZER_IMAGE}|" |
  sed "s|kserve/agent:latest|${KSERVE_AGENT_IMAGE}|" |
  sed "s|kserve/router:latest|${KSERVE_ROUTER_IMAGE}|" |
  sed "s|kserve/kserve-controller:latest|${KSERVE_CONTROLLER_IMAGE}|" |
  oc delete --server-side=true -f -

# Install DSC/DSCI for test. (sometimes there is timing issue when it is under the same kustomization so it is separated)
oc delete -f config/overlays/test/dsci.yaml
oc delete -f config/overlays/test/dsc.yaml

if [ "$1" != "raw" ]; then
  echo "Deleting authorino and kserve gateways"
  # TODO: authorino
  curl -sL https://raw.githubusercontent.com/Kuadrant/authorino-operator/main/utils/install.sh | sed "s|kubectl|oc|" |
    bash -s -- -v 0.16.0

  # kserve-local-gateway
  curl https://raw.githubusercontent.com/opendatahub-io/opendatahub-operator/bde4b4e8478b5d03195e2777b9d550922e3cdcbc/components/kserve/resources/servicemesh/routing/istio-kserve-local-gateway.tmpl.yaml |
    sed "s/{{ .ControlPlane.Namespace }}/istio-system/g" |
    oc delete -f -

  curl https://raw.githubusercontent.com/opendatahub-io/opendatahub-operator/bde4b4e8478b5d03195e2777b9d550922e3cdcbc/components/kserve/resources/servicemesh/routing/kserve-local-gateway-svc.tmpl.yaml |
    sed "s/{{ .ControlPlane.Namespace }}/istio-system/g" |
    oc delete -f -
fi

echo "Deleting ODH Model Controller"
kustomize build $PROJECT_ROOT/test/scripts/openshift-ci |
    sed "s|quay.io/opendatahub/odh-model-controller:fast|${ODH_MODEL_CONTROLLER_IMAGE}|" |
    oc delete -n kserve -f -
  oc wait --for=condition=ready pod -l app=odh-model-controller -n kserve --timeout=300s


echo "Delete CI namespace and  ServingRuntimes"
cat <<EOF | oc delete -f -
apiVersion: v1
kind: Namespace
metadata:
  name: kserve-ci-e2e-test
EOF

if [ "$1" != "raw" ]; then
  cat <<EOF | oc delete -f -
apiVersion: maistra.io/v1
kind: ServiceMeshMember
metadata:
  name: default
  namespace: kserve-ci-e2e-test
spec:
  controlPlaneRef:
    namespace: istio-system
    name: basic
EOF
fi

oc delete -f $PROJECT_ROOT/config/overlays/test/minio/minio-user-secret.yaml -n kserve-ci-e2e-test

kustomize build $PROJECT_ROOT/config/overlays/test/clusterresources |
  sed 's/ClusterServingRuntime/ServingRuntime/' |
  sed "s|kserve/sklearnserver:latest|${SKLEARN_IMAGE}|" |
  sed "s|kserve/storage-initializer:latest|${STORAGE_INITIALIZER_IMAGE}|" |
  oc delete -n kserve-ci-e2e-test -f -


cat <<EOF | oc delete -f -
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
oc delete kedacontroller -n openshift-keda keda --ignore-not-found
oc delete subscription -n openshift-keda openshift-custom-metrics-autoscaler-operator --ignore-not-found
oc delete namespace openshift-keda --ignore-not-found

echo "Teardown complete"
