# Copyright 2026 The KServe Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

from types import SimpleNamespace
from unittest.mock import Mock

import pytest
import requests

from kserve.api.kserve_client import KServeClient


@pytest.fixture
def readiness(monkeypatch):
    client = object.__new__(KServeClient)
    client.get = Mock(return_value={"status": {"url": "http://model.example"}})
    clock = SimpleNamespace(now=0)

    def sleep(seconds):
        clock.now += seconds

    monkeypatch.setattr("kserve.api.kserve_client.time.monotonic", lambda: clock.now)
    monkeypatch.setattr("kserve.api.kserve_client.time.sleep", sleep)
    return client, clock


def test_readiness_uses_remaining_request_budget(readiness, monkeypatch):
    client, clock = readiness
    budgets = []

    def get(url, **kwargs):
        budgets.append(kwargs["timeout"])
        clock.now += 2
        return SimpleNamespace(status_code=503 if len(budgets) == 1 else 200)

    monkeypatch.setattr("kserve.api.kserve_client.requests.get", get)
    client.wait_model_ready(
        "service",
        "model",
        cluster_ip="127.0.0.1",
        timeout_seconds=10,
        polling_interval=3,
    )
    assert budgets == [10, 5]


def test_readiness_handles_request_timeout(readiness, monkeypatch):
    client, clock = readiness

    def get(url, **kwargs):
        clock.now += kwargs.get("timeout", 10)
        raise requests.Timeout("model did not respond")

    monkeypatch.setattr("kserve.api.kserve_client.requests.get", get)
    with pytest.raises(RuntimeError, match="before the timeout"):
        client.wait_model_ready(
            "service",
            "model",
            cluster_ip="127.0.0.1",
            timeout_seconds=10,
            polling_interval=3,
        )
    assert clock.now == 10


def test_readiness_does_not_poll_after_deadline(readiness, monkeypatch):
    client, clock = readiness
    get = Mock(return_value=SimpleNamespace(status_code=503))
    monkeypatch.setattr("kserve.api.kserve_client.requests.get", get)
    with pytest.raises(RuntimeError, match="before the timeout"):
        client.wait_model_ready(
            "service",
            "model",
            cluster_ip="127.0.0.1",
            timeout_seconds=2,
            polling_interval=10,
        )
    assert get.call_count == 1
    assert clock.now == 2


@pytest.mark.parametrize("protocol,suffix", [("v1", ""), ("v2", "/ready")])
def test_readiness_success_preserves_endpoint(readiness, monkeypatch, protocol, suffix):
    client, _ = readiness
    get = Mock(return_value=SimpleNamespace(status_code=200))
    monkeypatch.setattr("kserve.api.kserve_client.requests.get", get)
    client.wait_model_ready(
        "service", "model", cluster_ip="127.0.0.1", protocol_version=protocol
    )
    assert get.call_args.args[0] == f"http://127.0.0.1/{protocol}/models/model{suffix}"
    assert get.call_args.kwargs["headers"] == {"Host": "model.example"}


def test_readiness_rejects_late_success(readiness, monkeypatch):
    client, clock = readiness

    def get(url, **kwargs):
        clock.now += 11
        return SimpleNamespace(status_code=200)

    monkeypatch.setattr("kserve.api.kserve_client.requests.get", get)
    with pytest.raises(RuntimeError, match="before the timeout"):
        client.wait_model_ready("service", "model", timeout_seconds=10)


@pytest.mark.parametrize("interval", [0, -1])
def test_readiness_requires_positive_polling_interval(readiness, interval):
    client, _ = readiness
    with pytest.raises(ValueError, match="polling_interval must be positive"):
        client.wait_model_ready("service", "model", polling_interval=interval)
