# Copyright 2025 The KServe Authors.
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

from kubernetes import client
from kserve.logging import trace_logger as logger

import copy


def create_or_update_gateway(kserve_client, gateway, namespace=None):
    """Create or update a Gateway resource."""
    gateway = copy.deepcopy(gateway)

    version = gateway["apiVersion"].split("/")[1]

    if namespace is None:
        namespace = gateway.get("metadata", {}).get("namespace", "default")

    name = gateway.get("metadata", {}).get("name")
    if not name:
        raise ValueError("Gateway must have a name in metadata")

    logger.info(f"Checking Gateway {name} in namespace {namespace}")

    try:
        existing_gateway = kserve_client.api_instance.get_namespaced_custom_object(
            "gateway.networking.k8s.io",
            version,
            namespace,
            "gateways",
            name,
        )

        gateway.setdefault("metadata", {})["resourceVersion"] = (
            existing_gateway["metadata"]["resourceVersion"]
        )

        outputs = kserve_client.api_instance.replace_namespaced_custom_object(
            "gateway.networking.k8s.io",
            version,
            namespace,
            "gateways",
            name,
            gateway,
        )
        logger.info(f"✓ Successfully updated Gateway {name}")
        return outputs

    except client.rest.ApiException as e:
        if e.status == 404:  # Not found - create it
            logger.info(f"Resource not found, creating Gateway {name}")
            outputs = kserve_client.api_instance.create_namespaced_custom_object(
                "gateway.networking.k8s.io",
                version,
                namespace,
                "gateways",
                gateway,
            )
            logger.info(f"✓ Successfully created Gateway {name}")
            return outputs
        raise


def delete_gateway(
    kserve_client, name, namespace, version="v1"
):
    try:
        logger.info(f"Deleting Gateway {name} in namespace {namespace}")
        return kserve_client.api_instance.delete_namespaced_custom_object(
            "gateway.networking.k8s.io",
            version,
            namespace,
            "gateways",
            name,
        )
    except client.rest.ApiException as e:
        raise RuntimeError(
            f"Exception when calling CustomObjectsApi->"
            f"delete_namespaced_custom_object for Gateway: {e}"
        ) from e


def get_gateway(
    kserve_client, name, namespace, version="v1"
):
    try:
        return kserve_client.api_instance.get_namespaced_custom_object(
            "gateway.networking.k8s.io",
            version,
            namespace,
            "gateways",
            name,
        )
    except client.rest.ApiException as e:
        raise RuntimeError(
            f"Exception when calling CustomObjectsApi->"
            f"get_namespaced_custom_object for Gateway: {e}"
        ) from e


def create_or_update_route(kserve_client, route, namespace=None):
    """Create or update a HttpRoute resource."""
    route = copy.deepcopy(route)

    version = route["apiVersion"].split("/")[1]

    if namespace is None:
        namespace = route.get("metadata", {}).get("namespace", "default")

    name = route.get("metadata", {}).get("name")
    if not name:
        raise ValueError("HttpRoute must have a name in metadata")

    logger.info(f"Checking HttpRoute {name} in namespace {namespace}")

    try:
        existing_route = kserve_client.api_instance.get_namespaced_custom_object(
            "gateway.networking.k8s.io",
            version,
            namespace,
            "httproutes",
            name,
        )

        route.setdefault("metadata", {})["resourceVersion"] = (
            existing_route["metadata"]["resourceVersion"]
        )

        outputs = kserve_client.api_instance.replace_namespaced_custom_object(
            "gateway.networking.k8s.io",
            version,
            namespace,
            "httproutes",
            name,
            route,
        )
        logger.info(f"✓ Successfully updated HttpRoute {name}")
        return outputs

    except client.rest.ApiException as e:
        if e.status == 404:  # Not found - create it
            logger.info(f"Resource not found, creating HttpRoute {name}")
            outputs = kserve_client.api_instance.create_namespaced_custom_object(
                "gateway.networking.k8s.io",
                version,
                namespace,
                "httproutes",
                route,
            )
            logger.info(f"✓ Successfully created HttpRoute {name}")
            return outputs
        raise


def delete_route(
    kserve_client, name, namespace, version="v1"
):
    try:
        logger.info(f"Deleting HttpRoute {name} in namespace {namespace}")
        return kserve_client.api_instance.delete_namespaced_custom_object(
            "gateway.networking.k8s.io",
            version,
            namespace,
            "httproutes",
            name,
        )
    except client.rest.ApiException as e:
        raise RuntimeError(
            f"Exception when calling CustomObjectsApi->"
            f"delete_namespaced_custom_object for HttpRoute: {e}"
        ) from e


def get_route(
    kserve_client, name, namespace, version="v1"
):
    try:
        return kserve_client.api_instance.get_namespaced_custom_object(
            "gateway.networking.k8s.io",
            version,
            namespace,
            "httproutes",
            name,
        )
    except client.rest.ApiException as e:
        raise RuntimeError(
            f"Exception when calling CustomObjectsApi->"
            f"get_namespaced_custom_object for HttpRoute: {e}"
        ) from e
