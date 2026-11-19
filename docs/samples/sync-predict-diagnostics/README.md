# Diagnose synchronous prediction on the event loop

A synchronous `Model.predict()` runs on the serving event-loop thread. While it
runs, other requests and health checks on that loop cannot progress. KServe warns
after an inline prediction occupies the thread for more than 100 ms, at most once
every 60 seconds per model instance and process. The warning includes the model
name, handler, elapsed time and configured threshold. It excludes request bodies,
headers and exception messages, and also runs when prediction raises an exception.

Configure the diagnostic in your model constructor:

```python
from kserve import Model


class MyModel(Model):
    def __init__(self, name):
        super().__init__(
            name,
            sync_predict_warning_threshold_ms=250,
            sync_predict_warning_interval_seconds=60,
        )
```

Both settings must be finite and non-negative. Set
`sync_predict_warning_threshold_ms=0` to disable measurement and warnings. Set
`sync_predict_warning_interval_seconds=0` to warn on every qualifying call.

The diagnostic uses a monotonic clock around synchronous prediction only. It does
not change execution mode, return values, response headers or exceptions. Direct
`model.predict(...)` calls, awaited async handlers, preprocessing, postprocessing
and explain handlers are not measured by this diagnostic.

## Choose the execution mode for your workload

For I/O with an asynchronous API, implement `async def predict(...)` and await that
API. Merely changing the function to `async def` does not make blocking code safe.

For blocking I/O that is safe to run concurrently on threads, explicitly offload
that work. For example:

```python
import asyncio

from kserve import Model


class ThreadSafeIOModel(Model):
    async def predict(self, payload, headers=None):
        return await asyncio.to_thread(self.read_prediction, payload)

    def read_prediction(self, payload):
        # Replace with thread-safe blocking I/O, with its own timeout.
        return {"predictions": payload["instances"]}
```

Threads share model state, clients and accelerator resources. Check their thread
safety and bound concurrency before enabling concurrent access. Python CPU-bound
work may still contend for the GIL; use an appropriate process-based model runtime
or serving engine rather than assuming threads will improve CPU throughput.

The constructor-level `offload_sync_predict=True` option is proposed separately in
[PR #5020](https://github.com/kserve/kserve/pull/5020). This diagnostic works without
that option and does not enable it automatically.

## Interpret the warning

Elapsed time measures event-loop thread occupancy, not proof of blocking I/O. CPU
computation, native code and process scheduling pauses can contribute. A warning
is emitted only after prediction returns or raises, so it cannot detect a handler
that remains stuck forever. Rate limiting may hide subsequent slow calls within
the interval; use the existing prediction-latency metrics for aggregate latency.

The defaults are a starting point for operator feedback. Tune the threshold for
your health-check and request-latency requirements.
