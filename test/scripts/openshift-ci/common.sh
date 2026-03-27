
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

# Helper function to wait for a CSV to reach "Succeeded" status
# Usage: wait_for_csv_ready <namespace> <csv-name-pattern> [timeout]
#   <namespace>        : namespace where the CSV exists
#   <csv-name-pattern> : pattern to match CSV name (e.g., "opendatahub-operator" matches "opendatahub-operator.v1.2.3")
#   [timeout]          : timeout in seconds (default: 300)
wait_for_csv_ready() {
  local namespace=${1:?namespace is required}
  local csv_name_pattern=${2:?CSV name pattern is required}
  local timeout=${3:-300}

  echo "Waiting for ${csv_name_pattern} CSV to be installed..."
  local counter=0
  local csv_status=""
  while [ "$counter" -lt "$timeout" ]; do
    csv_status=$(oc get csv -n "${namespace}" -o json | jq -r ".items[] | select(.metadata.name | startswith(\"${csv_name_pattern}\")) | .status.phase" 2>/dev/null || echo "")
    if [ "$csv_status" = "Succeeded" ]; then
      echo "${csv_name_pattern} CSV is ready"
      break
    fi
    echo "Waiting for CSV to be ready... (current status: ${csv_status:-NotFound}, $counter/$timeout)"
    sleep 5
    counter=$((counter + 5))
  done

  if [ "$counter" -ge "$timeout" ]; then
    echo "Timeout waiting for ${csv_name_pattern} CSV to be ready"
    return 1
  fi
}
