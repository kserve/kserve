# AutoGluon Server

[AutoGluon](https://auto.gluon.ai/) server is an implementation for serving AutoGluon TabularPredictor models in KServe. It provides prediction over the KServe inference protocol (v1 and v2), with optional probability outputs for classification.

Models must be saved with `TabularPredictor.save(path)` (a directory). The server loads that directory and converts request instances (list of dicts or list of lists) to a pandas DataFrame for `predictor.predict()` or `predictor.predict_proba()`.

## Run AutoGluon Server Locally

Install the [kserve](../kserve) package first, then from this directory:

```bash
uv sync --group test
```

**Note:** The dependency `autogluon.tabular[all]` pulls in CatBoost, which in the current lock file only has wheels for **Python 3.10** on some platforms. If you see an error like *"Distribution catboost can't be installed because it doesn't have a source distribution or wheel for the current platform"*, use Python 3.10 for this project (e.g. `uv venv .venv --python 3.10` then `uv sync --group test`). To install into an already-active virtualenv elsewhere (e.g. the repo root), use `uv sync --active --group test`.

Check that the server is available:

```bash
python -m autogluonserver
usage: __main__.py [-h] [--http_port HTTP_PORT] [--grpc_port GRPC_PORT]
                   --model_dir MODEL_DIR [--model_name MODEL_NAME]
__main__.py: error: the following arguments are required: --model_dir
```

Point the server at a model directory (saved with `TabularPredictor.save(path)`) to load and serve predictions. The model can be on the local filesystem, or in S3-compatible object storage, Azure Blob Storage, or Google Cloud Storage.

## Deploy on KServe

Deploy an InferenceService using the unified model spec:

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
      storageUri: "gs://your-bucket/autogluon-model/"
```

`storageUri` must point to a directory containing a model saved with `TabularPredictor.save(path)`.

## Environment

- `**PREDICT_PROBA**`: set to `"true"` to use `predict_proba()` instead of `predict()` when the predictor supports it (e.g. for classification).

## Development

Install development dependencies from this directory:

```shell
uv sync --group test
```

Run tests from this directory (discovery is limited to `tests/` via `pyproject.toml`):

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

To use a different AutoGluon version, change the version in `autogluonserver/pyproject.toml` (e.g. `autogluon.tabular==1.5.0`) and rebuild with a versioned tag.

Push the image to your registry:

```shell
docker push docker_user_name/autogluonserver:latest
```

Update the InferenceService or KServe API configuration to use your image if needed.