# AutoGluon Server

[AutoGluon](https://auto.gluon.ai/) server serves **TabularPredictor** and **TimeSeriesPredictor** models in KServe from a shared image (`kserve/autogluonserver`).

- **Tabular**: KServe inference protocol **v1 and v2**, optional `predict_proba` for classification.
- **Time series**: **REST v1 JSON only** (`POST /v1/models/{name}:predict`). v2 tensor payloads are not supported for time series in this release.

The server **auto-detects** whether the artifact is tabular or time series: it tries `TimeSeriesPredictor.load` on the model directory first, then `TabularPredictor.load`. Point `storageUri` at the **AutoGluon save directory** (the folder passed to `TabularPredictor.save(path)` or `TimeSeriesPredictor.save(path)`).

## Tabular models

Models must be saved with `TabularPredictor.save(path)` (a directory). The server loads that directory and converts request instances (list of dicts or list of lists) to a pandas `DataFrame` for `predict()` or `predict_proba()`.

`storageUri` must point at the directory produced by `TabularPredictor.save`.

## Time series models

Models must be saved with `TimeSeriesPredictor.save()` (a directory). Point `storageUri` at that **predictor directory** (the same path you would pass to `TimeSeriesPredictor.load`).

Column names for request JSON are taken from the loaded `TimeSeriesPredictor` where available. You can override id, timestamp, and target column names with environment variables (see below) if they are not sufficient.

### Time series JSON request (`:predict`)

**History** — top-level `instances`: array of row objects (long format), one row per time step, including `target` and any covariates present in training history.

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

**Response**: `{"predictions": [ ... ]}` — list of objects with forecast index columns (e.g. `item_id`, `timestamp`) plus `mean`, quantile columns (e.g. `"0.1"`), matching the trained predictor.

Use `modelFormat.name: autogluon` in `InferenceService` for both tabular and time series; the **same** runtime image auto-detects the artifact type from the save directory (see above). `ClusterServingRuntime` advertises a single format, `autogluon`.

## Run AutoGluon Server Locally

Install the [kserve](../kserve) package first. To install this package’s dependencies for local development, run the following from this directory (same pattern as [sklearnserver](../sklearnserver/README.md)):

```bash
make dev_install
```

**Note:** The dependency `autogluon.tabular[all]` pulls in CatBoost, which in the current lock file only has wheels for **Python 3.10** on some platforms. If you see an error like *"Distribution catboost can't be installed because it doesn't have a source distribution or wheel for the current platform"*, use Python 3.10 for this project (e.g. `uv venv .venv --python 3.10`, activate it, then `make dev_install`). To install into an already-active virtualenv elsewhere (e.g. the repo root), use `uv sync --active --group test`.

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

- **`PREDICT_PROBA`** (tabular): set to `"true"` to use `predict_proba()` instead of `predict()` when the predictor supports it (e.g. for classification).
- **`AUTOGLUON_TS_ID_COLUMN`**, **`AUTOGLUON_TS_TIMESTAMP_COLUMN`**, **`AUTOGLUON_TS_TARGET`**: override series id, timestamp, and target column names for time series JSON (defaults: `item_id`, `timestamp`, and predictor `target` or `target`).

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

To use a different AutoGluon version, change the version in `autogluonserver/pyproject.toml` (e.g. `autogluon.tabular==1.5.0` and `autogluon.timeseries==1.5.0`) and rebuild with a versioned tag.

Equivalent manual build from the `python` directory (replace the image name with your registry and tag):

```shell
docker build -t your-registry/autogluonserver:latest -f autogluon.Dockerfile .
docker push your-registry/autogluonserver:latest
```

Update the InferenceService or KServe API configuration to use your image if needed.