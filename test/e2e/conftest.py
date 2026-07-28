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
import hashlib
import logging
import os

import time

import pytest
import pytest_asyncio
from httpx_retries import Retry, RetryTransport
import httpx
from kubernetes import client, config

import kserve
from kserve import KServeClient, InferenceRESTClient, RESTConfig
from kserve.constants.constants import PredictorProtocol
from kserve.logging import logger, KSERVE_LOG_CONFIG

from .common.http_retry import (
    DEFAULT_RETRY_BACKOFF_FACTOR,
    DEFAULT_RETRY_STATUS_CODES,
    DEFAULT_RETRY_TOTAL,
)

_ns_logger = logging.getLogger("e2e.namespace")

SEED_NAMESPACE = os.environ.get("KSERVE_SEED_NAMESPACE", "kserve-ci-e2e-test")
S3_CREDENTIALS_SECRET = os.environ.get("S3_CREDENTIALS_SECRET", "seaweedfs-s3-creds")
STORAGE_CONFIG_SECRET = "storage-config"


@pytest.fixture(scope="session", autouse=True)
def configure_logger():
    KSERVE_LOG_CONFIG["loggers"]["kserve"]["propagate"] = True
    KSERVE_LOG_CONFIG["loggers"]["kserve.trace"]["propagate"] = True
    kserve.logging.configure_logging(KSERVE_LOG_CONFIG)
    logger.info("Logger configured")


@pytest.fixture(scope="session")
def event_loop():
    """Provide a dedicated loop for session-scoped async E2E fixtures."""
    loop = asyncio.get_event_loop_policy().new_event_loop()
    try:
        yield loop
    finally:
        loop.close()


def _build_retry_transport():
    """Build an httpx transport with retry logic and optional CA cert verification."""
    ca_cert_path = os.environ.get("REQUESTS_CA_BUNDLE")
    verify = ca_cert_path if ca_cert_path else True
    http_transport = httpx.AsyncHTTPTransport(verify=verify)
    return RetryTransport(
        transport=http_transport,
        retry=Retry(
            total=DEFAULT_RETRY_TOTAL,
            backoff_factor=DEFAULT_RETRY_BACKOFF_FACTOR,
            backoff_jitter=0.0,
            allowed_methods=["GET", "POST"],
            status_forcelist=list(DEFAULT_RETRY_STATUS_CODES),
            retry_on_exceptions=[
                httpx.TimeoutException,
                httpx.NetworkError,
                httpx.RemoteProtocolError,
            ],
        ),
    )


@pytest_asyncio.fixture(scope="session")
async def rest_v1_client():
    transport = _build_retry_transport()
    v1_client = InferenceRESTClient(
        config=RESTConfig(
            transport=transport,
            timeout=180,
            verbose=False,
            protocol=PredictorProtocol.REST_V1,
        )
    )
    yield v1_client
    await v1_client.close()


@pytest_asyncio.fixture(scope="session")
async def rest_v2_client():
    transport = _build_retry_transport()
    v2_client = InferenceRESTClient(
        config=RESTConfig(
            transport=transport,
            timeout=180,
            verbose=False,
            protocol=PredictorProtocol.REST_V2,
        )
    )
    yield v2_client
    await v2_client.close()


@pytest.fixture(scope="session")
def kserve_client():
    return KServeClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))


def pytest_addoption(parser):
    parser.addoption(
        "--network-layer",
        default="istio",
        type=str,
        help="Network layer to used for testing. Default is istio. Allowed values are istio, istio-ingress, envoy-gatewayapi, istio-gatewayapi, openshift-route, gateway-api",
    )


@pytest.fixture(scope="session")
def network_layer(request):
    return request.config.getoption("--network-layer")


def _get_core_api() -> client.CoreV1Api:
    try:
        config.load_incluster_config()
    except config.ConfigException:
        config.load_kube_config(
            config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
        )
    return client.CoreV1Api()


def _generate_namespace_name(node_name: str, prefix: str = "e2e") -> str:
    """Generate short DNS-safe namespace name from pytest node ID.

    Keeps names short (≤24 chars) to leave room for ISVC hostname construction
    which combines {isvc_name}-predictor-{namespace} under a 63-char DNS limit.
    """
    name_hash = hashlib.sha256(node_name.encode()).hexdigest()[:12]
    return f"{prefix}-{name_hash}"


def _create_namespace(core_v1: client.CoreV1Api, namespace: str) -> None:
    ns = client.V1Namespace(
        metadata=client.V1ObjectMeta(
            name=namespace,
            labels={"kserve.io/e2e-test": "true"},
        )
    )
    try:
        core_v1.create_namespace(ns)
        _ns_logger.info(f"Created namespace {namespace}")
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


def _provision_secrets(core_v1: client.CoreV1Api, namespace: str) -> None:
    deadline = time.monotonic() + 30
    while time.monotonic() < deadline:
        try:
            core_v1.read_namespaced_service_account("default", namespace)
            break
        except client.rest.ApiException as e:
            if e.status != 404:
                raise
        time.sleep(0.5)

    _copy_secret(core_v1, S3_CREDENTIALS_SECRET, SEED_NAMESPACE, namespace)
    _copy_secret(core_v1, STORAGE_CONFIG_SECRET, SEED_NAMESPACE, namespace)

    for attempt in range(10):
        try:
            core_v1.patch_namespaced_service_account(
                "default",
                namespace,
                {"secrets": [{"name": S3_CREDENTIALS_SECRET}]},
            )
            return
        except client.rest.ApiException as e:
            if e.status == 404 and attempt < 9:
                time.sleep(0.5)
                continue
            raise


def _delete_namespace(core_v1: client.CoreV1Api, namespace: str) -> None:
    try:
        core_v1.delete_namespace(namespace)
        _ns_logger.info(f"Deleted namespace {namespace}")
    except client.rest.ApiException as e:
        if e.status != 404:
            _ns_logger.error(f"Failed to delete {namespace}: {e}")


def _wait_pods_terminated(core_v1: client.CoreV1Api, namespace: str) -> None:
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


@pytest.fixture(scope="function")
def test_namespace(request):
    """Create isolated namespace for each test, cleanup after."""
    core_v1 = _get_core_api()
    ns_name = _generate_namespace_name(request.node.nodeid)

    _create_namespace(core_v1, ns_name)
    _provision_secrets(core_v1, ns_name)

    yield ns_name

    skip_del = os.getenv("SKIP_RESOURCE_DELETION", "").lower() in ("true", "1")
    skip_on_fail = os.getenv("SKIP_DELETION_ON_FAILURE", "").lower() in ("true", "1")
    failed = hasattr(request.node, "rep_call") and request.node.rep_call.failed

    if skip_del or (failed and skip_on_fail):
        _ns_logger.info(f"Preserving namespace {ns_name}")
        return

    _wait_pods_terminated(core_v1, ns_name)
    _delete_namespace(core_v1, ns_name)


@pytest.hookimpl(tryfirst=True, hookwrapper=True)
def pytest_runtest_makereport(item, call):
    outcome = yield
    setattr(item, f"rep_{call.when}", outcome.get_result())
