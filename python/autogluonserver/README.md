# AutoGluon Server

Model Server implementation for serving [AutoGluon](https://auto.gluon.ai/) TabularPredictor models in KServe.

## Overview

This server loads models saved with `TabularPredictor.save(path)` (a directory) and serves predictions via the KServe inference protocol (v1 and v2). Input is expected as instances (list of dicts or list of lists) and is converted to a pandas DataFrame for `predictor.predict()` or `predictor.predict_proba()`.

## Usage

Deploy an InferenceService with the unified model spec:

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

The `storageUri` must point to a directory containing a model saved with `TabularPredictor.save(path)`.

## Environment

- `PREDICT_PROBA`: set to `"true"` to use `predict_proba()` instead of `predict()` when the predictor supports it.

## Building

From the `python` directory:

```bash
docker build -t kserve-autogluonserver:latest -f autogluon.Dockerfile .
```
