# Benchmark

This benchmark focus on testing KFServing performance with and without Knative queue proxy/activator on the request path.

* Knative queue proxy does the following for the KFServing main container.
  - Enforces concurrency level for the pod
  - Emit metrics for autoscaling(KPA)
  - Timeout enforcement
  - Readiness probe
  - Queue limiting
  - Distributed tracing
  - Graceful shutdown handling

* Knative activator buffers the requests while pods are scaled down to zero and report metrics to autoscaler. The activator
also effectively acts as a load balancer which distributes the load across all the pods as they become available in a way that
does not overload them with regards to their concurrency settings. So it protects the app from burst so you do not see messages
queuing in the user pods.

## Environment Setup
- K8S: v1.14.10-gke.36(8 nodes n1-standard)
- Istio: 1.1.6
- Knative: 0.11.2
- KFServing: master(with fix for https://github.com/kubeflow/kfserving/issues/844)

Note that `v1.14.10-gke.36` suffers the [CFS throttling bug](https://github.com/kubernetes/kubernetes/issues/67577), 
and `1.15.11-gke.15` includes the CFS throttling fix.

## Benchmarking

### Results on KServe SKLearn Iris Example
- Create `InferenceService`
```bash
kubectl apply -f ./sklearn.yaml
```
- Create the input vegeta configmap
```bash
kubectl apply -f ./sklearn_vegeta_cfg.yaml
```
- Create the benchmark job using [vegeta](https://github.com/tsenart/vegeta)
Note that you can configure pod anti-affinity to run vegeta on a different node on which the inference pod is running.
```bash
kubectl create -f ./sk_benchmark.yaml
```

#### CC=8 With queue proxy and activator on the request path
Create an `InferenceService` with `ContainerCurrency`(cc) set to 8 which is equal to the number of cores on the node.
```yaml
apiVersion: "serving.kserve.io/v1beta1"
kind: "InferenceService"
metadata:
  name: "sklearn-iris"
spec:
  predictor:
    containerConcurrency: 8 # CC=8
    sklearn:
      storageUri: "gs://kfserving-examples/models/sklearn/1.0/model"
```

| QPS/Replicas | mean | p50 | p95 | p99 | Success Rate |
| --- | --- | --- | --- | --- | --- |
| 5/s minReplicas=1 | 6.213ms | 5.915ms | 6.992ms | 7.615ms | 100% |
| 50/s minReplicas=1 | 5.738ms | 5.608ms | 6.483ms | 6.801ms | 100% |
| 500/s minReplicas=1 | 4.083ms | 3.743ms | 4.929ms | 5.642ms | 100% |
| 1000/s minReplicas=1 | 398.562ms | 5.95ms | 2.945s | 3.691s | 100% |

#### Raw Kubernetes Service(Without queue proxy and activator on the request path)
- Update the SKLearn Iris `InferenceService` with following yaml to use HPA
```yaml
apiVersion: "serving.kserve.io/v1beta1"
kind: "InferenceService"
metadata:
  name: "sklearn-iris"
  annotations:
    serving.kserve.io/deploymentMode: RawDeployment
    serving.kserve.io/autoscalerClass: hpa
    serving.kserve.io/metric: cpu
    serving.kserve.io/targetUtilizationPercentage: "80"
spec:
  predictor:
    sklearn:
      storageUri: "gs://kfserving-examples/models/sklearn/1.0/model"
```
```bash
kubectl apply -f ./sklearn_hpa.yaml
```
| QPS/Replicas | mean | p50 | p95 | p99 | Success Rate |
| --- | --- | --- | --- | --- | --- |
| 5/s Replicas=1 | 2.673ms | 2.381ms | 4.352ms | 5.966ms | 100% | 
| 50/s Replicas=1 | 2.188ms | 2.117ms | 2.684ms | 3.02ms | 100% |
| 500/s Replicas=1 | 1.376ms | 1.283ms | 1.713ms | 2.205ms | 100% |
| 1000/s Replicas=1 | 7.969s | 8.658s | 16.669s | 20.307s | 93.72% |

So you can see that queue-proxy and activator adds 2-3 millisecond overhead, but you get the advantage of KPA and
smart load balancing. For this example we do not see much benefits because the request takes only 1-2 ms to process,
however you can see the obvious advantage when request volume goes to 1000/s and KPA reacts faster and performs better 
than HPA.

### Results on KFServing with TFServing Flower Example
- Create `InferenceService`
```bash
kubectl apply -f ../docs/samples/tensorflow/tensorflow.yaml
```
- Create the input vegeta configmap
```bash
kubectl apply -f ./tf_vegeta_cfg.yaml
```
- Create the benchmark job using [vegeta](https://github.com/tsenart/vegeta)
Note that you can configure pod anti-affinity to run vegeta on a different node on which the inference pod is running.
```bash
kubectl create -f ./tf_benchmark.yaml
```

#### CC=0
- Create `InferenceService` with default `ContainerConcurrency` set to 0 which is unlimited concurrency, activator in this case just pass
through and you would still expect requests queued on user container in case of request overload.
```yaml
apiVersion: "serving.kserve.io/v1beta1"
kind: "InferenceService"
metadata:
  name: "flowers-sample"
spec:
  predictor:
    tensorflow:
      storageUri: "gs://kfserving-samples/models/tensorflow/flowers
      resources:
        requests:
          cpu: "4"
          memory: 2Gi
        limits:
          cpu: "4"
          memory: 2Gi
```

```bash
kubectl apply -f ./tf_flowers.yaml
```

| QPS/Replicas | mean | p50 | p95 | p99 | Success Rate |
| --- | --- | --- | --- | --- | --- |
| 1/s minReplicas=1 | 110.54ms | 110.343ms | 116.116ms | 117.298ms | 100% |
| 5/s minReplicas=1 | 133.272ms | 131.242ms | 148.195ms | 153.291ms | 100% |
| 10/s minReplicas=1 | 946.376ms | 127.961ms | 4.635s | 6.934s | 100% |

#### CC=1
- Create `InferenceService` with `ContainerConcurrency` set to 1, activator respects container queue limit 1 so that requests do
not get queued on user pods and activator chooses to route the requests to the pods which have capacity.

```yaml
apiVersion: "serving.kserve.io/v1beta1"
kind: "InferenceService"
metadata:
  name: "flowers-sample"
spec:
  predictor:
    containerConcurrency: 1 #CC=1
    tensorflow:
      storageUri: "gs://kfserving-samples/models/tensorflow/flowers
      resources:
        requests:
          cpu: "4"
          memory: 2Gi
        limits:
          cpu: "4"
          memory: 2Gi
```

| QPS/Replicas | mean | p50 | p95 | p99 | Success Rate |
| --- | --- | --- | --- | --- | --- |
| 1/s minReplicas=1 | 103.766ms | 102.869ms | 111.559ms | 116.577ms | 100% |
| 5/s minReplicas=1 | 117.456ms | 117.117ms | 122.346ms | 126.139ms | 100% |
| 10/s minReplicas=1 | 702.249ms | 111.289ms | 3.469s | 3.831s | 100% |


So here you can see that with CC=1, when you send one request at a time the latency does not make much different with CC=0 or CC=1.
However when you send more concurrent requests you start to notice pronounced result when CC=1 because activator starts to take effect and you 
will observe better tail latency at p95 and p99 thanks to Knative activator [smarter load balancing](https://github.com/knative/serving/issues/5692) than random load balancing.

#### Raw Kubernetes Service(Without queue proxy and activator)
```yaml
apiVersion: "serving.kserve.io/v1beta1"
kind: "InferenceService"
metadata:
  name: "flowers-sample-hpa"
  annotations:
    serving.kserve.io/deploymentMode: RawDeployment
    serving.kserve.io/autoscalerClass: hpa
    serving.kserve.io/metric: cpu
    serving.kserve.io/targetUtilizationPercentage: "60"
spec:
  predictor:
    tensorflow:
      storageUri: "gs://kfserving-samples/models/tensorflow/flowers
      resources:
        requests:
          cpu: "4"
          memory: 2Gi
        limits:
          cpu: "4"
          memory: 2Gi
```
Setup virtual service to bypass the knative proxy and update vegeta config target URL to 
`http://flowers-sample-raw.default.svc.cluster.local/v1/models/flowers-sample-hpa:predict`


| QPS/Replicas | mean | p50 | p95 | p99 | Success Rate |
| --- | --- | --- | --- | --- | --- |
| 1/s Replicas=1 | 129.143ms | 112.853ms | 118.143ms | 128.557ms | 100% |
| 5/s Replicas=1 | 127.947ms | 127.549ms | 132.171ms | 135.801ms | 100% |
| 10/s Replicas=1 | 5.461s | 5.087s | 12.992s | 14.587s | 100% |

This experiment runs the `InferenceService` using HPA with average target utilization 80% of CPU and calls directly to Kubernetes Service bypassing
the Knative queue proxy and activator. You can see that KPA reacts faster with the load and performs better than HPA for both low latency and high latency 
requests.
