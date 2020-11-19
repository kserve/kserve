# Predict on a `InferenceService` with a saved model from a URI
This allows you to specify a model object via the URI (Uniform Resource Identifier) of the model object exposed via an `http` or `https` endpoint. 

This `storageUri` option supports single file models, like `sklearn` which is specified by a [joblib](https://joblib.readthedocs.io/en/latest/) file, or artifacts (e.g. `tar` or `zip`) which contain all the necessary dependencies for other model types (e.g. `tensorflow` or `pytorch`). Here, we'll show examples from both of the above.

## Setup
1. Your ~/.kube/config should point to a cluster with [KFServing installed](https://github.com/kubeflow/kfserving/#install-kfserving).
2. Your cluster's Istio Ingress gateway must be network accessible.
3. Your cluster's Istio Egress gateway must [allow http / https traffic](https://knative.dev/docs/serving/outbound-network-access/)

## Sklearn
### Train and freeze the model
Here, we'll train a simple iris model. Please note that `kfserving` requires `sklearn==0.20.3`. 

```python
from sklearn import svm
from sklearn import datasets
import joblib

def train(X, y):
    clf = svm.SVC(gamma='auto')
    clf.fit(X, y)
    return clf

def freeze(clf, path='../frozen'):
    joblib.dump(clf, f'{path}/model.joblib')
    return True

if __name__ == '__main__':
    iris = datasets.load_iris()
    X, y = iris.data, iris.target
    clf = train(X, y)
    freeze(clf)
```
Now, you'll need to take that frozen model object and put it somewhere on the web to expose it. For instance, pushing the `model.joblib` file to some repo on GitHub.

### Specify and create the `InferenceService`
```yaml
apiVersion: serving.kubeflow.org/v1alpha2
kind: InferenceService
metadata:
  name: sklearn-from-uri
spec:
  default:
    predictor:
      sklearn:
        storageUri: https://github.com/tduffy000/kfserving-uri-examples/blob/master/sklearn/frozen/model.joblib?raw=true

```

Apply the CRD,
```bash
kubectl apply -f sklearn_uri.yaml
```
Expected Output
```
$ inferenceservice.serving.kubeflow.org/sklearn-from-uri created
```
### Run a prediction
The first is to [determine the ingress IP and ports](https://github.com/kubeflow/kfserving/blob/master/README.md#determine-the-ingress-ip-and-ports) and set `INGRESS_HOST` and `INGRESS_PORT`.

Now, if everything went according to plan you should be able to hit the endpoint exposing the model we just uploaded.

```bash
MODEL_NAME=sklearn-from-uri
INPUT_PATH=@./input.json
curl -v -H "Host: ${SERVICE_HOSTNAME}" http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/$MODEL_NAME:predict -d $INPUT_PATH
```
Expected Output
```
$ *   Trying 10.0.1.16...
* TCP_NODELAY set
  % Total    % Received % Xferd  Average Speed   Time    Time     Time  Current
                                 Dload  Upload   Total   Spent    Left  Speed
  0     0    0     0    0     0      0      0 --:--:-- --:--:-- --:--:--     0* Connected to 10.0.1.16 (10.0.1.16) port 30749 (#0)
> POST /v1/models/sklearn-from-uri:predict HTTP/1.1
> Host: sklearn-from-uri.kfserving-uri-storage.example.com
> User-Agent: curl/7.58.0
> Accept: */*
> Content-Length: 86
> Content-Type: application/x-www-form-urlencoded
> 
} [86 bytes data]
* upload completely sent off: 86 out of 86 bytes
< HTTP/1.1 200 OK
< content-length: 23
< content-type: application/json; charset=UTF-8
< date: Thu, 06 Aug 2020 23:13:42 GMT
< server: istio-envoy
< x-envoy-upstream-service-time: 7
< 
{ [23 bytes data]
100   109  100    23  100    86    605   2263 --:--:-- --:--:-- --:--:--  2868
* Connection #0 to host 10.0.1.16 left intact
{
  "predictions": [
    1,
    1
  ]
}
```

## Tensorflow
This will serve as an example of the ability to also pull in a tarball containing all of the 
required model dependencies, for instance `tensorflow` requires multiple files in a strict directory structure in order to be servable. 
### Train and freeze the model

```python
from sklearn import datasets
import numpy as np
import tensorflow as tf

def _ohe(targets):
    y = np.zeros((150, 3))
    for i, label in enumerate(targets):
        y[i, label] = 1.0
    return y

def train(X, y, epochs, batch_size=16):
    model = tf.keras.Sequential([
        tf.keras.layers.InputLayer(input_shape=(4,)),
        tf.keras.layers.Dense(16, activation=tf.nn.relu),
        tf.keras.layers.Dense(16, activation=tf.nn.relu),
        tf.keras.layers.Dense(3, activation='softmax')
    ])
    model.compile(tf.keras.optimizers.RMSprop(learning_rate=0.001), loss='categorical_crossentropy', metrics=['accuracy'])
    model.fit(X, y, epochs=epochs)
    return model

def freeze(model, path='../frozen'):
    model.save(f'{path}/0001')
    return True

if __name__ == '__main__':
    iris = datasets.load_iris()
    X, targets = iris.data, iris.target
    y = _ohe(targets)
    model = train(X, y, epochs=50)
    freeze(model)
```
The post-training procedure here is a bit different. Instead of directly pushing the frozen output to some URI, we'll need to package them into a tarball. To do so, 
```bash
cd ../frozen
tar -cvf artifacts.tar 0001/
gzip < artifacts.tar > artifacts.tgz
```
Where we assume the `0001/` directory has the structure:
```
|-- 0001/
|-- saved_model.pb
|-- variables/
|--- variables.data-00000-of-00001
|--- variables.index
```
Note that building the tarball from the directory specifying a version number is required for `tensorflow`.

Now, you can either push the `.tar` or `.tgz` file to some remote uri.
### Specify and create the `InferenceService`
And again, if everything went to plan we should be able to pull down the tarball and expose the endpoint.

```yaml
apiVersion: serving.kubeflow.org/v1alpha2
kind: InferenceService
metadata:
  name: tensorflow-from-uri-gzip
spec:
  default:
    predictor:
      tensorflow:
        storageUri: https://raw.githubusercontent.com/tduffy000/kfserving-uri-examples/master/tensorflow/frozen/model_artifacts.tar.gz
```
Apply the CRD,
```bash
kubectl apply -f tensorflow_uri.yaml
```
Expected Output
```
$ inferenceservice.serving.kubeflow.org/tensorflow-from-uri created
```

## Run a prediction
Again, make sure to first [determine the ingress IP and ports](https://github.com/kubeflow/kfserving/blob/master/README.md#determine-the-ingress-ip-and-ports) and set `INGRESS_HOST` and `INGRESS_PORT`.

Now that our endpoint is up and running, we can get some predictions.

```bash
MODEL_NAME=tensorflow-from-uri
INPUT_PATH=@./input.json
curl -v -H "Host: ${SERVICE_HOSTNAME}" http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/$MODEL_NAME:predict -d $INPUT_PATH
```
Expected Output
```
$ *   Trying 10.0.1.16...
* TCP_NODELAY set
  % Total    % Received % Xferd  Average Speed   Time    Time     Time  Current
                                 Dload  Upload   Total   Spent    Left  Speed
  0     0    0     0    0     0      0      0 --:--:-- --:--:-- --:--:--     0* Connected to 10.0.1.16 (10.0.1.16) port 30749 (#0)
> POST /v1/models/tensorflow-from-uri:predict HTTP/1.1
> Host: tensorflow-from-uri.default.example.com
> User-Agent: curl/7.58.0
> Accept: */*
> Content-Length: 86
> Content-Type: application/x-www-form-urlencoded
> 
} [86 bytes data]
* upload completely sent off: 86 out of 86 bytes
< HTTP/1.1 200 OK
< content-length: 112
< content-type: application/json
< date: Thu, 06 Aug 2020 23:21:19 GMT
< x-envoy-upstream-service-time: 151
< server: istio-envoy
< 
{ [112 bytes data]
100   198  100   112  100    86    722    554 --:--:-- --:--:-- --:--:--  1285
* Connection #0 to host 10.0.1.16 left intact
{
  "predictions": [
    [
      0.0204100646,
      0.680984616,
      0.298605353
    ],
    [
      0.0296604875,
      0.658412039,
      0.311927497
    ]
  ]
}
```