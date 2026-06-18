# AutoGluon Server

[AutoGluon](https://auto.gluon.ai/) server serves **TabularPredictor** and **TimeSeriesPredictor** models in KServe from a shared image (`kserve/autogluonserver`).

- **Tabular**: KServe inference protocol **v1 and v2**; for classification models you can optionally return **class probabilities** instead of predicted labels (see [Classification probabilities](#classification-probabilities-predict_proba) below).
- **Time series**: **REST v1 JSON only** (`POST /v1/models/{name}:predict`). v2 tensor payloads are not supported for time series in this release.

**Auto-detection.** At startup the server downloads `storageUri` and decides whether the artifact is tabular or time series. You do not configure the predictor type in YAML. It calls `TimeSeriesPredictor.load` on that directory; if loading fails, it calls `TabularPredictor.load` on the same path.

**`storageUri`.** Set this to the directory AutoGluon wrote when you saved the model—the same folder you would pass to `TabularPredictor.load(...)` or `TimeSeriesPredictor.load(...)`. For example, if training ended with `predictor.save("models/iris/")`, use `storageUri: "gs://my-bucket/models/iris/"`. Do not use the parent bucket, raw training data, or a file inside the save tree.

## Tabular models

Models must be saved with `TabularPredictor.save(path)` (a directory). The server loads that directory and converts request instances (list of dicts or list of lists) to a pandas `DataFrame` for `predict()` or `predict_proba()`.

`storageUri` must point at the directory produced by `TabularPredictor.save`.

### Classification probabilities (`predict_proba`)

By default the server calls AutoGluon’s `TabularPredictor.predict()` and returns the **predicted label** for each row (for example `yes` or `no`).

For **binary and multiclass** models you can instead call `TabularPredictor.predict_proba()`, which returns the model’s estimated **probability for each class** (values between 0 and 1; per row they sum to 1). This is useful when you need confidence scores, thresholds, or ranking rather than a single hard label.

Enable it by setting the environment variable `PREDICT_PROBA=true` on the predictor container (see [Environment](#environment)). The predictor must support `predict_proba` (typical for classification).

**v1** responses use one object per instance, with a key per class name and the probability as the value, for example:

```json
{
  "predictions": [
    { "yes": 0.61, "no": 0.39 },
    { "yes": 0.42, "no": 0.58 }
  ]
}
```

**v2** responses expose one `FP64` output tensor per class (names like `proba_yes`, `proba_no`); see `GET /v2/models/{name}` for the exact output names for your model.

## Time series models

Models must be saved with `TimeSeriesPredictor.save()` (a directory). Point `storageUri` at that **predictor directory** (the same path you would pass to `TimeSeriesPredictor.load`).

Column names for request JSON come from the loaded `TimeSeriesPredictor` and optional `predictor_metadata.json`. The **target** column name is always `TimeSeriesPredictor.target` (not overridable by environment). You can override **id** and **timestamp** column names with environment variables (see [Environment](#environment)).

### Time series JSON request (`:predict`)

**History** — top-level `instances`: array of JSON objects, one object per time step (long format), each including the target column (name = `TimeSeriesPredictor.target`) and any covariates present in training history.

**Known covariates on the horizon** (only if the model was trained with known covariates): top-level `known_covariates`, same column names as training for those features, plus the configured id and timestamp columns, covering the forecast horizon steps per series.

Example (names must match your schema and env overrides):

```json
{
  "instances": [
    { "item_id": "A", "timestamp": "2024-01-01T00:00:00", "target": 12.3 },
    { "item_id": "A", "timestamp": "2024-01-02T00:00:00", "target": 11.1 }
  ],
  "known_covariates": [
    { "item_id": "A", "timestamp": "2024-01-03T00:00:00", "promo": 1 }
  ]
}
```

**Response**: `{"predictions": [ ... ]}` — list of objects with the same **id** and **timestamp** column names as the request (from `predictor_metadata.json`, env overrides, or defaults), plus `mean` and quantile columns (e.g. `"0.1"`) from the trained predictor.

Use `modelFormat.name: autogluon` in `InferenceService` for both tabular and time series; the **same** runtime image auto-detects the artifact type from the save directory (see above). `ClusterServingRuntime` advertises a single format, `autogluon`.

## Run AutoGluon Server Locally

Install the [kserve](../kserve) package first. To install this package’s dependencies for local development, run the following from this directory (same pattern as [sklearnserver](../sklearnserver/README.md)):

```bash
make dev_install
```

**Note:** Backend libraries are pinned in `pyproject.toml` (see [Inference stack](#inference-stack-and-dependency-versions)). Use the same Python version as the Docker image when resolving dependencies (currently **3.12**). If you see an error like *"Distribution catboost can't be installed because it doesn't have a source distribution or wheel for the current platform"*, try Python 3.10 or 3.12 (e.g. `uv venv .venv --python 3.12`, activate it, then `make dev_install`). To install into an already-active virtualenv elsewhere (e.g. the repo root), use `uv sync --active --group test`.

Check that the server is available:

```bash
python -m autogluonserver
usage: __main__.py [-h] [--http_port HTTP_PORT] [--grpc_port GRPC_PORT]
                   --model_dir MODEL_DIR [--model_name MODEL_NAME]
__main__.py: error: the following arguments are required: --model_dir
```

The model can be on the local filesystem, or in S3-compatible object storage, Azure Blob Storage, or Google Cloud Storage.

## Deploy on KServe

### Tabular

```yaml
apiVersion: serving.kserve.io/v1beta1
kind: InferenceService
metadata:
  name: autogluon-iris
spec:
  predictor:
    model:
      modelFormat:
        name: autogluon
      storageUri: "gs://your-bucket/autogluon-tabular-model/"
```

### Time series

```yaml
apiVersion: serving.kserve.io/v1beta1
kind: InferenceService
metadata:
  name: autogluon-ts-forecast
spec:
  predictor:
    model:
      modelFormat:
        name: autogluon
      storageUri: "gs://your-bucket/path/to/timeseries-predictor-save/"
```

## Environment

- **`PREDICT_PROBA`** (tabular): set to `"true"` to return [class probabilities](#classification-probabilities-predict_proba) via `predict_proba()` instead of predicted labels via `predict()`.
- **`AUTOGLUON_TS_ID_COLUMN`**, **`AUTOGLUON_TS_TIMESTAMP_COLUMN`** (time series): override id and timestamp column names in JSON requests. Defaults are `item_id` and `timestamp` when `predictor_metadata.json` is absent, or the values from that file when present. Non-empty values override after stripping whitespace.
- **Target column** (time series): always taken from `TimeSeriesPredictor.target` on the loaded model. There is no environment variable to override it; use the same column name in `instances` / `known_covariates` as at training time.

## Inference stack and dependency versions

The `kserve/autogluonserver` image ships a **fixed Python stack**: AutoGluon plus ML backends (CatBoost, LightGBM, XGBoost, PyTorch, fastai). Saved predictors must load in an environment compatible with how they were trained.

AutoGluon pins its own release (`autogluon.tabular`, `autogluon.timeseries`) but **does not publish exact backend versions** for each release. Optional extras (for example `catboost`) are declared without a single version. KServe therefore **pins backends explicitly** in `pyproject.toml` and records the resolved graph in `uv.lock`.

**Supported stack (current image)** — update this table when the stack changes:

| Package | Version | Notes |
|---------|---------|--------|
| autogluon.tabular | 1.5.0 | |
| autogluon.timeseries | 1.5.0 | Pulls tabular with catboost/lightgbm/xgboost extras |
| catboost | 1.2.8 | |
| lightgbm | 4.6.0 | |
| xgboost | 3.1.3 | |
| torch | 2.9.1 | Required by time series; tabular torch models |
| fastai | 2.8.5 | Not required by AutoGluon core; pinned for tabular NN models (fastai stack) and full tabular backend coverage in this image |

Patch-level AutoGluon mismatches at load time are handled by `version_compat.py` (major/minor must match). **Backend versions are not checked at runtime** — use this pinned stack or re-train models.

To inspect the resolved versions in the lockfile without installing packages:

```bash
rg '^name = "(autogluon-tabular|autogluon-timeseries|catboost|lightgbm|xgboost|torch|fastai)"' -A1 uv.lock | rg 'name|version'
```

### Upgrading the AutoGluon stack

Use this procedure when bumping `autogluon.tabular` / `autogluon.timeseries` (for example 1.5.0 → 1.6.0).

**Requirements**

- Same **Python version** as the Docker image (`../autogluon.Dockerfile`, currently 3.12).
- Working copies of `../kserve` and `../storage` (path dependencies).
- At least one saved **tabular** and **time series** predictor trained with the target AutoGluon version (for load/smoke tests).

**Steps**

1. Set the target AutoGluon version in `pyproject.toml`:

   ```toml
   "autogluon.tabular==1.6.0",
   "autogluon.timeseries==1.6.0",
   ```

2. **Resolve backend versions with `uv`** (do not guess versions manually). Temporarily replace explicit backend pins (`catboost==…`, and so on) with unpinned names:

   ```toml
   "catboost",
   "lightgbm",
   "xgboost",
   "torch",
   "fastai",
   ```

   From this directory:

   ```bash
   uv venv .venv --python 3.12
   source .venv/bin/activate   # Windows: .venv\Scripts\activate
   uv lock
   ```

   Read resolved versions from the lockfile (run from `python/autogluonserver`):

   ```bash
   for pkg in catboost lightgbm xgboost torch fastai; do
     echo -n "$pkg: "
     rg -A1 "^name = \"$pkg\"$" uv.lock | rg 'version = ' | head -1 | sed 's/.*"\(.*\)".*/\1/'
   done
   ```

   Example output:

   ```text
   catboost: 1.2.8
   lightgbm: 4.6.0
   xgboost: 3.1.3
   torch: 2.9.1
   fastai: 2.8.5
   ```

   Alternatively, list all stack packages in one command:

   ```bash
   rg '^name = "(autogluon-tabular|autogluon-timeseries|catboost|lightgbm|xgboost|torch|fastai)"' -A1 uv.lock | rg 'name|version'
   ```

3. **Pin those versions** in `pyproject.toml`: copy each number from the output above into the `dependencies` list as `package==version` (keep `kserve` and `kserve-storage` unchanged). For example, if the loop printed `catboost: 1.2.8` and `lightgbm: 4.6.0`, edit `dependencies` to:

   ```toml
   dependencies = [
       "kserve",
       "kserve-storage",
       "catboost==1.2.8",
       "lightgbm==4.6.0",
       "xgboost==3.1.3",
       "torch==2.9.1",
       "fastai==2.8.5",
       "autogluon.tabular==1.6.0",
       "autogluon.timeseries==1.6.0",
   ]
   ```

   Use the versions from **your** `uv.lock` after step 2, not necessarily the example numbers above.

4. Regenerate the lockfile with all pins in place and install the development environment (same venv as step 2):

   ```bash
   uv lock
   uv sync --active --group test
   ```

   `make dev_install` from this directory runs the same `uv sync --active --group test` if you prefer the Makefile target.

5. **Verify** (use the **activated venv** from step 4; you do not need a second install if step 4 already succeeded):

   Run unit tests — they catch server regressions but **are not sufficient on their own**; step 6 is still required:

   ```bash
   make test
   ```

   Confirm **every** pinned package version in the active environment matches step 3 (AutoGluon and all backends):

   ```bash
   python - <<'PY'
   import importlib.metadata as m
   # Replace each "..." with the version from step 2 (loop or rg output).
   expected = {
       "autogluon.tabular": "1.6.0",
       "autogluon.timeseries": "1.6.0",
       "catboost": "...",
       "lightgbm": "...",
       "xgboost": "...",
       "torch": "...",
       "fastai": "...",
   }
   for pkg, want in expected.items():
       try:
           got = m.version(pkg)
           status = "OK" if got == want else f"MISMATCH (got {got})"
       except m.PackageNotFoundError:
           status = "MISSING"
       print(f"{pkg:22} want={want:8} -> {status}")
   PY
   ```

6. **Smoke-test model load** (**required** — do not skip after a green `make test`):

   Use predictors **trained and saved with the target AutoGluon version** from step 1 (not artifacts from an older AutoGluon release). Run `TabularPredictor.load` / `TimeSeriesPredictor.load` on at least one tabular and one time series save directory, or start the server locally:

   ```bash
   python -m autogluonserver --model_dir /path/to/saved/predictor --http_port 8080
   ```

   This step is mandatory because **backend versions are not validated at runtime** (only AutoGluon major/minor via `version_compat.py`). Saved models can depend on exact backend behavior (pickle/load paths, torch/fastai stacks); resolver and unit tests alone do not prove your artifacts load and predict correctly.

7. **Update the [supported stack table](#inference-stack-and-dependency-versions)** in this README, rebuild the image (`make docker-build-autogluon` from the repository root), and use a **versioned image tag** (for example aligned with the AutoGluon minor release).

**If `uv lock` fails**

- Check conflicts between `torch` and `autogluon-timeseries` constraints.
- Inspect the dependency tree: `uv tree | rg -E 'autogluon|catboost|lightgbm|xgboost|torch|fastai'`.
- Use [AutoGluon release notes](https://auto.gluon.ai/) and install guides as hints only; **the lockfile plus load tests are the source of truth for KServe**.

**Optional: align with a training environment**

If models are trained outside this image, capture versions from the training environment:

```bash
pip freeze | rg -i 'autogluon|catboost|lightgbm|xgboost|torch|fastai'
```

When they differ from the resolver’s latest compatible set, prefer matching the training environment if models fail to load.

## Development

Install development dependencies from this directory:

```bash
make dev_install
```

Run tests from this directory (discovery is limited to `tests/` via `pyproject.toml`):

```bash
make test
```

Run static type checks:

```bash
make type_check
```

An empty result from mypy indicates success.

## Building the AutoGluon Server Docker Image

From the **repository root**, use the same Makefile targets as the other predictor images (`KO_DOCKER_REPO` and `AUTOGLUON_IMG` come from `kserve-images.env`; override `TAG` as needed):

```shell
make docker-build-autogluon
make docker-push-autogluon
```

To use a different AutoGluon version, follow [Upgrading the AutoGluon stack](#upgrading-the-autogluon-stack), then rebuild with a versioned tag.

Equivalent manual build from the `python` directory (replace the image name with your registry and tag):

```shell
docker build -t your-registry/autogluonserver:latest -f autogluon.Dockerfile .
docker push your-registry/autogluonserver:latest
```

Update the InferenceService or KServe API configuration to use your image if needed.