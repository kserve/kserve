#!/bin/bash

set -eo pipefail
############################################################
# Help                                                     #
############################################################
Help() {
   # Display Help
   echo "KServe quick install script."
   echo
   echo "Syntax: [-s|-r]"
   echo "options:"
   echo "s Serverless Mode."
   echo "r RawDeployment Mode."
   echo "u Uninstall."
   echo "d Install only dependencies."
   echo "k Install KEDA."
   echo
}

export ISTIO_VERSION=1.23.2
export KNATIVE_OPERATOR_VERSION=v1.15.7
export KNATIVE_SERVING_VERSION=1.15.2
export KSERVE_VERSION=v0.15.2
export CERT_MANAGER_VERSION=v1.16.1
export GATEWAY_API_VERSION=v1.2.1
export KEDA_VERSION=2.14.0
SCRIPT_DIR="$(dirname -- "${BASH_SOURCE[0]}")"
export SCRIPT_DIR

uninstall() {
   helm uninstall --ignore-not-found istio-ingressgateway -n istio-system
   helm uninstall --ignore-not-found istiod -n istio-system
   helm uninstall --ignore-not-found istio-base -n istio-system
   echo "😀 Successfully uninstalled Istio"

   helm uninstall --ignore-not-found cert-manager -n cert-manager
   echo "😀 Successfully uninstalled Cert Manager"

   helm uninstall --ignore-not-found keda -n keda
   echo "😀 Successfully uninstalled KEDA"
   
   kubectl delete --ignore-not-found=true KnativeServing knative-serving -n knative-serving --wait=True --timeout=300s
   helm uninstall --ignore-not-found knative-operator -n knative-serving
   echo "😀 Successfully uninstalled Knative"

   helm uninstall --ignore-not-found kserve -n kserve
   helm uninstall --ignore-not-found kserve-crd -n kserve
   echo "😀 Successfully uninstalled KServe"

   kubectl delete --ignore-not-found=true namespace istio-system
   kubectl delete --ignore-not-found=true namespace cert-manager
   kubectl delete --ignore-not-found=true namespace kserve
}

# Check if helm command is available
if ! command -v helm &>/dev/null; then
   echo "😱 Helm command not found. Please install Helm."
   exit 1
fi

deploymentMode="Serverless"
installKeda=false
while getopts ":hsrudk" option; do
   case $option in
   h) # display Help
      Help
      exit
      ;;
   r) # skip knative install
      deploymentMode="RawDeployment" ;;
   s) # install knative
      deploymentMode="Serverless" ;;
   u) # uninstall
      uninstall
      exit
      ;;
   d) # install only dependencies
      installKserve=false ;;
   k) # install KEDA
      installKeda=true ;;
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

echo "Installing Gateway API CRDs ..."
kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/${GATEWAY_API_VERSION}/standard-install.yaml

helm repo add istio https://istio-release.storage.googleapis.com/charts --force-update
helm install istio-base istio/base -n istio-system --wait --set defaultRevision=default --create-namespace --version ${ISTIO_VERSION}
helm install istiod istio/istiod -n istio-system --wait --version ${ISTIO_VERSION} \
   --set proxy.autoInject=disabled \
   --set-string pilot.podAnnotations."cluster-autoscaler\.kubernetes\.io/safe-to-evict"=true
helm install istio-ingressgateway istio/gateway -n istio-system --version ${ISTIO_VERSION} \
   --set-string podAnnotations."cluster-autoscaler\.kubernetes\.io/safe-to-evict"=true

# Wait for the istio ingressgateway pod to be created
sleep 10
# Wait for istio ingressgateway to be ready
kubectl wait --for=condition=Ready pod -l app=istio-ingressgateway -n istio-system --timeout=600s
echo "😀 Successfully installed Istio"

# Install Cert Manager
helm repo add jetstack https://charts.jetstack.io --force-update
helm install \
   cert-manager jetstack/cert-manager \
   --namespace cert-manager \
   --create-namespace \
   --version ${CERT_MANAGER_VERSION} \
   --set crds.enabled=true
echo "😀 Successfully installed Cert Manager"

if [ $installKeda = true ]; then
   #Install KEDA
   helm repo add kedacore https://kedacore.github.io/charts
   helm install keda kedacore/keda --version ${KEDA_VERSION} --namespace keda --create-namespace --wait
   echo "😀 Successfully installed KEDA"

   kubectl apply -f https://github.com/open-telemetry/opentelemetry-operator/releases/latest/download/opentelemetry-operator.yaml
   
   helm upgrade -i kedify-otel oci://ghcr.io/kedify/charts/otel-add-on --version=v0.0.6 --namespace keda --wait --set validatingAdmissionPolicy.enabled=false
   echo "😀 Successfully installed KEDA"
fi


# Install Knative
if [ "${deploymentMode}" = "Serverless" ]; then
   helm install knative-operator --namespace knative-serving --create-namespace --wait \
      https://github.com/knative/operator/releases/download/knative-${KNATIVE_OPERATOR_VERSION}/knative-operator-${KNATIVE_OPERATOR_VERSION}.tgz
   kubectl apply -f - <<EOF
   apiVersion: operator.knative.dev/v1beta1
   kind: KnativeServing
   metadata:
     name: knative-serving
     namespace: knative-serving
   spec:
     version: "${KNATIVE_SERVING_VERSION}"
     config:
       domain:
         # Patch the external domain as the default domain svc.cluster.local is not exposed on ingress (from knative 1.8)
         example.com: ""
EOF
   echo "😀 Successfully installed Knative"
fi

if [ "${installKserve}" = false ]; then
   exit
fi
# Install KServe
helm install kserve-crd oci://ghcr.io/kserve/charts/kserve-crd --version ${KSERVE_VERSION} --namespace kserve --create-namespace --wait
helm install kserve oci://ghcr.io/kserve/charts/kserve --version ${KSERVE_VERSION} --namespace kserve --create-namespace --wait \
   --set-string kserve.controller.deploymentMode="${deploymentMode}"
echo "😀 Successfully installed KServe"
