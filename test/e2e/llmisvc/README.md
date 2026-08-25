# LLM Inference Service E2E Tests

## Configuration Composition Pattern

Tests combine config fragments from different categories to create complete scenarios:

```python
pytest.param(
    TestCase(["router-managed", "workload-single-cpu", "model-fb-opt-125m"]),
    marks=pytest.mark.cluster_cpu,
)
```

The `llm_config_factory` fixture automatically creates/cleans up `LLMInferenceServiceConfig` objects.

## Test Markers

Tests carry two kinds of markers: **group markers** (what the test covers) and
**cluster markers** (what hardware/environment it needs). Group markers are
assigned automatically from the filename; cluster markers are set explicitly
via `pytest.param(..., marks=...)`.

### Auto-assigned group markers (implicit, from file naming)

`conftest.py` hooks into `pytest_collection_modifyitems` and assigns group
markers based on the test file name. **No `@pytest.mark` decorator is needed on
individual tests for these** — the filename alone decides the group.

All tests collected from the `llmisvc/` directory automatically receive the
`llminferenceservice` marker. In addition, each test gets a sub-group marker:

| File pattern | Assigned markers | Example file |
|---|---|---|
| `test_llm_autoscaling_<variant>.py` | `llminferenceservice` + `llmisvc_autoscaling` + `autoscaling_<variant>` | `test_llm_autoscaling_wva.py` → `llminferenceservice`, `llmisvc_autoscaling`, `autoscaling_wva` |
| Files in `_LLMISVC_CORE_EXCLUDED` | `llminferenceservice` only (manually marked for sub-group) | `test_llm_tracing.py` has its own `tracing` marker |
| Everything else | `llminferenceservice` + `llmisvc_core` | `test_llm_inference_service.py`, `test_pod_watch.py` |

**Key rules:**

1. **`llminferenceservice` is always automatic** — every test file under
   `test/e2e/llmisvc/` gets this marker. Do not add it manually.
2. **Autoscaling variants** — name the file `test_llm_autoscaling_<variant>.py`
   and the variant marker (`autoscaling_<variant>`) is derived automatically.
   Adding `test_llm_autoscaling_keda.py` creates the `autoscaling_keda` marker
   with no further code changes.
3. **Excluded files** — files in `_LLMISVC_CORE_EXCLUDED` (currently
   `test_llm_tracing.py`) opt out of automatic `llmisvc_core` and must carry
   their own explicit sub-group markers.
4. **Default** — every other `test_*.py` under `test/e2e/llmisvc/` receives
   `llmisvc_core` automatically.

When adding a new test file, decide which group it belongs to:

- Core LLMISVC functionality → just name it `test_*.py` (auto-gets `llminferenceservice` + `llmisvc_core`).
- New autoscaling backend → name it `test_llm_autoscaling_<backend>.py`.
- Completely new category → add the file to `_LLMISVC_CORE_EXCLUDED` and apply
  an explicit marker; register the marker in `pytest_configure` and `pytest.ini`.

### Cluster capability markers (explicit, per test param)

These are set on individual `pytest.param(...)` entries and describe what
hardware or environment the test needs:

- `@pytest.mark.cluster_cpu` - CPU-only tests
- `@pytest.mark.cluster_amd` - AMD GPU tests
- `@pytest.mark.cluster_nvidia` - NVIDIA GPU tests
- `@pytest.mark.cluster_nvidia_roce` - NVIDIA ROCe tests
- `@pytest.mark.cluster_intel` - Intel GPU tests
- `@pytest.mark.cluster_single_node` / `cluster_multi_node` - topology

Examples:

```bash
# Run all core LLMISVC tests
pytest -m "llmisvc_core" test/e2e/llmisvc/

# Run only WVA autoscaling tests
pytest -m "autoscaling_wva" test/e2e/llmisvc/

# Run all LLMISVC autoscaling variants
pytest -m "llmisvc_autoscaling" test/e2e/llmisvc/

# Run core tests on CPU clusters only
pytest -m "llmisvc_core and cluster_cpu" test/e2e/llmisvc/

# Run all LLM inference service tests (any group)
pytest -m "llminferenceservice" test/e2e/llmisvc/

# Run only CPU tests
pytest -m "llminferenceservice and cluster_cpu" test/e2e/llmisvc/

# Run only NVIDIA GPU tests
pytest -m "llminferenceservice and cluster_nvidia" test/e2e/llmisvc/

# Run all GPU tests (any vendor)
pytest -m "llminferenceservice and (cluster_amd or cluster_nvidia or cluster_intel)" test/e2e/llmisvc/

# Run CPU and AMD GPU tests only
pytest -m "llminferenceservice and (cluster_cpu or cluster_amd)" test/e2e/llmisvc/
```

## Adding New Configs

1. Add to `LLMINFERENCESERVICE_CONFIGS` in `fixtures.py`
2. Follow `category-descriptor` naming (described in the subsequent section)
3. Add new cluster capability test cases using `pytest.param` with appropriate marks:

   ```python
   pytest.param(
       TestCase(["router-managed", "workload-nvidia-a100-gpu", "model-llama-70b"]),
       marks=pytest.mark.cluster_nvidia,
   ),
   ```

   You can also customize test behavior with additional LlmDTestCase parameters:

   ```python
   pytest.param(
       TestCase(
           base_refs=["router-managed", "workload-single-cpu", "model-fb-opt-125m"],
           prompt="What is the capital of France?",
           max_tokens=50,
           response_assertion=lambda response: (
               response.status_code == 200
               and response.json().get("choices") is not None
               and len(response.json().get("choices", [])) > 0
           ),
       ),
       marks=pytest.mark.cluster_cpu,
   ),
   ```

## Config Naming Convention

Use prefixed categories that get composed together:

- **`workload-*`**: workload topology, container specs and resource specs (e.g., `workload-single-cpu`, `workload-multi-node-gpu`)
- **`model-*`**: Model sources (e.g., `model-fb-opt-125m`, `model-gpt2`)
- **`router-*`**: Routing configs (e.g., `router-managed`, `router-with-scheduler`)

Test IDs are generated by combining the cluster capability from pytest marks with all config names:

- Test ID format: `{cluster_capability}-{config1}-{config2}-{config3}`
- Example: `cluster_cpu-router-managed-workload-single-cpu-model-fb-opt-125m`
