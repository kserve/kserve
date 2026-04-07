
# find_project_root [start_dir] [marker]
#   start_dir : directory to begin the search (defaults to this script’s dir)
#   marker    : filename or directory name to look for (defaults to "go.mod")
#
# Prints the first dir containing the marker, or exits 1 if none found.
find_project_root() {
  local start_dir="${1:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)}"
  local marker="${2:-go.mod}"
  local dir="$start_dir"

  while [[ "$dir" != "/" && ! -e "$dir/$marker" ]]; do
    dir="$(dirname "$dir")"
  done

  if [[ -e "$dir/$marker" ]]; then
    printf '%s\n' "$dir"
  else
    echo "Error: couldn’t find '$marker' in any parent of '$start_dir'" >&2
    return 1
  fi
}

# Helper function to wait for a pod with a given label to be created
wait_for_pod_labeled() {
  local ns=${1:?namespace is required}
  local podlabel=${2:?pod label is required}

  echo "Waiting for pod -l \"$podlabel\" in namespace \"$ns\" to be created..."
  until oc get pod -n "$ns" -l "$podlabel" -o=jsonpath='{.items[0].metadata.name}' >/dev/null 2>&1; do
    sleep 2
  done
  echo "Pod -l \"$podlabel\" in namespace \"$ns\" found."
}

# Helper function to wait for a pod with a given label to become ready
wait_for_pod_ready() {
  local ns=${1:?namespace is required}
  local podlabel=${2:?pod label is required}
  local timeout=${3:-600s} # Default timeout 600s

  wait_for_pod_labeled "$ns" "$podlabel"
  sleep 5 # Brief pause to allow K8s to fully register pod status

  echo "Current pods for -l \"$podlabel\" in namespace \"$ns\":"
  oc get pod -n "$ns" -l "$podlabel"

  echo "Waiting up to $timeout for pod(s) -l \"$podlabel\" in namespace \"$ns\" to become ready..."
  if ! oc wait --for=condition=ready --timeout="$timeout" pod -n "$ns" -l "$podlabel"; then
    echo "ERROR: Pod(s) -l \"$podlabel\" in namespace \"$ns\" did not become ready in time."
    echo "Describing pod(s):"
    oc describe pod -n "$ns" -l "$podlabel"

    # Try to get logs from the first pod matching the label if any exist
    local first_pod_name
    first_pod_name=$(oc get pod -n "$ns" -l "$podlabel" -o=jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")

    if [ -n "$first_pod_name" ]; then
        echo "Logs for pod \"$first_pod_name\" in namespace \"$ns\" (last 100 lines per container):"
        oc logs -n "$ns" "$first_pod_name" --all-containers --tail=100 || echo "Could not retrieve logs for $first_pod_name."
    else
        echo "No pods found matching -l \"$podlabel\" in namespace \"$ns\" to retrieve logs from."
    fi
    return 1 # Indicate failure
  fi
  echo "Pod(s) -l \"$podlabel\" in namespace \"$ns\" are ready."
}

# Usage: wait_for_crd <crd-name> [timeout]
#   <crd-name> : the full CRD name (e.g. leaderworkersetoperators.operator.openshift.io)
#   [timeout]  : oc wait timeout (default “60s”)
wait_for_crd() {
  local crd="$1"
  local timeout="${2:-60s}"

  echo "Waiting for CRD ${crd} to appear (timeout: ${timeout})…"
  if ! timeout "$timeout" bash -c 'until oc get crd "$1" &>/dev/null; do sleep 2; done' _ "$crd"; then
    echo "Timed out after $timeout waiting for CRD $crd to appear." >&2
    return 1
  fi

  echo "CRD ${crd} detected — waiting for it to become Established (timeout: ${timeout})…"
  oc wait --for=condition=Established --timeout="$timeout" "crd/$crd"
}

# Poll until GET /apis/<group>/<version> lists the given resource (apiserver discovery).
# Stronger than CRD Established alone for admission paths that resolve owner GVK via REST mapping.
# Usage: wait_for_api_discovery <group/version> <resource-name> [timeout_seconds]
#   <group/version>   e.g. kuadrant.io/v1beta1
#   <resource-name>   plural list name from discovery, e.g. kuadrants
#   [timeout_seconds] default 120
wait_for_api_discovery() {
  local gv=${1:?group/version is required}
  local resource_name=${2:?resource name is required}
  local timeout_sec=${3:-120}

  echo "Waiting for apiserver discovery /apis/${gv} to list ${resource_name} (timeout: ${timeout_sec}s)…"
  local counter=0
  local raw=""
  # Match discovery JSON without jq (Prow e2e image may not ship jq; Konflux often does).
  local name_pattern
  name_pattern="\"name\":\"${resource_name}\""
  while [ "$counter" -lt "$timeout_sec" ]; do
    if raw=$(oc get --raw "/apis/${gv}" 2>/dev/null) && echo "$raw" | grep -Fq "$name_pattern"; then
      echo "Discovery for ${gv} includes ${resource_name}."
      return 0
    fi
    sleep 2
    counter=$((counter + 2))
  done

  echo "ERROR: Timed out after ${timeout_sec}s waiting for /apis/${gv} to list ${resource_name}."
  oc get --raw "/apis/${gv}" 2>/dev/null || echo "(GET /apis/${gv} failed)"
  return 1
}

# Helper function to wait for and approve an operator install plan
# Usage: wait_for_installplan_and_approve <namespace> <operator-name> [timeout]
#   <namespace>     : namespace where the operator is being installed
#   <operator-name> : name pattern to match in clusterServiceVersionNames (e.g., "opendatahub-operator")
#   [timeout]       : timeout in seconds (default: 60)
wait_for_installplan_and_approve() {
  local namespace=${1:?namespace is required}
  local operator_name=${2:?operator name is required}
  local timeout=${3:-60}

  echo "Waiting for ${operator_name} install plan to be created..."
  local counter=0
  local install_plan=""
  while [ "$counter" -lt "$timeout" ]; do
    install_plan=$(oc get installplan -n "${namespace}" -o json | jq -r ".items[] | select(.spec.clusterServiceVersionNames[]? | contains(\"${operator_name}\")) | select(.spec.approved == false) | .metadata.name" 2>/dev/null | head -1)
    if [ -n "$install_plan" ]; then
      echo "Found install plan: $install_plan"
      break
    fi
    sleep 2
    counter=$((counter + 2))
  done

  if [ -n "$install_plan" ]; then
    echo "Approving install plan $install_plan..."
    oc patch installplan "$install_plan" -n "${namespace}" --type merge --patch '{"spec":{"approved":true}}'
  else
    echo "No unapproved install plan found for ${operator_name} within timeout"
  fi
}

# Wait for an OLM Subscription's CSV to reach "Succeeded" status.
# Discovers the CSV name from the Subscription's status.installedCSV field,
# which is the authoritative source regardless of the CSV naming convention.
# Usage: wait_for_subscription_csv <subscription-name> <namespace> [timeout]
#   <subscription-name> : name of the OLM Subscription resource
#   <namespace>         : namespace where the Subscription exists
#   [timeout]           : timeout in seconds (default: 600)
wait_for_subscription_csv() {
  local sub_name=${1:?subscription name is required}
  local namespace=${2:?namespace is required}
  local timeout=${3:-600}

  echo "Waiting for ${sub_name} CSV to become ready..."
  local counter=0
  local csv_name=""
  while [ "$counter" -lt "$timeout" ]; do
    csv_name=$(oc get subscription "$sub_name" -n "$namespace" \
      -o=jsonpath='{.status.installedCSV}' 2>/dev/null || true)
    if [ -n "$csv_name" ]; then
      local csv_phase
      csv_phase=$(oc get csv "$csv_name" -n "$namespace" \
        -o=jsonpath='{.status.phase}' 2>/dev/null || true)
      if [ "$csv_phase" = "Succeeded" ]; then
        echo "CSV $csv_name is ready (Phase: Succeeded)."
        return 0
      fi
      echo "CSV $csv_name found, but not yet Succeeded (Phase: ${csv_phase:-Unknown}). Waiting... ($counter/$timeout)"
    else
      echo "Waiting for CSV to be installed for subscription $sub_name... ($counter/$timeout)"
    fi
    sleep 5
    counter=$((counter + 5))
  done

  echo "ERROR: Timeout waiting for ${sub_name} CSV to be ready after ${timeout}s"
  echo "Subscription status:"
  oc get subscription "$sub_name" -n "$namespace" -o yaml 2>/dev/null || true
  echo "CatalogSource status:"
  oc get catalogsource -n openshift-marketplace 2>/dev/null || true
  echo "CSVs in namespace ${namespace}:"
  oc get csv -n "$namespace" 2>/dev/null || true
  return 1
}
