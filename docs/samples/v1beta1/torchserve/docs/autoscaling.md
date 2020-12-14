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
spec:
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

Install hey load generator (go get -u github.com/rakyll/hey).

```bash
MODEL_NAME=mnist
SERVICE_HOSTNAME=$(kubectl get inferenceservice torchserve -o jsonpath='{.status.url}' | cut -d "/" -f 3)

./hey -m POST -z 30s -D ./mnist.json -host ${SERVICE_HOSTNAME} http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/${MODEL_NAME}:predict
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
