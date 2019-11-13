# Knative Eventing Inference Logger Demo

This demo assumes you have a cluster running with [KNative eventing installed](https://knative.dev/docs/eventing/getting-started/), along with kfserving.

First let us create a message dumper KNative service which will print out the Cloud Events it receives.
We will use the following resource yaml:

```
apiVersion: serving.knative.dev/v1alpha1
kind: Service
metadata:
  name: message-dumper
spec:
  template:
    spec:
      containers:
      - image: gcr.io/knative-releases/github.com/knative/eventing-sources/cmd/event_display

```

Let's apply the resource to the cluster:

```
kubectl create -f message-dumper.yaml
```

Now lets activate the default broker on the default namespace:

```
kubectl label namespace default knative-eventing-injection=enabled
```

We now create a trigger to pass events to our message-dumper service with the following resource yaml:

```
apiVersion: eventing.knative.dev/v1alpha1
kind: Trigger
metadata:
  name: message-dumper-trigger
  namespace: default
spec:
  subscriber:
    ref:
      apiVersion: serving.knative.dev/v1alpha1
      kind: Service
      name: message-dumper

```

Let's apply this resource to the cluster.

```
kubectl create -f trigger.yaml
```

We can now create a sklearn predictor with a logger:

```
apiVersion: "serving.kubeflow.org/v1alpha2"
kind: "InferenceService"
metadata:
  name: "sklearn-iris"
spec:
  default:
    predictor:
      minReplicas: 1
      logger:
        mode: all
      sklearn:
        storageUri: "gs://kfserving-samples/models/sklearn/iris"
```

Let's apply this yaml:

```
kubectl create -f sklearn-logging.yaml
```

We can now send a request to the sklearn model:

```
MODEL_NAME=sklearn-iris
INPUT_PATH=@./iris-input.json
CLUSTER_IP=$(kubectl -n istio-system get service istio-ingressgateway -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
SERVICE_HOSTNAME=$(kubectl get inferenceservice sklearn-iris -o jsonpath='{.status.url}' | cut -d "/" -f 3)
curl -v -H "Host: ${SERVICE_HOSTNAME}" http://$CLUSTER_IP/v1/models/$MODEL_NAME:predict -d $INPUT_PATH  -H "Content-Type: application/json"
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

If we now check the logs of the message dumper:

```
kubectl logs $(kubectl get pod -l serving.knative.dev/service=message-dumper -o jsonpath='{.items[0].metadata.name}') user-container
```

Expected output:

```
☁️  cloudevents.Event
Validation: valid
Context Attributes,
  specversion: 0.2
  type: org.kubeflow.serving.inference.request
  source: http://localhost:8080/
  id: 0f99bba8-dfa1-415f-b053-970890db8a60
  time: 2019-10-20T14:36:01.54878345Z
  contenttype: application/json
Data,
  {
    "instances": [
      [
        6.8,
        2.8,
        4.8,
        1.4
      ],
      [
        6.0,
        3.4,
        4.5,
        1.6
      ]
    ]
  }
☁️  cloudevents.Event
Validation: valid
Context Attributes,
  specversion: 0.2
  type: org.kubeflow.serving.inference.response
  source: http://localhost:8080/
  id: 0f99bba8-dfa1-415f-b053-970890db8a60
  time: 2019-10-20T14:36:01.564571486Z
  contenttype: application/json
Data,
  {
    "predictions": [
      1,
      1
    ]
  }
```
