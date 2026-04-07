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

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../common.sh"
KUADRANT_NS="${KUADRANT_NS:-kuadrant-system}"
# Seconds to sleep after discovery passes (apiserver RESTMapper can lag discovery).
KUADRANT_PRE_CREATE_SLEEP="${KUADRANT_PRE_CREATE_SLEEP:-30}"
# How many times to wait for Ready before delete/recreate and final failure (default: initial + one retry).
KUADRANT_READY_MAX_ATTEMPTS="${KUADRANT_READY_MAX_ATTEMPTS:-2}"
# Seconds to sleep after deleting Kuadrant before recreating (stabilization).
KUADRANT_POST_DELETE_SLEEP="${KUADRANT_POST_DELETE_SLEEP:-15}"
# Per-attempt timeout for oc wait on Kuadrant Ready (two attempts default; use 10m on very slow clusters).
KUADRANT_READY_TIMEOUT="${KUADRANT_READY_TIMEOUT:-5m}"

create_kuadrant_cr() {
  oc create -f - <<EOF
apiVersion: kuadrant.io/v1beta1
kind: Kuadrant
metadata:
  name: kuadrant
  namespace: ${KUADRANT_NS}
EOF
}

echo "⏳ Installing RHCL(Kuadrant) operator"
oc create ns ${KUADRANT_NS} || true

{
cat <<EOF | oc create -f -
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: rhcl-operator
  namespace: ${KUADRANT_NS}
spec:
  channel: stable
  installPlanApproval: Automatic
  name: rhcl-operator
  source: redhat-operators
  sourceNamespace: openshift-marketplace
---
kind: OperatorGroup
apiVersion: operators.coreos.com/v1
metadata:
  name: kuadrant
  namespace: ${KUADRANT_NS}
spec:
  upgradeStrategy: Default
EOF
} || true

wait_for_subscription_csv "rhcl-operator" "${KUADRANT_NS}" 600
wait_for_crd  kuadrants.kuadrant.io  90s
# Let apiserver discovery include kuadrants before first reconcile creates child resources with owner refs.
wait_for_api_discovery "kuadrant.io/v1beta1" "kuadrants" 120

echo "⏳ sleeping ${KUADRANT_PRE_CREATE_SLEEP}s after discovery (RESTMapper can trail discovery)…"
sleep "${KUADRANT_PRE_CREATE_SLEEP}"

create_kuadrant_cr || true

kuadrant_ready_attempt=1
while (( kuadrant_ready_attempt <= KUADRANT_READY_MAX_ATTEMPTS )); do
  echo "⏳ waiting for Kuadrant Ready (attempt ${kuadrant_ready_attempt}/${KUADRANT_READY_MAX_ATTEMPTS}, timeout ${KUADRANT_READY_TIMEOUT})…"
  if oc wait Kuadrant -n "${KUADRANT_NS}" kuadrant --for=condition=Ready --timeout="${KUADRANT_READY_TIMEOUT}"; then
    break
  fi
  if (( kuadrant_ready_attempt >= KUADRANT_READY_MAX_ATTEMPTS )); then
    oc get Kuadrant -n "${KUADRANT_NS}" kuadrant -oyaml
    oc get pods -n "${KUADRANT_NS}" -oyaml
    oc get deployments -n "${KUADRANT_NS}" -oyaml
    oc get csv -n "${KUADRANT_NS}" -oyaml

    oc describe Kuadrant -n "${KUADRANT_NS}" kuadrant
    oc describe pods -n "${KUADRANT_NS}"
    oc describe deployments -n "${KUADRANT_NS}"
    oc describe csv -n "${KUADRANT_NS}"

    echo "=== Controller manager logs ==="
    oc logs -n "${KUADRANT_NS}" deployment/kuadrant-operator-controller-manager --tail=200 || true
    exit 1
  fi
  echo "Kuadrant not Ready; deleting and recreating CR to trigger a new Create reconcile (helps operator versions that only subscribe to Create)…"
  oc delete kuadrant kuadrant -n "${KUADRANT_NS}" --ignore-not-found=true --wait=true --timeout=300s
  echo "⏳ sleeping ${KUADRANT_POST_DELETE_SLEEP}s before recreating Kuadrant…"
  sleep "${KUADRANT_POST_DELETE_SLEEP}"
  create_kuadrant_cr || true
  kuadrant_ready_attempt=$((kuadrant_ready_attempt + 1))
done

wait_for_pod_ready "${KUADRANT_NS}" "control-plane=authorino-operator"

# Wait for service to be created
echo "⏳ waiting for authorino service to be created..."
cert_secret="authorino-server-cert"
oc wait --for=jsonpath='{.metadata.name}'=authorino-authorino-authorization svc/authorino-authorino-authorization -n "${KUADRANT_NS}" --timeout=2m

oc annotate svc/authorino-authorino-authorization  service.beta.openshift.io/serving-cert-secret-name="${cert_secret}" -n "${KUADRANT_NS}"

# Update Authorino to configure SSL
oc apply -f - <<EOF
apiVersion: operator.authorino.kuadrant.io/v1beta1
kind: Authorino
metadata:
  name: authorino
  namespace: kuadrant-system
spec:
  replicas: 1
  clusterWide: true
  listener:
    tls:
      enabled: true
      certSecretRef:
        name: authorino-server-cert
  oidcServer:
    tls:
      enabled: false
EOF

wait_for_pod_ready "${KUADRANT_NS}" "control-plane=authorino-operator"

echo "✅ kuadrant(authorino) installed"
