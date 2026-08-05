# Test Selector

CI-agnostic test selector that uses AST parsing and Go dependency trees to determine which tests need to run for a given
set of changed files. Works with GitHub Actions, Prow, and Tekton.

The core idea: CRD types are the natural bridge between Python e2e tests (which create CRD resources via typed SDK
constructors) and Go controllers (which register for specific CRD types via `SetupWithManager`/`For()`). The tool
dynamically discovers these relationships, so no hardcoded mapping tables need to be maintained.

## Requirements

- Python 3.10+
- PyYAML (`pip install -r tools/test_selector/requirements.txt`) - required to parse CRD YAML; without it the CRD
  tables build empty and every config/chart change escalates to running all tests
- Go toolchain (for `go list` during `learn`)

## Quick Start

```bash
# Build the mapping (run from repo root, ~10-30s)
PYTHONPATH=tools .venv/bin/python3 -m test_selector learn

# Query which tests to run for a set of changed files
git diff --name-only origin/master...HEAD | \
  PYTHONPATH=tools .venv/bin/python3 -m test_selector query --format json
```

## Commands

### `learn`

Scans the entire repo and writes `mapping.json` (~40s, dominated by `go list`):

1. Discovers `cmd/*/main.go` entrypoints
2. Runs `go list -json -deps` per entrypoint for full transitive dependency trees
3. Pattern-matches Go source for CRD registrations (`+kubebuilder:object:root=true`, `For()`, `Owns()`, `Watches()`,
   `ComponentImplementation` assertions)
4. AST-parses `test/e2e/**/*.py` for pytest markers, CRD constructors, framework specs, and model formats
5. Analyzes Python packages under `python/`

```bash
PYTHONPATH=tools .venv/bin/python3 -m test_selector learn
PYTHONPATH=tools .venv/bin/python3 -m test_selector learn --repo /path/to/kserve
```

The generated `mapping.json` should be committed so CI doesn't need to recompute it per-PR.

### `query`

Reads `mapping.json`, takes changed files (stdin or `--changed-files`), outputs test selection:

```bash
# From stdin
echo "pkg/apis/serving/v1beta1/predictor_sklearn.go" | \
  PYTHONPATH=tools .venv/bin/python3 -m test_selector query

# From arguments
PYTHONPATH=tools .venv/bin/python3 -m test_selector query \
  --changed-files pkg/controller/v1alpha2/llmisvc/controller.go

# YAML output
git diff --name-only origin/master...HEAD | \
  PYTHONPATH=tools .venv/bin/python3 -m test_selector query --format yaml
```

## Output Format

```json
{
  "go_tests": {
    "run": true,
    "packages": [
      "./pkg/apis/serving/v1beta1/...",
      "./cmd/manager..."
    ]
  },
  "python_tests": {
    "run": false
  },
  "e2e_tests": {
    "run": true,
    "markers": [
      "predictor"
    ]
  },
  "e2e_llmisvc": {
    "run": false
  },
  "e2e_modelcache": {
    "run": false
  },
  "reasons": [
    "pkg/apis/serving/v1beta1/predictor_sklearn.go -> pkg:pkg/apis/serving/v1beta1 -> framework:sklearn -> e2e:predictor"
  ]
}
```

Fields:

| Field                   | Description                                          |
|-------------------------|------------------------------------------------------|
| `go_tests.run`          | Whether to run Go unit/integration tests             |
| `go_tests.packages`     | Specific Go packages to test (empty if `all: true`)  |
| `go_tests.all`          | Run all Go tests (shared infra changed)              |
| `python_tests.run`      | Whether to run Python package tests                  |
| `python_tests.packages` | Specific Python packages to test                     |
| `e2e_tests.run`         | Whether to run core e2e tests                        |
| `e2e_tests.markers`     | Pytest markers to select (`-m "predictor or graph"`) |
| `e2e_llmisvc.run`       | Whether to run LLMInferenceService e2e tests         |
| `e2e_modelcache.run`    | Whether to run LocalModelCache e2e tests             |
| `reasons`               | Human-readable trace of how each file was classified |

## How Selection Works

For a changed Go file:

1. Find the Go package it belongs to
2. Walk the reverse dependency graph to find all affected packages
3. Determine which entrypoints (`cmd/*`) include those packages
4. Look up which CRD types those entrypoints manage
5. If the file is framework-specific (e.g., `predictor_sklearn.go`), narrow to tests exercising that framework
6. Otherwise, select all e2e tests that create those CRD types
7. Output the corresponding pytest markers

For a changed Python file under `python/`:

- SDK/storage packages (`kserve`, `storage`) trigger all e2e tests
- Server packages (`sklearnserver`, `xgbserver`, etc.) trigger only tests for that framework

For config/chart changes, pattern-based overrides in `config.json` determine scope.

## Conservative Escalation

The tool prioritizes never missing required tests (false positives are acceptable, false negatives are not):

- Unknown/unclassified files trigger all tests
- Shared Go packages (`pkg/constants/`, `pkg/utils/`) trigger everything
- Webhook changes trigger all e2e tests
- SDK changes (`python/kserve/`) trigger all e2e tests

## Configuration

All configuration tables live in `config.json`. This includes CRD-to-marker mappings, framework lists, entrypoint
classifications, test suite directory mappings, and pattern-based overrides. When the project evolves (new CRD, test
suite, framework, or server), update `config.json` instead of modifying Python source.

### Local overrides

If a `config.override.json` exists next to `config.json`, it is loaded **instead of** `config.json`. Use it to
experiment with selection behavior locally without touching the committed defaults (leave it uncommitted). It fully
replaces the default file (it is not merged), so start by copying `config.json` and editing the copy:

```bash
cp tools/test_selector/config.json tools/test_selector/config.override.json
# edit config.override.json, then run `learn`/`query` as usual
```

### Overrides

The `overrides` section in `config.json` contains pattern-based rules for files that AST analysis can't classify
(config, charts, Makefile, etc.). Overrides can only widen scope, never narrow it.

```json
{
  "overrides": [
    {
      "pattern": "config/crd/**",
      "trigger": "all",
      "reason": "CRD schema changes affect all controllers"
    },
    {
      "pattern": "config/llmisvc/**",
      "trigger": {
        "e2e": [
          "llminferenceservice"
        ]
      },
      "reason": "LLMISvc-specific config"
    }
  ]
}
```

## Keeping the Mapping Fresh

Re-run `learn` and commit the updated `mapping.json` when:

- New controllers or entrypoints are added
- New e2e test files are added
- CRD types are added or renamed
- Python server packages are added

The tool will still work with a stale mapping, but may over-trigger (conservative escalation kicks in for unknown
files/packages).

## Pattern Assumptions and Maintenance

The tool relies on regex patterns and lookup tables to discover CRD types, controllers, and test mappings. When the codebase evolves, some of these may need updating. This section documents every assumption so divergence is easy to diagnose.

### Fail-safe behavior

If a pattern stops matching, the affected file becomes "unclassified" and triggers all tests. You will notice this as CI running more tests than expected, never fewer. The risk is not missed coverage, it is wasted CI time.

### Go CRD discovery (`analyzers/go_crd_discovery.py`)

| Pattern | What it matches | Breaks if |
|---|---|---|
| `+kubebuilder:object:root=true` | Root CRD type definitions | Project stops using kubebuilder markers |
| `For(\s*&(\w+)\.(\w+)\{` | Controller-CRD bindings via controller-runtime builder | controller-runtime changes its builder API (stable since v0.6) |
| `var _ ComponentImplementation = &(\w+)\{` | Framework spec types | Interface name or assertion pattern changes |
| `PredictorSpec` struct field extraction | Framework JSON tags | Frameworks move out of `PredictorSpec` |

These patterns match idiomatic controller-runtime code, not AST, because Go AST parsing is not available from Python without a separate tool. A new framework spec is auto-discovered if it follows the `ComponentImplementation` assertion or is a field on `PredictorSpec`.

### Python e2e test patterns (`analyzers/e2e_mapper.py`)

| Lookup | When to update |
|---|---|
| (auto-discovered) | CRD constructors are derived from `crd_to_e2e_markers` kinds + version prefix |
| `_FRAMEWORK_KWARG_NAMES` | New framework keyword arg added to `V1beta1PredictorSpec` |
| `_SKIP_MARKERS` | New non-test-suite pytest markers commonly used |

### Config/chart/hack discovery (`analyzers/config_discovery.py`)

| Pattern | Breaks if |
|---|---|
| `(\w+)\.serving\.kserve\.io` | API group changes from `serving.kserve.io` |
| `^kind:\s+(\w+)` | Standard Kubernetes YAML (unlikely) |
| `serving\.kserve\.io_(\w+)\.yaml` | CRD generation tool changes output naming |

Plural/singular/shortNames tables are built dynamically from CRD YAML definitions at learn time.

### Configuration tables (`config.json`)

All configuration tables that need updating when the project evolves are centralized in `config.json`:

| Section | When to update |
|---|---|
| `crd_groups` | New CRDs or test buckets added |
| `keyword_aliases` | New directory naming convention that can't be derived from CRD spec.names |
| `framework_kwarg_names` | New framework added to PredictorSpec |
| `suite_dir_map` | New e2e test directory added |
| `go_entrypoints.utility` / `go_entrypoints.sidecar` | New cmd/ entrypoint added |
| `python.server_to_framework` | New model server added under `python/` |
| `ignorable_patterns` | New non-code file types that should be skipped |
| `overrides` | New pattern-based rules for config/chart/infra files |

### Diagnosing a divergence

1. Run `learn` and check `mapping.json` for completeness (entrypoints, config_to_crds, test_files)
2. Test a specific file: `echo "path/to/file" | PYTHONPATH=tools .venv/bin/python3 -m test_selector query`. The `reasons` field traces exactly how the file was classified.
3. If a file triggers `conservative:all` unexpectedly, `reasons` explains why (e.g., `no config mapping`, `unclassified`, `unknown Go package`)

### Adding a new CRD type (checklist)

All tables referenced below are in `config.json`:

1. Go patterns should auto-discover it if it follows `+kubebuilder:object:root=true` and `SetupWithManager`/`For()`
2. Add the kind to `crd_to_e2e_markers` with the appropriate pytest markers
3. CRD constructors in e2e tests are auto-discovered (no manual step needed if the kind is in `crd_to_e2e_markers`)
4. Plural/singular/shortNames are auto-discovered from CRD YAML definitions (no manual step needed)
5. If routing to a new test bucket, add it to `crd_groups.llmisvc`/`crd_groups.modelcache` or create a new bucket in `schema.py`
6. If directories use a name that can't be derived from CRD spec.names, add an entry to `keyword_aliases`
7. Re-run `learn` and verify with `query`

## Design

See [DESIGN.md](DESIGN.md) for the full architectural design, algorithm details, and implementation phases.
