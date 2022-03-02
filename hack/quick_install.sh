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

export ISTIO_VERSION=1.9.0
export KNATIVE_VERSION=knative-v1.0.0
export KSERVE_VERSION=v0.8.0
export CERT_MANAGER_VERSION=v1.3.0

KUBE_VERSION=$(kubectl version --short=true)

echo ${KUBE_VERSION:43:2}

if [ ${KUBE_VERSION:43:2} -gt 20 ]; then export ISTIO_VERSION=1.10.3; fi

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
      # The third-party-jwt is not enabled on all k8s.
      # See: https://istio.io/docs/ops/best-practices/security/#configure-third-party-service-account-tokens
      jwtPolicy: first-party-jwt

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

if [ $(cut -d '.' -f 2,2 <<< $ISTIO_VERSION) -gt 9 ]
then
    bin/istioctl install --set profile=demo -y;
else
    bin/istioctl manifest apply -f istio-minimal-operator.yaml -y;
fi

# Install Knative
if [ $deploymentMode = serverless ]; then
   kubectl apply --filename https://github.com/knative/serving/releases/download/${KNATIVE_VERSION}/serving-crds.yaml
   kubectl apply --filename https://github.com/knative/serving/releases/download/${KNATIVE_VERSION}/serving-core.yaml
   kubectl apply --filename https://github.com/knative/net-istio/releases/download/${KNATIVE_VERSION}/release.yaml
fi

# Install Cert Manager
kubectl apply --validate=false -f https://github.com/jetstack/cert-manager/releases/download/${CERT_MANAGER_VERSION}/cert-manager.yaml
kubectl wait --for=condition=available --timeout=600s deployment/cert-manager-webhook -n cert-manager
cd ..
# Install KFServing
KSERVE_CONFIG=kfserving.yaml
if [ ${KSERVE_VERSION:3:1} -gt 6 ]; then KSERVE_CONFIG=kserve.yaml; fi

# Retry inorder to handle that it may take a minute or so for the TLS assets required for the webhook to function to be provisioned
for i in 1 2 3 4 5 ; do kubectl apply -f https://github.com/kserve/kserve/releases/download/${KSERVE_VERSION}/${KSERVE_CONFIG} && break || sleep 15; done
# Install KServe built-in servingruntimes
kubectl wait --for=condition=ready pod -l control-plane=kserve-controller-manager -n kserve --timeout=300s
kubectl apply -f https://github.com/kserve/kserve/releases/download/${KSERVE_VERSION}/kserve-runtimes.yaml
# Clean up
rm -rf istio-${ISTIO_VERSION}
