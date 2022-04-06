# Deploying XGBoost models

This example walks you through how to deploy a `xgboost` model leveraging the
`v1beta1` version of the `InferenceService` CRD.
Note that, by default the `v1beta1` version will expose your model through an
API compatible with the existing V1 Dataplane.
However, this example will show you how to serve a model through an API
compatible with the new [V2 Dataplane](../../../predict-api/v2).

## Training

The first step will be to train a sample `xgboost` model.
We will save this model as `model.bst`.

```python
import xgboost as xgb
from sklearn.datasets import load_iris
import os

model_dir = "."
BST_FILE = "model.bst"

iris = load_iris()
y = iris['target']
X = iris['data']
dtrain = xgb.DMatrix(X, label=y)
param = {'max_depth': 6,
            'eta': 0.1,
            'silent': 1,
            'nthread': 4,
            'num_class': 10,
            'objective': 'multi:softmax'
            }
xgb_model = xgb.train(params=param, dtrain=dtrain)
model_file = os.path.join((model_dir), BST_FILE)
xgb_model.save_model(model_file)
```

## Testing locally

Once we've got our `model.bst` model serialised, we can then use
[MLServer](https://github.com/SeldonIO/MLServer) to spin up a local server.
For more details on MLServer, feel free to check the [XGBoost example in their
docs](https://github.com/SeldonIO/MLServer/tree/master/examples/xgboost).

> Note that this step is optional and just meant for testing.
> Feel free to jump straight to [deploying your trained model](#deployment).

### Pre-requisites

Firstly, to use MLServer locally, you will first need to install the `mlserver`
package in your local environment as well as the XGBoost runtime.

```bash
pip install mlserver mlserver-xgboost
```

### Model settings

The next step will be providing some model settings so that
MLServer knows:

- The inference runtime that we want our model to use (i.e.
  `mlserver_xgboost.XGBoostModel`)
- Our model's name and version

These can be specified through environment variables or by creating a local
`model-settings.json` file:

```json
{
  "name": "xgboost-iris",
  "version": "v1.0.0",
  "implementation": "mlserver_xgboost.XGBoostModel"
}
```

Note that, when we [deploy our model](#deployment), **KServe will already
inject some sensible defaults** so that it runs out-of-the-box without any
further configuration.
However, you can still override these defaults by providing a
`model-settings.json` file similar to your local one.
You can even provide a [set of `model-settings.json` files to load multiple
models](https://github.com/SeldonIO/MLServer/tree/master/examples/mms).

### Serving our model locally

With the `mlserver` package installed locally and a local `model-settings.json`
file, we should now be ready to start our server as:

```bash
mlserver start .
```

## Deployment

Lastly, we will use KServe to deploy our trained model.
For this, we will just need to use **version `v1beta1`** of the
`InferenceService` CRD and set the the **`protocolVersion` field to `v2`**.

```yaml
apiVersion: "serving.kserve.io/v1beta1"
kind: "InferenceService"
metadata:
  name: "xgboost-iris"
spec:
  predictor:
    xgboost:
      protocolVersion: "v2"
      storageUri: "gs://kfserving-examples/models/xgboost/iris"
```

Note that this makes the following assumptions:

- Your model weights (i.e. your `model.bst` file) have already been uploaded
  to a "model repository" (GCS in this example) and can be accessed as
  `gs://kfserving-examples/models/xgboost/iris`.
- There is a K8s cluster available, accessible through `kubectl`.
- KServe has already been [installed in your
  cluster](https://github.com/kserve/kserve#installation).

Assuming that we've got a cluster accessible through `kubectl` with KServe
already installed, we can deploy our model as:

```
kubectl apply -f ./xgboost.yaml
```

### Testing deployed model

We can now test our deployed model by sending a sample request.

Note that this request **needs to follow the [V2 Dataplane
protocol](../../../predict-api/v2)**.
You can see an example payload below:

```json
{
  "inputs": [
    {
      "name": "input-0",
      "shape": [2, 4],
      "datatype": "FP32",
      "data": [
        [6.8, 2.8, 4.8, 1.4],
        [6.0, 3.4, 4.5, 1.6]
      ]
    }
  ]
}
```

Now, assuming that our ingress can be accessed at
`${INGRESS_HOST}:${INGRESS_PORT}`, we can use `curl` to send our inference
request as:

> You can follow [these instructions](../../../../README.md#determine-the-ingress-ip-and-ports) to find
> out your ingress IP and port.

```bash
SERVICE_HOSTNAME=$(kubectl get inferenceservice xgboost-iris -o jsonpath='{.status.url}' | cut -d "/" -f 3)

curl -v \
  -H "Host: ${SERVICE_HOSTNAME}" \
  -d @./iris-input.json \
  http://${INGRESS_HOST}:${INGRESS_PORT}/v2/models/xgboost-iris/infer
```

The output will be something similar to:

```json
{
  "id": "4e546709-0887-490a-abd6-00cbc4c26cf4",
  "model_name": "xgboost-iris",
  "model_version": "v1.0.0",
  "outputs": [
    {
      "data": [1.0, 1.0],
      "datatype": "FP32",
      "name": "predict",
      "parameters": null,
      "shape": [2]
    }
  ]
}
```
