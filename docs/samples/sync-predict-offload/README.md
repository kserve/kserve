# Keep the server responsive during synchronous prediction

Use `Model(..., offload_sync_predict=True)` when a synchronous `predict()` calls
blocking I/O, such as a synchronous HTTP client, database client or file operation.
The default is `False`: choosing a worker thread must be explicit because existing
models may depend on thread affinity or mutable state that is not thread-safe.

This example requires a KServe checkout containing this option. It is not a claim
that an already published PyPI release includes it.

## Choose an execution mode

| Handler | Recommended approach |
| --- | --- |
| Synchronous handler with blocking I/O and thread-safe state | Enable `offload_sync_predict` |
| Native async handler using awaited, nonblocking clients | Keep the async handler; the option does not offload it |
| Async handler calling blocking code | Replace the blocking client or explicitly offload that operation; this option does not fix async handlers |
| CPU-bound Python computation | Consider separate processes or a specialized serving runtime; threads do not generally provide Python CPU parallelism under the GIL |
| Thread-affine or non-thread-safe model/client | Keep the default until the implementation supports concurrent worker calls |

## Enable the option in an existing model

```python
class MyModel(Model):
    def __init__(self, name):
        super().__init__(name, offload_sync_predict=True)

    def predict(self, payload, headers=None):
        return synchronous_inference(payload)
```

No new base class is needed. Only prediction dispatched through the model's async
request pipeline is offloaded. A direct `model.predict(...)` call is still
synchronous. Preprocess and postprocess stay on the serving event loop, so blocking
work in those hooks needs separate attention. Request context is copied into the
worker; worker changes to context variables do not propagate back to the request.

## Run the comparison

From the repository root, in a Python 3.11 virtual environment:

```sh
python -m pip install -e python/kserve
python docs/samples/sync-predict-offload/model.py
```

In another terminal:

```sh
curl -sS -H 'Content-Type: application/json' \
  -d '{"instances":["slow"]}' http://localhost:8080/v1/models/blocking:predict &
sleep 0.5
curl -sS -w '\nreadiness: %{time_total}s\n' http://localhost:8080/v1/models/blocking
curl -sS -w '\nfast prediction: %{time_total}s\n' -H 'Content-Type: application/json' \
  -d '{"instances":["fast"]}' http://localhost:8080/v1/models/blocking:predict
wait
```

The slow prediction deliberately takes about three seconds. With offloading,
readiness and the fast prediction can respond while it is still running. Stop the
server and restart it with `--inline`, then repeat the requests: the slow prediction
blocks the event loop and delays readiness. Timings vary with scheduling; these
commands illustrate behavior rather than establish a latency benchmark.

## Bound concurrency and handle cancellation

The example uses two default-executor workers. `ModelServer(max_asyncio_workers=2)`
configures that executor on the serving loop. Alternatively, a positive
`ASYNC_MODEL_WORKERS` selects a dedicated pool shared by opted-in models within each
process. Worker processes have independent pools; this is not a pod-wide limit.
Unset, invalid or nonpositive values use the loop's default executor.

A worker limit bounds executing threads, not the number of queued requests. Use
request admission controls and client timeouts appropriate to your service.
Cancelling an awaiting request does not forcibly stop an already running thread;
set timeouts on the underlying blocking I/O as well.

KServe does not currently inspect handlers for blocking I/O or automatically enable
this option. A `def predict` declaration alone cannot establish whether its work
blocks or whether moving it to a thread is safe.
