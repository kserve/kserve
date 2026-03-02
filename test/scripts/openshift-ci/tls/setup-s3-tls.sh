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

# This is a helper script to create and configure the resources needed
# for SeaweedFS S3 storage to have TLS enabled with either an OpenShift serving certificate
# or a custom certificate.
# Usage: setup-s3-tls.sh [serving|custom]
set -o errexit
set -o nounset
set -o pipefail

CERT_TYPE="${1:-serving}"
if [[ "$CERT_TYPE" != "serving" && "$CERT_TYPE" != "custom" ]]; then
    echo "Error: Certificate type must be 'serving' or 'custom'"
    echo "Usage: $0 [serving|custom]"
    exit 1
fi

MY_PATH=$(dirname "$0")
PROJECT_ROOT=$MY_PATH/../../../../
TLS_DIR=$PROJECT_ROOT/test/scripts/openshift-ci/tls

# Set variables based on certificate type
if [[ "$CERT_TYPE" == "serving" ]]; then
    DEPLOYMENT_NAME="seaweedfs-tls-serving"
    APP_LABEL="seaweedfs-tls-serving"
    KUSTOMIZE_PATH="seaweedfs-tls-serving-cert"
    SECRET_NAME="seaweedfs-tls-serving"
    SERVICE_NAME="seaweedfs-tls-serving-service"
    STORAGE_CONFIG_KEY="localTLSS3Serving"
    CERT_DIR="serving"
    CERT_DESC="Openshift serving certificate"
else
    DEPLOYMENT_NAME="seaweedfs-tls-custom"
    APP_LABEL="seaweedfs-tls-custom"
    KUSTOMIZE_PATH="seaweedfs-tls-custom-cert"
    SECRET_NAME="seaweedfs-tls-custom"
    SERVICE_NAME="seaweedfs-tls-custom-service"
    STORAGE_CONFIG_KEY="localTLSS3Custom"
    CERT_DIR="custom"
    CERT_DESC="custom certificate"
fi

# If Kustomize is not installed, install it
if ! command -v kustomize &>/dev/null; then
  echo "Installing Kustomize"
  curl -s "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh" | bash -s -- 5.0.1 $HOME/.local/bin
fi

# Create kserve namespace if it does not already exist
if oc get namespace kserve > /dev/null 2>&1; then
    echo "Namespace kserve exists."
else
    cat <<EOF | oc apply -f -
apiVersion: v1
kind: Namespace
metadata:
    name: kserve
EOF
fi

# Cleanup existing resources for idempotency
echo "Cleaning up existing $CERT_TYPE TLS SeaweedFS resources for idempotency..."
# Delete deployment if it exists (needed for custom cert to allow re-patching)
if oc get deployment $DEPLOYMENT_NAME -n kserve > /dev/null 2>&1; then
    echo "Deleting existing $DEPLOYMENT_NAME deployment"
    oc delete deployment $DEPLOYMENT_NAME -n kserve --ignore-not-found || true
fi
# Delete TLS secret if it exists (only for custom cert - serving cert is managed by OpenShift)
if [[ "$CERT_TYPE" == "custom" ]]; then
    if oc get secret $SECRET_NAME -n kserve > /dev/null 2>&1; then
        echo "Deleting existing $SECRET_NAME secret"
        oc delete secret $SECRET_NAME -n kserve --ignore-not-found || true
    fi
fi
# Clean up storage-config entry for this certificate type (will be recreated)
if oc get secret storage-config -n kserve-ci-e2e-test > /dev/null 2>&1; then
    oc patch secret storage-config -n kserve-ci-e2e-test --type=json \
      -p="[{\"op\": \"remove\", \"path\": \"/data/${STORAGE_CONFIG_KEY}\"}]" 2>/dev/null || true
fi

# Create TLS SeaweedFS resources
kustomize build $PROJECT_ROOT/test/overlays/openshift-ci/$KUSTOMIZE_PATH |
  oc apply -n kserve --server-side=true -f -

# Wait for SeaweedFS deployment to be ready
echo "Waiting for $DEPLOYMENT_NAME deployment to be ready..."
oc rollout status deployment/$DEPLOYMENT_NAME -n kserve --timeout=300s

echo "Configuring SeaweedFS S3 for TLS with $CERT_DESC and adding models to storage"
# Handle certificate setup based on type
if [[ "$CERT_TYPE" == "serving" ]]; then
    # Add openshift generated serving certificates to certs directory
    if ! [ -d $TLS_DIR/certs/$CERT_DIR ]; then
        mkdir -p $TLS_DIR/certs/$CERT_DIR
    fi
    (oc get secret $SECRET_NAME -n kserve -o jsonpath="{.data['tls\.crt']}" | base64 -d) > $TLS_DIR/certs/$CERT_DIR/tls.crt
    (oc get secret $SECRET_NAME -n kserve -o jsonpath="{.data['tls\.key']}" | base64 -d) > $TLS_DIR/certs/$CERT_DIR/tls.key
    ROUTE_CERT="${TLS_DIR}/certs/${CERT_DIR}/tls.crt"
else
    # Create custom certs
    ${PROJECT_ROOT}/test/scripts/openshift-ci/tls/generate-custom-certs.sh
    # Generate secret to store the custom certs (already cleaned up in idempotency section above)
    oc create secret generic $SECRET_NAME --from-file=${TLS_DIR}/certs/custom/root.crt  --from-file=${TLS_DIR}/certs/custom/custom.crt --from-file=${TLS_DIR}/certs/custom/custom.key -n kserve
    # Patch deployment to mount certificates and enable TLS
    oc patch deployment $DEPLOYMENT_NAME -n kserve -p "{\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"name\":\"$DEPLOYMENT_NAME\",\"args\":[\"mini\",\"-dir=/data\",\"-s3\",\"-s3.cert.file=/certs/custom.crt\",\"-s3.key.file=/certs/custom.key\"],\"volumeMounts\":[{\"mountPath\":\"/certs\",\"name\":\"$SECRET_NAME\",\"readOnly\":true},{\"mountPath\":\"/data\",\"name\":\"data\"}]}], \"volumes\":[{\"name\":\"$SECRET_NAME\",\"projected\":{\"defaultMode\":420,\"sources\":[{\"secret\":{\"name\":\"$SECRET_NAME\",\"items\":[{\"key\":\"custom.crt\",\"path\":\"custom.crt\"},{\"key\":\"custom.key\", \"path\":\"custom.key\"},{\"key\":\"root.crt\",\"path\":\"root.crt\"}]}}]}}]}}}}"

    # Wait for patched deployment to be ready
    echo "Waiting for patched $DEPLOYMENT_NAME deployment to be ready..."
    oc rollout status deployment/$DEPLOYMENT_NAME -n kserve --timeout=300s
    ROUTE_CERT="${TLS_DIR}/certs/custom/root.crt"
fi

# Upload the model using the S3 init job from config/overlays/test/s3-local-backend
S3_INIT_JOB_NAME="s3-tls-init-${CERT_TYPE}"
oc delete job "${S3_INIT_JOB_NAME}" -n kserve --ignore-not-found
sed -e "s/name: s3-init/name: ${S3_INIT_JOB_NAME}/" \
    -e "s|http://s3-service.kserve:8333|https://${SERVICE_NAME}.kserve.svc:8333|" \
    -e "s/mlpipeline-s3-artifact/${DEPLOYMENT_NAME}-artifact/" \
    -e "s|aws s3 mb s3://example-models|aws s3 mb s3://example-models --no-verify-ssl|" \
    "${PROJECT_ROOT}/config/overlays/test/s3-local-backend/seaweedfs-init-job.yaml" | \
  oc apply -n kserve -f -

echo "Waiting for S3 TLS init job to complete..."
if ! oc wait --for=condition=complete "job/${S3_INIT_JOB_NAME}" -n kserve --timeout=300s; then
  echo "S3 TLS init job failed. Pod status and logs:"
  oc get pods -l "job-name=${S3_INIT_JOB_NAME}" -n kserve
  oc logs -l "job-name=${S3_INIT_JOB_NAME}" -n kserve --tail=50 || true
  exit 1
fi