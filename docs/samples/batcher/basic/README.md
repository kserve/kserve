# Inference Batcher Demo

We first create a pytorch predictor with a batcher. The "maxLatency" is set to a big value (5000.0 milliseconds) to make us be able to observe the batching process.

```
apiVersion: "serving.kubeflow.org/v1alpha2"
kind: "InferenceService"
metadata:
  name: "pytorch-cifar10"
spec:
  default:
    predictor:
      minReplicas: 1
      batcher:
        maxBatchSize: "32"
        maxLatency: "5000.0"
        timeout: "60"
      pytorch:
        storageUri: "gs://kfserving-samples/models/pytorch/cifar10/"
```

Let's apply this yaml:

```
kubectl create -f pytorch-batcher.yaml
```

We can now send requests to the pytorch model using hey.

```
MODEL_NAME=pytorch-cifar10
INPUT_PATH=@./input.json
INGRESS_GATEWAY=istio-ingressgateway
CLUSTER_IP=$(kubectl -n istio-system get service $INGRESS_GATEWAY -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
SERVICE_HOSTNAME=$(kubectl get inferenceservice pytorch-cifar10 -o jsonpath='{.status.url}' | cut -d "/" -f 3)
hey -z 10s -c 5 -m POST -host "${SERVICE_HOSTNAME}" -H "Content-Type: application/json" -D ./input.json "http://$CLUSTER_IP/v1/models/$MODEL_NAME:predict"
```

The request will go to the batcher container first, and then the batcher container will do batching and send the batching request to the predictor container.

Notice: If the interval of sending the two requests is less than "maxLatency", the returned "batchId" will be the same.

Expected Output for each ssh terminal tab.

```
Summary:
  Total:        10.6268 secs
  Slowest:      1.6477 secs
  Fastest:      0.0050 secs
  Average:      0.1006 secs
  Requests/sec: 48.1800

  Total data:   167424 bytes
  Size/request: 327 bytes

Response time histogram:
  0.005 [1]     |
  0.169 [447]   |■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■
  0.334 [30]    |■■■
  0.498 [7]     |■
  0.662 [10]    |■
  0.826 [3]     |
  0.991 [6]     |■
  1.155 [5]     |
  1.319 [1]     |
  1.483 [1]     |
  1.648 [1]     |


Latency distribution:
  10% in 0.0079 secs
  25% in 0.0114 secs
  50% in 0.0398 secs
  75% in 0.0867 secs
  90% in 0.2029 secs
  95% in 0.5170 secs
  99% in 1.1428 secs

Details (average, fastest, slowest):
  DNS+dialup:   0.0000 secs, 0.0050 secs, 1.6477 secs
  DNS-lookup:   0.0000 secs, 0.0000 secs, 0.0000 secs
  req write:    0.0002 secs, 0.0001 secs, 0.0004 secs
  resp wait:    0.1000 secs, 0.0046 secs, 1.6473 secs
  resp read:    0.0003 secs, 0.0000 secs, 0.0620 secs

Status code distribution:
  [200] 512 responses
```
