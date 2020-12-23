# Canary Rollouts

## Creating model storage with model archive file

Refer [model archive file generation](../model-archiver/README.md) for PV, PVC storage and marfile creation.

## Deployment yaml

### Main model

```yaml
apiVersion: "serving.kubeflow.org/v1beta1"
kind: "InferenceService"
metadata:
  name: "torch-pred"
spec:
  predictor:
    pytorch:
      protocolVersion: v2
      storageUri: "pvc://model-pv-claim"
```

### Canary model

Change the path and deploy

```yaml
apiVersion: "serving.kubeflow.org/v1beta1"
kind: "InferenceService"
metadata:
  name: "torch-pred"
spec:
  predictor:
    canaryTrafficPercent: 20
    pytorch:
      storageUri: "pvc://model-store-claim-1"
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
SERVICE_HOSTNAME=$(kubectl get inferenceservice torch-pred  -o jsonpath='{.status.url}' | cut -d "/" -f 3)

curl -v -H "Host: ${SERVICE_HOSTNAME}" http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/${MODEL_NAME}:predict -d @./mnist.json
```

Expected Output

```bash
*   Trying 52.89.19.61...
* Connected to a881f5a8c676a41edbccdb0a394a80d6-2069247558.us-west-2.elb.amazonaws.com (52.89.19.61) port 80 (#0)
> PUT /v1/models/mnist:predict HTTP/1.1
> Host: torch-pred.kfserving-test.example.com
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
{"predictions": ["2"]}
```

### Get Pods

```bash
kubectl get pods -n kfserving-test 

NAME                                                             READY   STATUS        RESTARTS   AGE
torch-pred-predictor-default-cj2d8-deployment-69444c9c74-tsrwr   2/2     Running       0          113s
torch-pred-predictor-default-cj2d8-deployment-69444c9c74-vvpjl   2/2     Running       0          109s
```
