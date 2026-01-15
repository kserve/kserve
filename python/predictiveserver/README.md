# Predictive Server

Unified model serving runtime for KServe that supports multiple ML frameworks (scikit-learn, XGBoost, LightGBM) in a single container image.

## Overview

Predictive Server is a **wrapper runtime** that unifies three existing KServe model servers into a single deployable unit:

- **Scikit-learn** (`sklearnserver`): Support for `.joblib`, `.pkl`, and `.pickle` model files
- **XGBoost** (`xgbserver`): Support for `.bst`, `.json`, and `.ubj` model files
- **LightGBM** (`lgbserver`): Support for `.bst` model files

Instead of maintaining separate container images and deployments for each framework, Predictive Server provides a single runtime that delegates to the appropriate framework server based on the `--framework` argument. This approach:

- **Eliminates code duplication**: Reuses existing, well-tested server implementations
- **Simplifies deployment**: One container image instead of three
- **Maintains compatibility**: Uses the same underlying servers as individual runtimes
- **Reduces maintenance**: Changes to individual servers automatically propagate

## Features

- **Multi-framework support**: Single runtime for sklearn, XGBoost, and LightGBM
- **KServe integration**: Full compatibility with KServe inference protocol
- **Thread control**: Configurable thread count for XGBoost and LightGBM
- **Probability predictions**: Support for `predict_proba` in scikit-learn (via `PREDICT_PROBA` environment variable)

## Quick Start Example

### Deploy on Kubernetes

1. **Deploy the ClusterServingRuntime:**

```bash
kubectl apply -k config/runtimes
```

2. **Create an InferenceService:**

```bash
cat <<EOF | kubectl apply -f -
apiVersion: serving.kserve.io/v1beta1
kind: InferenceService
metadata:
  name: sklearn-iris
spec:
  predictor:
    model:
      modelFormat:
        name: sklearn
      storageUri: gs://kfserving-examples/models/sklearn/1.0/model
EOF
```

> **Note**: If other ClusterServingRuntimes or ServingRuntimes for sklearn, xgboost, or lightgbm already exist with higher priority values, those will be selected instead. Check existing runtimes with `kubectl get clusterservingruntimes` and ensure the predictive runtime has appropriate priority settings.


3. **Wait for the InferenceService to be ready:**

```bash
kubectl wait --for=condition=Ready inferenceservice/sklearn-iris --timeout=300s
```

4. **Test the deployment:**

```bash
# Get the predictor pod name
POD_NAME=$(kubectl get pods -l serving.kserve.io/inferenceservice=sklearn-iris -o jsonpath='{.items[0].metadata.name}')

# Port-forward to the predictor pod
kubectl port-forward pod/${POD_NAME} 8082:8080 &

# Send a prediction request (V1 protocol)
curl -X POST http://localhost:8082/v1/models/sklearn-iris:predict \
  -H "Content-Type: application/json" \
  -d '{"instances": [[6.8, 2.8, 4.8, 1.4], [6.0, 3.4, 4.5, 1.6]]}'

# Expected response:
# {"predictions": [1, 1]}
```

Alternatively, using V2 protocol:

```bash
curl -X POST http://localhost:8082/v2/models/sklearn-iris/infer \
  -H "Content-Type: application/json" \
  -d '{
    "inputs": [
      {
        "name": "input-0",
        "shape": [2, 4],
        "datatype": "FP64",
        "data": [[6.8, 2.8, 4.8, 1.4], [6.0, 3.4, 4.5, 1.6]]
      }
    ]
  }'
```

## Development Setup

```bash
cd python/predictiveserver

# Create virtual environment
uv venv

# Activate virtual environment
source .venv/bin/activate

# Install dependencies
make dev_install
```

## Usage

### Download a sample model (optional)

For local testing, you can download sample models:

```bash
# Scikit-learn model
mkdir -p /tmp/models/sklearn/1.0/model
curl -o /tmp/models/sklearn/1.0/model/model.joblib \
  https://storage.googleapis.com/kfserving-examples/models/sklearn/1.0/model/model.joblib

# XGBoost model
mkdir -p /tmp/models/xgboost/iris
curl -o /tmp/models/xgboost/iris/model.bst \
  https://storage.googleapis.com/kfserving-examples/models/xgboost/iris/model.bst

# LightGBM model
mkdir -p /tmp/models/lightgbm/iris
curl -o /tmp/models/lightgbm/iris/model.bst \
  https://storage.googleapis.com/kfserving-examples/models/lightgbm/iris/model.bst
```

### Basic Usage

Start the server with a specific framework:

```bash
# Scikit-learn model
python -m predictiveserver --model_name sklearn-model --model_dir /tmp/models/sklearn/1.0/model --framework sklearn

# XGBoost model
python -m predictiveserver --model_name xgb-model --model_dir /tmp/models/xgboost/iris --framework xgboost

# LightGBM model
python -m predictiveserver --model_name lgb-model --model_dir /tmp/models/lightgbm/iris --framework lightgbm
```

### Command-line Arguments

- `--model_name`: Name of the model (required)
- `--model_dir`: Directory containing the model file (required)
- `--framework`: ML framework to use - `sklearn`, `xgboost`, or `lightgbm` (default: `sklearn`)
- `--nthread`: Number of threads for XGBoost/LightGBM (default: `1`)

### Environment Variables

- `PREDICT_PROBA`: Set to `"true"` to use `predict_proba()` for scikit-learn models (default: `"false"`)

### Worker Configuration

The Predictive Server automatically configures workers based on the framework:

- **LightGBM**: Always uses `workers=1` (multi-process not supported)
- **Scikit-learn/XGBoost**: Defaults to `workers=1`, configurable via `--workers` argument

This is handled automatically in `__main__.py` to prevent runtime errors with LightGBM's threading limitations.

## KServe Deployment

### ClusterServingRuntime Configuration

The Predictive Server runtime is defined in [config/runtimes/kserve-predictiveserver.yaml](../../../config/runtimes/kserve-predictiveserver.yaml).

Key points:

- Supports sklearn (v1), xgboost (v2), and lightgbm (v4)
- Framework is selected via `--framework={{.Annotations.modelFormat}}` argument
- Priority set to 3 for all supported model formats

### InferenceService Examples

Deploy a model using the Predictive Server runtime. The framework is automatically detected from `modelFormat.name`:

<details>
<summary>Scikit-learn Example</summary>

```yaml
apiVersion: serving.kserve.io/v1beta1
kind: InferenceService
metadata:
  name: sklearn-iris
spec:
  predictor:
    model:
      modelFormat:
        name: sklearn
      storageUri: gs://kfserving-examples/models/sklearn/1.0/model
```
</details>

<details>
<summary>XGBoost Example</summary>

```yaml
apiVersion: serving.kserve.io/v1beta1
kind: InferenceService
metadata:
  name: xgboost-iris
spec:
  predictor:
    model:
      modelFormat:
        name: xgboost
      storageUri: gs://kfserving-examples/models/xgboost/1.0/model
```
</details>

<details>
<summary>LightGBM Example</summary>

```yaml
apiVersion: serving.kserve.io/v1beta1
kind: InferenceService
metadata:
  name: lightgbm-iris
spec:
  predictor:
    model:
      modelFormat:
        name: lightgbm
      storageUri: gs://kfserving-examples/models/lightgbm/1.0/model
```
</details>

> **Note**: The KServe controller automatically adds a `modelFormat` annotation based on `modelFormat.name`. This annotation is then passed to the container via the `--framework` argument, telling Predictive Server which underlying framework server to use. You don't need to add any labels or annotations manually.

## Architecture

The Predictive Server uses a **facade/wrapper pattern** where:

1. `PredictiveServerModel` acts as a unified interface
2. Delegates to existing framework-specific servers (`sklearnserver`, `xgbserver`, `lgbserver`)
3. Framework selection happens at initialization based on `--framework` argument
4. All framework models implement the same KServe `Model` interface
5. Avoids code duplication by reusing existing, well-tested server implementations

## Container

Build the container image using the Makefile:

```bash
# From kserve repository root
# Use default image name (kserve/predictiveserver)
make docker-build-predictive
make docker-push-predictive

# Or build with custom registry and image name
KO_DOCKER_REPO=quay.io/myorg make docker-build-predictive
KO_DOCKER_REPO=quay.io/myorg make docker-push-predictive
```

Run the container:

```bash
podman run -p 8080:8080 \
  -v /tmp/models/sklearn/1.0/model:/mnt/models \
  kserve/predictiveserver:latest \
  --model_name sklearn-model \
  --model_dir /mnt/models \
  --framework sklearn
```

## Dependencies

Predictive Server depends on the following KServe components:

- **kserve** (>=0.16.0): Core KServe inference protocol and model serving framework
- **kserve-storage** (>=0.16.0): Storage abstraction for model loading
- **sklearnserver** (>=0.16.0): Scikit-learn model server
- **xgbserver** (>=0.16.0): XGBoost model server
- **lgbserver** (>=0.16.0): LightGBM model server
