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

# Deploy Serverless operator
cat <<EOF | oc apply -f -
apiVersion: v1
kind: Namespace
metadata:
  name: openshift-serverless
---
apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: serverless-operators
  namespace: openshift-serverless
spec: {}
---
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: serverless-operator
  namespace: openshift-serverless
spec:
  channel: stable
  name: serverless-operator
  source: redhat-operators
  sourceNamespace: openshift-marketplace
---
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
apiVersion: operator.knative.dev/v1beta1
kind: KnativeServing
metadata:
  name: knative-serving
  namespace: knative-serving
  annotations:
    serverless.openshift.io/default-enable-http2: "true"
spec:
  deployments:
    - annotations:
        sidecar.istio.io/inject: "true"
        sidecar.istio.io/rewriteAppHTTPProbers: "true"
      name: activator
    - annotations:
        sidecar.istio.io/inject: "true"
        sidecar.istio.io/rewriteAppHTTPProbers: "true"
      name: autoscaler
  ingress:
    istio:
      enabled: true
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
    istio: ingressgateway
  type: ClusterIP
---
apiVersion: networking.istio.io/v1beta1
kind: Gateway
metadata:
  name: knative-ingress-gateway
  namespace: knative-serving
spec:
  selector:
    istio: ingressgateway
  servers:
    - hosts:
        - '*'
      port:
        name: http
        number: 80
        protocol: HTTP
---
apiVersion: networking.istio.io/v1beta1
kind: Gateway
metadata:
  name: knative-local-gateway
  namespace: knative-serving
spec:
  selector:
    istio: ingressgateway
  servers:
    - hosts:
        - '*'
      port:
        name: http
        number: 8081
        protocol: HTTP
---
EOF
