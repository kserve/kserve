## Creating your own model and testing the SKLearn server.

To test the XGBoost Server, first we need to generate a simple XGBoost model using Python. 

```python
import xgboost as xgb
from sklearn.datasets import load_iris
import os
from xgbserver import XGBoostModel

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

Then, we can run the XGBoost Server using the generated model and test for prediction. Models can be on local filesystem, S3 compatible object storage, Azure Blob Storage, or Google Cloud Storage.

```shell
python -m xgbserver --model_dir /path/to/model_dir --model_name xgb
```

We can also use the inbuilt sklearn support for sample datasets and do some simple predictions

```python
from sklearn.datasets import load_iris
import requests
from xgbserver import XGBoostModel

model_dir = "."
BST_FILE = "model.bst"

iris = load_iris()
y = iris['target']
X = iris['data']

request = [X[0].tolist()]
formData = {
    'instances': request
}
res = requests.post('http://localhost:8080/models/xgb:predict', json=formData)
print(res)
print(res.text)
```

## Predict on a KFService using XGBoost

## Setup
1. Your ~/.kube/config should point to a cluster with [KFServing installed](https://github.com/kubeflow/kfserving/blob/master/docs/DEVELOPER_GUIDE.md#deploy-kfserving).
2. Your cluster's Istio Ingress gateway must be network accessible.
3. Your cluster's Istio Egresss gateway must [allow Google Cloud Storage](https://knative.dev/docs/serving/outbound-network-access/)

## Create the KFService

Apply the CRD
```
kubectl apply -f xgboost.yaml
```

Expected Output
```
$ kfservice.serving.kubeflow.org/xgboost-iris created
```

## Run a prediction

```
MODEL_NAME=xgboost-iris
INPUT_PATH=@./iris-input.json
CLUSTER_IP=$(kubectl -n istio-system get service istio-ingressgateway -o jsonpath='{.status.loadBalancer.ingress[0].ip}')

curl -v -H "Host: xgboost-iris.default.svc.cluster.local" http://$CLUSTER_IP/models/$MODEL_NAME:predict -d $INPUT_PATH
```

Expected Output

```
*   Trying 169.63.251.68...
* TCP_NODELAY set
* Connected to 169.63.251.68 (169.63.251.68) port 80 (#0)
> POST /models/xgboost-iris:predict HTTP/1.1
> Host: xgboost-iris.default.svc.cluster.local
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
{"predictions": [1.0, 1.0]}
```
