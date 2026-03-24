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

wait_for_crd  kuadrants.kuadrant.io  90s

{
cat <<EOF | oc create -f -
apiVersion: kuadrant.io/v1beta1
kind: Kuadrant
metadata:
  name: kuadrant
  namespace: ${KUADRANT_NS}
EOF
} || true

echo "⏳ waiting for authorino-operator to be ready.…"

oc wait Kuadrant -n "${KUADRANT_NS}" kuadrant --for=condition=Ready --timeout=10m || {
  oc get Kuadrant -n "${KUADRANT_NS}" kuadrant -oyaml
  oc get pods -n "${KUADRANT_NS}" -oyaml
  oc get deployments -n "${KUADRANT_NS}" -oyaml
  oc get csv -n "${KUADRANT_NS}" -oyaml

  oc describe Kuadrant -n "${KUADRANT_NS}" kuadrant
  oc describe pods -n "${KUADRANT_NS}"
  oc describe deployments -n "${KUADRANT_NS}"
  oc describe csv -n "${KUADRANT_NS}"
  exit 1
}

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
