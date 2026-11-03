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
import sys
import time
import os

import ray
import requests
from kserve.logging import logger


def initialize_ray_cluster(ray_address="auto"):
    try:
        if ray.is_initialized():  # Check if Ray is already initialized
            return
        else:
            ray.init(address=ray_address)
            return
    except Exception as e:
        logger.error(f"Failed to initialize Ray: {e}")
        sys.exit(1)


def show_ray_cluster_status(ray_address="auto"):
    initialize_ray_cluster(ray_address)
    try:
        resources = ray.cluster_resources()
        logger.info("Cluster resources:", resources)
    except Exception as e:
        logger.error(f"Error getting Ray nodes status: {e}")


def verify_status(result, probe_type):
    if result in ("Healthy", True):
        logger.info(f"{probe_type} Probe: Healthy")
        sys.exit(0)
    else:
        logger.error(f"{probe_type} Probe: Unhealthy")
        sys.exit(1)


# Function for startup check using Ray API
def check_registered_node_and_runtime_health(
    ray_node_count, health_check_url, ray_address="auto"
):
    initialize_ray_cluster(ray_address)
    # Check if the registered nodes count matches RAY_NODE_COUNT
    check_registered_nodes_status = check_registered_nodes(ray_node_count, ray_address)

    # Check if server health return 200
    check_runtime_health_status = check_runtime_health(health_check_url)
    logger.debug(
        f"check_registered_nodes_status: {check_registered_nodes_status},check_runtime_health_status: {check_runtime_health_status}"
    )

    if (
        check_registered_nodes_status == "Healthy"
        and check_runtime_health_status == "Healthy"
    ):
        return "Healthy"
    else:
        return "Unhealthy"


def check_registered_node_and_runtime_models(
    ray_node_count, runtime_url, ray_address, isvc_name
):
    result1 = check_registered_nodes(ray_node_count, ray_address)
    result2 = check_runtime_models(runtime_url, isvc_name)
    logger.debug(f"check_registered_nodes: {result1}, check_runtime_models: {result2}")
    # Check both results
    if (result1 == "Healthy" or result1 is True) and (
        result2 == "Healthy" or result2 is True
    ):
        return "Healthy"
    else:
        return "Unhealthy"


def check_gpu_usage(ray_address="auto"):
    try:
        initialize_ray_cluster(ray_address)
        nodes = ray.nodes()
        total_gpus = 0
        used_gpus = 0
        for node in nodes:
            total_gpus += node["Resources"].get("GPU", 0)
            used_gpus += node["Resources"].get("GPU_group_0", 0)

        # Determine health status based on GPU usage
        if total_gpus == 0 or total_gpus != used_gpus:
            logger.error(
                f"GPU Usage: Unhealthy - Used: {used_gpus}, Total: {total_gpus}"
            )
            return "Unhealthy"
        else:
            logger.info(f"GPU Usage: Healthy - Used: {used_gpus}, Total: {total_gpus}")
            return "Healthy"
    except Exception as e:
        logger.error(f"GPU Usage: Error - Failed to get GPU status: {str(e)}")
        return "Unhealthy"


def check_registered_nodes(ray_node_count, ray_address="auto", retries=0, interval=2):
    try:
        ray_node_count = int(ray_node_count)  # Ensure it's an integer
    except ValueError:
        logger.error(f"Invalid ray_node_count: {ray_node_count}")
        return "Unhealthy"

    for attempt in range(1, retries + 2):
        try:
            initialize_ray_cluster(ray_address)
            # Get list of alive nodes
            nodes = ray.nodes()
            registered_node_count = len([node for node in nodes if node["Alive"]])
            logger.debug(
                f"registered_node_count: {registered_node_count}, ray_node_count: {ray_node_count}"
            )
            # Check if the registered nodes count matches RAY_NODE_COUNT
            if not registered_node_count >= ray_node_count:
                logger.error(
                    f"Waiting - Registered nodes count ({registered_node_count}) does not match RAY_NODE_COUNT ({ray_node_count})."
                )
            else:
                logger.info(
                    f"Success - Registered nodes count ({registered_node_count}) matches RAY_NODE_COUNT ({ray_node_count})."
                )
                return "Healthy"

        except Exception as e:
            logger.error(f"Error checking registered nodes: {str(e)}")

        if attempt < retries:
            time.sleep(interval)
    logger.error(
        "Max retries reached. Node count did not match the expected Ray node count."
    )
    return "Unhealthy"


def check_runtime_health(health_check_url, retries=1, interval=1):
    # Check if runtime server health
    for attempt in range(1, retries + 2):
        try:
            response = requests.get(health_check_url, timeout=5)
            if response.status_code != 200:
                logger.error(f"Server({health_check_url}) did not return 200 code.")
            else:
                logger.info(f"Server({health_check_url}) is reachable.")
                return "Healthy"
        except requests.RequestException:
            logger.error(f"Server({health_check_url}) is not reachable.")

        if attempt < retries:
            time.sleep(interval)

    return "Unhealthy"


def check_runtime_models(health_check_url, isvc_name, retries=1, interval=1):
    # Check if runtime server health
    for attempt in range(1, retries + 2):
        try:
            response = requests.get(health_check_url, timeout=5)
            if isvc_name in response.text:
                logger.info(f"Model({isvc_name}) is Ready to serve")
                return True
            else:
                logger.error(f"Model({isvc_name}) is Not ready to serve")
        except requests.RequestException:
            logger.error(f"Server({health_check_url}) is not reachable.")

        if attempt < retries:
            time.sleep(interval)

    return False


# Main logic to handle CLI commands using argparse
def main():
    # Get default values from environment variables if available
    default_ray_address = os.getenv("RAY_ADDRESS", "auto")
    default_isvc_name = os.getenv("ISVC_NAME", "")
    default_ray_node_count = int(
        os.getenv("RAY_NODE_COUNT", 2)
    )  # Default to 2 if not set

    # Create the top-level parser
    parser = argparse.ArgumentParser(description="Perform multinode operations")
    parser.add_argument(
        "--ray_address", default=default_ray_address, help="Ray head address"
    )
    parser.add_argument(
        "--isvc_name", default=default_isvc_name, help="InferenceService name"
    )

    # Define subcommands (readiness,,liveness, startup, gpu_usage, registered_nodes)
    subparsers = parser.add_subparsers(dest="command", help="Sub-command to run")

    # Check runtime health subcommand
    runtime_health_parser = subparsers.add_parser(
        "runtime_health", help="Check runtime health"
    )
    runtime_health_parser.add_argument("--health_check_url", help="Health check URL")
    runtime_health_parser.add_argument("--probe_name", help="Probe name")

    # Check if registered node is the same as rayNodeCount
    reigstered_node_parser = subparsers.add_parser(
        "registered_nodes",
        help="Check if registered nodes are the same as Ray node count",
    )
    reigstered_node_parser.add_argument(
        "--ray_node_count",
        type=int,
        default=default_ray_node_count,
        help="Ray node count",
    )
    reigstered_node_parser.add_argument(
        "--retries", type=int, default=0, help="Ray node count"
    )
    reigstered_node_parser.add_argument("--probe_name", help="Probe name")

    # Check if registered node is the same as rayNodeCount/ runtime health subcommand
    registered_node_and_runtime_health_parser = subparsers.add_parser(
        "registered_node_and_runtime_health",
        help="Check node counts and runtime health",
    )
    registered_node_and_runtime_health_parser.add_argument(
        "--ray_node_count",
        type=int,
        default=default_ray_node_count,
        help="Ray node count",
    )
    registered_node_and_runtime_health_parser.add_argument(
        "--health_check_url", help="Health check URL"
    )
    registered_node_and_runtime_health_parser.add_argument(
        "--probe_name", help="Probe name"
    )

    # Check if registered node is the same as rayNodeCount/ model loaded on runtime subcommand
    registered_node_and_runtime_models_parser = subparsers.add_parser(
        "registered_node_and_runtime_models",
        help="Check node counts and loaded model on runtime",
    )
    registered_node_and_runtime_models_parser.add_argument(
        "--ray_node_count",
        type=int,
        default=default_ray_node_count,
        help="Ray node count",
    )
    registered_node_and_runtime_models_parser.add_argument(
        "--runtime_url", help="Health check URL"
    )
    registered_node_and_runtime_models_parser.add_argument(
        "--probe_name", help="Probe name"
    )

    # Parse the arguments
    args = parser.parse_args()

    # Route to appropriate function based on command using if-elif-else
    if args.command == "runtime_health":
        result = check_runtime_health(args.health_check_url)
        verify_status(result, args.probe_name)
    elif args.command == "registered_node_and_runtime_health":
        result = check_registered_node_and_runtime_health(
            args.ray_node_count, args.health_check_url, args.ray_address
        )
        verify_status(result, args.probe_name)
    elif args.command == "registered_node_and_runtime_models":
        result = check_registered_node_and_runtime_models(
            args.ray_node_count,
            args.runtime_url,
            args.ray_address,
            args.isvc_name,
        )
        verify_status(result, args.probe_name)
    elif args.command == "registered_nodes":
        result = check_registered_nodes(
            args.ray_node_count, args.ray_address, args.retries
        )
        verify_status(result, args.probe_name)
    else:
        parser.print_help()


if __name__ == "__main__":
    main()
