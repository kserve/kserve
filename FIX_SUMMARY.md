# Fix for GitHub Issue #5231: nil pointer in config template execution

## Problem
The e2e test `cluster_nvidia` fails with error:
```
nil pointer evaluating *v1alpha2.ParallelismSpec.Expert
```

This occurs in `ReplaceVariables()` function when Go template tries to access `.Spec.Parallelism.Expert` but `.Spec.Parallelism` is `nil`.

## Root Cause
When merging multiple LLMInferenceServiceConfig base refs (like `workload-dp-ep-gpu` and `workload-dp-ep-prefill-gpu`), the decode workload configuration may not have a `Parallelism` struct initialized. The Go templates in the config files attempt to access nested fields without checking for nil parent structs:

```yaml
{{- if .Spec.Parallelism.Expert -}}--enable-expert-parallel{{- end }}
```

If `.Spec.Parallelism` is nil, this causes a template execution error.

## Solution
Added defensive nil checks to all template expressions accessing nested Parallelism fields:

**Before:**
```yaml
{{- if .Spec.Parallelism.Expert -}}--enable-expert-parallel{{- end }}
{{- if .Spec.Parallelism.Tensor -}}--tensor-parallel-size {{ .Spec.Parallelism.Tensor }}{{- end }}
```

**After:**
```yaml
{{- if and .Spec.Parallelism .Spec.Parallelism.Expert -}}--enable-expert-parallel{{- end }}
{{- if and .Spec.Parallelism .Spec.Parallelism.Tensor -}}--tensor-parallel-size {{ .Spec.Parallelism.Tensor }}{{- end }}
```

Similarly for Prefill workloads:
```yaml
{{- if and .Spec.Prefill .Spec.Prefill.Parallelism .Spec.Prefill.Parallelism.Expert -}}--enable-expert-parallel{{- end }}
{{- if and .Spec.Prefill .Spec.Prefill.Parallelism .Spec.Prefill.Parallelism.Tensor -}}--tensor-parallel-size {{ .Spec.Prefill.Parallelism.Tensor }}{{- end }}
```

## Files Changed

### Config Templates (3 files)
1. `config/llmisvcconfig/config-llm-decode-worker-data-parallel.yaml`
   - Fixed 4 template expressions (2 for Expert, 2 for Tensor)

2. `config/llmisvcconfig/config-llm-worker-data-parallel.yaml`
   - Fixed 4 template expressions (2 for Expert, 2 for Tensor)

3. `config/llmisvcconfig/config-llm-prefill-worker-data-parallel.yaml`
   - Fixed 4 template expressions (2 for Expert, 2 for Tensor)

### Tests (1 file)
4. `pkg/controller/v1alpha2/llmisvc/config_merge_test.go`
   - Added 2 new test cases to `TestReplaceVariables`:
     - `nil Parallelism should not cause template error`
     - `nil Prefill.Parallelism should not cause template error`

## Test Results
All unit tests pass:
```bash
cd pkg/controller/v1alpha2/llmisvc && go test -v -run TestReplaceVariables
PASS
ok  	github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc	0.023s
```

## How to Test the Fix

### 1. Run the unit tests:
```bash
cd pkg/controller/v1alpha2/llmisvc
go test -v -run TestReplaceVariables
```

### 2. Run the failing e2e test:
```bash
uv run --project python/kserve pytest test/e2e/llmisvc -m "llminferenceservice and cluster_nvidia" -v
```

### 3. Test with the specific configuration that was failing:
The test uses these base refs which trigger the issue:
- `router-managed`
- `workload-dp-ep-gpu`
- `workload-dp-ep-prefill-gpu`
- `model-deepseek-v2-lite`

## Benefits
- **Prevents nil pointer errors**: Templates now gracefully handle nil Parallelism structs
- **Maintains functionality**: When Parallelism is set, the behavior is unchanged
- **Better error handling**: Templates won't crash when configs are partially merged
- **Test coverage**: New unit tests ensure this scenario is covered going forward

## Related Code
The fix addresses template execution in `ReplaceVariables()` at:
`pkg/controller/v1alpha2/llmisvc/config_merge.go:411`

The templates are used when merging well-known default configs for different deployment patterns:
- Single-node deployments
- Multi-node data parallel
- Multi-node pipeline parallel
- Disaggregated prefill/decode (P/D)
