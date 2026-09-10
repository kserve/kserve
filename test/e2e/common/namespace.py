# Copyright 2026 The KServe Authors.
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

"""Worker-scoped namespace helpers for e2e tests.

Each pytest-xdist worker gets one Kubernetes namespace. InferenceServices are
deleted after each test so workers do not collide or starve Knative ingress.
"""

import logging
import os
import time

from kubernetes import client, config

logger = logging.getLogger("e2e.namespace")

SEED_NAMESPACE = os.environ.get("KSERVE_SEED_NAMESPACE", "kserve-ci-e2e-test")
S3_CREDENTIALS_SECRET = os.environ.get("S3_CREDENTIALS_SECRET", "seaweedfs-s3-creds")
STORAGE_CONFIG_SECRET = "storage-config"
WORKER_NAMESPACE_PREFIX = "e2e"


def skip_resource_deletion() -> bool:
    return os.getenv("SKIP_RESOURCE_DELETION", "").lower() in ("true", "1")


def worker_namespace_name(worker_id: str) -> str:
    return f"{WORKER_NAMESPACE_PREFIX}-{worker_id}"[:24]


def get_core_api() -> client.CoreV1Api:
    try:
        config.load_incluster_config()
    except config.ConfigException:
        config.load_kube_config(
            config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
        )
    return client.CoreV1Api()


def _namespace_labels(core_v1: client.CoreV1Api) -> dict:
    labels = {"kserve.io/e2e-test": "true"}
    try:
        seed = core_v1.read_namespace(SEED_NAMESPACE)
    except client.rest.ApiException:
        return labels
    seed_labels = seed.metadata.labels or {}
    for key, value in seed_labels.items():
        if key == "istio-injection" or key.startswith("pod-security.kubernetes.io/"):
            labels[key] = value
    return labels


def create_namespace(core_v1: client.CoreV1Api, namespace: str) -> None:
    ns = client.V1Namespace(
        metadata=client.V1ObjectMeta(
            name=namespace,
            labels=_namespace_labels(core_v1),
        )
    )
    try:
        core_v1.create_namespace(ns)
        logger.info("Created namespace %s", namespace)
    except client.rest.ApiException as e:
        if e.status != 409:
            raise

    deadline = time.monotonic() + 30
    while time.monotonic() < deadline:
        try:
            ns = core_v1.read_namespace(namespace)
            if ns.status.phase == "Active":
                return
        except client.rest.ApiException:
            pass
        time.sleep(0.5)
    raise TimeoutError(f"Namespace {namespace} did not become Active within 30 seconds")


def _copy_secret(core_v1: client.CoreV1Api, name: str, src: str, dst: str) -> None:
    try:
        secret = core_v1.read_namespaced_secret(name, src)
    except client.rest.ApiException as e:
        if e.status == 404:
            return
        raise

    secret.metadata = client.V1ObjectMeta(
        name=name,
        namespace=dst,
        annotations=secret.metadata.annotations,
        labels=secret.metadata.labels,
    )
    try:
        core_v1.create_namespaced_secret(dst, secret)
    except client.rest.ApiException as e:
        if e.status != 409:
            raise


def provision_secrets(core_v1: client.CoreV1Api, namespace: str) -> None:
    deadline = time.monotonic() + 60
    service_account_ready = False
    while time.monotonic() < deadline:
        try:
            core_v1.read_namespaced_service_account("default", namespace)
            service_account_ready = True
            break
        except client.rest.ApiException as e:
            if e.status != 404:
                raise
        time.sleep(0.5)

    if not service_account_ready:
        raise TimeoutError(
            f"Default service account in namespace {namespace} was not ready within 60 seconds"
        )

    _copy_secret(core_v1, S3_CREDENTIALS_SECRET, SEED_NAMESPACE, namespace)
    _copy_secret(core_v1, STORAGE_CONFIG_SECRET, SEED_NAMESPACE, namespace)

    for attempt in range(20):
        try:
            core_v1.patch_namespaced_service_account(
                "default",
                namespace,
                {"secrets": [{"name": S3_CREDENTIALS_SECRET}]},
            )
            return
        except client.rest.ApiException as e:
            if e.status == 404 and attempt < 19:
                time.sleep(1)
                continue
            raise


def delete_namespace(core_v1: client.CoreV1Api, namespace: str) -> None:
    try:
        core_v1.delete_namespace(namespace)
        logger.info("Deleted namespace %s", namespace)
    except client.rest.ApiException as e:
        if e.status != 404:
            logger.error("Failed to delete %s: %s", namespace, e)


def wait_pods_terminated(core_v1: client.CoreV1Api, namespace: str) -> None:
    deadline = time.monotonic() + 60
    while time.monotonic() < deadline:
        try:
            pods = core_v1.list_namespaced_pod(
                namespace, label_selector="serving.kserve.io/inferenceservice"
            ).items
            if not any(p.metadata.deletion_timestamp for p in pods):
                return
        except client.rest.ApiException:
            return
        time.sleep(2)


def _delete_namespaced_custom_objects(
    api: client.CustomObjectsApi,
    group: str,
    version: str,
    plural: str,
    namespace: str,
) -> None:
    try:
        resp = api.list_namespaced_custom_object(group, version, namespace, plural)
    except client.rest.ApiException:
        return
    for item in resp.get("items", []):
        name = item["metadata"]["name"]
        try:
            api.delete_namespaced_custom_object(group, version, namespace, plural, name)
        except client.rest.ApiException as e:
            if e.status != 404:
                logger.warning(
                    "Failed to delete %s/%s/%s: %s", namespace, plural, name, e
                )


def cleanup_isvcs(namespace: str) -> None:
    """Delete leftover InferenceServices and TrainedModels in a worker namespace."""
    get_core_api()
    api = client.CustomObjectsApi()
    group = "serving.kserve.io"

    _delete_namespaced_custom_objects(
        api, group, "v1alpha1", "trainedmodels", namespace
    )
    _delete_namespaced_custom_objects(
        api, group, "v1beta1", "inferenceservices", namespace
    )

    deadline = time.monotonic() + 45
    while time.monotonic() < deadline:
        try:
            isvcs = api.list_namespaced_custom_object(
                group, "v1beta1", namespace, "inferenceservices"
            )
            models = api.list_namespaced_custom_object(
                group, "v1alpha1", namespace, "trainedmodels"
            )
            if not isvcs.get("items") and not models.get("items"):
                return
        except client.rest.ApiException:
            return
        time.sleep(1)
