
# Autoscale KFService with your inference workload
## Setup
1. Your ~/.kube/config should point to a cluster with [KFServing installed](https://github.com/kubeflow/kfserving/blob/master/docs/DEVELOPER_GUIDE.md#deploy-kfserving).
2. Your cluster's Istio Ingress gateway must be network accessible.
3. Your cluster's Istio Egresss gateway must [allow Google Cloud Storage](https://knative.dev/docs/serving/outbound-network-access/)
4. [Metrics installation](https://knative.dev/docs/serving/installing-logging-metrics-traces) for viewing scaling graphs (optional).
5. The hey load generator installed (go get -u github.com/rakyll/hey).

## Create the KFService
Apply the CRD
```
kubectl apply -f ../tensorflow/tensorflow.yaml 
```

Expected Output
```
$ kfservice.serving.kubeflow.org/flowers-sample configured
```

## Load the KFService with concurrent requests
Send 30 seconds of traffic maintaining 5 in-flight requests
```
MODEL_NAME=flowers-sample
INPUT_PATH=../tensorflow/input.json
CLUSTER_IP=$(kubectl -n istio-system get service istio-ingressgateway -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
HOST=$(kubectl get kfservice $MODEL_NAME -o jsonpath='{.status.url}')
hey -z 30s -c 5 -host ${HOST} -D $INPUT_PATH http://$CLUSTER_IP/v1/models/$MODEL_NAME:predict
```
Expected Output
```shell
Summary:
  Total:	30.0193 secs
  Slowest:	10.1458 secs
  Fastest:	0.0127 secs
  Average:	0.0364 secs
  Requests/sec:	137.4449
  
  Total data:	1019122 bytes
  Size/request:	247 bytes

Response time histogram:
  0.013 [1]	|
  1.026 [4120]	|■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■
  2.039 [0]	|
  3.053 [0]	|
  4.066 [0]	|
  5.079 [0]	|
  6.093 [0]	|
  7.106 [0]	|
  8.119 [0]	|
  9.133 [0]	|
  10.146 [5]	|


Latency distribution:
  10% in 0.0178 secs
  25% in 0.0188 secs
  50% in 0.0199 secs
  75% in 0.0210 secs
  90% in 0.0231 secs
  95% in 0.0328 secs
  99% in 0.1501 secs

Details (average, fastest, slowest):
  DNS+dialup:	0.0002 secs, 0.0127 secs, 10.1458 secs
  DNS-lookup:	0.0002 secs, 0.0000 secs, 0.1502 secs
  req write:	0.0000 secs, 0.0000 secs, 0.0020 secs
  resp wait:	0.0360 secs, 0.0125 secs, 9.9791 secs
  resp read:	0.0001 secs, 0.0000 secs, 0.0021 secs

Status code distribution:
  [200]	4126 responses
```
Check the number of pods that are running now, `KFServing` uses `Knative Serving`'s autoscaling which is based on the 
average number of in-flight requests per pod(concurrency). `KFServing` by default sets target concurrency to be 1 and we
load the service with 5 concurrent requests so that autoscaler tries scaling up to 5 pods. Notice that out of all the requests there are 
5 requests on the histogram that take around 10s, that's the cold start time cost to initially spawn the pods, download model to be ready
to serve the workload. The cold start may take longer(to pull the serving image) if the image is not cached on the node that the pod is scheduled on.
```
$ kubectl get pods
NAME                                                       READY   STATUS            RESTARTS   AGE
flowers-sample-default-7kqt6-deployment-75d577dcdb-sr5wd         3/3     Running       0          42s
flowers-sample-default-7kqt6-deployment-75d577dcdb-swnk5         3/3     Running       0          62s
flowers-sample-default-7kqt6-deployment-75d577dcdb-t2njf         3/3     Running       0          62s
flowers-sample-default-7kqt6-deployment-75d577dcdb-vdlp9         3/3     Running       0          64s
flowers-sample-default-7kqt6-deployment-75d577dcdb-vm58d         3/3     Running       0          42s
```

## Check Dashboard
View the Knative Serving Scaling dashboards (if configured).

```bash
kubectl port-forward --namespace knative-monitoring $(kubectl get pods --namespace knative-monitoring --selector=app=grafana  --output=jsonpath="{.items..metadata.name}") 3000
```

![scaling dashboard](scaling_debug.png)


## Load your KFService with target QPS
Send 30 seconds of traffic maintaining 50 qps
```bash
MODEL_NAME=flowers-sample
INPUT_PATH=../tensorflow/input.json
CLUSTER_IP=$(kubectl -n istio-system get service istio-ingressgateway -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
HOST=$(kubectl get kfservice $MODEL_NAME -o jsonpath='{.status.url}')
hey -z 30s -q 50 -host ${HOST} -D $INPUT_PATH http://$CLUSTER_IP/v1/models/$MODEL_NAME:predict
```

```shell
Summary:
  Total:	30.0264 secs
  Slowest:	10.8113 secs
  Fastest:	0.0145 secs
  Average:	0.0731 secs
  Requests/sec:	683.5644
  
  Total data:	5069675 bytes
  Size/request:	247 bytes

Response time histogram:
  0.014 [1]	|
  1.094 [20474]	|■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■
  2.174 [0]	|
  3.254 [0]	|
  4.333 [0]	|
  5.413 [0]	|
  6.493 [0]	|
  7.572 [0]	|
  8.652 [0]	|
  9.732 [0]	|
  10.811 [50]	|


Latency distribution:
  10% in 0.0284 secs
  25% in 0.0334 secs
  50% in 0.0408 secs
  75% in 0.0527 secs
  90% in 0.0765 secs
  95% in 0.0949 secs
  99% in 0.1334 secs

Details (average, fastest, slowest):
  DNS+dialup:	0.0001 secs, 0.0145 secs, 10.8113 secs
  DNS-lookup:	0.0000 secs, 0.0000 secs, 0.0196 secs
  req write:	0.0000 secs, 0.0000 secs, 0.0031 secs
  resp wait:	0.0728 secs, 0.0144 secs, 10.7688 secs
  resp read:	0.0000 secs, 0.0000 secs, 0.0031 secs

Status code distribution:
  [200]	20525 responses
```

Check the number of pods now, we are loading the service with 50 requests per second, and from the dashboard you can see
that it hits the max concurrency 10 and autoscaler tries scaling up to 10 pods.

## Check Dashboard
View the Knative Serving Scaling dashboards (if configured).

```bash
kubectl port-forward --namespace knative-monitoring $(kubectl get pods --namespace knative-monitoring --selector=app=grafana  --output=jsonpath="{.items..metadata.name}") 3000
```

![scaling dashboard](scaling_debug_qps.png)

## Scaling customization
You can also customize the target concurrency by adding annotation `autoscaling.knative.dev/target: "10"` on `KFService` CRD.
```yaml
apiVersion: "serving.kubeflow.org/v1alpha1"
kind: "KFService"
metadata:
  name: "flowers-sample"
  annotations:
    autoscaling.knative.dev/target: "10"
spec:
  default:
    tensorflow:
      modelUri: "gs://kfserving-samples/models/tensorflow/flowers"
```

