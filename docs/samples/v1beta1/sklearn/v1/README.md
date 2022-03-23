## Creating your own model and testing the SKLearn Server locally.

To test the [Scikit-Learn](https://scikit-learn.org/stable/) server, first we need to generate a simple scikit-learn model using Python. 

```python
from sklearn import svm
from sklearn import datasets
from joblib import dump
clf = svm.SVC(gamma='scale')
iris = datasets.load_iris()
X, y = iris.data, iris.target
clf.fit(X, y)
dump(clf, 'model.joblib')
```

Then, we can install and run the [SKLearn Server](../../../../../python/sklearnserver) using the generated model and test for prediction. Models can be on local filesystem, S3 compatible object storage, Azure Blob Storage, or Google Cloud Storage.

```shell
# we should indicate the directory containing the model file (model.joblib) by --model_dir
python -m sklearnserver --model_dir ./  --model_name svm
```

We can also use the inbuilt sklearn support for sample datasets and do some simple predictions

```python
from sklearn import datasets
import requests
iris = datasets.load_iris()
X, y = iris.data, iris.target
formData = {
    'instances': X[0:1].tolist()
}
res = requests.post('http://localhost:8080/v1/models/svm:predict', json=formData)
print(res)
print(res.text)
```

# Predict on an InferenceService using SKLearnServer

## Setup
1. Your ~/.kube/config should point to a cluster with [KServe installed](https://github.com/kserve/kserve#installation).
2. Your cluster's Istio Ingress gateway must be [network accessible](https://istio.io/latest/docs/tasks/traffic-management/ingress/ingress-control/).

## Create the InferenceService

Apply the CRD
```
kubectl apply -f sklearn.yaml
```

Expected Output
```
$ inferenceservice.serving.kserve.io/sklearn-iris created
```
## Run a prediction
The first step is to [determine the ingress IP and ports](https://kserve.github.io/website/get_started/first_isvc/#3-determine-the-ingress-ip-and-ports) and set `INGRESS_HOST` and `INGRESS_PORT`

```
MODEL_NAME=sklearn-iris
INPUT_PATH=@./iris-input.json
SERVICE_HOSTNAME=$(kubectl get inferenceservice sklearn-iris -o jsonpath='{.status.url}' | cut -d "/" -f 3)
curl -v -H "Host: ${SERVICE_HOSTNAME}" http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/$MODEL_NAME:predict -d $INPUT_PATH
```

Expected Output

```
*   Trying 169.63.251.68...
* TCP_NODELAY set
* Connected to 169.63.251.68 (169.63.251.68) port 80 (#0)
> POST /models/sklearn-iris:predict HTTP/1.1
> Host: sklearn-iris.default.svc.cluster.local
> User-Agent: curl/7.60.0
> Accept: */*
> Content-Length: 76
> Content-Type: application/x-www-form-urlencoded
>
* upload completely sent off: 76 out of 76 bytes
< HTTP/1.1 200 OK
< content-length: 23
< content-type: application/json; charset=UTF-8
< date: Mon, 20 May 2019 20:49:02 GMT
< server: istio-envoy
< x-envoy-upstream-service-time: 1943
<
* Connection #0 to host 169.63.251.68 left intact
{"predictions": [1, 1]}
```

## Run SKLearn InferenceService with your own image
Since the KServe SKLearnServer image is built from a specific version of `scikit-learn` pip package, sometimes it might not be compatible with the pickled model
you saved from your training environment, however you can build your own SKLearnServer image following [these instructions](../../../../../python/sklearnserver/README.md#building-your-own-scikit-learn-server-docker-image
).

To use your SKLearnServer image:
- Add the image to the KServe [configmap](https://github.com/kserve/kserve/blob/master/config/configmap/inferenceservice.yaml)
```yaml
        "sklearn": {
            "image": "<your-dockerhub-id>/kfserving/sklearnserver",
        },
```
- Specify the `runtimeVersion` on `InferenceService` spec
```yaml
apiVersion: "serving.kserve.io/v1beta1"
kind: "InferenceService"
metadata:
  name: "sklearn-iris"
spec:
  predictor:
    sklearn:
      storageUri: "gs://kfserving-examples/models/sklearn/1.0/model"
      runtimeVersion: X.X.X
```
