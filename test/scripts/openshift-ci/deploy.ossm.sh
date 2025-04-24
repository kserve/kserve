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
  echo "Waiting for pod -l $podlabel to become ready"
  oc wait --for=condition=ready --timeout=180s pod -n $ns -l $podlabel
}

# Deploy OSSM operator
cat <<EOF | oc apply -f -
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  labels:
    operators.coreos.com/servicemeshoperator.openshift-operators: ""
  name: servicemeshoperator
  namespace: openshift-operators
spec:
  channel: stable
  installPlanApproval: Automatic
  name: servicemeshoperator
  source: redhat-operators
  sourceNamespace: openshift-marketplace
EOF

waitpodready "openshift-operators" "name=istio-operator"

# Create new namespace
oc new-project istio-system
# Install OSSM
cat <<EOF | oc apply -f -
apiVersion: maistra.io/v2
kind: ServiceMeshControlPlane
metadata:
  name: basic
  namespace: istio-system
spec:
  tracing:
    type: None
  addons:
    grafana:
      enabled: false
    kiali:
      name: kiali
      enabled: false
    prometheus:
      enabled: false
    jaeger:
      name: jaeger
  security:
    dataPlane:
      mtls: true # otherwise inference-graph will not work. We use PeerAuthentication resources to force mTLS
    identity:
      type: ThirdParty
  techPreview:
    meshConfig:
      defaultConfig:
        terminationDrainDuration: 35s
  gateways:
    openshiftRoute:
      enabled: false
    ingress:
      service:
        metadata:
          labels:
            knative: ingressgateway
  proxy:
    networking:
      trafficControl:
        inbound:
          excludedPorts:
            - 8444 # metrics
            - 8022 # serving: wait-for-drain k8s pre-stop hook
EOF

# Waiting for OSSM minimum start
waitpodready "istio-system" "app=istiod"

echo -e "\n  OSSM has partially started and should be fully ready soon."
