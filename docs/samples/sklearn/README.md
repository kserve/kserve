## Creating your own model and testing the Scikit-Learn server.

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

Then, we can run the scikit-learn server using the generated model and test for prediction. Models can be on local filesystem, S3 compatible object storage or Google Cloud Storage.

```shell
python -m sklearnserver --model_dir model.joblib --model_name svm
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
res = requests.post('http://localhost:8080/models/svm:predict', json=formData)
print(res)
print(res.text)
```

# Predict on a KFService using SKLearn

## Setup
1. Your ~/.kube/config should point to a cluster with [KFServing installed](https://github.com/kubeflow/kfserving/blob/master/docs/DEVELOPER_GUIDE.md#deploy-kfserving).
2. Your cluster's Istio Ingress gateway must be network accessible.
3. Your cluster's Istio Egresss gateway must [allow Google Cloud Storage](https://knative.dev/docs/serving/outbound-network-access/)

## Create the KFService

Apply the CRD
```
kubectl apply -f sklearn.yaml
```

Expected Output
```
$ kfservice.serving.kubeflow.org/sklearn-iris created
```
## Run a prediction

```
MODEL_NAME=sklearn-iris
INPUT_PATH=@./iris-input.json
CLUSTER_IP=$(kubectl -n istio-system get service istio-ingressgateway -o jsonpath='{.status.loadBalancer.ingress[0].ip}')

curl -v -H "Host: sklearn-iris.default.svc.cluster.local" http://$CLUSTER_IP/models/$MODEL_NAME:predict -d $INPUT_PATH
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
