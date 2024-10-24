# Copyright 2023 The KServe Authors.
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

import argparse
import ray
import requests
import sys


def initialize_ray_cluster():
    if not ray.is_initialized():  # Check if Ray is already initialized
        ray.init(address="auto")
        return "Ray initialized"
    else:
        return "Ray already initialized"


def verify_status(result):
    if result == "Healthy":
        sys.exit(0)
    else:
        sys.exit(1)


# Function for startup check using Ray API
def check_startup():
    try:
        initialize_ray_cluster()
        print("Ray is accessible")
        return "Healthy"
    except Exception as e:
        print(f"Ray is NOT accessible: {e}")
        return "Unhealthy"


def check_gpu_usage(probe_type):
    try:
        initialize_ray_cluster()
        nodes = ray.nodes()
        total_gpus = 0
        used_gpus = 0
        for node in nodes:
            total_gpus += node["Resources"].get("GPU", 0)
            used_gpus += node["Resources"].get("GPU_group_0", 0)

        # Determine health status based on GPU usage
        if total_gpus == 0 or total_gpus != used_gpus:
            print(f"{probe_type}: Unhealthy - Used: {used_gpus}, Total: {total_gpus}")
            return "Unhealthy"
        else:
            print(f"{probe_type}: Healthy - Used: {used_gpus}, Total: {total_gpus}")
            return "Healthy"
    except Exception as e:
        print(f"{probe_type}: Error - Failed to get GPU status: {str(e)}")
        return "Unhealthy"


def check_registered_nodes(pipeline_parallel_size):
    try:
        initialize_ray_cluster()
        # Get list of alive nodes
        nodes = ray.nodes()
        registered_node_count = len([node for node in nodes if node["Alive"]])

        # Check if the registered nodes count matches PIPELINE_PARALLEL_SIZE
        if registered_node_count != int(pipeline_parallel_size):
            print(
                f"Unhealthy - Registered nodes count ({registered_node_count}) does not match PIPELINE_PARALLEL_SIZE ({pipeline_parallel_size})."
            )
            return "Unhealthy"
        else:
            print(
                f"Healthy - Registered nodes count ({registered_node_count}) match PIPELINE_PARALLEL_SIZE ({pipeline_parallel_size})."
            )
            return "Healthy"
    except Exception as e:
        print(f"Error checking registered nodes: {str(e)}")
        return "Unhealthy"


def check_runtime_health(health_check_url):
    # Check if Huggingface server health
    try:
        response = requests.get(health_check_url, timeout=5)
        if response.status_code != 200:
            print(f"Hugging Face server({health_check_url}) is not reachable.")
            return "Unhealthy"
        else:
            return "Healthy"
    except requests.RequestException:
        print(f"Hugging Face server({health_check_url}) is not reachable.")
        return "Unhealthy"


def check_readiness(pipeline_parallel_size, health_check_url):
    # Check if the registered nodes count matches PIPELINE_PARALLEL_SIZE
    check_registered_nodes_status = check_registered_nodes(pipeline_parallel_size)

    # Check GPU usage
    check_gpu_usage_status = check_gpu_usage("Readiness Probe")

    # Check if Huggingface server health
    check_runtime_health_status = check_runtime_health(health_check_url)

    if (
        check_registered_nodes_status == "Healthy"
        and check_gpu_usage_status == "Healthy"
        and check_runtime_health_status == "Healthy"
    ):
        print("Readiness Probe: Healthy")
        return "Healthy"
    else:
        print("Readiness Probe: Unhealthy")
        return "Unhealthy"


# Main logic to handle CLI commands using argparse
def main():
    # Create the top-level parser
    parser = argparse.ArgumentParser(description="Perform multinode health checks.")

    # Define subcommands (readiness, startup, gpu_usage, registered_nodes)
    subparsers = parser.add_subparsers(dest="command", help="Sub-command to run")

    # Readiness subcommand
    readiness_parser = subparsers.add_parser(
        "readiness", help="Perform readiness check"
    )
    readiness_parser.add_argument(
        "pipeline_parallel_size", type=int, help="Pipeline parallel size"
    )
    readiness_parser.add_argument("health_check_url", help="Health check URL")

    # Liveness subcommand
    subparsers.add_parser("liveness", help="Perform liveness check")
    # Startup subcommand
    subparsers.add_parser("startup", help="Perform startup check")
    # GPU Usage subcommand
    subparsers.add_parser("gpu_usage", help="Check GPU usage")

    # Registered Nodes subcommand
    registered_nodes_parser = subparsers.add_parser(
        "registered_nodes", help="Check registered nodes"
    )
    registered_nodes_parser.add_argument(
        "pipeline_parallel_size", type=int, help="Pipeline parallel size"
    )

    # Parse the arguments
    args = parser.parse_args()

    # Route to appropriate function based on command using if-elif-else
    if args.command == "readiness":
        result = check_readiness(args.pipeline_parallel_size, args.health_check_url)
        verify_status(result)
    elif args.command == "startup":
        result = check_startup()
        verify_status(result)
    elif args.command == "liveness":
        result = check_gpu_usage("Liveness Probe")
        verify_status(result)
    elif args.command == "gpu_usage":
        result = check_gpu_usage("GPU Usage")
        verify_status(result)
    elif args.command == "registered_nodes":
        result = check_registered_nodes(args.pipeline_parallel_size)
        verify_status(result)
    else:
        parser.print_help()


if __name__ == "__main__":
    main()
