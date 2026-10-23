#!/usr/bin/env bash

# test-controller-priority-class.sh
#
# Purpose: Render the KServe and LLMISVC controller charts and check that
#          priorityClassName is omitted unless a user configures it.
#
# What it checks:
#   - The default chart render does not set priorityClassName.
#   - An explicit empty value still omits priorityClassName.
#   - A configured value is rendered on the controller Deployment PodSpec.
#   - Rendering the same values twice is identical, so a Helm upgrade keeps
#     the configured priority in the release's desired state.
#
# Usage: bash hack/setup/scripts/test-controller-priority-class.sh
#
# Exit codes:
#   0 - Controller priorityClassName renders as expected
#   1 - A render did not match the expected PodSpec

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"

if ! command -v helm >/dev/null 2>&1; then
  echo "ERROR: helm is required to test controller priorityClassName" >&2
  exit 1
fi

YQ="${YQ:-}"
if [[ -z "${YQ}" ]]; then
  if [[ -x "${REPO_ROOT}/bin/yq" ]]; then
    YQ="${REPO_ROOT}/bin/yq"
  elif command -v yq >/dev/null 2>&1; then
    YQ="$(command -v yq)"
  else
    echo "ERROR: yq is required to test controller priorityClassName" >&2
    exit 1
  fi
fi

priority_class_name() {
  local chart="$1"
  local deployment="$2"
  shift 2

  helm template priority-class-test "${chart}" --namespace kserve "$@" \
    | "${YQ}" eval-all "select(.kind == \"Deployment\" and .metadata.name == \"${deployment}\") | .spec.template.spec.priorityClassName // \"ABSENT\""
}

expect_absent() {
  local label="$1"
  local actual="$2"
  if [[ "${actual}" != "ABSENT" ]]; then
    echo "ERROR: ${label}: expected priorityClassName to be omitted, got '${actual}'" >&2
    exit 1
  fi
  echo "✓ ${label}"
}

expect_value() {
  local label="$1"
  local actual="$2"
  local expected="$3"
  if [[ "${actual}" != "${expected}" ]]; then
    echo "ERROR: ${label}: expected priorityClassName '${expected}', got '${actual}'" >&2
    exit 1
  fi
  echo "✓ ${label}"
}

expect_stable_upgrade() {
  local chart="$1"
  local set_arg="$2"
  local first second
  first="$(mktemp)"
  second="$(mktemp)"
  helm template priority-class-test "${chart}" --namespace kserve --set "${set_arg}" >"${first}"
  helm template priority-class-test "${chart}" --namespace kserve --set "${set_arg}" >"${second}"
  if ! cmp -s "${first}" "${second}"; then
    echo "ERROR: ${chart}: repeated helm template output differs, so a Helm upgrade would not preserve priorityClassName" >&2
    rm -f "${first}" "${second}"
    exit 1
  fi
  rm -f "${first}" "${second}"
  echo "✓ ${chart}: helm template is stable for ${set_arg}"
}

echo "Checking controller priorityClassName Helm rendering..."

kserve_chart="${REPO_ROOT}/charts/kserve-resources"
llmisvc_chart="${REPO_ROOT}/charts/kserve-llmisvc-resources"

kserve_default="$(priority_class_name "${kserve_chart}" "kserve-controller-manager")"
expect_absent "kserve-controller-manager default" "${kserve_default}"

kserve_empty="$(priority_class_name "${kserve_chart}" "kserve-controller-manager" --set-string kserve.controller.priorityClassName=)"
expect_absent "kserve-controller-manager empty value" "${kserve_empty}"

kserve_set="$(priority_class_name "${kserve_chart}" "kserve-controller-manager" --set kserve.controller.priorityClassName=kserve-control-plane)"
expect_value "kserve-controller-manager configured value" "${kserve_set}" "kserve-control-plane"

llmisvc_default="$(priority_class_name "${llmisvc_chart}" "llmisvc-controller-manager")"
expect_absent "llmisvc-controller-manager default" "${llmisvc_default}"

llmisvc_empty="$(priority_class_name "${llmisvc_chart}" "llmisvc-controller-manager" --set-string kserve.llmisvc.controller.priorityClassName=)"
expect_absent "llmisvc-controller-manager empty value" "${llmisvc_empty}"

llmisvc_set="$(priority_class_name "${llmisvc_chart}" "llmisvc-controller-manager" --set kserve.llmisvc.controller.priorityClassName=kserve-control-plane)"
expect_value "llmisvc-controller-manager configured value" "${llmisvc_set}" "kserve-control-plane"

expect_stable_upgrade "${kserve_chart}" "kserve.controller.priorityClassName=kserve-control-plane"
expect_stable_upgrade "${llmisvc_chart}" "kserve.llmisvc.controller.priorityClassName=kserve-control-plane"

echo "✓ Controller priorityClassName Helm rendering passed"
