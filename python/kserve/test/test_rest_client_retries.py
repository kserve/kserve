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

import subprocess
import sys

import httpcore
import httpx
import pytest

from kserve.inference_client import InferenceRESTClient, RESTConfig


def test_inference_client_import_without_private_httpx_proxy_helper():
    result = subprocess.run(
        [
            sys.executable,
            "-c",
            "import importlib\n"
            "try:\n"
            "    utils = importlib.import_module('httpx._utils')\n"
            "except ModuleNotFoundError:\n"
            "    pass\n"
            "else:\n"
            "    vars(utils).pop('get_environment_proxies', None)\n"
            "import kserve.inference_client\n",
        ],
        capture_output=True,
        text=True,
    )
    assert result.returncode == 0, result.stderr


@pytest.fixture(autouse=True)
def clear_proxy_environment(monkeypatch):
    for name in ("HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY"):
        monkeypatch.delenv(name, raising=False)
        monkeypatch.delenv(name.lower(), raising=False)


@pytest.mark.asyncio
@pytest.mark.parametrize("retries, expected_attempts", [(0, 1), (2, 3)])
@pytest.mark.parametrize(
    "core_error, client_error",
    [
        (httpcore.ConnectError, httpx.ConnectError),
        (httpcore.ConnectTimeout, httpx.ConnectTimeout),
    ],
)
async def test_rest_client_retries_connection_errors(
    monkeypatch, retries, expected_attempts, core_error, client_error
):
    attempts = 0

    async def fail_connect(
        self, host, port, timeout=None, local_address=None, socket_options=None
    ):
        nonlocal attempts
        attempts += 1
        raise core_error("connection unavailable")

    monkeypatch.setattr(
        httpcore._backends.auto.AutoBackend, "connect_tcp", fail_connect
    )

    client = InferenceRESTClient(config=RESTConfig(retries=retries))
    try:
        with pytest.raises(client_error):
            await client.is_server_live("http://example.test")
        assert attempts == expected_attempts
    finally:
        await client.close()


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "proxy_environment, target_url, expected_host",
    [
        (
            {"HTTP_PROXY": "http://http-proxy.test:8080"},
            "http://predictor.test",
            "http-proxy.test",
        ),
        (
            {"HTTPS_PROXY": "http://https-proxy.test:8080"},
            "https://predictor.test",
            "https-proxy.test",
        ),
        (
            {"ALL_PROXY": "http://all-proxy.test:8080"},
            "http://predictor.test",
            "all-proxy.test",
        ),
        (
            {"HTTP_PROXY": "http-proxy.test:8080"},
            "http://predictor.test",
            "http-proxy.test",
        ),
        (
            {"HTTP_PROXY": "http://http-proxy.test:8080", "NO_PROXY": "predictor.test"},
            "http://predictor.test",
            "predictor.test",
        ),
        (
            {"ALL_PROXY": "http://all-proxy.test:8080", "NO_PROXY": "*"},
            "http://predictor.test",
            "predictor.test",
        ),
        (
            {"HTTP_PROXY": "http://http-proxy.test:8080", "NO_PROXY": "127.0.0.1"},
            "http://127.0.0.1",
            "127.0.0.1",
        ),
        (
            {"HTTP_PROXY": "http://http-proxy.test:8080", "NO_PROXY": "::1"},
            "http://[::1]",
            "::1",
        ),
    ],
)
async def test_rest_client_retries_with_environment_proxies(
    monkeypatch, proxy_environment, target_url, expected_host
):
    for name, value in proxy_environment.items():
        monkeypatch.setenv(name, value)

    connected_hosts = []

    async def fail_connect(
        self, host, port, timeout=None, local_address=None, socket_options=None
    ):
        connected_hosts.append(host)
        raise httpcore.ConnectError("connection unavailable")

    monkeypatch.setattr(
        httpcore._backends.auto.AutoBackend, "connect_tcp", fail_connect
    )

    client = InferenceRESTClient(config=RESTConfig(retries=2))
    try:
        with pytest.raises(httpx.ConnectError):
            await client.is_server_live(target_url)
        assert connected_hosts == [expected_host] * 3
    finally:
        await client.close()


@pytest.mark.asyncio
async def test_rest_client_preserves_custom_transport(monkeypatch):
    monkeypatch.setenv("HTTP_PROXY", "http://http-proxy.test:8080")
    transport = httpx.MockTransport(
        lambda request: httpx.Response(200, json={"live": True})
    )
    config = RESTConfig(transport=transport, retries=2, protocol="v2")
    client = InferenceRESTClient(config=config)
    try:
        assert config.transport is transport
        assert await client.is_server_live("http://example.test") is True
    finally:
        await client.close()
