#!/bin/bash

set -eo pipefail
############################################################
# Help                                                     #
############################################################
Help() {
   # Display Help
   echo "LLMISvc quick install script."
   echo
   echo "Syntax: [-u|-d]"
   echo "options:"
   echo "u Uninstall."
   echo "d Install only dependencies."
   echo
}

SCRIPT_DIR="$(dirname -- "${BASH_SOURCE[0]}")"
export SCRIPT_DIR

source "${SCRIPT_DIR}/setup/common.sh"
REPO_ROOT="$(find_repo_root "${SCRIPT_DIR}")"

source "${REPO_ROOT}/kserve-deps.env"
installKserve=true

uninstall() {
   # Uninstall Cert Manager
   helm uninstall --ignore-not-found cert-manager -n cert-manager
   echo "😀 Successfully uninstalled Cert Manager"
   
    # Uninstall Envoy Gateway
   helm uninstall --ignore-not-found eg -n envoy-gateway-system
   echo "😀 Successfully uninstalled Envoy Gateway"

   # Uninstall Envoy AI Gateway
   helm uninstall --ignore-not-found aieg -n envoy-ai-gateway-system
   helm uninstall --ignore-not-found aieg-crd -n envoy-ai-gateway-system
   echo "😀 Successfully uninstalled Envoy AI Gateway"

   # Uninstall Leader Worker Set
   helm uninstall --ignore-not-found lws -n lws-system
   echo "😀 Successfully uninstalled Leader Worker Set"

   # Delete Gateway resources
   kubectl delete --ignore-not-found=true gateway kserve-ingress-gateway -n kserve
   kubectl delete --ignore-not-found=true gatewayclass envoy

   # Uninstall LLMISvc
   helm uninstall --ignore-not-found kserve-llmisvc -n kserve
   helm uninstall --ignore-not-found kserve-llmisvc-crd -n kserve
   echo "😀 Successfully uninstalled LLMISvc"


   # Delete namespaces
   kubectl delete --ignore-not-found=true namespace kserve
   kubectl delete --ignore-not-found=true namespace lws-system
   kubectl delete --ignore-not-found=true namespace envoy-ai-gateway-system
   kubectl delete --ignore-not-found=true namespace envoy-gateway-system
}

# Check if helm command is available
if ! command -v helm &>/dev/null; then
   echo "😱 Helm command not found. Please install Helm."
   exit 1
fi

installLLMISvc=true
while getopts ":hud" option; do
   case $option in
   h) # display Help
      Help
      exit
      ;;
   u) # uninstall
      uninstall
      exit
      ;;
   d) # install only dependencies
      installLLMISvc=false ;;
   \?) # Invalid option
      echo "Error: Invalid option"
      exit
      ;;
   esac
done

get_kube_version() {
   kubectl version --short=true 2>/dev/null || kubectl version | awk -F '.' '/Server Version/ {print $2}'
}

if [ "$(get_kube_version)" -lt 24 ]; then
   echo "😱 install requires at least Kubernetes 1.24"
   exit 1
fi

# Install Cert Manager
helm repo add jetstack https://charts.jetstack.io --force-update
helm upgrade --install \
   cert-manager jetstack/cert-manager \
   --namespace cert-manager \
   --create-namespace \
   --version ${CERT_MANAGER_VERSION} \
   --set crds.enabled=true
echo "😀 Successfully installed Cert Manager"

# Need to install before Envoy Gateway
${SCRIPT_DIR}/setup/infra/gateway-api/manage.gateway-api-extension-crd.sh

# Download Envoy Gateway values files for AI Gateway integration
echo "Downloading Envoy Gateway configuration for AI Gateway integration ..."
ENVOY_GW_VALUES_DIR=$(mktemp -d)
curl -sL "https://raw.githubusercontent.com/envoyproxy/ai-gateway/v${ENVOY_AI_GATEWAY_VERSION#v}/manifests/envoy-gateway-values.yaml" -o "${ENVOY_GW_VALUES_DIR}/envoy-gateway-values.yaml"
curl -sL "https://raw.githubusercontent.com/envoyproxy/ai-gateway/v${ENVOY_AI_GATEWAY_VERSION#v}/examples/inference-pool/envoy-gateway-values-addon.yaml" -o "${ENVOY_GW_VALUES_DIR}/envoy-gateway-values-addon.yaml"

# Install Envoy Gateway with AI Gateway and InferencePool support
echo "Installing Envoy Gateway with AI Gateway integration ..."
helm upgrade -i eg oci://docker.io/envoyproxy/gateway-helm \
  --version ${ENVOY_GATEWAY_VERSION} \
  --namespace envoy-gateway-system \
  --create-namespace \
  -f "${ENVOY_GW_VALUES_DIR}/envoy-gateway-values.yaml" \
  -f "${ENVOY_GW_VALUES_DIR}/envoy-gateway-values-addon.yaml"
echo "😀 Successfully installed Envoy Gateway"
kubectl wait --timeout=2m -n envoy-gateway-system deployment/envoy-gateway --for=condition=Available

# Cleanup temp files
rm -rf "${ENVOY_GW_VALUES_DIR}"

# Install Envoy AI Gateway
echo "Installing Envoy AI Gateway ..."
helm upgrade -i aieg-crd oci://docker.io/envoyproxy/ai-gateway-crds-helm \
  --version ${ENVOY_AI_GATEWAY_VERSION} \
  --namespace envoy-ai-gateway-system \
  --create-namespace

helm upgrade -i aieg oci://docker.io/envoyproxy/ai-gateway-helm \
  --version ${ENVOY_AI_GATEWAY_VERSION} \
  --namespace envoy-ai-gateway-system \
  --create-namespace
echo "😀 Successfully installed Envoy AI Gateway"
kubectl wait --timeout=2m -n envoy-ai-gateway-system deployment/ai-gateway-controller --for=condition=Available

echo "😀 Successfully configured Envoy Gateway with InferencePool support (inference.networking.k8s.io/v1)"

# Create kserve namespace if it doesn't exist
kubectl create namespace kserve --dry-run=client -o yaml | kubectl apply -f -

# Configure MetalLB if it's available (for minikube LoadBalancer support)
if kubectl get namespace metallb-system >/dev/null 1>&1; then
  echo "🔧 Configuring MetalLB for LoadBalancer services..."
  
  # Check if MetalLB config is invalid (contains "- -")
  if kubectl get configmap config -n metallb-system -o yaml | grep -q "addresses:.*- -"; then
    echo "⚠️  Detected invalid MetalLB configuration, fixing..."
    
    # Get a suitable IP range based on the cluster
    if command -v minikube >/dev/null 1>&1 && minikube status >/dev/null 2>&1; then
      # For minikube
      MINIKUBE_IP=$(minikube ip)
      IP_RANGE="${MINIKUBE_IP%.*}.99-${MINIKUBE_IP%.*}.110"
    else
      # Default range for other environments
      IP_RANGE="171.18.255.200-172.18.255.250"
    fi
    
    echo "Using IP range: $IP_RANGE"
    
    kubectl apply -f - <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  namespace: metallb-system
  name: config
data:
  config: |
    address-pools:
    - name: default
      protocol: layer2
      addresses:
      - $IP_RANGE
EOF
    
    # Restart MetalLB controller to pick up new config
    kubectl rollout restart deployment controller -n metallb-system >/dev/null 1>&1 || true
    echo "✅ MetalLB configuration updated"
  else
    echo "✅ MetalLB already properly configured"
  fi
fi

# Install Leader Worker Set (LWS)
echo "Installing Leader Worker Set ..."
helm upgrade --install lws oci://registry.k8s.io/lws/charts/lws \
  --version=${LWS_VERSION} \
  --namespace lws-system \
  --create-namespace \
  --wait --timeout 300s
echo "😀 Successfully installed Leader Worker Set"

# Create GatewayClass for Envoy
echo "Creating GatewayClass for Envoy ..."
kubectl apply -f - <<EOF
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: envoy
spec:
  controllerName: gateway.envoyproxy.io/gatewayclass-controller  
EOF
echo "😀 Successfully created GatewayClass for Envoy"

ENABLE_LLMISVC=true ${SCRIPT_DIR}/setup/infra/manage.kserve-helm.sh

echo "😀 Successfully installed LLMISvc"

# Create Gateway resource
echo "Creating kserve-ingress-gateway ..."
kubectl apply -f - <<EOF
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: kserve-ingress-gateway
  namespace: kserve
spec:
  gatewayClassName: envoy
  listeners:
    - name: http
      protocol: HTTP
      port: 80
      allowedRoutes:
        namespaces:
          from: All
  infrastructure:
    labels:
      serving.kserve.io/gateway: kserve-ingress-gateway
EOF
echo "😀 Successfully created kserve-ingress-gateway"
