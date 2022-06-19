set -e
############################################################
# Help                                                     #
############################################################
Help()
{
   # Display Help
   echo "KServe quick install script."
   echo
   echo "Syntax: [-s|-r]"
   echo "options:"
   echo "s Serverless Mode."
   echo "r RawDeployment Mode."
   echo
}

deploymentMode=serverless
while getopts ":hsr" option; do
   case $option in
      h) # display Help
         Help
         exit;;
      r) # skip knative install
	 deploymentMode=kubernetes;;
      s) # install knative
         deploymentMode=serverless;;
     \?) # Invalid option
         echo "Error: Invalid option"
         exit;;
   esac
done

export ISTIO_VERSION=1.14.0
export KNATIVE_VERSION=knative-v1.4.0
export KSERVE_VERSION=v0.9.0-rc0
export CERT_MANAGER_VERSION=v1.3.0
export SCRIPT_DIR="$( dirname -- "${BASH_SOURCE[0]}" )"

KUBE_VERSION=$(kubectl version --short=true)
if [ ${KUBE_VERSION:43:2} -lt 22 ];
then
   echo "😱 install requires at least Kubernetes 1.22";
   exit 1;
fi

curl -L https://istio.io/downloadIstio | sh -
cd istio-${ISTIO_VERSION}

# Create istio-system namespace
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Namespace
metadata:
  name: istio-system
  labels:
    istio-injection: disabled
EOF

cat << EOF > ./istio-minimal-operator.yaml
apiVersion: install.istio.io/v1beta1
kind: IstioOperator
spec:
  values:
    global:
      proxy:
        autoInject: disabled
      useMCP: false

  meshConfig:
    accessLogFile: /dev/stdout

  addonComponents:
    pilot:
      enabled: true

  components:
    ingressGateways:
      - name: istio-ingressgateway
        enabled: true
EOF

bin/istioctl manifest apply -f istio-minimal-operator.yaml -y;

echo "😀 Successfully installed Istio"

# Install Knative
if [ $deploymentMode = serverless ]; then
   kubectl apply --filename https://github.com/knative/serving/releases/download/${KNATIVE_VERSION}/serving-crds.yaml
   kubectl apply --filename https://github.com/knative/serving/releases/download/${KNATIVE_VERSION}/serving-core.yaml
   kubectl apply --filename https://github.com/knative/net-istio/releases/download/${KNATIVE_VERSION}/release.yaml
   echo "😀 Successfully installed Knative"
fi

# Install Cert Manager
kubectl apply --validate=false -f https://github.com/jetstack/cert-manager/releases/download/${CERT_MANAGER_VERSION}/cert-manager.yaml
kubectl wait --for=condition=available --timeout=600s deployment/cert-manager-webhook -n cert-manager
cd ..
echo "😀 Successfully installed Cert Manager"

# Install KServe
KSERVE_CONFIG=kfserving.yaml
if [ ${KSERVE_VERSION:3:1} -gt 6 ]; then KSERVE_CONFIG=kserve.yaml; fi

# Retry inorder to handle that it may take a minute or so for the TLS assets required for the webhook to function to be provisioned
kubectl apply -f https://github.com/kserve/kserve/releases/download/${KSERVE_VERSION}/${KSERVE_CONFIG}

# Install KServe built-in servingruntimes
kubectl wait --for=condition=ready pod -l control-plane=kserve-controller-manager -n kserve --timeout=300s
kubectl apply -f https://github.com/kserve/kserve/releases/download/${KSERVE_VERSION}/kserve-runtimes.yaml
echo "😀 Successfully installed KServe"

# Clean up
rm -rf istio-${ISTIO_VERSION}
