#!/usr/bin/env bash
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -eu # Exit on error and undefined variables

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
source "${SCRIPT_DIR}/common.sh"

: "${SUBSCRIPTION_NAME:=openshift-custom-metrics-autoscaler-operator}"
: "${KEDA_NAMESPACE:=openshift-keda}"
: "${KEDA_OPERATOR_POD_LABEL:=app=keda-operator}"
: "${KEDA_METRICS_API_SERVER_POD_LABEL:=app=keda-metrics-apiserver}"
: "${KEDA_WEBHOOK_POD_LABEL:=app=keda-admission-webhooks}"

echo "Creating namespace openshift-keda..."
cat <<EOF | oc apply -f -
apiVersion: v1
kind: Namespace
metadata:
  name: ${KEDA_NAMESPACE}
  labels:
    openshift.io/cluster-monitoring: "true"
EOF
echo "Namespace openshift-keda created/ensured."
echo "---"

echo "Creating OperatorGroup openshift-keda..."
cat <<EOF | oc apply -f -
apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: openshift-keda
  namespace: ${KEDA_NAMESPACE}
spec:
  targetNamespaces:
    - openshift-keda
  upgradeStrategy: Default
EOF
echo "OperatorGroup openshift-keda created/ensured."
echo "---"

echo "Creating Subscription for openshift-custom-metrics-autoscaler-operator..."
cat <<EOF | oc apply -f -
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  labels:
    operators.coreos.com/${SUBSCRIPTION_NAME}.${KEDA_NAMESPACE}: ""
  name: ${SUBSCRIPTION_NAME}
  namespace: ${KEDA_NAMESPACE}
spec:
  channel: stable
  installPlanApproval: Automatic
  name: ${SUBSCRIPTION_NAME}
  source: redhat-operators
  sourceNamespace: openshift-marketplace
EOF
echo "Subscription ${SUBSCRIPTION_NAME} created/ensured."
echo "---"

echo "Waiting for Custom Metrics Autoscaler Operator CSV to be ready..."
CSV_NAME=""
# Wait for the CSV to be installed and report success (typically up to 5-10 minutes for operator installs)
for i in $(seq 1 120); do # Wait up to 10 minutes (120 * 5s = 600s)
  CSV_NAME=$(oc get subscriptions "$SUBSCRIPTION_NAME" -n "$KEDA_NAMESPACE" -o=jsonpath='{.status.installedCSV}' 2>/dev/null || true)
  if [ -n "$CSV_NAME" ]; then
    CSV_PHASE=$(oc get csv "$CSV_NAME" -n "$KEDA_NAMESPACE" -o=jsonpath='{.status.phase}' 2>/dev/null || true)
    if [ "$CSV_PHASE" == "Succeeded" ]; then
      echo "CSV $CSV_NAME is ready (Phase: $CSV_PHASE)."
      break
    else
      echo "CSV $CSV_NAME found, but not yet Succeeded (Phase: $CSV_PHASE). Waiting... (Attempt $i/120)"
    fi
  else
    echo "Waiting for CSV to be installed for subscription $SUBSCRIPTION_NAME... (Attempt $i/120)"
  fi
  sleep 5
  CSV_NAME="" # Reset for next loop iteration if not found or not ready
done

if [ -z "$CSV_NAME" ]; then
  echo "ERROR: Could not find installed CSV for $SUBSCRIPTION_NAME in namespace $KEDA_NAMESPACE after waiting."
  echo "Status of subscription:"
  oc get subscription "$SUBSCRIPTION_NAME" -n "$KEDA_NAMESPACE" -o yaml
  exit 1
fi

echo "Custom Metrics Autoscaler Operator is ready."
echo "---"

# 5. Apply KedaController Custom Resource
echo "Applying KedaController custom resource..."
cat <<EOF | oc apply -f -
apiVersion: keda.sh/v1alpha1
kind: KedaController
metadata:
  name: keda
  namespace: ${KEDA_NAMESPACE}
spec:
  watchNamespace: ""     # watch all namespaces
  operator:
    logLevel: info
    logEncoder: console
  metricsServer:
    logLevel: "0"
  admissionWebhooks:
    logLevel: info
    logEncoder: console
EOF
echo "KedaController custom resource applied."
echo "---"

# These components are deployed based on the KedaController CR.
# It might take a moment for the operator to process the KedaController CR and create these deployments.
echo "Allowing time for KEDA components to be provisioned by the operator ..."
sleep 10

echo "Waiting for KEDA Operator pod (selector: \"$KEDA_OPERATOR_POD_LABEL\") to be ready in namespace $KEDA_NAMESPACE..."
if ! wait_for_pod_ready "$KEDA_NAMESPACE" "$KEDA_OPERATOR_POD_LABEL" 120s; then
    echo "ERROR: KEDA Operator pod failed to become ready."
    exit 1
fi
echo "KEDA Operator pod is ready."

echo "Waiting for KEDA Metrics API Server pod (selector: \"$KEDA_METRICS_API_SERVER_POD_LABEL\") to be ready in namespace $KEDA_NAMESPACE..."
if ! wait_for_pod_ready "$KEDA_NAMESPACE" "$KEDA_METRICS_API_SERVER_POD_LABEL" 120s; then
    echo "ERROR: KEDA Metrics API Server pod failed to become ready."
    exit 1
fi
echo "KEDA Metrics API Server pod is ready."

echo "Waiting for KEDA Webhook pod (selector: \"$KEDA_WEBHOOK_POD_LABEL\") to be ready in namespace $KEDA_NAMESPACE..."
if ! wait_for_pod_ready "$KEDA_NAMESPACE" "$KEDA_WEBHOOK_POD_LABEL" 120s; then
    echo "ERROR: KEDA Webhook pod failed to become ready."
    exit 1
fi
echo "KEDA Webhook pod is ready."

echo "---"
echo "✅ KEDA deployment script finished successfully."
