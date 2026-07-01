# traffic - continuous traffic testing for LLMInferenceService

A lightweight traffic driver for validating system behavior *during* state changes, not just at rest. Existing kserve e2e tests probe endpoints after mutations settle - this package sends traffic continuously while the test mutates cluster state, then lets you query the results by time window.

## Why

Step-by-step "mutate, wait, probe" tests miss transient failures. A canary weight change that drops 5% of requests for 2 seconds looks fine if you only check after it settles. Continuous traffic catches it.

The package is ~250 lines, pure Python, no external deps beyond `requests` (already in the test deps). No k6, no locust, no subprocess.

## What's in the box

**`TrafficDriver`** - background thread that sends HTTP requests at a controlled rate and records every response.

**`TrafficReport`** - mark-based windowing over the recorded responses. Drop marks at key moments (before mutation, after settle), then query phases.

**`Slice`** - chainable queryable view with filtering, grouping, distribution, latency percentiles, and assertion helpers.

## Quick example

```python
def test_rolling_upgrade_under_load(traffic_driver):
    driver = traffic_driver(url=gateway_url, rate=2, warmup=True)

    driver.mark("baseline")
    time.sleep(10)

    driver.mark("mutation")
    patch_weight(api, v2, weight=5, ns=ns)
    wait_for_group_weight(api, v1, v2, 5, ns)
    driver.mark("settled")
    time.sleep(10)

    report = driver.stop()

    # Zero errors during transition
    report.around("mutation", before=0, after=10).assert_no_errors("weight change")

    # Distribution shifted
    baseline_v1 = report.phase("baseline").where(is_v1).count
    settled_v1 = report.phase("settled").where(is_v1).count
    assert settled_v1 < baseline_v1
```

## API

### TrafficDriver

```python
driver = TrafficDriver(
    url="http://gateway/v1/completions",
    method="POST",                          # default
    headers={"X-Gateway-Model-Name": "..."},
    payload={"model": "m", "prompt": "hi", "max_tokens": 5},
    rate=2.0,                               # requests per second
    timeout=15.0,                           # per-request timeout
    session=None,                           # optional shared session
)

driver.start(warmup=True)   # blocks until first 2xx, warmup excluded from report
driver.mark("phase_name")   # record a named timestamp
report = driver.stop()       # stop sending, return TrafficReport
```

### TrafficReport

```python
report.all                              # Slice over everything
report.phase("baseline")               # between "baseline" mark and next mark
report.between("a", "b")               # between two marks
report.around("mutation", before=2, after=10)  # window around a mark
report.since("settled")                 # from mark to end
report.until("mutation")               # from start to mark
report.marks                            # dict of mark_name -> monotonic timestamp
```

### Slice

```python
s = report.phase("baseline")

s.count                                 # total requests
s.successes()                           # Slice of 2xx responses
s.errors()                              # Slice of non-2xx / connection errors
s.where(lambda r: "v1" in r.headers.get("x-inference-pod", ""))

s.by_header("x-inference-pod")          # Dict[str, Slice] grouped by header value
s.by_status()                           # Dict[int, Slice] grouped by status code
s.header_values("x-inference-pod")      # Counter[str]

s.latency_percentile(0.95)             # p95 latency in seconds
s.achieved_rate                         # actual requests/sec
s.first_seen(predicate)                 # first ResponseRecord matching predicate

s.assert_no_errors("context msg")       # raises with rich summary on failure
s.assert_error_rate(0.01, "msg")        # max 1% error rate
s.assert_min_samples(10, "msg")         # minimum sample count
s.summary()                             # human-readable summary string
```

## Fixture

The `traffic_driver` fixture in `conftest.py` is a factory - call it to create a driver, it auto-starts and auto-stops on teardown:

```python
@pytest.fixture
def traffic_driver():
    drivers = []
    def factory(url, *, warmup=False, **kwargs):
        driver = TrafficDriver(url, **kwargs)
        drivers.append(driver)
        driver.start(warmup=warmup)
        return driver
    yield factory
    for d in reversed(drivers):
        if d.is_running:
            d.stop()
```

## Running

Tests are marked `llminferenceservice`, `traffic`, and `cluster_cpu`. They use Service backends and work with any gateway (Envoy Gateway, Istio).

```bash
# From kserve repo root
source python/kserve/.venv/bin/activate
cd test/e2e

pytest llmisvc/test_llm_canary_lifecycle.py \
  -v -s --network-layer=envoy-gateway --log-cli-level=INFO

# Via CI runner
./test/scripts/gh-actions/run-e2e-tests.sh "traffic and cluster_cpu" 0 "envoy-gateway"
```

## Why not the test_case fixture?

The canary lifecycle tests use their own resource management (`apply_member`, `apply_config`, `wait_ready`, etc.) rather than the `test_case` fixture from `fixtures.py`. The `test_case`/`TestCase` pattern is designed for single-service "create, wait, query, delete" workflows - it does not support group membership, mid-test weight patching, or multi-member lifecycle mutations. This is consistent with other specialized test files (`test_flow_control.py`, `test_rolling_upgrade.py`). A future unification could extend `TestCase` with group/weight fields.

## Design choices

- **Threading over asyncio** - the existing kserve e2e framework uses sync k8s clients. A daemon thread is the simplest model that works with sync mutations in the main thread.
- **Rate-limited, not burst** - drift-compensating `next_tick` scheduling at a fixed rate. This is behavioral validation, not load testing. Consistent rate makes distribution assertions meaningful.
- **Headers captured** - every response's headers are recorded. Tests pick which header to use for attribution (`x-inference-pod`, `x-served-by`, or anything the workload exposes).
- **Mark-based phases** - no upfront phase declaration. Drop marks wherever you want, query any window after the fact.
