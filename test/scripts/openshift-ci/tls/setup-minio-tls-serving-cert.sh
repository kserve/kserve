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
# for minio storage to have tls enabled with an openshift serving certificate.
set -o errexit
set -o nounset
set -o pipefail

MY_PATH=$(dirname "$0")
PROJECT_ROOT=$MY_PATH/../../../../
TLS_DIR=$PROJECT_ROOT/test/scripts/openshift-ci/tls

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

# Create tls minio resources
kustomize build $PROJECT_ROOT/test/overlays/openshift-ci/minio-tls-serving-cert |
  oc apply -n kserve --server-side=true -f - && sleep 5

echo "Configuring MinIO for TLS with Openshift serving certificate and adding models to storage"
# Add openshift generated serving certificates to certs directory
if ! [ -d $TLS_DIR/certs/serving ]; then
    mkdir -p $TLS_DIR/certs/serving
fi
(oc get secret minio-tls-serving -n kserve -o jsonpath="{.data['tls\.crt']}" | base64 -d) > $TLS_DIR/certs/serving/tls.crt
(oc get secret minio-tls-serving -n kserve -o jsonpath="{.data['tls\.key']}" | base64 -d) > $TLS_DIR/certs/serving/tls.key
# Expose the route with tls enabled
oc create route reencrypt minio-tls-serving-service \
  --service=minio-tls-serving-service \
  --dest-ca-cert="${TLS_DIR}/certs/serving/tls.crt" \
  -n kserve && sleep 5
MINIO_TLS_SERVING_ROUTE=$(oc get routes -n kserve minio-tls-serving-service -o jsonpath="{.spec.host}")
# Upload the model
mc alias set storage-tls-serving https://$MINIO_TLS_SERVING_ROUTE minio minio123 --insecure
if ! mc ls storage-tls-serving/example-models --insecure >/dev/null 2>&1; then
  mc mb storage-tls-serving/example-models --insecure
else
  echo "Bucket 'example-models' already exists."
fi
if [[ $(mc ls storage-tls-serving/example-models/sklearn/model.joblib --insecure |wc -l) == "1" ]]; then
  echo "Test model exists"
else
  echo "Copy test model"
  curl -L https://storage.googleapis.com/kfserving-examples/models/sklearn/1.0/model/model.joblib -o /tmp/sklearn-model.joblib
  mc cp /tmp/sklearn-model.joblib storage-tls-serving/example-models/sklearn/model.joblib --insecure
fi
# Delete the route after upload
oc delete route -n kserve minio-tls-serving-service

# Create kserve-ci-e2e-test namespace if it does not already exist
if oc get namespace kserve-ci-e2e-test > /dev/null 2>&1; then
    echo "Namespace kserve-ci-e2e-test exists."
else
    cat <<EOF | oc apply -f -
apiVersion: v1
kind: Namespace
metadata:
    name: kserve-ci-e2e-test
EOF
fi

echo "Adding localTLSMinIOServing configuration to storage-config secret"
# Creating/Updating storage-config secret with ca created ca bundle
LOCAL_TLS_MINIO_SERVING="{\"type\": \"s3\",\"access_key_id\":\"minio\",\"secret_access_key\":\"minio123\",\"endpoint_url\":\"https://minio-tls-serving-service.kserve.svc:9000\",\"bucket\":\"mlpipeline\",\"region\":\"us-south\",\"cabundle_configmap\":\"odh-kserve-custom-ca-bundle\",\"anonymous\":\"False\"}" 
LOCAL_TLS_MINIO_SERVING_BASE64=$(echo ${LOCAL_TLS_MINIO_SERVING} | base64 -w 0)
if oc get secret storage-config -n kserve-ci-e2e-test > /dev/null 2>&1; then
    oc patch secret storage-config -n kserve-ci-e2e-test -p "{\"data\":{\"localTLSMinIOServing\":\"${LOCAL_TLS_MINIO_SERVING_BASE64}\"}}"
else
    oc create secret generic storage-config --from-literal=localTLSMinIOServing="${LOCAL_TLS_MINIO_SERVING}" -n kserve-ci-e2e-test
fi