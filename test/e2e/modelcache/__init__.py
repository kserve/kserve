# Copyright 2024 The KServe Authors.
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

import asyncio
import logging

from kubernetes import client
from kubernetes.client.exceptions import ApiException

logger = logging.getLogger(__name__)

_CLEANUP_POLL_INTERVAL = 5
_CLEANUP_TIMEOUT = 120


def _pv_exists(core_api: client.CoreV1Api, name: str) -> bool:
    try:
        core_api.read_persistent_volume(name)
        return True
    except ApiException as e:
        if e.status == 404:
            return False
        raise


def _pvc_exists(core_api: client.CoreV1Api, name: str, namespace: str) -> bool:
    try:
        core_api.read_namespaced_persistent_volume_claim(name, namespace)
        return True
    except ApiException as e:
        if e.status == 404:
            return False
        raise


async def assert_pv_deleted(core_api: client.CoreV1Api, name: str):
    """Poll until a PersistentVolume is deleted, or raise on timeout."""
    for _ in range(_CLEANUP_TIMEOUT // _CLEANUP_POLL_INTERVAL):
        if not _pv_exists(core_api, name):
            logger.info("PV %s cleaned up", name)
            return
        await asyncio.sleep(_CLEANUP_POLL_INTERVAL)
    raise AssertionError(f"PV {name} was not cleaned up within {_CLEANUP_TIMEOUT}s")


async def assert_pvc_deleted(core_api: client.CoreV1Api, name: str, namespace: str):
    """Poll until a PersistentVolumeClaim is deleted, or raise on timeout."""
    for _ in range(_CLEANUP_TIMEOUT // _CLEANUP_POLL_INTERVAL):
        if not _pvc_exists(core_api, name, namespace):
            logger.info("PVC %s/%s cleaned up", namespace, name)
            return
        await asyncio.sleep(_CLEANUP_POLL_INTERVAL)
    raise AssertionError(
        f"PVC {name} in {namespace} was not cleaned up within {_CLEANUP_TIMEOUT}s"
    )
