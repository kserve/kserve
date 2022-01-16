# Autoscaling
KServe supports the implementation of Knative Pod Autoscaler (KPA) and Kubernetes’ Horizontal Pod Autoscaler (HPA).
The features and limitations of each of these Autoscalers are listed below.

IMPORTANT: If you want to use Kubernetes Horizontal Pod Autoscaler (HPA), you must install [HPA extension](https://knative.dev/docs/install/any-kubernetes-cluster/#optional-serving-extensions)
 after you install Knative Serving.

Knative Pod Autoscaler (KPA)
- Part of the Knative Serving core and enabled by default once Knative Serving is installed.
- Supports scale to zero functionality.
- Does not support CPU-based autoscaling.

Horizontal Pod Autoscaler (HPA)
- Not part of the Knative Serving core, and must be enabled after Knative Serving installation.
- Does not support scale to zero functionality.
- Supports CPU-based autoscaling.

## Create InferenceService with concurrency target


### Soft limit
You can configure InferenceService with annotation `autoscaling.knative.dev/target` for a soft limit. The soft limit is a targeted limit rather than
a strictly enforced bound, particularly if there is a sudden burst of requests, this value can be exceeded.

```yaml
apiVersion: "serving.kserve.io/v1beta1"
kind: "InferenceService"
metadata:
  name: "torchserve"
  annotations:
    autoscaling.knative.dev/target: "10"
spec:
  predictor:
    pytorch:
      storageUri: "gs://kfserving-examples/models/torchserve/image_classifier"
```

### Hard limit

You can also configure InferenceService with field `containerConcurrency` for a hard limit. The hard limit is an enforced upper bound. 
If concurrency reaches the hard limit, surplus requests will be buffered and must wait until enough capacity is free to execute the requests.

```yaml
apiVersion: "serving.kserve.io/v1beta1"
kind: "InferenceService"
metadata:
  name: "torchserve"
spec:
  predictor:
    containerConcurrency: 10
    pytorch:
      storageUri: "gs://kfserving-examples/models/torchserve/image_classifier"
```

### Create the InferenceService

```bash
kubectl apply -f torchserve.yaml
```

Expected Output

```bash
$inferenceservice.serving.kserve.io/torchserve created
```

## Run inference with concurrent requests

The first step is to [determine the ingress IP and ports](../../../../../README.md#determine-the-ingress-ip-and-ports) and set `INGRESS_HOST` and `INGRESS_PORT`

Install hey load generator 
```bash
go get -u github.com/rakyll/hey
```

Send concurrent inference requests
```bash
MODEL_NAME=mnist
SERVICE_HOSTNAME=$(kubectl get inferenceservice torchserve -o jsonpath='{.status.url}' | cut -d "/" -f 3)

./hey -m POST -z 30s -D ./mnist.json -host ${SERVICE_HOSTNAME} http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/${MODEL_NAME}:predict
```

### Check the pods that are scaled up
`hey` by default generates 50 requests concurrently, so you can see that the InferenceService scales to 5 pods as the container concurrency target is 10.

```bash
kubectl get pods -n kserve-test 

NAME                                                             READY   STATUS        RESTARTS   AGE
torchserve-predictor-default-cj2d8-deployment-69444c9c74-67qwb   2/2     Terminating   0          103s
torchserve-predictor-default-cj2d8-deployment-69444c9c74-nnxk8   2/2     Terminating   0          95s
torchserve-predictor-default-cj2d8-deployment-69444c9c74-rq8jq   2/2     Running       0          50m
torchserve-predictor-default-cj2d8-deployment-69444c9c74-tsrwr   2/2     Running       0          113s
torchserve-predictor-default-cj2d8-deployment-69444c9c74-vvpjl   2/2     Running       0          109s
torchserve-predictor-default-cj2d8-deployment-69444c9c74-xvn7t   2/2     Terminating   0          103s
```
