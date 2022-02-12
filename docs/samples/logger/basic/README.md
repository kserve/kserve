# Inference Logger Demo

First let us create a message dumper Knative service which will print out the CloudEvents it receives.
We will use the following resource YAML:

```
apiVersion: serving.knative.dev/v1
kind: Service
metadata:
  name: message-dumper
spec:
  template:
    spec:
      containers:
      - image: gcr.io/knative-releases/knative.dev/eventing-contrib/cmd/event_display

```

Let's apply the resource to the cluster:

```
kubectl create -f message-dumper.yaml
```

We can now create an sklearn predictor with a logger which points at the message dumper. The YAML is shown below.

```
apiVersion: serving.kserve.io/v1beta1
kind: InferenceService
metadata:
  name: sklearn-iris
spec:
  predictor:
    logger:
      mode: all
      url: http://message-dumper.default/
    sklearn:
      storageUri: gs://kfserving-examples/models/sklearn/1.0/model
```

(Here we set the url explicitly. otherwise it defaults to the namespace knative broker or the value of DefaultUrl in the logger section of the controller configmap.)

Let's apply this YAML:

```
kubectl create -f sklearn-logging.yaml
```

We can now send a request to the sklearn model. Check the README [here](https://kserve.github.io/website/get_started/first_isvc/#3-determine-the-ingress-ip-and-ports)
to learn how to determine the INGRESS_HOST and INGRESS_PORT used in curling the InferenceService.

```
MODEL_NAME=sklearn-iris
INPUT_PATH=@./iris-input.json
SERVICE_HOSTNAME=$(kubectl get inferenceservice sklearn-iris -o jsonpath='{.status.url}' | cut -d "/" -f 3)
curl -v -H "Host: ${SERVICE_HOSTNAME}" http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/$MODEL_NAME:predict -d $INPUT_PATH
```

Expected response:

```
{"predictions": [1, 1]}
```

If we now check the logs of the message dumper, we can see the CloudEvents associated with our previous curl request.

```
kubectl logs $(kubectl get pod -l serving.knative.dev/service=message-dumper -o jsonpath='{.items[0].metadata.name}') user-container
```

Expected output:

```
☁️  cloudevents.Event
Validation: valid
Context Attributes,
  specversion: 1.0
  type: org.kubeflow.serving.inference.request
  source: http://localhost:9081/
  id: 0009174a-24a8-4603-b098-09c8799950e9
  time: 2021-04-10T00:23:26.0789529Z
  datacontenttype: application/json
Extensions,
  endpoint:
  inferenceservicename: sklearn-iris
  namespace: default
  traceparent: 00-90bdf848647d50283394155d2df58f19-84dacdfdf07cadfc-00
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
  specversion: 1.0
  type: org.kubeflow.serving.inference.response
  source: http://localhost:9081/
  id: 0009174a-24a8-4603-b098-09c8799950e9
  time: 2021-04-10T00:23:26.080736102Z
  datacontenttype: application/json
Extensions,
  endpoint:
  inferenceservicename: sklearn-iris
  namespace: default
  traceparent: 00-55de1514e1d23ee17eb50dda6167bb8c-b6c6e0f6dd8f741d-00
Data,
  {
    "predictions": [
      1,
      1
    ]
  }
```
