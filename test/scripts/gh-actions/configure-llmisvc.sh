#!/bin/bash

# Copyright 2024 The KServe Authors.
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

# The script will configure the LLM controller with custom image and webhooks in the GH Actions environment.
# Usage: configure-llmisvc.sh

set -o errexit
set -o nounset
set -o pipefail

echo "🔧 Configuring LLM controller with custom image and webhooks..."

# Check required environment variable
if [ -z "${LLMISVC_CONTROLLER_IMG_TAG:-}" ]; then
    echo "❌ LLMISVC_CONTROLLER_IMG_TAG environment variable is required"
    exit 1
fi

echo "🔍 Verifying LLM controller image is available..."
# Extract just the image name and tag for verification
LLMISVC_IMG_NAME=$(echo "$LLMISVC_CONTROLLER_IMG_TAG" | cut -d':' -f1)
LLMISVC_IMG_TAG=$(echo "$LLMISVC_CONTROLLER_IMG_TAG" | cut -d':' -f2)

echo "Looking for image: $LLMISVC_IMG_NAME with tag: $LLMISVC_IMG_TAG"
if ! docker images --format "{{.Repository}}:{{.Tag}}" | grep -q "^${LLMISVC_IMG_NAME}:${LLMISVC_IMG_TAG}$"; then
    echo "❌ LLM controller image $LLMISVC_CONTROLLER_IMG_TAG not found!"
    echo "All available images:"
    docker images --format "table {{.Repository}}\t{{.Tag}}\t{{.ID}}" | head -15
    exit 1
fi
echo "✅ LLM controller image $LLMISVC_CONTROLLER_IMG_TAG is available"

echo "🔧 Patching LLM controller deployment with correct image tag and volume mount..."
kubectl patch deployment kserve-llmisvc-controller-manager -n kserve --type='merge' -p="{
  \"spec\": {
    \"template\": {
      \"spec\": {
        \"containers\": [{
          \"name\": \"manager\",
          \"image\": \"$LLMISVC_CONTROLLER_IMG_TAG\",
          \"imagePullPolicy\": \"Never\",
          \"volumeMounts\": [{
            \"mountPath\": \"/tmp/k8s-webhook-server/serving-certs\",
            \"name\": \"cert\",
            \"readOnly\": true
          }]
        }]
      }
    }
  }
}"

echo "📋 Waiting for LLM controller deployment rollout..."
kubectl rollout status deployment/kserve-llmisvc-controller-manager -n kserve --timeout=120s

echo "🔧 Updating webhook configurations to point to LLM controller..."
kubectl patch validatingwebhookconfiguration llminferenceserviceconfig.serving.kserve.io --type='json' -p='[{"op": "replace", "path": "/webhooks/0/clientConfig/service/name", "value": "kserve-llmisvc-controller-manager-service"}]'
kubectl patch validatingwebhookconfiguration llminferenceservice.serving.kserve.io --type='json' -p='[{"op": "replace", "path": "/webhooks/0/clientConfig/service/name", "value": "kserve-llmisvc-controller-manager-service"}]'

echo "🔧 Updating webhook CA bundles..."
LLM_CA_BUNDLE=$(kubectl get secret kserve-llmisvc-webhook-server-cert -n kserve -o jsonpath='{.data.ca\.crt}')
kubectl get validatingwebhookconfiguration llminferenceserviceconfig.serving.kserve.io -o json | jq --arg ca_bundle "$LLM_CA_BUNDLE" '.webhooks[0].clientConfig.caBundle = $ca_bundle' | kubectl replace -f -
kubectl get validatingwebhookconfiguration llminferenceservice.serving.kserve.io -o json | jq --arg ca_bundle "$LLM_CA_BUNDLE" '.webhooks[0].clientConfig.caBundle = $ca_bundle' | kubectl replace -f -

echo "✅ LLM controller configuration complete!"
echo "📋 Controllers running:"
kubectl get pods -n kserve -l control-plane
