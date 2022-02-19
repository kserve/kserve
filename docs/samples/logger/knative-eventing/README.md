# Knative Eventing Inference Logger Demo

This demo assumes you have a cluster running with [Knative Eventing installed](https://knative.dev/docs/eventing/getting-started/),
along with KFServing.

Note: this was tested using Knative Eventing v0.17.

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

Next, a channel broker needs to be created, so we will use the following resource YAML:

```
apiVersion: eventing.knative.dev/v1
kind: broker
metadata:
 name: default
```

Let's apply the resource to the cluster:

```
kubectl create -f broker.yaml
```

Check the broker status with the following:

```
kubectl get broker default
```

Example Output:

```
NAME      URL                                                                        AGE     READY   REASON
default   http://broker-ingress.knative-eventing.svc.cluster.local/default/default   1m2s    True

```

Take note of the broker **URL** as that is what we'll be using in the InferenceService later on.

We now create a trigger to pass events to our message-dumper service with the following resource YAML:

```
apiVersion: eventing.knative.dev/v1
kind: Trigger
metadata:
  name: message-dumper-trigger
spec:
  broker: default
  subscriber:
    ref:
      apiVersion: serving.knative.dev/v1
      kind: Service
      name: message-dumper
```

Let's apply this resource to the cluster.

```
kubectl create -f trigger.yaml
```

We can now create an sklearn predictor with a logger:

```
apiVersion: serving.kserve.io/v1beta1
kind: InferenceService
metadata:
  name: sklearn-iris
spec:
  predictor:
    minReplicas: 1
    logger:
      mode: all
      url: http://broker-ingress.knative-eventing.svc.cluster.local/default/default
    sklearn:
      storageUri: gs://kfserving-examples/models/sklearn/1.0/model
```

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
  id: defb5816-35f7-4947-a2b1-b9e5d7764ad2
  time: 2021-04-10T01:22:16.498917288Z
  datacontenttype: application/json
Extensions,
  endpoint:
  inferenceservicename: sklearn-iris
  knativearrivaltime: 2021-04-10T01:22:16.500656431Z
  knativehistory: default-kne-trigger-kn-channel.default.svc.cluster.local
  namespace: default
  traceparent: 00-16456300519c5227ffe5f784a88da2f7-2db26af1daae870c-00
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
  id: defb5816-35f7-4947-a2b1-b9e5d7764ad2
  time: 2021-04-10T01:22:16.500492939Z
  datacontenttype: application/json
Extensions,
  endpoint:
  inferenceservicename: sklearn-iris
  knativearrivaltime: 2021-04-10T01:22:16.501931207Z
  knativehistory: default-kne-trigger-kn-channel.default.svc.cluster.local
  namespace: default
  traceparent: 00-2156a24451a4d4ea575fcf6c4f52a672-2b6ea035c83d3200-00
Data,
  {
    "predictions": [
      1,
      1
    ]
  }

```
