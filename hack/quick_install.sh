set -eo pipefail
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

export ISTIO_VERSION=1.20.4
export ISTIO_DIR=istio-${ISTIO_VERSION}
export KNATIVE_SERVING_VERSION=knative-v1.13.1
export KNATIVE_ISTIO_VERSION=knative-v1.13.1
export KSERVE_VERSION=v0.13.0
export CERT_MANAGER_VERSION=v1.9.0
export SCRIPT_DIR="$( dirname -- "${BASH_SOURCE[0]}" )"

cleanup(){
  rm -rf deploy-config-patch.yaml
}
trap cleanup EXIT

get_kube_version(){
    kubectl version --short=true 2>/dev/null || kubectl version | awk -F '.' '/Server Version/ {print $2}'
}

if [ $(get_kube_version) -lt 24 ];
then
   echo "😱 install requires at least Kubernetes 1.24";
   exit 1;
fi

if [ -d ${ISTIO_DIR} ]; then
  echo "Already downloaded ${ISTIO_DIR}"
else
  curl -L https://istio.io/downloadIstio | sh -
fi
pushd ${ISTIO_DIR} >> /dev/null

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

  meshConfig:
    accessLogFile: /dev/stdout

  components:
    ingressGateways:
      - name: istio-ingressgateway
        enabled: true
        k8s:
          podAnnotations:
            cluster-autoscaler.kubernetes.io/safe-to-evict: "true"
    pilot:
      enabled: true
      k8s:
        resources:
          requests:
            cpu: 200m
            memory: 200Mi
        podAnnotations:
          cluster-autoscaler.kubernetes.io/safe-to-evict: "true"
EOF

bin/istioctl manifest apply -f istio-minimal-operator.yaml -y;

echo "😀 Successfully installed Istio"
popd >> /dev/null
rm -rf ${ISTIO_DIR}

# Install Knative
if [ $deploymentMode = serverless ]; then
   kubectl apply --filename https://github.com/knative/serving/releases/download/${KNATIVE_SERVING_VERSION}/serving-crds.yaml
   kubectl apply --filename https://github.com/knative/serving/releases/download/${KNATIVE_SERVING_VERSION}/serving-core.yaml
   kubectl apply --filename https://github.com/knative/net-istio/releases/download/${KNATIVE_ISTIO_VERSION}/release.yaml
   # Patch the external domain as the default domain svc.cluster.local is not exposed on ingress
   kubectl patch cm config-domain --patch '{"data":{"example.com":""}}' -n knative-serving
   echo "😀 Successfully installed Knative"
fi

# Install Cert Manager
kubectl apply --validate=false -f https://github.com/jetstack/cert-manager/releases/download/${CERT_MANAGER_VERSION}/cert-manager.yaml
kubectl wait --for=condition=available --timeout=600s deployment/cert-manager-webhook -n cert-manager
cd ..
echo "😀 Successfully installed Cert Manager"

# Install KServe
KSERVE_CONFIG=kserve.yaml
MAJOR_VERSION=$(echo ${KSERVE_VERSION:1} | cut -d "." -f1)
MINOR_VERSION=$(echo ${KSERVE_VERSION} | cut -d "." -f2)
if [ ${MAJOR_VERSION} -eq 0 ] && [ ${MINOR_VERSION} -le 6 ]; then KSERVE_CONFIG=kfserving.yaml; fi

# Retry inorder to handle that it may take a minute or so for the TLS assets required for the webhook to function to be provisioned
kubectl apply -f https://github.com/kserve/kserve/releases/download/${KSERVE_VERSION}/${KSERVE_CONFIG}

# Install KServe built-in servingruntimes and storagecontainers
kubectl wait --for=condition=ready pod -l control-plane=kserve-controller-manager -n kserve --timeout=300s

if [ ${MAJOR_VERSION} -eq 0 ] && [ ${MINOR_VERSION} -le 11 ]; then
    kubectl apply -f https://github.com/kserve/kserve/releases/download/${KSERVE_VERSION}/kserve-runtimes.yaml
else
    kubectl apply -f https://github.com/kserve/kserve/releases/download/${KSERVE_VERSION}/kserve-cluster-resources.yaml
fi

# Patch default deployment mode for raw deployment
if [ $deploymentMode = kubernetes ]; then
cat <<EOF > deploy-config-patch.yaml
data:
  deploy: |
    {
      "defaultDeploymentMode": "RawDeployment"
    }
EOF
kubectl patch cm inferenceservice-config -n kserve --type=merge --patch-file=deploy-config-patch.yaml
fi
echo "😀 Successfully installed KServe"
