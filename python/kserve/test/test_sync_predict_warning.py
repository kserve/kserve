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
import logging
import pickle
import time

import pytest

from kserve import Model
from kserve import model as model_module


class Echo(Model):
    def predict(self, payload, headers=None, response_headers=None):
        if response_headers is not None:
            response_headers["x-model"] = self.name
        return payload


@pytest.fixture
def warnings(caplog, monkeypatch):
    logger = logging.getLogger("kserve")
    monkeypatch.setattr(logger, "propagate", False)
    logger.addHandler(caplog.handler)
    with caplog.at_level(logging.WARNING, logger="kserve"):
        yield caplog
    logger.removeHandler(caplog.handler)


def clock(monkeypatch, *values):
    ticks = iter(values)
    monkeypatch.setattr(
        model_module, "perf_counter", lambda: next(ticks), raising=False
    )


@pytest.mark.asyncio
async def test_default_slow_inline_prediction_warns(warnings):
    class Slow(Echo):
        def predict(self, payload, headers=None):
            time.sleep(0.12)
            return payload

    payload = {"instances": [1]}
    assert await Slow("slow")(payload) == (payload, {})
    assert "occupied the event-loop thread" in warnings.text
    assert "100.000 ms" in warnings.text


@pytest.mark.asyncio
async def test_fast_inline_prediction_does_not_warn(monkeypatch, warnings):
    clock(monkeypatch, 1.0, 1.001)
    await Echo("fast")({})
    assert not warnings.records


@pytest.mark.asyncio
@pytest.mark.parametrize("duration,expected", [(0.099, 0), (0.100, 0), (0.101, 1)])
async def test_threshold_is_strictly_exceeded(
    monkeypatch, warnings, duration, expected
):
    clock(monkeypatch, 0, duration)
    await Echo("boundary")({})
    assert len(warnings.records) == expected


def test_direct_prediction_is_not_instrumented(monkeypatch, warnings):
    def forbidden():
        pytest.fail("direct predict call read the diagnostic clock")

    monkeypatch.setattr(model_module, "perf_counter", forbidden, raising=False)
    assert Echo("direct").predict({"result": 1}) == {"result": 1}
    assert not warnings.records


@pytest.mark.asyncio
async def test_warning_is_actionable_without_request_data(monkeypatch, warnings):
    clock(monkeypatch, 1.0, 1.2)
    payload = {"instances": ["secret-payload"]}
    model = Echo("identified-model", return_response_headers=True)
    result, headers = await model(payload, {"authorization": "secret-header"})
    assert result is payload
    assert headers == {"x-model": "identified-model"}
    assert len(warnings.records) == 1
    message = warnings.records[0].getMessage()
    assert "identified-model" in message and "Echo.predict" in message
    assert "200.000 ms" in message and "100.000 ms" in message
    assert "thread-safe" in message and "https://" in message
    assert "secret-payload" not in message and "secret-header" not in message


@pytest.mark.asyncio
async def test_rate_limit_is_per_model_and_expires(monkeypatch, warnings):
    clock(monkeypatch, 0, 0.25, 1, 1.25, 2, 2.25, 60, 60.25)
    first, second = Echo("first"), Echo("second")
    await first({})
    await first({})
    await second({})
    await first({})
    assert len(warnings.records) == 3
    assert [record.args[0] for record in warnings.records] == [
        "first",
        "second",
        "first",
    ]


@pytest.mark.asyncio
async def test_zero_interval_warns_for_each_slow_call(monkeypatch, warnings):
    clock(monkeypatch, 1, 1.2, 2, 2.2)
    model = Echo("unlimited", sync_predict_warning_interval_seconds=0)
    await model({})
    await model({})
    assert len(warnings.records) == 2


@pytest.mark.asyncio
async def test_disabled_warning_skips_clock(monkeypatch, warnings):
    def forbidden():
        pytest.fail("disabled diagnostic read the clock")

    monkeypatch.setattr(model_module, "perf_counter", forbidden, raising=False)
    await Echo("disabled", sync_predict_warning_threshold_ms=0)({})
    assert not warnings.records


@pytest.mark.asyncio
@pytest.mark.parametrize("elapsed,expected", [(0.001, 0), (0.2, 1)])
async def test_exception_preserves_identity(monkeypatch, warnings, elapsed, expected):
    error = ValueError("private-error")

    class Failing(Model):
        def predict(self, payload, headers=None):
            raise error

    clock(monkeypatch, 0, elapsed)
    with pytest.raises(ValueError) as caught:
        await Failing("failing")({})
    assert caught.value is error
    assert len(warnings.records) == expected
    assert "private-error" not in warnings.text


@pytest.mark.asyncio
async def test_custom_threshold_is_applied(monkeypatch, warnings):
    clock(monkeypatch, 0, 0.025)
    await Echo("custom", sync_predict_warning_threshold_ms=20)({})
    assert len(warnings.records) == 1
    assert "threshold: 20.000 ms" in warnings.text


@pytest.mark.asyncio
async def test_preprocess_and_postprocess_are_not_measured(monkeypatch, warnings):
    now = [0.0]
    monkeypatch.setattr(model_module, "perf_counter", lambda: now[0])

    class Pipeline(Echo):
        def preprocess(self, payload, headers=None):
            now[0] += 1
            return payload

        def postprocess(self, payload, headers=None):
            now[0] += 1
            return payload

    await Pipeline("pipeline")({})
    assert now[0] == 2
    assert not warnings.records


@pytest.mark.asyncio
@pytest.mark.parametrize("offload", [False, True])
async def test_async_wait_and_thread_offload_are_not_timed(
    monkeypatch, warnings, offload
):
    class AsyncModel(Model):
        async def predict(self, payload, headers=None):
            if offload:
                await asyncio.to_thread(time.sleep, 0.01)
            else:
                await asyncio.sleep(0.01)
            return payload

    def forbidden():
        pytest.fail("awaited prediction was timed as inline occupancy")

    monkeypatch.setattr(model_module, "perf_counter", forbidden, raising=False)
    await AsyncModel("async", sync_predict_warning_threshold_ms=0.1)({})
    assert not warnings.records


@pytest.mark.asyncio
async def test_cpu_work_is_reported_as_occupancy(monkeypatch, warnings):
    class CPUModel(Model):
        def predict(self, payload, headers=None):
            return {"result": sum(i * i for i in range(1000))}

    clock(monkeypatch, 1, 1.2)
    result, _ = await CPUModel("cpu")({})
    assert result["result"] > 0
    assert "occupied the event-loop thread" in warnings.text
    assert "blocking I/O detected" not in warnings.text


@pytest.mark.parametrize(
    "setting",
    ["sync_predict_warning_threshold_ms", "sync_predict_warning_interval_seconds"],
)
@pytest.mark.parametrize("value", [-1, float("inf"), float("nan")])
def test_invalid_warning_configuration_is_rejected(setting, value):
    with pytest.raises(ValueError, match=setting):
        Echo("invalid", **{setting: value})


def test_warning_configuration_remains_picklable():
    model = Echo("pickle", sync_predict_warning_threshold_ms=25)
    restored = pickle.loads(pickle.dumps(model))
    assert restored.sync_predict_warning_threshold_ms == 25


@pytest.mark.asyncio
@pytest.mark.parametrize("failing_predict", [False, True])
async def test_logging_failure_does_not_change_prediction(monkeypatch, failing_predict):
    error = ValueError("original prediction failure")

    class Predictor(Echo):
        def predict(self, payload, headers=None):
            if failing_predict:
                raise error
            return payload

    class BrokenHandler(logging.Handler):
        def emit(self, record):
            raise RuntimeError("logging sink failed")

    clock(monkeypatch, 0, 0.2)
    logger = logging.getLogger("kserve")
    handler = BrokenHandler()
    logger.addHandler(handler)
    try:
        if failing_predict:
            with pytest.raises(ValueError) as caught:
                await Predictor("logging")({})
            assert caught.value is error
        else:
            assert await Predictor("logging")({"result": 1}) == ({"result": 1}, {})
    finally:
        logger.removeHandler(handler)
