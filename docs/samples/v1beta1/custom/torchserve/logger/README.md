# Inference Logger Demo

First let us create a message dumper KNative service which will print out the Cloud Events it receives.
We will use the following resource yaml:

```yaml
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

```bash
kubectl apply -f message-dumper.yaml -n kserve-test
```

We can now create a torchserve predictor with a logger which points at the message dumper. The yaml is shown below.

```yaml
apiVersion: "serving.kserve.io/v1beta1"
kind: "InferenceService"
metadata:
  name: "torchserve-logger"
spec:
  predictor:
    minReplicas: 1
    logger:
      url: http://message-dumper.default.svc.cluster.local
      mode: all
    containers:
      - image: {username}/torchserve:latest
        name: torchserve-container
```

Let's apply this yaml:

```bash
kubectl apply -f torchserve-logger.yaml -n kserve-test
```

We can now send a request to the torchserve model.

```bash
wget https://raw.githubusercontent.com/pytorch/serve/master/examples/image_classifier/mnist/test_data/1.png

MODEL_NAME=torchserve-logger
INGRESS_GATEWAY=istio-ingressgateway
CLUSTER_IP=$(kubectl -n istio-system get service $INGRESS_GATEWAY -o jsonpath='{.status.loadBalancer.ingress[0].hostname}')
SERVICE_HOSTNAME=$(kubectl get inferenceservice torchserve-logger -o jsonpath='{.status.url}' | cut -d "/" -f 3)
curl -v -H "Host: ${SERVICE_HOSTNAME}" http://$CLUSTER_IP/predictions/mnist -T 1.png
```

Expected Output

```bash
*   Trying 44.240.85.24...
* Connected to a881f5a8c676a41edbccdb0a394a80d6-2069247558.us-west-2.elb.amazonaws.com (44.240.85.24) port 80 (#0)
> PUT /predictions/mnist HTTP/1.1
> Host: torchserve-custom.kserve-test.a881f5a8c676a41edbccdb0a394a80d6-2069247558.us-west-2.elb.amazonaws.com
> User-Agent: curl/7.47.0
> Accept: */*
> Content-Length: 273
> Expect: 100-continue
> 
< HTTP/1.1 100 Continue
* We are completely uploaded and fine
< HTTP/1.1 200 OK
< content-length: 1
< content-type: text/plain; charset=utf-8
< date: Wed, 25 Nov 2020 08:45:37 GMT
< x-envoy-upstream-service-time: 5010
< server: istio-envoy
< 
* Connection #0 to host a881f5a8c676a41edbccdb0a394a80d6-2069247558.us-west-2.elb.amazonaws.com left intact
1
```

If we now check the logs of the message dumper:

```bash
kubectl logs $(kubectl get pod -l serving.knative.dev/service=message-dumper -o jsonpath='{.items[0].metadata.name}') -c user-container
```

Expected output:

```bash
☁️  cloudevents.Event
Validation: valid
Context Attributes,
  specversion: 1.0
  type: org.kubeflow.serving.inference.request
  source: http://localhost:9081/
  id: 5011cc37-0749-4899-8422-f7ac39898cee
  time: 2020-11-25T08:45:32.947168928Z
  datacontenttype: application/json
Extensions,
  endpoint: 
  inferenceservicename: 
  namespace: kserve-test
  traceparent: 00-8dafd130bae82f9e470a43da22340b56-839d6d891e0d1343-00
Data,
  �PNG

IHDRWf�H�IDATx�c`� ��W�1E.��߿.P6Ϧ�^HRғ~��{Fʳ���8�d�߿O�B9ڏ���C�	\���{"�WT��� ��=�ùU��5��5.4��q���W:3����B���;w>����#�ի�"��0��ܰa;VI(����#��]��	(7��+��/�ph�
           ��CN��߿�xqH65n�!���I�x$L>�O��%I:|?����IEND�B`�
☁️  cloudevents.Event
Validation: valid
Context Attributes,
  specversion: 1.0
  type: org.kubeflow.serving.inference.response
  source: http://localhost:9081/
  id: 5011cc37-0749-4899-8422-f7ac39898cee
  time: 2020-11-25T08:45:37.95671692Z
  datacontenttype: application/json
Extensions,
  endpoint: 
  inferenceservicename: 
  namespace: kserve-test
  traceparent: 00-911901cf3b9c713a51516861833ce742-710557d1afcc3736-00
Data,
  1
```
