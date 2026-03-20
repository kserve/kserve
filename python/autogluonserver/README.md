# AutoGluon Server

[AutoGluon](https://auto.gluon.ai/) server serves **TabularPredictor** and **TimeSeriesPredictor** models in KServe from a shared image (`kserve/autogluonserver`).

- **Tabular**: KServe inference protocol **v1 and v2**, optional `predict_proba` for classification.
- **Time series**: **REST v1 JSON only** (`POST /v1/models/{name}:predict`). v2 tensor payloads are not supported for time series in this release.

## Tabular models

Models must be saved with `TabularPredictor.save(path)` (a directory). The server loads that directory and converts request instances (list of dicts or list of lists) to a pandas `DataFrame` for `predict()` or `predict_proba()`.

`storageUri` must point at the directory produced by `TabularPredictor.save`.

## Time series models

Models must be saved with `TimeSeriesPredictor.save()` (a directory). Typical Kubeflow / pipeline layout:

```text
<MODEL>_FULL/
  predictor/                 # AutoGluon save directory (load path)
  predictor_metadata.json    # Inference contract (see below)
  metrics/
  notebooks/
```

Point `storageUri` at **either**:

- `<MODEL>_FULL/` (recommended), or  
- the inner `predictor/` directory only.

If you use the inner path only, place `predictor_metadata.json` next to that directory (sibling) or inside it.

### `predictor_metadata.json`

Required fields for robust serving (your pipeline should emit this file):

| Field | Description |
| ----- | ----------- |
| `target` | Target column name in history rows |
| `id_column` | Series identifier column (e.g. `item_id`) |
| `timestamp_column` | Timestamp column |
| `prediction_length` | (optional) Horizon; defaults to loaded predictor |
| `known_covariates_names` | (optional) List of known covariate column names for the horizon |

If this file is **missing**, set `AUTOGLUON_PREDICTOR_TYPE=timeseries` and optional `AUTOGLUON_TS_ID_COLUMN`, `AUTOGLUON_TS_TIMESTAMP_COLUMN`, `AUTOGLUON_TS_TARGET` for column defaults.

### Time series JSON request (`:predict`)

**History** — top-level `instances`: array of row objects (long format), one row per time step, including `target` and any covariates present in training history.

**Known covariates on the horizon** (only if `known_covariates_names` is non-empty): top-level `known_covariates`, same column names as training for those features, plus `id_column` and `timestamp_column`, covering the forecast horizon steps per series.

Example (names must match your metadata):

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

**Response**: `{"predictions": [ ... ]}` — list of objects with forecast index columns (`item_id`, `timestamp`) plus `mean`, quantile columns (e.g. `"0.1"`), matching the trained predictor.

## Predictor selection (`AUTOGLUON_PREDICTOR_TYPE`)

| Value | Behavior |
| ----- | -------- |
| `tabular` | Always `TabularPredictor` |
| `timeseries` | Always `TimeSeriesPredictor` |
| `auto` (default) | If `predictor_metadata.json` is found next to the artifact (see layout above), time series; otherwise tabular |

Use `autogluon` or `autogluon-timeseries` as `modelFormat.name` in `InferenceService` to pick a runtime; the **same** `ClusterServingRuntime` image supports both formats. When in doubt, set `AUTOGLUON_PREDICTOR_TYPE` explicitly on the predictor container.

## Run AutoGluon Server Locally

Install the [kserve](../kserve) package first, then from this directory:

```bash
uv sync --group test
```

**Note:** The dependency `autogluon.tabular[all]` pulls in CatBoost, which in the current lock file only has wheels for **Python 3.10** on some platforms. If you see an error like *"Distribution \`catboost\` can't be installed because it doesn't have a source distribution or wheel for the current platform"*, use Python 3.10 for this project (e.g. `uv venv .venv --python 3.10` then `uv sync --group test`). To install into an already-active virtualenv elsewhere (e.g. the repo root), use `uv sync --active --group test`.

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
        name: autogluon-timeseries
      storageUri: "gs://your-bucket/run-123/MODEL_FULL/"
      env:
        - name: AUTOGLUON_PREDICTOR_TYPE
          value: "timeseries"
```

Omit `AUTOGLUON_PREDICTOR_TYPE` if `predictor_metadata.json` is present at the artifact root (auto mode).

## Environment

- **`PREDICT_PROBA`** (tabular): set to `"true"` to use `predict_proba()` instead of `predict()` when supported.
- **`AUTOGLUON_PREDICTOR_TYPE`**: `tabular` | `timeseries` | `auto`.
- **`AUTOGLUON_TS_ID_COLUMN`**, **`AUTOGLUON_TS_TIMESTAMP_COLUMN`**, **`AUTOGLUON_TS_TARGET`**: fallbacks when `predictor_metadata.json` is absent (time series only).

## Development

Install development dependencies from this directory:

```shell
uv sync --group test
```

Run tests:

```shell
pytest -W ignore
```

Run static type checks:

```bash
mypy --ignore-missing-imports autogluonserver
```

An empty result from mypy indicates success.

## Building the AutoGluon Server Docker Image

Build your own image for development or custom deployments. From the `python` directory (one level up from this folder):

```shell
docker build -t docker_user_name/autogluonserver:latest -f autogluon.Dockerfile .
```

To use a different AutoGluon version, change the version in `autogluonserver/pyproject.toml` (e.g. `autogluon.tabular==1.5.0` and `autogluon.timeseries==1.5.0`) and rebuild with a versioned tag.

Push the image to your registry:

```shell
docker push docker_user_name/autogluonserver:latest
```

Update the InferenceService or KServe API configuration to use your image if needed.
