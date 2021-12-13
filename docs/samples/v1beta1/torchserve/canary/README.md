# Canary Rollout

## Create InferenceService with default model

```yaml
apiVersion: "serving.kserve.io/v1beta1"
kind: "InferenceService"
metadata:
  name: "torchserve"
spec:
  predictor:
    pytorch:
      storageUri: "gs://kfserving-examples/models/torchserve/image_classifier"
```

Apply the InferenceService

```bash
kubectl apply -f torchserve.yaml
```

Expected Output

```bash
$inferenceservice.serving.kserve.io/torchserve created
```

## Create InferenceService with canary model

Change the `storageUri` for the new model version and apply the InferenceService

```yaml
apiVersion: "serving.kserve.io/v1beta1"
kind: "InferenceService"
metadata:
  name: "torchserve"
spec:
  predictor:
    canaryTrafficPercent: 20
    pytorch:
      storageUri: "gs://kfserving-examples/models/torchserve/image_classifier/v2"
```

Apply the InferenceService

```bash
kubectl apply -f canary.yaml
```

You should now see two revisions created

```bash
kubectl get revisions -l serving.kserve.io/inferenceservice=torchserve
NAME                                 CONFIG NAME                    K8S SERVICE NAME                     GENERATION   READY   REASON
torchserve-predictor-default-9lttm   torchserve-predictor-default   torchserve-predictor-default-9lttm   1            True
torchserve-predictor-default-kxp96   torchserve-predictor-default   torchserve-predictor-default-kxp96   2            True
```

## Run a prediction

The first step is to [determine the ingress IP and ports](../../../../../README.md#determine-the-ingress-ip-and-ports) and set `INGRESS_HOST` and `INGRESS_PORT`

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
> Host: torchserve.kserve-test.example.com
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

## Check the traffic split between the two revisions

```bash
kubectl get pods -l serving.kserve.io/inferenceservice=torchserve
NAME                                                             READY   STATUS    RESTARTS   AGE
torchserve-predictor-default-9lttm-deployment-7dd5cff4cb-tmmlc   2/2     Running   0          21m
torchserve-predictor-default-kxp96-deployment-5d949864df-bmzfk   2/2     Running   0          20m
```

Check the traffic split

```bash
kubectl get ksvc torchserve-predictor-default -oyaml
  status:
    address:
      url: http://torchserve-predictor-default.default.svc.cluster.local
    traffic:
    - latestRevision: true
      percent: 20
      revisionName: torchserve-predictor-default-kxp96
      tag: latest
      url: http://latest-torchserve-predictor-default.default.example.com
    - latestRevision: false
      percent: 80
      revisionName: torchserve-predictor-default-9lttm
      tag: prev
      url: http://prev-torchserve-predictor-default.default.example.com
    url: http://torchserve-predictor-default.default.example.com
```
