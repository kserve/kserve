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

echo "🔧 Fixing remaining template variables in webhooks..."
# Fix the namespace template variable in both webhook configurations
kubectl patch validatingwebhookconfiguration llminferenceserviceconfig.serving.kserve.io --type='json' -p='[{"op": "replace", "path": "/webhooks/0/clientConfig/service/namespace", "value": "kserve"}]'
kubectl patch validatingwebhookconfiguration llminferenceservice.serving.kserve.io --type='json' -p='[{"op": "replace", "path": "/webhooks/0/clientConfig/service/namespace", "value": "kserve"}]'
echo "✅ All template variables fixed!"

echo "🔧 Updating webhook CA bundles..."
LLM_CA_BUNDLE=$(kubectl get secret kserve-llmisvc-webhook-server-cert -n kserve -o jsonpath='{.data.ca\.crt}')
kubectl get validatingwebhookconfiguration llminferenceserviceconfig.serving.kserve.io -o json | jq --arg ca_bundle "$LLM_CA_BUNDLE" '.webhooks[0].clientConfig.caBundle = $ca_bundle' | kubectl replace -f -
kubectl get validatingwebhookconfiguration llminferenceservice.serving.kserve.io -o json | jq --arg ca_bundle "$LLM_CA_BUNDLE" '.webhooks[0].clientConfig.caBundle = $ca_bundle' | kubectl replace -f -

echo "🔧 Setting up EPP (Endpoint Picker Plugin) controller for InferencePool management..."
# Create RBAC for EPP controller
kubectl apply -f - << 'EOF'
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: epp-controller
rules:
- apiGroups: ["inference.networking.x-k8s.io"]
  resources: ["inferencepools", "inferencemodels"]
  verbs: ["get", "watch", "list", "update", "patch"]
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get", "watch", "list"]
- apiGroups: ["authentication.k8s.io"]
  resources: ["tokenreviews"]
  verbs: ["create"]
- apiGroups: ["authorization.k8s.io"]
  resources: ["subjectaccessreviews"]
  verbs: ["create"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: epp-controller
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: epp-controller
subjects:
- kind: ServiceAccount
  name: default
  namespace: kserve-ci-e2e-test
EOF

echo "🔧 Setting up EPP controller monitoring for InferencePools..."
# Create a background script to monitor and create EPP controllers as needed
cat > /tmp/epp-monitor.sh << 'EOF'
#!/bin/bash
while true; do
  # Check for InferencePools that need EPP controllers
  for pool in $(kubectl get inferencepools -A -o jsonpath='{range .items[*]}{.metadata.namespace}/{.metadata.name}{"\n"}{end}'); do
    namespace=$(echo $pool | cut -d'/' -f1)
    name=$(echo $pool | cut -d'/' -f2)
    epp_name="${name}-epp"
    
    # Check if EPP service already exists
    if ! kubectl get service "$epp_name-service" -n "$namespace" >/dev/null 2>&1; then
      echo "Creating EPP controller for InferencePool $namespace/$name"
      
      # Create EPP service
      kubectl apply -f - << EOFINNER
apiVersion: v1
kind: Service
metadata:
  name: ${epp_name}-service
  namespace: $namespace
spec:
  selector:
    app: ${epp_name}
  ports:
    - protocol: TCP
      port: 9002
      targetPort: 9002
      appProtocol: http2
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ${epp_name}
  namespace: $namespace
spec:
  replicas: 1
  selector:
    matchLabels:
      app: ${epp_name}
  template:
    metadata:
      labels:
        app: ${epp_name}
    spec:
      containers:
      - name: epp
        image: registry.k8s.io/gateway-api-inference-extension/epp:v0.5.0
        args:
          - --poolName=${name}
          - --poolNamespace=${namespace}
          - --v=4
        ports:
          - containerPort: 9002
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
        livenessProbe:
          grpc:
            port: 9002
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          grpc:
            port: 9002
          initialDelaySeconds: 5
          periodSeconds: 5
EOFINNER
    fi
  done
  sleep 10
done
EOF

chmod +x /tmp/epp-monitor.sh
# Start the monitor in the background
/tmp/epp-monitor.sh &
echo "✅ EPP controller monitoring started"

echo "✅ LLM controller configuration complete!"
echo "📋 Controllers running:"
kubectl get pods -n kserve -l control-plane
