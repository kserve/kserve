#!/usr/bin/env bash
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -eu

waitforpodlabeled() {
  local ns=${1?namespace is required}; shift
  local podlabel=${1?pod label is required}; shift

  echo "Waiting for pod -l $podlabel to be created"
  until oc get pod -n "$ns" -l $podlabel -o=jsonpath='{.items[0].metadata.name}' >/dev/null 2>&1; do
    sleep 1
  done
}

waitpodready() {
  local ns=${1?namespace is required}; shift
  local podlabel=${1?pod label is required}; shift

  waitforpodlabeled "$ns" "$podlabel"
  sleep 2
  oc get pod -n $ns -l $podlabel

  echo "Waiting for pod -l $podlabel to become ready"
  oc wait --for=condition=ready --timeout=600s pod -n $ns -l $podlabel || (oc get pod -n $ns -l $podlabel && false)
}

# Create namespaces(openshift-serverless)
oc create ns openshift-serverless

# Create operatorGroup
cat <<EOF | oc create -f -
apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  annotations:
    olm.providedAPIs: KnativeEventing.v1beta1.operator.knative.dev,KnativeKafka.v1alpha1.operator.serverless.openshift.io,KnativeServing.v1beta1.operator.knative.dev
  name: openshift-serverless-2bt52
  namespace: openshift-serverless  
spec:
  upgradeStrategy: Default
EOF

# Install Serverless operator
cat <<EOF | oc create -f -
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  labels:
    operators.coreos.com/serverless-operator.openshift-serverless: ""
  name: serverless-operator
  namespace: openshift-serverless
spec:
  channel: stable
  installPlanApproval: Automatic
  name: serverless-operator
  source: redhat-operators
  sourceNamespace: openshift-marketplace
EOF

waitpodready "openshift-serverless" "name=knative-openshift"
waitpodready "openshift-serverless" "name=knative-openshift-ingress"
waitpodready "openshift-serverless" "name=knative-operator"

# Install KNative
cat <<EOF | oc apply -f -
apiVersion: maistra.io/v1
kind: ServiceMeshMember
metadata:
  name: default
  namespace: knative-serving
spec:
  controlPlaneRef:
    namespace: istio-system
    name: basic
---
apiVersion: networking.istio.io/v1beta1
kind: Gateway
metadata:
  name: kserve-local-gateway
  namespace: istio-system
spec:
  selector:
    knative: ingressgateway
  servers:
    - hosts:
        - 'default.host'
      port:
        name: https
        number: 8445
        protocol: HTTPS
      tls:
        mode: ISTIO_MUTUAL
---
apiVersion: v1
kind: Service
metadata:
  labels:
    experimental.istio.io/disable-gateway-port-translation: "true"
  name: kserve-local-gateway
  namespace: istio-system
spec:
  ports:
    - name: https
      protocol: TCP
      port: 443
      targetPort: 8445
  selector:
    knative: ingressgateway
  type: ClusterIP
---
apiVersion: operator.knative.dev/v1beta1
kind: KnativeServing
metadata:
  annotations:
    serverless.openshift.io/default-enable-http2: "true"
  name: knative-serving
  namespace: knative-serving
spec:
  config:
    features:
      kubernetes.podspec-affinity: enabled
      kubernetes.podspec-nodeselector: enabled
      kubernetes.podspec-persistent-volume-claim: enabled
      kubernetes.podspec-persistent-volume-write: enabled
      kubernetes.podspec-tolerations: enabled
    istio:
      local-gateway.knative-serving.knative-local-gateway: knative-local-gateway.istio-system.svc.cluster.local
  controller-custom-certs:
    name: ""
    type: ""
  ingress:
    contour:
      enabled: false
    istio:
      enabled: true
    kourier:
      enabled: false
  registry: {}
  workloads:
  - annotations:
      sidecar.istio.io/inject: "true"
      sidecar.istio.io/rewriteAppHTTPProbers: "true"
    name: activator
  - annotations:
      sidecar.istio.io/inject: "true"
      sidecar.istio.io/rewriteAppHTTPProbers: "true"
    name: autoscaler
  - env:
    - container: controller
      envVars:
      - name: ENABLE_SECRET_INFORMER_FILTERING_BY_CERT_UID
        value: "true"
    name: net-istio-controller
EOF


# Apparently, as part of KNative installation, deployments can be restarted because
# of configuration changes, leading to waitpodready to fail sometimes.
# Let's sleep 2minutes to let the KNative operator to stabilize the installation before
# checking for the readiness of KNative stack.
sleep 120

waitpodready "knative-serving" "app=controller"
waitpodready "knative-serving" "app=net-istio-controller"
waitpodready "knative-serving" "app=net-istio-webhook"
waitpodready "knative-serving" "app=autoscaler-hpa"
waitpodready "knative-serving" "app=webhook"
waitpodready "knative-serving" "app=activator"
waitpodready "knative-serving" "app=autoscaler"

export secret_name=$(oc get IngressController default -n openshift-ingress-operator -o yaml -o=jsonpath='{ .spec.defaultCertificate.name}')
if [ -z "$secret_name" ]; then
  # Fallback to the default secret name for crpkg/controller/v1beta1/inferenceservice/rawkube_controller_test.goc
  if $RUNNING_LOCAL; then
    export secret_name=router-certs-default
  else
    # In OpenShift 4.12, the default secret name is changed to default-ingress-cert
    export secret_name=default-ingress-cert
  fi
fi
oc get secret -n openshift-ingress
oc get IngressController -n openshift-ingress-operator default -o yaml
export tls_cert=$(oc get secret $secret_name -n openshift-ingress -o=jsonpath='{.data.tls\.crt}')
export CA_CERT_PATH="/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
if [ -f "$CA_CERT_PATH" ]; then
  # This is for python requests to work
  export REQUESTS_CA_BUNDLE=$CA_CERT_PATH
fi
export tls_key=$(oc get secret $secret_name -n openshift-ingress -o=jsonpath='{.data.tls\.key}')
oc create secret tls knative-serving-cert \
  --cert=<(echo $tls_cert | base64 -d) \
  --key=<(echo $tls_key | base64 -d) \
  -n istio-system
export domain=$(oc get ingresses.config/cluster -o=jsonpath='{ .spec.domain}')


# Install the Gateways
cat <<EOF | oc apply -f -
apiVersion: v1
kind: Service
metadata:
  labels:
    experimental.istio.io/disable-gateway-port-translation: "true"
  name: knative-local-gateway
  namespace: istio-system
spec:
  ports:
    - name: http2
      port: 80
      protocol: TCP
      targetPort: 8081
  selector:
    knative: ingressgateway
  type: ClusterIP
---
apiVersion: v1
kind: Service
metadata:
  name: knative-ingressgateway
  namespace: istio-system
  labels:
    knative: ingressgateway
spec:
  type: ClusterIP
  ports:
    - name: http2
      port: 80
      protocol: TCP
      targetPort: 8080
    - name: https
      port: 443
      protocol: TCP
      targetPort: 8443
  selector:
    knative: ingressgateway
---
apiVersion: networking.istio.io/v1beta1
kind: Gateway
metadata:
  name: knative-ingress-gateway
  namespace: knative-serving
spec:
  selector:
    knative: ingressgateway
  servers:
    - hosts:
        - '*.${domain}'
      port:
        name: https
        number: 443
        protocol: HTTPS
      tls:
        credentialName: knative-serving-cert
        mode: SIMPLE
---
apiVersion: networking.istio.io/v1beta1
kind: Gateway
metadata:
  name: knative-local-gateway
  namespace: knative-serving
spec:
  selector:
    knative: ingressgateway
  servers:
    - hosts:
        - '*.svc.cluster.local'
      port:
        name: https
        number: 8081
        protocol: HTTPS
      tls:
        mode: ISTIO_MUTUAL
EOF
