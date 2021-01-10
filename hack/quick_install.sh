set -e

export ISTIO_VERSION=1.6.2
export KNATIVE_VERSION=v0.18.0
export KFSERVING_VERSION=v0.5.0-rc2
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

  addonComponents:
    pilot:
      enabled: true
    tracing:
      enabled: true
    kiali:
      enabled: true
    prometheus:
      enabled: true
    grafana:
      enabled: true

  components:
    ingressGateways:
      - name: istio-ingressgateway
        enabled: true
      - name: cluster-local-gateway
        enabled: true
        label:
          istio: cluster-local-gateway
          app: cluster-local-gateway
        k8s:
          service:
            type: ClusterIP
            ports:
            - port: 15020
              name: status-port
            - port: 80
              name: http2
            - port: 443
              name: https
EOF

bin/istioctl manifest apply -f istio-minimal-operator.yaml

# Install Knative
kubectl apply --filename https://github.com/knative/serving/releases/download/${KNATIVE_VERSION}/serving-crds.yaml
kubectl apply --filename https://github.com/knative/serving/releases/download/${KNATIVE_VERSION}/serving-core.yaml
kubectl apply --filename https://github.com/knative/net-istio/releases/download/${KNATIVE_VERSION}/release.yaml

# Install Cert Manager
kubectl apply --validate=false -f https://github.com/jetstack/cert-manager/releases/download/v0.15.1/cert-manager.yaml
kubectl wait --for=condition=available --timeout=600s deployment/cert-manager-webhook -n cert-manager
cd ..
# Install KFServing
K8S_MINOR=$(kubectl version | perl -ne 'print $1."\n" if /Server Version:.*?Minor:"(\d+)"/')
if [[ $K8S_MINOR -lt 16 ]]; then
  kubectl apply -f install/${KFSERVING_VERSION}/kfserving.yaml --validate=false
else
  kubectl apply -f install/${KFSERVING_VERSION}/kfserving.yaml
fi

# Clean up
rm -rf istio-${ISTIO_VERSION}
