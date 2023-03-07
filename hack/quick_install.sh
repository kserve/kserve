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

export ISTIO_VERSION=1.15.0
export KNATIVE_VERSION=knative-v1.7.0
export KSERVE_VERSION=v0.10.1
export CERT_MANAGER_VERSION=v1.3.0
export SCRIPT_DIR="$( dirname -- "${BASH_SOURCE[0]}" )"

KUBE_VERSION=$(kubectl version --short=true | grep "Server Version" | awk -F '.' '{print $2}')
if [ ${KUBE_VERSION} -lt 22 ];
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
        env:
        - name: PILOT_ENABLE_CONFIG_DISTRIBUTION_TRACKING
          value: "false"
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
KSERVE_CONFIG=kserve.yaml
MAJOR_VERSION=$(echo ${KSERVE_VERSION:1} | cut -d "." -f1)
MINOR_VERSION=$(echo ${KSERVE_VERSION} | cut -d "." -f2)
if [ ${MAJOR_VERSION} -eq 0 ] && [ ${MINOR_VERSION} -le 6 ]; then KSERVE_CONFIG=kfserving.yaml; fi

# Retry inorder to handle that it may take a minute or so for the TLS assets required for the webhook to function to be provisioned
kubectl apply -f https://github.com/kserve/kserve/releases/download/${KSERVE_VERSION}/${KSERVE_CONFIG}

# Install KServe built-in servingruntimes
kubectl wait --for=condition=ready pod -l control-plane=kserve-controller-manager -n kserve --timeout=300s
kubectl apply -f https://github.com/kserve/kserve/releases/download/${KSERVE_VERSION}/kserve-runtimes.yaml
echo "😀 Successfully installed KServe"

# Clean up
rm -rf istio-${ISTIO_VERSION}
