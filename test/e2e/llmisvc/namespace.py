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

"""Per-test namespace lifecycle for llmisvc e2e tests.

Each test gets its own Kubernetes namespace to prevent resource collisions
between parallel pytest workers. The seed namespace (kserve-ci-e2e-test)
holds secrets that are copied into each per-test namespace.
"""

import hashlib
import logging
import os
import re
import time

from kubernetes import client

logger = logging.getLogger("e2e.llmisvc.namespace")

SEED_NAMESPACE = "kserve-ci-e2e-test"
TEST_NAMESPACE_LABEL_KEY = "kserve.io/e2e-test"
TEST_NAMESPACE_LABEL_VALUE = "true"

S3_CREDENTIALS_SECRET = os.environ.get("S3_CREDENTIALS_SECRET", "seaweedfs-s3-creds")

_NON_DNS_CHARS = re.compile(r"[^a-z0-9]+")


def _sanitize_for_dns(s: str) -> str:
    return _NON_DNS_CHARS.sub("-", s.lower()).strip("-")


def generate_namespace_name(node_name: str) -> str:
    """Generate a DNS-safe namespace name from a pytest node name."""
    sanitized = _sanitize_for_dns(node_name.split("[", 1)[0])
    name_hash = hashlib.sha256(node_name.encode()).hexdigest()[:8]
    prefix = "e2e-"
    max_total = 63
    sep = "-"
    max_base = max_total - len(prefix) - len(sep) - len(name_hash)
    safe_base = sanitized[:max_base].rstrip(sep)
    return f"{prefix}{safe_base}{sep}{name_hash}"


def skip_deletion() -> bool:
    return os.getenv("SKIP_RESOURCE_DELETION", "False").lower() in ("true", "1", "t")


def create_test_namespace(namespace: str) -> None:
    """Create a labeled namespace for a single test."""
    core_v1 = client.CoreV1Api()
    ns = client.V1Namespace(
        metadata=client.V1ObjectMeta(
            name=namespace,
            labels={
                TEST_NAMESPACE_LABEL_KEY: TEST_NAMESPACE_LABEL_VALUE,
            },
        )
    )
    try:
        core_v1.create_namespace(ns)
        logger.info(f"Created test namespace {namespace}")
    except client.rest.ApiException as e:
        if e.status == 409:
            logger.info(f"Test namespace {namespace} already exists")
        else:
            raise


def provision_namespace_secrets(namespace: str) -> None:
    """Copy secrets from the seed namespace and patch the default SA."""
    core_v1 = client.CoreV1Api()
    _copy_secret(core_v1, S3_CREDENTIALS_SECRET, SEED_NAMESPACE, namespace)
    _copy_secret(core_v1, "storage-config", SEED_NAMESPACE, namespace)
    _patch_default_sa_secret(core_v1, namespace, S3_CREDENTIALS_SECRET)


def delete_test_namespace(namespace: str) -> None:
    """Delete a test namespace (cascades all resource cleanup)."""
    core_v1 = client.CoreV1Api()
    try:
        core_v1.delete_namespace(namespace)
        logger.info(f"Deleted test namespace {namespace}")
    except client.rest.ApiException as e:
        if e.status != 404:
            logger.error(f"Failed to delete namespace {namespace}: {e}")


def _copy_secret(
    core_v1: client.CoreV1Api, secret_name: str, src_ns: str, dst_ns: str
) -> None:
    """Copy a secret from one namespace to another, skipping if source doesn't exist."""
    try:
        secret = core_v1.read_namespaced_secret(secret_name, src_ns)
    except client.rest.ApiException as e:
        if e.status == 404:
            logger.info(f"Secret {secret_name} not found in {src_ns}, skipping copy")
            return
        raise

    secret.metadata = client.V1ObjectMeta(
        name=secret_name,
        namespace=dst_ns,
        annotations=secret.metadata.annotations,
        labels=secret.metadata.labels,
    )
    try:
        core_v1.create_namespaced_secret(dst_ns, secret)
        logger.info(f"Copied secret {secret_name} from {src_ns} to {dst_ns}")
    except client.rest.ApiException as e:
        if e.status == 409:
            logger.info(f"Secret {secret_name} already exists in {dst_ns}")
        else:
            raise


def _patch_default_sa_secret(
    core_v1: client.CoreV1Api, namespace: str, secret_name: str
) -> None:
    """Add a secret ref to the default SA. Retries until the SA exists."""
    for attempt in range(5):
        try:
            core_v1.patch_namespaced_service_account(
                "default",
                namespace,
                {"secrets": [{"name": secret_name}]},
            )
            logger.info(f"Patched default SA in {namespace} with secret {secret_name}")
            return
        except client.rest.ApiException as e:
            if e.status == 404 and attempt < 4:
                time.sleep(1)
                continue
            raise
