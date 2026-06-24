package llmisvc

import (
	"encoding/json"

	corev1 "k8s.io/api/core/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

// rdmaDetectStr auto-detects RDMA/InfiniBand HCAs
// exports NCCL_IB_HCA, NVSHMEM_HCA_LIST, UCX_NET_DEVICES, and *_GID_INDEX.
// Gated by KSERVE_INFER_ROCE env var.
const rdmaDetectStr = `if [ "$KSERVE_INFER_ROCE" = "true" ]; then
  echo "Trying to infer RoCE configs ... "
  grep -H . /sys/class/infiniband/*/ports/*/gids/* 2>/dev/null
  grep -H . /sys/class/infiniband/*/ports/*/gid_attrs/types/* 2>/dev/null

  cat /proc/driver/nvidia/params

  KSERVE_INFER_IB_GID_INDEX_GREP=${KSERVE_INFER_IB_GID_INDEX_GREP:-"RoCE v2"}

  echo "[Infer RoCE] Discovering active HCAs ..."
  active_hcas=()
  # Loop through all mlx5 devices found in sysfs
  for hca_dir in /sys/class/infiniband/mlx5_*; do
      # Ensure it's a directory before proceeding
      if [ -d "$hca_dir" ]; then
          hca_name=$(basename "$hca_dir")
          port_state_file="$hca_dir/ports/1/state" # Assume port 1
          type_file="$hca_dir/ports/1/gid_attrs/types/*"

          echo "[Infer RoCE] Check if the port state file ${port_state_file} exists and contains 'ACTIVE'"
          if [ -f "$port_state_file" ] && grep -q "ACTIVE" "$port_state_file" && grep -q "${KSERVE_INFER_IB_GID_INDEX_GREP}" ${type_file} 2>/dev/null; then
              echo "[Infer RoCE] Found active HCA: $hca_name"
              active_hcas+=("$hca_name")
          else
              echo "[Infer RoCE] Skipping inactive or down HCA: $hca_name"
          fi
      fi
  done

  # Check if we found any active HCAs
  if [ ${#active_hcas[@]} -gt 0 ]; then
      # Join the array elements with a comma
      hca_port_pairs=()
      for hca in "${active_hcas[@]}"; do
        hca_port_pairs+=("${hca}:1")
      done

      active_hca_list=$(IFS=,; echo "${active_hcas[*]}")
      hca_port_pairs_list=$(IFS=,; echo "${hca_port_pairs[*]}")
      echo "[Infer RoCE] Setting active HCAs: ${active_hca_list}"
      export NCCL_IB_HCA=${NCCL_IB_HCA:-${active_hca_list}}
      export NVSHMEM_HCA_LIST=${NVSHMEM_HCA_LIST:-${hca_port_pairs_list}}
      export UCX_NET_DEVICES=${UCX_NET_DEVICES:-${hca_port_pairs_list}}

      echo "[Infer RoCE] NCCL_IB_HCA=${NCCL_IB_HCA}"
      echo "[Infer RoCE] NVSHMEM_HCA_LIST=${NVSHMEM_HCA_LIST}"
      echo "[Infer RoCE] UCX_NET_DEVICES=${UCX_NET_DEVICES}"
  else
      echo "[Infer RoCE] WARNING: No active RoCE HCAs found. NCCL_IB_HCA will not be set."
  fi

  if [ ${#active_hcas[@]} -gt 0 ]; then
      echo "[Infer RoCE] Finding GID_INDEX for each active HCA (SR-IOV compatible)..."

      # For SR-IOV environments, find the most common IPv4 RoCE v2 GID index across all HCAs
      declare -A gid_index_count
      declare -A hca_gid_index

      for hca_name in "${active_hcas[@]}"; do
          echo "[Infer RoCE] Processing HCA: ${hca_name}"

          # Find all RoCE v2 IPv4 GIDs for this HCA and count by index
          for tpath in /sys/class/infiniband/${hca_name}/ports/1/gid_attrs/types/*; do
              if grep -q "${KSERVE_INFER_IB_GID_INDEX_GREP}" "$tpath" 2>/dev/null; then
                  idx=$(basename "$tpath")
                  gid_file="/sys/class/infiniband/${hca_name}/ports/1/gids/${idx}"
                  # Check for IPv4 GID (contains ffff:)
                  if [ -f "$gid_file" ] && grep -q "ffff:" "$gid_file"; then
                      gid_value=$(cat "$gid_file" 2>/dev/null || echo "")
                      echo "[Infer RoCE] Found IPv4 RoCE v2 GID for ${hca_name}: index=${idx}, gid=${gid_value}"
                      hca_gid_index["${hca_name}"]="${idx}"
                      gid_index_count["${idx}"]=$((${gid_index_count["${idx}"]} + 1))
                      break  # Use first found IPv4 GID per HCA
                  fi
              fi
          done
      done

      # Find the most common GID index (most likely to be consistent across nodes)
      best_gid_index=""
      max_count=0
      for idx in "${!gid_index_count[@]}"; do
          count=${gid_index_count["${idx}"]}
          echo "[Infer RoCE] GID_INDEX ${idx} found on ${count} HCAs"
          if [ $count -gt $max_count ]; then
              max_count=$count
              best_gid_index="$idx"
          fi
      done

      # Use deterministic fallback if tied - prefer index 3 (SR-IOV standard)
      if [ ${#gid_index_count[@]} -gt 1 ]; then
          echo "[Infer RoCE] Multiple GID indices found, selecting most common: ${best_gid_index}"
          # If there's a tie, prefer index 3 as it's most common in SR-IOV setups
          if [ -n "${gid_index_count['3']}" ] && [ "${gid_index_count['3']}" -eq "$max_count" ]; then
              best_gid_index="3"
              echo "[Infer RoCE] Using deterministic fallback: GID_INDEX=3 (SR-IOV standard)"
          fi
      fi

      # Check if GID_INDEX is already set via environment variables
      if [ -n "${NCCL_IB_GID_INDEX}" ]; then
          echo "[Infer RoCE] Using pre-configured NCCL_IB_GID_INDEX=${NCCL_IB_GID_INDEX} from environment"
          export NVSHMEM_IB_GID_INDEX=${NVSHMEM_IB_GID_INDEX:-$NCCL_IB_GID_INDEX}
          export UCX_IB_GID_INDEX=${UCX_IB_GID_INDEX:-$NCCL_IB_GID_INDEX}
          echo "[Infer RoCE] Using pre-configured GID_INDEX=${NCCL_IB_GID_INDEX} for NCCL, NVSHMEM, and UCX"
      elif [ -n "$best_gid_index" ]; then
          echo "[Infer RoCE] Selected GID_INDEX: ${best_gid_index} (found on ${max_count} HCAs)"

          export NCCL_IB_GID_INDEX=${NCCL_IB_GID_INDEX:-$best_gid_index}
          export NVSHMEM_IB_GID_INDEX=${NVSHMEM_IB_GID_INDEX:-$best_gid_index}
          export UCX_IB_GID_INDEX=${UCX_IB_GID_INDEX:-$best_gid_index}

          echo "[Infer RoCE] Exported GID_INDEX=${best_gid_index} for NCCL, NVSHMEM, and UCX"
      else
          echo "[Infer RoCE] ERROR: No valid IPv4 ${KSERVE_INFER_IB_GID_INDEX_GREP} GID_INDEX found on any HCA."
      fi
  else
      echo "[Infer RoCE] No active HCAs found, skipping GID_INDEX inference."
  fi
fi`


// dpAddressResolveStr retries DNS for LWS_LEADER_ADDRESS into DP_ADDRESS for ZMQ
const dpAddressResolveStr = `# In some versions, ZMQ bind doesn't resolve the address through DNS
# Retry DP_ADDRESS resolution (configurable attempts, default 30)
RESOLVE_ATTEMPTS=${DP_ADDRESS_RESOLVE_ATTEMPTS:-30}
for ((i=1; i<=RESOLVE_ATTEMPTS; i++)); do
  DP_ADDRESS=$(getent hosts ${LWS_LEADER_ADDRESS} | cut -d' ' -f1)
  if [ -n "$DP_ADDRESS" ]; then
    echo "DP_ADDRESS=${DP_ADDRESS} (resolved on attempt $i)"
    break
  else
    echo "DP_ADDRESS resolution failed on attempt $i, retrying..."
    sleep 1
  fi
done

if [ -z "$DP_ADDRESS" ]; then
  echo "WARNING: Failed to resolve DP_ADDRESS after ${RESOLVE_ATTEMPTS} attempts, falling back to LWS_LEADER_ADDRESS"
  DP_ADDRESS=${LWS_LEADER_ADDRESS}
  echo "DP_ADDRESS=${DP_ADDRESS} (fallback)"
fi`

// vllmVersionStr detects the running vLLM version and exports VLLM_VERSION.
const vllmVersionStr = `VLLM_VERSION=$(vllm --version 2>/dev/null | tail -1 | awk '{print $NF}')
echo "[vllm-version] vllm version='${VLLM_VERSION}'"`

// vllmAccessLogStr sets ACCESS_LOG_ARGS based on VLLM_VERSION.
const vllmAccessLogStr = `# --disable-access-log-for-endpoints landed in vLLM 0.16.0 (vllm-project/vllm#30011).
# Older versions still need the blanket --disable-uvicorn-access-log.
ACCESS_LOG_ARGS="--disable-uvicorn-access-log"
if [[ "$VLLM_VERSION" =~ ^[0-9]+\.[0-9]+ ]] && [ "$(printf '%s\n%s\n' "0.16.0" "${VLLM_VERSION}" | sort -V | head -1)" = "0.16.0" ]; then
  ACCESS_LOG_ARGS="--disable-access-log-for-endpoints /health,/metrics,/ping"
fi
echo "[access-log-detect] selected ACCESS_LOG_ARGS='${ACCESS_LOG_ARGS}'"`

// shutdownTimeoutStr sets SHUTDOWN_TIMEOUT_ARGS based on VLLM_VERSION
const shutdownTimeoutStr = `# --shutdown-timeout landed in vLLM 0.18.0 (vllm-project/vllm#36666).
SHUTDOWN_TIMEOUT_ARGS=""
if [[ "$VLLM_VERSION" =~ ^[0-9]+\.[0-9]+ ]] && [ "$(printf '%s\n%s\n' "0.18.0" "${VLLM_VERSION}" | sort -V | head -1)" = "0.18.0" ]; then
  SHUTDOWN_TIMEOUT_ARGS="--shutdown-timeout SHUTDOWN_TIMEOUT_VALUE"
fi`

// computeShutdownTimeout derives the vLLM --shutdown-timeout value from a
// *corev1.PodSpec (or nil): max(0, tgps - preStop - min(5, tgps)).
// tgps defaults to 60 when unset. The 5-second buffer reserves time for signal
// propagation and final process cleanup before Kubernetes sends SIGKILL.
// escapeForJSON returns s with characters escaped for safe embedding inside
// a JSON string value (without the surrounding quotes). This is necessary
// because ReplaceVariables renders Go templates over the JSON-marshalled config,
// so template output lands inside JSON string literals.
func computeShutdownTimeout(spec any, preStop int64) int64 {
	const defaultTGPS = int64(60)
	var tgpsVal int64
	if spec != nil {
		if ps, ok := spec.(*corev1.PodSpec); ok && ps != nil && ps.TerminationGracePeriodSeconds != nil {
			tgpsVal = *ps.TerminationGracePeriodSeconds
		} else {
			tgpsVal = defaultTGPS
		}
	} else {
		tgpsVal = defaultTGPS
	}
	buf := min(int64(5), tgpsVal)
	result := tgpsVal - preStop - buf
	if result < 0 {
		return 0
	}
	return result
}

// kvTransferStr sets KV_TRANSFER_ARGS based on VLLM_VERSION and the resolved kv-transfer-config.
const kvTransferStr = `# --kv-transfer-config with OffloadingConnector requires vLLM 0.22.0+ (vllm-project/vllm#40020).
KV_TRANSFER_ARGS=""
if [[ "$VLLM_VERSION" =~ ^[0-9]+\.[0-9]+ ]] && [ "$(printf '%s\n%s\n' "0.22.0" "${VLLM_VERSION}" | sort -V | head -1)" = "0.22.0" ]; then
  if [[ "${VLLM_ADDITIONAL_ARGS:-}" != *"--kv-transfer-config"* ]] && [[ "${VLLM_ADDITIONAL_ARGS:-}" != *"--kv_transfer_config"* ]] && [[ "$*" != *"--kv-transfer-config"* ]] && [[ "$*" != *"--kv_transfer_config"* ]]; then
    KV_TRANSFER_ARGS="KV_TRANSFER_CONFIG_VALUE"
  fi
fi`

// computeKVTransferConfig builds the --kv-transfer-config on vllm 
// return "" when kv cache offloading is not configured.
func computeKVTransferConfig(spec any) string {
	if spec == nil {
		return ""
	}
	kv, ok := spec.(*v1alpha2.KVCacheOffloadingSpec)
	if !ok || kv == nil {
		return ""
	}
	extraConfig := map[string]any{
		"spec_name":        "TieringOffloadingSpec",
		"cpu_bytes_to_use": kv.CPU.Value(),
	}
	if kv.EvictionPolicy != "" {
		extraConfig["eviction_policy"] = kv.EvictionPolicy
	}
	kvConfig := map[string]any{
		"kv_connector":              "OffloadingConnector",
		"kv_role":                   "kv_both",
		"kv_connector_extra_config": extraConfig,
	}
	kc, err := json.Marshal(kvConfig)
	if err != nil {
		return ""
	}
	return "--kv-transfer-config '" + string(kc) + "'"
}

// escapeForJSON JSON-marshals s and strips the surrounding quotes.
func escapeForJSON(s string) string {
	b, _ := json.Marshal(s)
	return string(b[1 : len(b)-1])
}