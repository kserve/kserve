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
import os

import pytest
import pytest_asyncio
from httpx_retries import Retry, RetryTransport
import httpx

import kserve
from kserve import KServeClient, InferenceRESTClient, RESTConfig
from kserve.constants.constants import PredictorProtocol
from kserve.logging import logger, KSERVE_LOG_CONFIG

from .common.http_retry import (
    DEFAULT_RETRY_BACKOFF_FACTOR,
    DEFAULT_RETRY_STATUS_CODES,
    DEFAULT_RETRY_TOTAL,
)


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
