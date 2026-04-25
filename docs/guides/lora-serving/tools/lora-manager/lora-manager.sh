#!/bin/bash
# lora-manager — load, unload, or swap LoRA adapters on vLLM pods.
#
# Targets a single pod (POD_NAME) or all pods matching an LLMInferenceService.
# This lets you place different adapters on different replicas — the LoRA
# affinity scorer will route traffic accordingly.
#
# Commands:
#   load   — load an adapter
#   unload — unload an adapter
#   swap   — unload one adapter and load another
#
# Environment:
#   POD_NAME             — target a specific pod (recommended for per-replica control)
#   LLMISVC_NAME         — target all pods of an LLMInferenceService (used when POD_NAME is unset)
#   ADAPTER_NAME         — adapter name (required for load/unload)
#   ADAPTER_SOURCE       — HuggingFace repo or path (required for load)
#   LOAD_ADAPTER         — adapter to load (required for swap)
#   LOAD_ADAPTER_SOURCE  — source for adapter to load (required for swap)
#   UNLOAD_ADAPTER       — adapter to unload (required for swap)
#   PORT                 — vLLM port (default: 8000)

set -euo pipefail

COMMAND="${1:?Usage: lora-manager <load|unload|swap>}"
PORT="${PORT:-8000}"

# --- Kubernetes API plumbing ---
APISERVER="https://kubernetes.default.svc"
TOKEN=$(cat /var/run/secrets/kubernetes.io/serviceaccount/token)
NAMESPACE=$(cat /var/run/secrets/kubernetes.io/serviceaccount/namespace)
CACERT="/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"

kube_get() {
  curl -s --cacert "${CACERT}" -H "Authorization: Bearer ${TOKEN}" \
    "${APISERVER}/api/v1/namespaces/${NAMESPACE}/$1"
}

# --- Resolve target pod IP(s) ---
resolve_pod_ips() {
  if [ -n "${POD_NAME:-}" ]; then
    echo "Targeting pod: ${POD_NAME}"
    local pod_json
    pod_json=$(kube_get "pods/${POD_NAME}")
    local ip
    ip=$(echo "${pod_json}" | jq -r '.status.podIP // empty')
    if [ -z "${ip}" ]; then
      echo "ERROR: Pod '${POD_NAME}' not found or has no IP"
      exit 1
    fi
    echo "${ip}"
  else
    : "${LLMISVC_NAME:?Either POD_NAME or LLMISVC_NAME is required}"
    echo "Targeting all pods for LLMInferenceService: ${LLMISVC_NAME}"
    local selector="app.kubernetes.io/part-of=llminferenceservice,app.kubernetes.io/name=${LLMISVC_NAME}"
    local pods_json
    pods_json=$(kube_get "pods?labelSelector=${selector}")
    local ips
    ips=$(echo "${pods_json}" | jq -r '.items[].status.podIP // empty')
    if [ -z "${ips}" ]; then
      echo "ERROR: No pods found for LLMInferenceService '${LLMISVC_NAME}'"
      exit 1
    fi
    echo "${ips}"
  fi
}

POD_IPS=$(resolve_pod_ips)
echo "Pod IPs: ${POD_IPS}"

# --- Adapter operations ---
load_adapter() {
  local ip="$1" name="$2" source="$3"
  echo "  Loading '${name}' on ${ip}..."
  local code
  code=$(curl -s -o /dev/null -w "%{http_code}" -k \
    -X POST "https://${ip}:${PORT}/v1/load_lora_adapter" \
    -H "Content-Type: application/json" \
    -d "{\"lora_name\": \"${name}\", \"lora_path\": \"${source}\"}")
  if [ "${code}" -eq 200 ]; then
    echo "  OK"
  else
    echo "  FAILED (HTTP ${code})"
    return 1
  fi
}

unload_adapter() {
  local ip="$1" name="$2"
  echo "  Unloading '${name}' from ${ip}..."
  local code
  code=$(curl -s -o /dev/null -w "%{http_code}" -k \
    -X POST "https://${ip}:${PORT}/v1/unload_lora_adapter" \
    -H "Content-Type: application/json" \
    -d "{\"lora_name\": \"${name}\"}")
  if [ "${code}" -eq 200 ]; then
    echo "  OK"
  else
    echo "  FAILED (HTTP ${code})"
    return 1
  fi
}

# --- Run the requested command ---
FAILED=0

case "${COMMAND}" in
  load)
    : "${ADAPTER_NAME:?ADAPTER_NAME is required for load}"
    : "${ADAPTER_SOURCE:?ADAPTER_SOURCE is required for load}"
    for ip in ${POD_IPS}; do
      load_adapter "${ip}" "${ADAPTER_NAME}" "${ADAPTER_SOURCE}" || FAILED=$((FAILED + 1))
    done
    ;;

  unload)
    : "${ADAPTER_NAME:?ADAPTER_NAME is required for unload}"
    for ip in ${POD_IPS}; do
      unload_adapter "${ip}" "${ADAPTER_NAME}" || FAILED=$((FAILED + 1))
    done
    ;;

  swap)
    : "${UNLOAD_ADAPTER:?UNLOAD_ADAPTER is required for swap}"
    : "${LOAD_ADAPTER:?LOAD_ADAPTER is required for swap}"
    : "${LOAD_ADAPTER_SOURCE:?LOAD_ADAPTER_SOURCE is required for swap}"
    for ip in ${POD_IPS}; do
      echo "=== Pod ${ip} ==="
      unload_adapter "${ip}" "${UNLOAD_ADAPTER}" || true
      load_adapter "${ip}" "${LOAD_ADAPTER}" "${LOAD_ADAPTER_SOURCE}" || FAILED=$((FAILED + 1))
    done
    ;;

  *)
    echo "Unknown command: ${COMMAND}"
    echo "Usage: lora-manager <load|unload|swap>"
    exit 1
    ;;
esac

if [ "${FAILED}" -gt 0 ]; then
  echo "ERROR: ${FAILED} pod(s) failed"
  exit 1
fi

echo "Done."
