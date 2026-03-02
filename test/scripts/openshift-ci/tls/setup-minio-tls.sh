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
# for minio storage to have tls enabled with either an openshift serving certificate
# or a custom certificate.
# Usage: setup-minio-tls.sh [serving|custom]
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
    DEPLOYMENT_NAME="minio-tls-serving"
    APP_LABEL="minio-tls-serving"
    KUSTOMIZE_PATH="minio-tls-serving-cert"
    SECRET_NAME="minio-tls-serving"
    SERVICE_NAME="minio-tls-serving-service"
    ROUTE_NAME="minio-tls-serving-service"
    MC_ALIAS="storage-tls-serving"
    STORAGE_CONFIG_KEY="localTLSMinIOServing"
    CERT_DIR="serving"
    CERT_DESC="Openshift serving certificate"
else
    DEPLOYMENT_NAME="minio-tls-custom"
    APP_LABEL="minio-tls-custom"
    KUSTOMIZE_PATH="minio-tls-custom-cert"
    SECRET_NAME="minio-tls-custom"
    SERVICE_NAME="minio-tls-custom-service"
    ROUTE_NAME="minio-tls-custom-service"
    MC_ALIAS="storage-tls-custom"
    STORAGE_CONFIG_KEY="localTLSMinIOCustom"
    CERT_DIR="custom"
    CERT_DESC="custom certificate"
fi

# If Kustomize is not installed, install it
if ! command -v kustomize &>/dev/null; then
  echo "Installing Kustomize"
  curl -s "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh" | bash -s -- 5.0.1 $HOME/.local/bin
fi

# If minio CLI is not installed, install it
if ! command -v mc &>/dev/null; then
  echo "Installing MinIO CLI"
  curl https://dl.min.io/client/mc/release/linux-amd64/mc --create-dirs -o $HOME/.local/bin/mc
  chmod +x $HOME/.local/bin/mc
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
echo "Cleaning up existing $CERT_TYPE TLS MinIO resources for idempotency..."
# Delete route if it exists (from previous run)
oc delete route $ROUTE_NAME -n kserve --ignore-not-found || true
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

# Create tls minio resources
kustomize build $PROJECT_ROOT/test/overlays/openshift-ci/$KUSTOMIZE_PATH |
  oc apply -n kserve --server-side=true -f - 

# Wait for minio deployment to be ready
echo "Waiting for $DEPLOYMENT_NAME deployment to be ready..."
oc rollout status deployment/$DEPLOYMENT_NAME -n kserve --timeout=300s

echo "Configuring MinIO for TLS with $CERT_DESC and adding models to storage"
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
    # Mount certificates to minio-tls-custom container
    oc patch deployment $DEPLOYMENT_NAME -n kserve -p "{\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"name\":\"$DEPLOYMENT_NAME\",\"volumeMounts\":[{\"mountPath\":\".minio/certs\",\"name\":\"$SECRET_NAME\"}]}], \"volumes\":[{\"name\":\"$SECRET_NAME\",\"projected\":{\"defaultMode\":420,\"sources\":[{\"secret\":{\"name\":\"$SECRET_NAME\",\"items\":[{\"key\":\"custom.crt\",\"path\":\"public.crt\"},{\"key\":\"custom.key\", \"path\":\"private.key\"},{\"key\":\"root.crt\",\"path\":\"CAs/root.crt\"}]}}]}}]}}}}"
    
    # Wait for patched deployment to be ready
    echo "Waiting for patched $DEPLOYMENT_NAME deployment to be ready..."
    oc rollout status deployment/$DEPLOYMENT_NAME -n kserve --timeout=300s
    ROUTE_CERT="${TLS_DIR}/certs/custom/root.crt"
fi

# Expose the route with tls enabled
oc create route reencrypt $ROUTE_NAME \
  --service=$SERVICE_NAME \
  --dest-ca-cert="${ROUTE_CERT}" \
  -n kserve && sleep 5
MINIO_TLS_ROUTE=$(oc get routes -n kserve $ROUTE_NAME -o jsonpath="{.spec.host}")

# Wait for minio TLS endpoint to be accessible
echo "Waiting for minio TLS $CERT_TYPE endpoint to be accessible..."
timeout=60
counter=0
while [ $counter -lt $timeout ]; do
  if curl -f -s -k "https://$MINIO_TLS_ROUTE/minio/health/live" >/dev/null 2>&1; then
    echo "Minio TLS $CERT_TYPE is ready!"
    break
  fi
  echo "Waiting for minio TLS $CERT_TYPE to be ready... ($counter/$timeout)"
  sleep 2
  counter=$((counter + 2))
done

if [ $counter -ge $timeout ]; then
  echo "Timeout waiting for minio TLS $CERT_TYPE to be ready"
  exit 1
fi

# Upload the model
mc alias set $MC_ALIAS https://$MINIO_TLS_ROUTE minio minio123 --insecure
if ! mc ls $MC_ALIAS/example-models --insecure >/dev/null 2>&1; then
  mc mb $MC_ALIAS/example-models --insecure
else
  echo "Bucket 'example-models' already exists."
fi
if [[ $(mc ls $MC_ALIAS/example-models/sklearn/model.joblib --insecure |wc -l) == "1" ]]; then
  echo "Test model exists"
else
  echo "Copy test model"
  curl -s -L https://storage.googleapis.com/kfserving-examples/models/sklearn/1.0/model/model.joblib -o /tmp/sklearn-model.joblib
  mc cp /tmp/sklearn-model.joblib $MC_ALIAS/example-models/sklearn/model.joblib --insecure
fi
# Delete the route after upload
oc delete route -n kserve $ROUTE_NAME

