# CI-Agnostic Test Selector Tool

## Context

The kserve repo has 60+ e2e test files, 15+ Python packages, and a large Go codebase with controllers for multiple CRD
types. Current CI uses path-based filters (`dorny/paths-filter`, workflow `paths:` triggers) to decide which tests to
run, but these are fragile: package restructuring silently breaks the filters and risks merging PRs without full test
coverage.

This tool uses AST parsing and dependency graphs to determine which tests must run for a given set of changed files. The
CRD type is the natural bridge between Python e2e tests (which create CRD resources via typed SDK constructors) and Go
controllers (which dispatch on CRD type/spec fields). The tool is CI-system agnostic so it works with GitHub Actions,
Prow, and Tekton.

## Language: Python (stdlib only)

- Primary complexity is Python AST parsing of e2e tests and SDK packages
- Go dependency graph is best handled by `go list -json -deps` (subprocess)
- Zero external dependencies: `ast`, `json`, `pathlib`, `subprocess`, `argparse`, `dataclasses`
- CI environments already have Python 3.10+

## Architecture

```
tools/test_selector/
    __init__.py
    __main__.py              # entry point: python -m tools.test_selector
    cli.py                   # argparse: learn, query subcommands
    config.json              # all configuration tables and overrides (committed)
    DESIGN.md                # this document (committed)

    analyzers/
        __init__.py
        go_deps.py           # Go dependency graph via `go list -json -deps`
        go_crd_discovery.py  # Discover CRD types, controllers, framework specs
        python_imports.py    # Python import analysis for intra-repo deps
        e2e_mapper.py        # AST-parse e2e tests -> CRD types, frameworks, markers
        config_discovery.py  # Map config/, charts/, hack/ dirs to CRD kinds
        config_mapper.py     # Load and match override rules from config.json

    graph/
        __init__.py
        depgraph.py          # Directed graph with BFS reverse-walk

    mapping/
        __init__.py
        builder.py           # Combines all analyzers -> Mapping object / mapping.json
        loader.py            # Loads mapping.json + overrides at query time
        schema.py            # Dataclasses for all mapping structures

    selector/
        __init__.py
        engine.py            # Changed files -> TestSelection
        rules.py             # Config-derived constants (keyword aliases, markers, etc.)

    output/
        __init__.py
        formatter.py         # JSON, YAML, or GH Actions matrix output

    tests/
        __init__.py
        conftest.py          # Session-scoped mapping fixture (runs learn once)
        test_engine.py       # query-mode tests

    mapping.json             # Auto-generated (uncommitted)
```

## Two Modes

### `learn` (expensive, ~10-30s, run on-demand)

Scans the entire repo, builds the dependency graph and CRD-to-test mapping, writes `mapping.json`.

### `query` (cheap, <1s, run in CI per-PR)

Reads `mapping.json` + `config.json`, takes changed files, outputs test selection.

```bash
git diff --name-only origin/master...HEAD | python -m tools.test_selector query --format=json
```

## Core Algorithm: Dynamic CRD Discovery

The tool dynamically discovers the full chain from Go binary entrypoints to CRD types to e2e tests, with no hardcoded
mapping tables.

### Step 1: Go Entrypoint Discovery

Dynamically find all `cmd/*/main.go` entrypoints. All are included in the mapping; the tool derives which are
controllers (have CRD types) vs sidecars (no CRDs) from the dependency graph:

```
cmd/manager/main.go        -> controller (InferenceService, InferenceGraph, TrainedModel)
cmd/llmisvc/main.go        -> controller (LLMInferenceService, LLMInferenceServiceConfig)
cmd/localmodel/main.go     -> controller (LocalModelCache, LocalModelNamespaceCache)
cmd/localmodelnode/main.go -> controller (LocalModelNode)
cmd/router/main.go         -> sidecar (no CRDs, no e2e triggered)
cmd/agent/main.go          -> sidecar (no CRDs, no e2e triggered)
cmd/crd-gen/main.go        -> utility (no CRDs, no e2e triggered)
cmd/spec-gen/main.go       -> utility (no CRDs, no e2e triggered)
```

### Step 2: Go Dependency Trees Per Entrypoint

A single `go list -json ./...` call builds the full dependency graph. For each entrypoint, the tool computes the
transitive dependency tree:

```
cmd/manager -> [pkg/controller/v1beta1/inferenceservice/,
                pkg/controller/v1alpha1/inferencegraph/,
                pkg/apis/serving/v1beta1/,
                pkg/webhook/,
                pkg/constants/,
                ...]

cmd/llmisvc -> [pkg/controller/v1alpha2/llmisvc/,
                pkg/apis/serving/v1alpha2/,
                ...]
```

### Step 3: CRD Type and Controller Discovery (Go Pattern Matching)

**Pattern 1: `+kubebuilder:object:root=true` markers**

Scans `pkg/apis/serving/` for root CRD type definitions. The scanner reads past comment lines
(kubebuilder annotations) to find the `type X struct` declaration:

```go
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:...
// ... (may be many annotation lines)
type InferenceService struct { ... }
```

**Pattern 2: `SetupWithManager` + `For()` / `Watches()` calls**

Finds controller-to-CRD bindings. Primary CRDs come from `For()`, secondary watches from `Watches()`:

```go
ctrl.NewControllerManagedBy(mgr).
    For(&v1beta1.InferenceService{}).              // primary CRD
    Owns(&appsv1.Deployment{}).                    // owned resource (ignored)
    Watches(&v1alpha1.ServingRuntime{}, ...)        // secondary watch
```

- `For()` matches are the entrypoint's primary CRDs
- `Watches()` matches are captured as secondary watches (filtered to known KServe CRDs)
- `Owns()` is not captured (owned resources are infrastructure types like Deployment, Service)

**Pattern 3: Framework dispatch via `ComponentImplementation`**

```go
var _ ComponentImplementation = &SKLearnSpec{}
```

And PredictorSpec fields with JSON tags:

```go
SKLearn    *SKLearnSpec    `json:"sklearn,omitempty"`
```

This discovers: `InferenceService -> PredictorSpec -> {sklearn, xgboost, tensorflow, ...}`

### Step 4: Build Entrypoint-CRD Map

Result of steps 1-3, with `is_controller` derived from whether any controllers exist in the dep tree:

```
cmd/manager:
  primary CRDs: [InferenceGraph, TrainedModel, InferenceService]
  watched CRDs: [ServingRuntime, ClusterServingRuntime]

cmd/llmisvc:
  primary CRDs: [LLMInferenceService, LLMInferenceServiceConfig]
  watched CRDs: [LLMInferenceServiceConfig]

cmd/localmodel:
  primary CRDs: [LocalModelCache, LocalModelNamespaceCache]
  watched CRDs: [InferenceService, LLMInferenceService, LocalModelNode]

cmd/localmodelnode:
  primary CRDs: [LocalModelNode]

cmd/router:   (sidecar, no CRDs)
cmd/agent:    (sidecar, no CRDs)
cmd/crd-gen:  (utility, no CRDs)
cmd/spec-gen: (utility, no CRDs)
```

### Step 5: E2E Test CRD Discovery (Python AST)

AST-parse every `test/e2e/**/*.py` file to extract:

**Pytest markers** from `@pytest.mark.*` decorators:
```python
@pytest.mark.predictor    -> marker: "predictor"
@pytest.mark.llminferenceservice -> marker: "llminferenceservice"
```

**CRD constructor calls** (two patterns):
```python
# SDK constructors: V1beta1InferenceService(...)
V1beta1InferenceService(...)  -> kind: "InferenceService", version: "v1beta1"

# Dict literals: {"kind": "LLMInferenceServiceConfig", ...}
{"kind": "LLMInferenceServiceConfig"}  -> kind: "LLMInferenceServiceConfig"
```

**Framework specs from keyword args:**
```python
V1beta1PredictorSpec(sklearn=V1beta1SKLearnSpec(...))  -> framework: "sklearn"
```

### Step 6: CRD-to-Markers Mapping

Built from the e2e test data: for each CRD kind, collect all pytest markers from test files that construct that kind.
This creates the bridge between Go controllers and e2e tests:

```
InferenceService -> [predictor, graph, transformer, explainer, raw, autoscaling, ...]
LLMInferenceService -> [llminferenceservice, cluster_cpu, ...]
LocalModelCache -> [modelcache]
```

### Step 7: Config/Chart/Hack Discovery

Scans `config/`, `charts/`, and `hack/` directories. Uses directory names and file content to map each directory to the
CRD kinds it relates to (via `keyword_aliases` in config.json). For example:

```
config/crd/full/llmisvc/  -> [LLMInferenceService, LLMInferenceServiceConfig]
charts/kserve-resources/  -> [InferenceService, InferenceGraph, TrainedModel]
config/overlays/test/s3-local-backend/ -> [InferenceGraph]
```

## Selection Engine

For a changed file, the engine classifies it and routes to the appropriate handler:

| File pattern | Handler | Behavior |
|---|---|---|
| `*.go` | `_process_go_file` | Reverse dep walk, find entrypoints, map CRDs to markers |
| `python/` | `_process_python_file` | SDK model narrowing, package classification, framework matching |
| `test/e2e/` | `_process_e2e_test_file` | Look up markers from mapping or infer from directory |
| `config/`, `charts/`, `hack/` | `_process_config_or_infra_file` | CRD discovery from directory structure |
| Override match | `_apply_override` | Config-defined escalation |
| Ignorable | skip | Docs, images, CI config |
| Unclassified | conservative:all | Run everything |

### Go File Processing

1. Find which Go package the file belongs to
2. Walk the reverse dependency graph to find all affected packages
3. Determine which entrypoints include those packages
4. **Framework check first**: if the file is framework-specific (e.g. `predictor_sklearn.go`), narrow to that
   framework's markers regardless of how many entrypoints are affected
5. **All-entrypoints escalation**: if all *controller* entrypoints (those with CRDs) are affected, trigger all e2e
6. Otherwise, iterate affected entrypoints and collect markers from their primary CRDs
7. For secondary watches (`watched_crd_types`), look up markers without fallback to all_e2e_markers (unknown watched
   CRDs are skipped rather than escalated)

### Python SDK Model Narrowing

The Python SDK models (`python/kserve/kserve/models/`) are auto-generated from the OpenAPI spec via
`hack/python-sdk/client-gen.sh`. Model filenames encode the CRD kind in snake_case (e.g.
`v1alpha2_llm_inference_service.py` corresponds to `LLMInferenceService`). Related types (Spec, Status, List) share the
root CRD as a filename prefix.

When a model file changes, the engine converts its filename back to a CRD kind and triggers only that CRD's markers
instead of all e2e tests. This matters because a PR that changes llmisvc Go types will regenerate only llmisvc model
files, not the entire SDK.

Files that don't match a known CRD kind (e.g. `knative_condition.py`) or `__init__.py` (changed when models are
added/removed) fall through to the standard `all_e2e` path for the `kserve` package.

### Watched CRD Handling

For primary CRDs (`For()`), unknown kinds fall back to `all_e2e_markers` (conservative). For watched CRDs
(`Watches()`), unknown kinds return no markers. This prevents a controller watching a common type (like a core
Kubernetes resource) from escalating to all tests.

## Configuration

All configuration lives in `config.json`. No entrypoint classification or CRD-to-marker mappings are hardcoded;
they are discovered from the codebase.

| Section | Purpose |
|---|---|
| `keyword_aliases` | Map short names to CRD kinds for config directory discovery |
| `python.all_e2e_packages` | Python packages that trigger all e2e (kserve, storage) |
| `ignorable_patterns` | File suffixes/prefixes to skip |
| `overrides` | Pattern-based escalation rules |

The `suite_dir_to_markers` mapping (which `test/e2e/<dir>/` directories map to which pytest markers) is auto-discovered
during `learn` and stored in the mapping, not in config. The analyzer uses a two-pass approach: first collect explicit
markers (from `@pytest.mark.*` decorators and `item.add_marker(pytest.mark.X)` conftest hooks) per directory, then
assign directory markers to files that have none of their own.

The `server_to_frameworks` mapping (which Python server packages support which model formats) is auto-discovered from
`config/runtimes/kserve-*.yaml` files. Each runtime YAML contains a `serving.kserve.io/server-type` annotation and a
`supportedModelFormats` section. Only servers with a matching `python/<server>/` directory are included. This handles
multi-format servers correctly (e.g. `predictiveserver` supports sklearn, xgboost, and lightgbm).

## Output Format

The selector outputs a flat `TestSelection` with a single `e2e_tests` field. CI workflows decide how to route markers
to specific jobs (e2e-test.yml, e2e-test-llmisvc.yaml, e2e-test-modelcache.yaml).

```json
{
  "go_tests": {
    "run": true,
    "packages": ["./pkg/controller/v1alpha2/llmisvc/..."],
    "all": false
  },
  "python_tests": {
    "run": false
  },
  "e2e_tests": {
    "run": true,
    "markers": ["cluster_cpu", "llminferenceservice", "modelcache"]
  },
  "reasons": [
    "pkg/controller/v1alpha2/llmisvc/scheduler.go -> pkg:pkg/controller/v1alpha2/llmisvc -> entrypoint:./cmd/llmisvc -> crd:LLMInferenceService -> e2e:..."
  ]
}
```

## Conservative Escalation Rules

The tool must never produce false negatives. Rules applied in order:

1. **Unknown file**: if a changed file can't be classified -> run all tests
2. **All controller entrypoints**: if reverse deps reach every controller entrypoint -> all Go + all e2e
3. **Framework-specific** (takes priority over #2): a framework spec file -> only tests using that framework
4. **SDK model files**: `python/kserve/kserve/models/v*_*.py` -> narrow to matching CRD kind's markers
5. **SDK non-model changes**: `python/kserve/` or `python/storage/` (non-model files) -> all e2e tests
6. **Dependency changes**: `go.mod` or `go.sum` -> all tests (indirect controller changes)
7. **Watched CRDs**: no fallback to all markers (only escalate on known CRDs)
8. **Overrides only add**: `config.json` can only widen scope, never narrow it

## Tests

Tests in `tools/test_selector/tests/test_engine.py` covering all dispatch paths:

```bash
PYTHONPATH=tools pytest tools/test_selector/tests/ -v
```

Tests use a session-scoped fixture that runs `build_mapping()` once against the live repo. Assertions use subset and
membership checks (not exact marker lists) so tests don't break when the repo gains new CRDs or markers. Negative
assertions use the CI workflow marker sets (`E2E_MARKERS`, `LLMISVC_MARKERS`, `MODELCACHE_MARKERS`) to verify
cross-domain isolation.
