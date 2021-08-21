set -e

export ISTIO_VERSION=1.9.0
export KNATIVE_VERSION=v0.22.0
export KFSERVING_VERSION=v0.6.0
export CERT_MANAGER_VERSION=v1.3.0
curl -L https://git.io/getLatestIstio | sh -
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
apiVersion: install.istio.io/v1alpha1
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

bin/istioctl manifest apply -f istio-minimal-operator.yaml -y

# Install Knative
kubectl apply --filename https://github.com/knative/serving/releases/download/${KNATIVE_VERSION}/serving-crds.yaml
kubectl apply --filename https://github.com/knative/serving/releases/download/${KNATIVE_VERSION}/serving-core.yaml
kubectl apply --filename https://github.com/knative/net-istio/releases/download/${KNATIVE_VERSION}/release.yaml

# Install Cert Manager
kubectl apply --validate=false -f https://github.com/jetstack/cert-manager/releases/download/${CERT_MANAGER_VERSION}/cert-manager.yaml
kubectl wait --for=condition=available --timeout=600s deployment/cert-manager-webhook -n cert-manager
cd ..
# Install KFServing
# Retry inorder to handle that it may take a minute or so for the TLS assets required for the webhook to function to be provisioned
for i in 1 2 3 4 5 ; do kubectl apply -f https://github.com/kubeflow/kfserving/releases/download/$KFSERVING_VERSION/kfserving.yaml && break || sleep 15; done
# Clean up
rm -rf istio-${ISTIO_VERSION}
