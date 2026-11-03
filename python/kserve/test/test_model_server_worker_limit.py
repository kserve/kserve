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

import asyncio
import multiprocessing
import os
import signal
import socket
import time
from collections import Counter
from concurrent.futures import ThreadPoolExecutor

import pytest
import requests

from kserve import Model, ModelRepository, ModelServer, model_server


class _GatedModel(Model):
    def __init__(self, started, release):
        super().__init__("blocking", offload_sync_predict=True)
        self.started = started
        self.release = release
        self.ready = True

    def predict(self, payload, headers=None):
        self.started.put(os.getpid())
        if not self.release.wait(30):
            raise TimeoutError("test did not release prediction")
        return {"predictions": [os.getpid()]}


class _AsyncProbe(Model):
    def __init__(self):
        super().__init__("async", offload_sync_predict=True)
        self.ready = True

    async def predict(self, payload, headers=None):
        await asyncio.sleep(0)
        # This test explicitly selects asyncio. Inspect the actual serving
        # loop so the check does not depend on when an extra POST is scheduled.
        executor = asyncio.get_running_loop()._default_executor
        return {
            "predictions": [os.getpid()],
            "default_executor_workers": getattr(executor, "_max_workers", None),
            "dedicated_executor_workers": getattr(
                Model._shared_executor, "_max_workers", None
            ),
        }


def _run_server(port, workers, limit, started, release):
    # Make the default deterministic, including when children use spawn.
    model_server.utils.cpu_count = lambda: 0
    ModelServer(
        http_port=port,
        workers=workers,
        max_asyncio_workers=limit,
        enable_grpc=False,
        event_loop="asyncio",
        grace_period=5,
        registered_models=ModelRepository(),
    ).start([_GatedModel(started, release), _AsyncProbe()])


def _request(url, predict=False):
    with requests.Session() as session:
        session.trust_env = False
        if predict:
            response = session.post(url, json={"instances": [1]}, timeout=30)
        else:
            response = session.get(url, timeout=1)
        response.raise_for_status()
        return response.json()


def _predict(session, base):
    response = session.post(
        f"{base}/v1/models/blocking:predict", json={"instances": [1]}, timeout=30
    )
    response.raise_for_status()
    return response.json()


@pytest.mark.parametrize(
    "workers,limit,override,expected",
    [
        (1, 1, None, 1),
        (2, 1, None, 1),
        (2, 2, None, 2),
        (2, None, None, 4),
        (2, 1, "2", 2),
    ],
)
def test_http_predictions_respect_worker_limit(
    monkeypatch, workers, limit, override, expected
):
    if override is None:
        monkeypatch.delenv("ASYNC_MODEL_WORKERS", raising=False)
    else:
        monkeypatch.setenv("ASYNC_MODEL_WORKERS", override)
    ctx = multiprocessing.get_context("spawn")
    started, release = ctx.Queue(), ctx.Event()
    with socket.socket() as reservation:
        reservation.bind(("127.0.0.1", 0))
        port = reservation.getsockname()[1]
    process = ctx.Process(
        target=_run_server, args=(port, workers, limit, started, release)
    )
    process.start()
    base = f"http://127.0.0.1:{port}"
    sessions = []
    try:
        deadline = time.monotonic() + 30
        while True:
            assert process.is_alive(), "model server exited before becoming ready"
            try:
                _request(f"{base}/v1/models/blocking")
                break
            except requests.RequestException:
                if time.monotonic() >= deadline:
                    raise
                release.wait(0.05)

        # Pin keep-alive connections to each worker before saturating it. A burst
        # of new connections can otherwise all be accepted by the same worker.
        connections = Counter()
        deadline = time.monotonic() + 15
        while len(sessions) < workers * (expected + 1):
            assert time.monotonic() < deadline, dict(connections)
            session = requests.Session()
            session.trust_env = False
            try:
                response = session.post(
                    f"{base}/v1/models/async:predict",
                    json={"instances": [1]},
                    timeout=5,
                )
                response.raise_for_status()
                result = response.json()
                pid = result["predictions"][0]
                expected_default = limit if limit is not None else expected
                assert result["default_executor_workers"] == expected_default, result
                if connections[pid] < expected + 1:
                    connections[pid] += 1
                    sessions.append((session, pid))
                    continue
            except BaseException:
                session.close()
                raise
            session.close()
            release.wait(0.01)

        count = len(sessions)
        with ThreadPoolExecutor(max_workers=count + 1) as clients:
            predictions = [
                (clients.submit(_predict, session, base), pid)
                for session, pid in sessions
            ]
            try:
                # Each prediction stays blocked. Count actual executions per PID,
                # not executor internals or the sum across independent workers.
                active = Counter()
                deadline = time.monotonic() + 15
                while len(active) < workers or min(active.values()) < expected:
                    pid = started.get(timeout=max(0.01, deadline - time.monotonic()))
                    active[pid] += 1
                    assert active[pid] <= expected, dict(active)
                assert all(value == expected for value in active.values())
                # Native async prediction must still run while all sync slots
                # are occupied, including with offload_sync_predict=True.
                probe = clients.submit(
                    _request, f"{base}/v1/models/async:predict", True
                )
                result = probe.result(timeout=5)
                assert result["predictions"][0] in active
                assert result["dedicated_executor_workers"] == (
                    expected if override is not None else None
                )
            finally:
                release.set()
            for future, pid in predictions:
                assert future.result(timeout=10)["predictions"] == [pid]
    finally:
        release.set()
        for session, _ in sessions:
            session.close()
        if process.is_alive():
            os.kill(process.pid, signal.SIGTERM)
        process.join(15)
        if process.is_alive():
            process.kill()
            process.join(5)
        started.close()
        started.join_thread()
    assert process.exitcode == 0
