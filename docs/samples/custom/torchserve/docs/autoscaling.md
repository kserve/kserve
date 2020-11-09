# Autoscaling

## Deployment yaml

For example, specify a “concurrency target” of “5”, the autoscaler will try to make sure that every replica receives on average 5 requests at a time.
By default the pod scale with concurrency metrics

Here we use bert model to test autoscaling. Refer [bert-sample](../bert-sample/hugging-face-bert-sample.md) for running bert example.

```yaml
apiVersion: "serving.kubeflow.org/v1beta1"
kind: "InferenceService"
metadata:
  name: torchserve-bert
  annotations:
    autoscaling.knative.dev/target: "5"
spec:
  predictor:
    containers:
    - image: {username}/torchserve-bert:latest
      name: kfserving-container
      env:
        - name: STORAGE_URI
          value: "gs://kfserving-examples/models/torchserve/bert/"
```

## Create the InferenceService

Apply the CRD

```bash
kubectl apply -f autoscale.yaml
```

Expected Output

```bash
$inferenceservice.serving.kubeflow.org/torchserve-bert created
```

## Run a prediction

The first step is to [determine the ingress IP and ports](../../../README.md#determine-the-ingress-ip-and-ports) and set `INGRESS_HOST` and `INGRESS_PORT`

### Steps

1. Copy input file from [here](https://github.com/pytorch/serve/tree/master/examples/Huggingface_Transformers/Seq_classification_artifacts) to the working directory.
2. Install hey load generator (go get -u github.com/rakyll/hey).

```bash
MODEL_NAME=torchserve-bert
SERVICE_HOSTNAME=$(kubectl get inferenceservice ${MODEL_NAME} <namespace> -o jsonpath='{.status.url}' | cut -d "/" -f 3)

./hey -m POST -z 30s -T "application/x-www-form-urlencoded; charset=utf-8" -d 'data=$(cat sample_text.txt)' -host ${SERVICE_HOSTNAME} http://${INGRESS_HOST}:${INGRESS_PORT}/predictions/BERTSeqClassification
```

### Get Pods

```bash
Kubectl get pods -n kfserving-test

NAME                                                             READY   STATUS        RESTARTS   AGE
torchserve-bert-cj2d8-deployment-69444c9c74-rq8jq   2/2     Running       0          50m
torchserve-bert-cj2d8-deployment-69444c9c74-tsrwr   2/2     Running       0          113s
torchserve-bert-cj2d8-deployment-69444c9c74-vvpjl   2/2     Running       0          109s
torchserve-bert-cj2d8-deployment-69444c9c74-xvn7t   2/2     PodInitializing   0          103s
```
