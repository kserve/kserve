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

import logging

import pytest
from kubernetes import client
from kserve.constants.constants import (
    KSERVE_GROUP,
    KSERVE_PLURAL_KERNELCACHE,
    KSERVE_PLURAL_KERNELCACHECAPTURE,
    KSERVE_V1ALPHA1_VERSION,
)

from .utils import _load_k8s_config, wait_for, wait_for_resource_deleted

_logger = logging.getLogger(__name__)

_REPORTER_MANAGED_LABEL = "internal.serving.kserve.io/kernelcache-reporter"


def _capture_managed_resources(namespace: str, capture_uid: str) -> list[str]:
    """Return controller-managed capture resources still owned by a KCC."""
    core = client.CoreV1Api()
    rbac = client.RbacAuthorizationV1Api()
    resources = []

    for kind, objects in (
        (
            "Secret",
            core.list_namespaced_secret(
                namespace, label_selector=_REPORTER_MANAGED_LABEL + "=true"
            ).items,
        ),
        (
            "ServiceAccount",
            core.list_namespaced_service_account(namespace).items,
        ),
        ("Role", rbac.list_namespaced_role(namespace).items),
        ("RoleBinding", rbac.list_namespaced_role_binding(namespace).items),
    ):
        for resource in objects:
            owner_references = resource.metadata.owner_references or []
            if any(
                owner.uid == capture_uid
                and owner.kind == "KernelCacheCapture"
                and owner.api_version
                == f"{KSERVE_GROUP}/{KSERVE_V1ALPHA1_VERSION}"
                for owner in owner_references
            ):
                resources.append(f"{kind}/{resource.metadata.name}")

    return resources


@pytest.mark.kernelcache
def test_kernelcache_deletion(kc_session):
    """Delete the shared capture and cache after all KernelCache consumers run.

    Capture-scoped reporter identities are deleted by the capture reconciler when
    the capture reaches a terminal phase. KernelCache artifact-prefetch Jobs are
    intentionally retained until their configured TTL expires and are therefore
    not asserted here.
    """
    _load_k8s_config()
    api = client.CustomObjectsApi()

    namespace = kc_session["namespace"]
    kcc_name = kc_session["kcc_name"]
    kc_name = kc_session["kc_name"]
    kc_namespace = kc_session["kc_namespace"]

    kcc = api.get_namespaced_custom_object(
        KSERVE_GROUP,
        KSERVE_V1ALPHA1_VERSION,
        namespace,
        KSERVE_PLURAL_KERNELCACHECAPTURE,
        kcc_name,
    )
    kcc_uid = kcc["metadata"]["uid"]

    def assert_capture_resources_cleaned() -> None:
        remaining = _capture_managed_resources(namespace, kcc_uid)
        assert not remaining, (
            f"KernelCacheCapture {namespace}/{kcc_name} still owns managed "
            f"resources: {remaining}"
        )

    wait_for(assert_capture_resources_cleaned, timeout=30, interval=1.0)

    api.delete_namespaced_custom_object(
        KSERVE_GROUP,
        KSERVE_V1ALPHA1_VERSION,
        namespace,
        KSERVE_PLURAL_KERNELCACHECAPTURE,
        kcc_name,
    )
    wait_for_resource_deleted(
        KSERVE_GROUP,
        KSERVE_V1ALPHA1_VERSION,
        KSERVE_PLURAL_KERNELCACHECAPTURE,
        kcc_name,
        namespace=namespace,
    )

    kernelcache = api.get_namespaced_custom_object(
        KSERVE_GROUP,
        KSERVE_V1ALPHA1_VERSION,
        kc_namespace,
        KSERVE_PLURAL_KERNELCACHE,
        kc_name,
    )
    _logger.info(
        "Deleting KernelCache %s/%s with finalizers %s",
        kc_namespace,
        kc_name,
        kernelcache.get("metadata", {}).get("finalizers", []),
    )

    api.delete_namespaced_custom_object(
        KSERVE_GROUP,
        KSERVE_V1ALPHA1_VERSION,
        kc_namespace,
        KSERVE_PLURAL_KERNELCACHE,
        kc_name,
    )
    wait_for_resource_deleted(
        KSERVE_GROUP,
        KSERVE_V1ALPHA1_VERSION,
        KSERVE_PLURAL_KERNELCACHE,
        kc_name,
        namespace=kc_namespace,
    )
