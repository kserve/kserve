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
# like kustomize are installed by the script if not available.
# The oc CLI is assumed to be configured with the credentials of the
# target cluster. The target cluster is assumed to be a clean cluster.
set -o errexit
set -o nounset
set -o pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"

source "$SCRIPT_DIR/common.sh"

# Parse command line options
: "${INSTALL_ODH_OPERATOR:=false}"

# Set the applications namespace based on installation method
# ODH operator uses 'opendatahub', manual installation uses 'kserve'
if [[ "$INSTALL_ODH_OPERATOR" == "true" ]]; then
  KSERVE_NAMESPACE="opendatahub"
else
  KSERVE_NAMESPACE="kserve"
fi

echo "Using namespace: $KSERVE_NAMESPACE for KServe components"
: "${SKLEARN_IMAGE:=kserve/sklearnserver:latest}"
: "${KSERVE_CONTROLLER_IMAGE:=quay.io/opendatahub/kserve-controller:latest}"
: "${KSERVE_AGENT_IMAGE:=quay.io/opendatahub/kserve-agent:latest}"
: "${KSERVE_ROUTER_IMAGE:=quay.io/opendatahub/kserve-router:latest}"
: "${STORAGE_INITIALIZER_IMAGE:=quay.io/opendatahub/kserve-storage-initializer:latest}"
: "${ODH_MODEL_CONTROLLER_IMAGE:=quay.io/opendatahub/odh-model-controller:fast}"
: "${ERROR_404_ISVC_IMAGE:=error-404-isvc:latest}"
: "${SUCCESS_200_ISVC_IMAGE:=success-200-isvc:latest}"
: "${LLMISVC_CONTROLLER_IMAGE:=quay.io/opendatahub/llmisvc-controller:latest}"

echo "SKLEARN_IMAGE=$SKLEARN_IMAGE"
echo "KSERVE_CONTROLLER_IMAGE=$KSERVE_CONTROLLER_IMAGE"
echo "LLMISVC_CONTROLLER_IMAGE=$LLMISVC_CONTROLLER_IMAGE"
echo "KSERVE_AGENT_IMAGE=$KSERVE_AGENT_IMAGE"
echo "KSERVE_ROUTER_IMAGE=$KSERVE_ROUTER_IMAGE"
echo "STORAGE_INITIALIZER_IMAGE=$STORAGE_INITIALIZER_IMAGE"
echo "ERROR_404_ISVC_IMAGE=$ERROR_404_ISVC_IMAGE"
echo "SUCCESS_200_ISVC_IMAGE=$SUCCESS_200_ISVC_IMAGE"

# Install Kustomize using the centralized install script
$PROJECT_ROOT/hack/setup/cli/install-kustomize.sh
export PATH="${PROJECT_ROOT}/bin:${PATH}"

echo "Installing KServe Python SDK ..."
pushd $PROJECT_ROOT >/dev/null
  ./test/scripts/gh-actions/setup-uv.sh
  # Add bin directory to PATH so uv command is available
  export PATH="${PROJECT_ROOT}/bin:${PATH}"
popd
pushd $PROJECT_ROOT/python/kserve >/dev/null
  uv sync --active --group test
  uv pip install timeout-sampler
popd

# Install autoscaler only if NOT using ODH operator (operator handles it)
if [[ "$INSTALL_ODH_OPERATOR" == "false" ]]; then
  $SCRIPT_DIR/deploy.cma.sh
fi

# Install ODH operator if requested
if [[ "$INSTALL_ODH_OPERATOR" == "true" ]]; then
  $SCRIPT_DIR/deploy.odh.sh
fi

# Add CA certificate extraction for raw deployments
if [[ "$1" =~ raw ]]; then
  echo "⏳ Extracting OpenShift CA certificates for raw deployment"
  # Get comprehensive CA bundle including both cluster and service CAs
  {
    # Cluster root CA bundle
    oc get configmap kube-root-ca.crt -o jsonpath='{.data.ca\.crt}' 2>/dev/null && echo ""

    # OpenShift service CA
    oc get configmap openshift-service-ca.crt -n openshift-config-managed -o jsonpath='{.data.service-ca\.crt}' 2>/dev/null || \
    oc get secret service-ca -n openshift-service-ca -o jsonpath='{.data.service-ca\.crt}' 2>/dev/null | base64 -d || true
  } > /tmp/ca.crt

  # Verify we got a valid CA bundle
  if [ -s "/tmp/ca.crt" ] && grep -q "BEGIN CERTIFICATE" "/tmp/ca.crt"; then
    echo "✅ CA certificate bundle extracted ($(grep -c "BEGIN CERTIFICATE" /tmp/ca.crt) certificates)"
  else
    echo "❌ Failed to extract CA certificates"
  fi
fi

# Ensure the target namespace exists
oc new-project ${KSERVE_NAMESPACE} || true

# Install KServe components based on method
if [[ "$INSTALL_ODH_OPERATOR" == "false" ]]; then
  # Manual installation: Install KServe directly with PR images
  echo "⏳ Installing LLMISvc CRDs"
  kustomize build $PROJECT_ROOT/config/crd/full/llmisvc | oc apply --server-side=true --force-conflicts -f -
  wait_for_crd llminferenceserviceconfigs.serving.kserve.io 90s

  echo "⏳ Installing KServe with SeaweedFS"
  kustomize build $PROJECT_ROOT/config/overlays/odh-test |
    sed "s|kserve/storage-initializer:latest|${STORAGE_INITIALIZER_IMAGE}|" |
    sed "s|kserve/agent:latest|${KSERVE_AGENT_IMAGE}|" |
    sed "s|kserve/router:latest|${KSERVE_ROUTER_IMAGE}|" |
    sed "s|kserve/kserve-controller:latest|${KSERVE_CONTROLLER_IMAGE}|" |
    sed "s|kserve/llmisvc-controller:latest|${LLMISVC_CONTROLLER_IMAGE}|" |
    oc apply --server-side=true --force-conflicts -f -

  echo "⏳ Waiting for LLMISvc CRD to be ready"
  wait_for_crd llminferenceserviceconfigs.serving.kserve.io 90s

  # Install DSC/DSCI for manual installation
  echo "Installing DSC/DSCI resources..."
  oc apply -f config/overlays/odh-test/dsci.yaml
  oc apply -f config/overlays/odh-test/dsc.yaml

else
  # ODH operator path: Copy full kustomize directory structure to operator PVC
  echo "⏳ Preparing PR manifests for ODH operator..."

  # Copy PR manifests into ODH operator PVC using the helper script
  echo "Copying PR manifests into ODH operator PVC..."
  $SCRIPT_DIR/copy-kserve-manifests-to-pvc.sh

  # Apply DSC/DSCI to trigger deployment with custom manifests
  # Sed the DSCI to use opendatahub namespace for ODH operator mode
  echo "Applying DSC/DSCI to trigger ODH operator deployment with PR manifests..."
  sed 's/applicationsNamespace:  kserve/applicationsNamespace: opendatahub/' config/overlays/odh-test/dsci.yaml | oc apply -f -
  oc apply -f config/overlays/odh-test/dsc.yaml

  # Wait for KServe controller to be deployed by the operator
  echo "Waiting for ODH operator to deploy KServe components with PR manifests..."
  wait_for_pod_ready "${KSERVE_NAMESPACE}" "control-plane=kserve-controller-manager" 600s

  echo "ODH operator deployed KServe using PR manifests and images"
fi

# Patch the inferenceservice-config ConfigMap, when running RawDeployment tests
if skip_serverless "$1"; then
  echo "Patching RAW deployment, markers: $1"
  export OPENSHIFT_INGRESS_DOMAIN=$(oc get ingresses.config cluster -o jsonpath='{.spec.domain}')
  oc patch configmap inferenceservice-config -n ${KSERVE_NAMESPACE} --type=strategic \
    --patch-file=<(cat config/overlays/odh-test/configmap/inferenceservice-openshift-ci-raw.yaml | \
    sed "s/namespace: kserve/namespace: ${KSERVE_NAMESPACE}/" | \
    envsubst)
  oc delete pod -n ${KSERVE_NAMESPACE} -l control-plane=kserve-controller-manager

  # Patch DSC only in manual mode (operator mode uses yaml files directly)
  if [[ "$INSTALL_ODH_OPERATOR" == "false" ]]; then
    oc patch DataScienceCluster test-dsc --type='json' -p='[{"op": "replace", "path": "/spec/components/kserve/defaultDeploymentMode", "value": "RawDeployment"}]'
  fi
else
  export OPENSHIFT_INGRESS_DOMAIN=$(oc get ingresses.config cluster -o jsonpath='{.spec.domain}')
  oc patch configmap inferenceservice-config -n ${KSERVE_NAMESPACE} --type=strategic \
    --patch-file=<(cat config/overlays/odh-test/configmap/inferenceservice-openshift-ci-serverless-predictor.yaml | \
    sed "s/namespace: kserve/namespace: ${KSERVE_NAMESPACE}/" | \
    envsubst)
fi

# Wait until KServe starts
echo "waiting kserve-controller get ready..."
oc wait --for=condition=ready pod -l control-plane=kserve-controller-manager -n ${KSERVE_NAMESPACE} --timeout=300s

if ! skip_serverless "$1"; then
  echo "Installing authorino and kserve gateways"
  # authorino
  curl -sL https://raw.githubusercontent.com/Kuadrant/authorino-operator/main/utils/install.sh | sed "s|kubectl|oc|" |
    bash -s -- -v 0.16.0

fi

# Wait for/Install ODH Model Controller based on method
if [[ "$INSTALL_ODH_OPERATOR" == "false" ]]; then
  echo "Installing ODH Model Controller manually with PR images"
  kustomize build $PROJECT_ROOT/test/scripts/openshift-ci |
      sed "s|quay.io/opendatahub/odh-model-controller:fast|${ODH_MODEL_CONTROLLER_IMAGE}|" |
      oc apply -n ${KSERVE_NAMESPACE} -f -
  oc rollout status deployment/odh-model-controller -n ${KSERVE_NAMESPACE} --timeout=300s
else
  # ODH operator deploys odh-model-controller using custom manifests from PVC
  # The image was already configured in copy-kserve-manifests-to-pvc.sh via params.env
  echo "Waiting for ODH operator to deploy ODH Model Controller with PR image..."
  wait_for_pod_ready "${KSERVE_NAMESPACE}" "app=odh-model-controller" 600s

  echo "Verifying ODH Model Controller deployment..."
  oc rollout status deployment/odh-model-controller -n ${KSERVE_NAMESPACE} --timeout=300s

  # Verify the correct image is being used
  ACTUAL_IMAGE=$(oc get deployment odh-model-controller -n ${KSERVE_NAMESPACE} -o jsonpath='{.spec.template.spec.containers[0].image}')
  echo "ODH Model Controller deployed with image: $ACTUAL_IMAGE"
fi

# Configure certs for the python requests by getting the CA cert from the kserve controller pod
export CA_CERT_PATH="/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
# The run-e2e-tests script expects the CA cert to be in /tmp/ca.crt
oc exec deploy/kserve-controller-manager -n ${KSERVE_NAMESPACE} -- cat $CA_CERT_PATH > /tmp/ca.crt

echo "Add testing models to SeaweedFS S3 storage ..."

# Wait for SeaweedFS deployment to be ready
echo "Waiting for SeaweedFS deployment to be ready..."
oc rollout status deployment/seaweedfs -n ${KSERVE_NAMESPACE} --timeout=300s

# The s3-init job is already created by the kustomize build above.
# It may have failed if SeaweedFS wasn't ready yet, so check and re-create if needed.
if oc wait --for=condition=complete job/s3-init -n ${KSERVE_NAMESPACE} --timeout=60s 2>/dev/null; then
  echo "S3 init job already completed successfully"
else
  echo "S3 init job not completed, re-creating..."
  oc delete job s3-init -n ${KSERVE_NAMESPACE} --wait=true --ignore-not-found
  sed "s/s3-service.kserve/s3-service.${KSERVE_NAMESPACE}/" \
    "$PROJECT_ROOT/config/overlays/test/s3-local-backend/seaweedfs-init-job.yaml" | \
    oc apply -n ${KSERVE_NAMESPACE} -f -

  echo "Waiting for S3 init job to complete..."
  if ! oc wait --for=condition=complete job/s3-init -n ${KSERVE_NAMESPACE} --timeout=300s; then
    echo "S3 init job failed. Pod status and logs:"
    oc get pods -l job-name=s3-init -n ${KSERVE_NAMESPACE}
    oc logs -l job-name=s3-init -n ${KSERVE_NAMESPACE} --tail=50 || true
    exit 1
  fi
fi

# Configure S3 TLS if needed
if [[ "$1" =~ "kserve_on_openshift" ]]; then
  echo "Configuring SeaweedFS S3 TLS"
  "$PROJECT_ROOT/test/scripts/openshift-ci/tls/setup-s3-tls.sh" custom
  "$PROJECT_ROOT/test/scripts/openshift-ci/tls/setup-s3-tls.sh" serving
fi

# Allow all traffic to the KServe namespace. Without this networkpolicy, webhook will return 500
# error msg: 'http: server gave HTTP response to HTTPS client"}]},"code":500}'
cat <<EOF | oc apply -f -
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-all
  namespace: ${KSERVE_NAMESPACE}
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

echo "Prepare CI namespace and install ServingRuntimes"
$SCRIPT_DIR/setup-ci-namespace.sh "$1"

echo "Setup complete"
