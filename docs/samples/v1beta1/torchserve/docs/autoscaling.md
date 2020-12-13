# Autoscaling

## Deploymnet yaml

For example, specify a “concurrency target” of “10”, the autoscaler will try to make sure that every replica receives on average 10 requests at a time.
By default the pod scale with concurrency metrics

Refer [model archive file generation](../model-archiver/README.md) for PV, PVC storage and marfile creation.

autoscaling.yaml
```yaml
apiVersion: "serving.kubeflow.org/v1beta1"
kind: "InferenceService"
metadata:
  name: "torchserve"
  annotations:
    autoscaling.knative.dev/target: "5"
"spec:
  predictor:
    pytorch:
      protocolVersion: v2
      storageUri: "pvc://model-pv-claim"
```

## Create the InferenceService

Apply the CRD

```bash
kubectl apply -f torchserve.yaml
```

Expected Output

```bash
$inferenceservice.serving.kubeflow.org/torchserve created
```

## Run a prediction

The first step is to [determine the ingress IP and ports](../../../README.md#determine-the-ingress-ip-and-ports) and set `INGRESS_HOST` and `INGRESS_PORT`

```bash
MODEL_NAME=mnist
SERVICE_HOSTNAME=$(kubectl get inferenceservice torchserve -o jsonpath='{.status.url}' | cut -d "/" -f 3)

curl -v -H "Host: ${SERVICE_HOSTNAME}" http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/${MODEL_NAME}:predict -d @./mnist.json
```

Expected Output

```bash
*   Trying 52.89.19.61...
* Connected to a881f5a8c676a41edbccdb0a394a80d6-2069247558.us-west-2.elb.amazonaws.com (52.89.19.61) port 80 (#0)
> PUT /v1/models/mnist:predict HTTP/1.1
> Host: torchserve-predictor-default.kfserving-test.example.com
> User-Agent: curl/7.47.0
> Accept: */*
> Content-Length: 167
> Expect: 100-continue
> 
< HTTP/1.1 100 Continue
* We are completely uploaded and fine
< HTTP/1.1 200 OK
< cache-control: no-cache; no-store, must-revalidate, private
< content-length: 1
< date: Tue, 27 Oct 2020 08:26:19 GMT
< expires: Thu, 01 Jan 1970 00:00:00 UTC
< pragma: no-cache
< x-request-id: b10cfc9f-cd0f-4cda-9c6c-194c2cdaa517
< x-envoy-upstream-service-time: 6
< server: istio-envoy
< 
* Connection #0 to host a881f5a8c676a41edbccdb0a394a80d6-2069247558.us-west-2.elb.amazonaws.com left intact
{"predictions": [["2"]]}
```

### Get Pods

```bash
Kubectl get pods -n kfserving-test 

NAME                                                             READY   STATUS        RESTARTS   AGE
torchserve-predictor-default-cj2d8-deployment-69444c9c74-67qwb   2/2     Terminating   0          103s
torchserve-predictor-default-cj2d8-deployment-69444c9c74-nnxk8   2/2     Terminating   0          95s
torchserve-predictor-default-cj2d8-deployment-69444c9c74-rq8jq   2/2     Running       0          50m
torchserve-predictor-default-cj2d8-deployment-69444c9c74-tsrwr   2/2     Running       0          113s
torchserve-predictor-default-cj2d8-deployment-69444c9c74-vvpjl   2/2     Running       0          109s
torchserve-predictor-default-cj2d8-deployment-69444c9c74-xvn7t   2/2     Terminating   0          103s
```
