## Creating your own model and testing the LightGBM server.

To test the LightGBM Server, first we need to generate a simple LightGBM model using Python. 

```python
import lightgbm as lgb
from sklearn.datasets import load_iris
import os

model_dir = "."
BST_FILE = "model.bst"

iris = load_iris()
y = iris['target']
X = iris['data']
dtrain = lgb.Dataset(X, label=y)

params = {
    'objective':'multiclass', 
    'metric':'softmax',
    'num_class': 3
}
lgb_model = lgb.train(params=params, train_set=dtrain)
model_file = os.path.join(model_dir, BST_FILE)
lgb_model.save_model(model_file)
```

Then, we can install and run the [LightGBM Server](../../../../python/lgbserver) using the generated model and test for prediction. Models can be on local filesystem, S3 compatible object storage, Azure Blob Storage, or Google Cloud Storage.

```shell
python -m lgbserver --model_dir /path/to/model_dir --model_name lgb
```

We can also do some simple predictions

```python
import requests

request = {'sepal_width_(cm)': {0: 3.5}, 'petal_length_(cm)': {0: 1.4}, 'petal_width_(cm)': {0: 0.2},'sepal_length_(cm)': {0: 5.1} }
formData = {
    'inputs': [request]
}
res = requests.post('http://localhost:8080/v1/models/lgb:predict', json=formData)
print(res)
print(res.text)
```

## Predict on a InferenceService using LightGBM Server

## Setup
1. Your ~/.kube/config should point to a cluster with [KServe installed](https://github.com/kserve/kserve#installation).
2. Your cluster's Istio Ingress gateway must be [network accessible](https://istio.io/latest/docs/tasks/traffic-management/ingress/ingress-control/).

## Create the InferenceService

Apply the CRD
```
kubectl apply -f lightgbm.yaml
```

Expected Output
```
$ inferenceservice.serving.kserve.io/lightgbm-iris created
```

## Run a prediction
The first step is to [determine the ingress IP and ports](../../../../README.md#determine-the-ingress-ip-and-ports) and set `INGRESS_HOST` and `INGRESS_PORT`

```
MODEL_NAME=lightgbm-iris
INPUT_PATH=@./iris-input.json
SERVICE_HOSTNAME=$(kubectl get inferenceservice lightgbm-iris -o jsonpath='{.status.url}' | cut -d "/" -f 3)
curl -v -H "Host: ${SERVICE_HOSTNAME}" http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/$MODEL_NAME:predict -d $INPUT_PATH
```

Expected Output

```
*   Trying 169.63.251.68...
* TCP_NODELAY set
* Connected to 169.63.251.68 (169.63.251.68) port 80 (#0)
> POST /models/lightgbm-iris:predict HTTP/1.1
> Host: lightgbm-iris.default.svc.cluster.local
> User-Agent: curl/7.60.0
> Accept: */*
> Content-Length: 76
> Content-Type: application/x-www-form-urlencoded
>
* upload completely sent off: 76 out of 76 bytes
< HTTP/1.1 200 OK
< content-length: 27
< content-type: application/json; charset=UTF-8
< date: Tue, 21 May 2019 22:40:09 GMT
< server: istio-envoy
< x-envoy-upstream-service-time: 13032
<
* Connection #0 to host 169.63.251.68 left intact
{"predictions": [[0.9, 0.05, 0.05]]}
```

## Run LightGBM InferenceService with your own image
Since the KServe LightGBM image is built from a specific version of `lightgbm` pip package, sometimes it might not be compatible with the pickled model
you saved from your training environment, however you can build your own lgbserver image following [this instruction](../../../python/lgbserver/README.md#building-your-own-ligthgbm-server-docker-image).

To use your lgbserver image:
- Add the image to the KServe [configmap](../../../../config/configmap/inferenceservice.yaml)
```yaml
        "lightgbm": {
            "image": "<your-dockerhub-id>/kfserving/lgbserver",
        },
```
- Specify the `runtimeVersion` on `InferenceService` spec
```yaml
apiVersion: "serving.kserve.io/v1beta1"
kind: "InferenceService"
metadata:
  name: "lightgbm-iris"
spec:
  predictor:
    lightgbm:
      storageUri: "gs://kfserving-examples/models/lightgbm/iris"
      runtimeVersion: X.X.X
```
