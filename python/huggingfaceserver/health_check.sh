#!/bin/bash

# Function to check GPU usage
check_gpu_usage() {
  gpu_status=$(ray status | grep GPU)
  if [[ -z $gpu_status ]]; then
    echo "$1: GPU does not exist"
    exit 1
  fi

  used_gpu=$(echo "$gpu_status" | awk '{print $1}' | cut -d'/' -f1)
  reserved_gpu=$(echo "$gpu_status" | awk '{print $1}' | cut -d'/' -f2)

  # Determine health status based on GPU usage
  if [[ "$used_gpu" != "$reserved_gpu" ]]; then
    echo "$1: Unhealthy - Used: $used_gpu, Reserved: $reserved_gpu"
    exit 1
  fi
}

check_registered_nodes() {
  local pipeline_parallel_size="$1" # Accept pipeline_parallel_size as a parameter
  local registered_node_count

  # Count the registered ray nodes
  registered_node_count=$(ray status | grep -c node_)

  # Check if the registered nodes count matches PIPELINE_PARALLEL_SIZE
  if [[ $registered_node_count -ne "$pipeline_parallel_size" ]]; then
    echo "Readiness Probe: Unhealthy - Registered nodes count ($registered_node_count) does not match PIPELINE_PARALLEL_SIZE ($pipeline_parallel_size)."
    exit 1
  fi
}

# Function for readiness check
readiness_check() {
  local pipeline_parallel_size="$1"

  # Check if the registered nodes count matches PIPELINE_PARALLEL_SIZE
  check_registered_nodes ${pipeline_parallel_size}

  # Check GPU usage
  check_gpu_usage "Readiness Probe"

  # Check if huggingface server health
  if ! curl --silent --max-time 5 $2; then
    echo "Readiness Probe: Unhealthy - Hugging Face server is not reachable."
    exit 1
  fi

  echo "Readiness Probe: Healthy"
  exit 0
}

# Function for liveness check
liveness_check() {
  # Check GPU usage
  check_gpu_usage "Liveness Probe"

  echo "Liveness Probe: Healthy"
  exit 0
}

# Function for startup check
startup_check() {
  # Check the status of Ray nodes
  ray_status=$(ray status 2>&1) # Capture both stdout and stderr
  if [[ $? -ne 0 ]]; then
    echo "Startup Check: Error - Failed to get Ray status: $ray_status"
    exit 1
  fi

  echo "$ray_status"
  exit 0
}

# Main logic to route the command
case "$1" in
readiness)
  # Check parameter count
  if [ "$#" -lt 3 ]; then
    echo "Error: Insufficient parameters. At least 2 parameters are required.[PIPELINE_PARALLEL_SIZE],[health check api]"
    exit 1
  fi
  readiness "$2" "$3"
  ;;
startup_check)
  startup
  ;;
check_gpu_usage)
  check_gpu_usage
  ;;
check_registered_nodes)
  # Check parameter count
  if [ "$#" -lt 2 ]; then
    echo "Error: Insufficient parameters. At least 1 parameters are required.[PIPELINE_PARALLEL_SIZE]"
    exit 1
  fi
  check_registered_nodes "$2"
  ;;
*)
  echo "Usage: $0 {readiness_check|startup_check|check_gpu_usage|check_registered_nodes} [PIPELINE_PARALLEL_SIZE] [health check api]"
  echo "       $0 readiness_check 2 http://localhost:8080"
  echo "       $0 check_gpu_usage"
  echo "       $0 startup_check"
  echo "       $0 check_registered_nodes 2"
  exit 1
  ;;
esac
