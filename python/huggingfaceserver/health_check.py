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

# Initialize Ray API
ray.init(address="auto")


def check_gpu_usage(probe_type):
    try:
        # Fetch cluster resources from Ray API
        cluster_resources = ray.cluster_resources()
        available_gpus = cluster_resources.get("GPU", 0)
        used_gpus = cluster_resources.get("GPU", 0) - cluster_resources.get(
            "GPU_group_0", 0
        )

        # Determine health status based on GPU usage
        if used_gpus != available_gpus:
            print(
                f"{probe_type}: Unhealthy - Used: {used_gpus}, Available: {available_gpus}"
            )
            sys.exit(1)
        print(f"{probe_type}: Healthy - Used: {used_gpus}, Available: {available_gpus}")
    except Exception as e:
        print(f"{probe_type}: Error - Failed to get GPU status: {str(e)}")
        sys.exit(1)


def check_registered_nodes(pipeline_parallel_size):
    try:
        # Get list of alive nodes
        nodes = ray.nodes()
        registered_node_count = len([node for node in nodes if node["Alive"]])

        # Check if the registered nodes count matches PIPELINE_PARALLEL_SIZE
        if registered_node_count != int(pipeline_parallel_size):
            print(
                f"Readiness Probe: Unhealthy - Registered nodes count ({registered_node_count}) does not match PIPELINE_PARALLEL_SIZE ({pipeline_parallel_size})."
            )
            sys.exit(1)
    except Exception as e:
        print(f"Error checking registered nodes: {str(e)}")
        sys.exit(1)


def check_readiness(pipeline_parallel_size, health_check_url):
    # Check if the registered nodes count matches PIPELINE_PARALLEL_SIZE
    check_registered_nodes(pipeline_parallel_size)

    # Check GPU usage
    check_gpu_usage("Readiness Probe")

    # Check if Huggingface server health
    try:
        response = requests.get(health_check_url, timeout=5)
        if response.status_code != 200:
            print(f"Readiness Probe: Unhealthy - Hugging Face server is not reachable.")
            sys.exit(1)
    except requests.RequestException:
        print(f"Readiness Probe: Unhealthy - Hugging Face server is not reachable.")
        sys.exit(1)

    print("Readiness Probe: Healthy")
    sys.exit(0)


def liveness_check():
    check_gpu_usage("Liveness Probe")
    print("Liveness Probe: Healthy")
    sys.exit(0)


# Function for startup check using Ray API
def check_startup():
    try:
        # Check if Ray is running by checking node status
        nodes = ray.nodes()
        print(f"Ray is running with {len(nodes)} nodes.")
        sys.exit(0)
    except Exception as e:
        print(f"Startup Check: Error - Failed to get Ray status: {str(e)}")
        sys.exit(1)


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

    # Route to appropriate function based on command using match-case
    match args.command:
        case "readiness":
            check_readiness(args.pipeline_parallel_size, args.health_check_url)
        case "startup":
            check_startup()
        case "liveness":
            liveness_check()
        case "gpu_usage":
            check_gpu_usage("GPU Usage")
        case "registered_nodes":
            check_registered_nodes(args.pipeline_parallel_size)
        case _:
            parser.print_help()


if __name__ == "__main__":
    main()
