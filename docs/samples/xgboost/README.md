## Creating your own model and testing the SKLearn server.

To test the XGBoost Server, first we need to generate a simple XGBoost model using Python. 

```python
import xgboost as xgb
from sklearn.datasets import load_digits
from xgbserver import XGBoostModel
digits = load_digits(2)
y = digits['target']
X = digits['data']
xgb_model = xgb.XGBClassifier(random_state=42).fit(X, y)
xgb_model.save_model("model.bst") 
```

Then, we can run the XGBoost Server using the generated model and test for prediction. Models can be on local filesystem, S3 compatible object storage or Google Cloud Storage.

```shell
python -m xgboostserver --model_dir /path/to/model_dir --model_name xgboost
```

We can also use the inbuilt sklearn support for sample datasets and do some simple predictions

```python
import xgboost as xgb
import requests
from sklearn.datasets import load_digits
from xgbserver import XGBoostModel
digits = load_digits(2)
y = digits['target']
X = digits['data']
formData = {
    'instances': X[0].tolist()
}
res = requests.post('http://localhost:8080/models/xgboost:predict', json=formData)
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

```
