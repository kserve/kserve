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
- K8S: v1.14.10-gke.36
- Istio: 1.1.6
- Knative: 0.11.2
- KFServing: master

## Benchmarking

### Results on KFServing SKLearn Iris Example
- Create InferenceService
```bash
kubectl apply -f ../docs/samples/sklearn/sklearn.yaml
```

- Create the input vegeta configmap
```bash
kubectl apply -f ./sklearn_vegeta_cfg.yaml
```
- Create the benchmark job
```bash
kubectl create -f ./benchmark.yaml
```

#### With queue proxy and activator on the request path
| QPS/Replicas | mean | p50 | p95 | p99 |
| --- | --- | --- | --- | --- |
| 5/s minReplicas=1 | 6.213ms | 5.915ms | 6.992ms | 7.615ms |
| 50/s minReplicas=1 | 5.738ms | 5.608ms | 6.483ms | 6.801ms |
| 500/s minReplicas=1 | 4.083ms | 3.743ms | 4.929ms | 5.642ms |
| 1000/s minReplicas=5 | 4.326ms | 3.674ms | 5.303ms | 6.651ms |

#### Raw Kubernetes Service(Without queue proxy and activator on the request path)
| QPS/Replicas | mean | p50 | p95 | p99 |
| --- | --- | --- | --- | --- |
| 5/s replicas=1 | 1.694ms | 1.614ms | 1.836ms | 2.004ms |
| 50/s replicas=1 | 1.499ms | 1.476ms | 1.631ms | 1.777ms |
| 500/s replicas=1 | 942.766µs | 894.353µs | 1.072ms | 1.19ms |
| 1000/s replicas=5 | 895.969µs | 821.75µs | 1.033ms | 1.214ms |

So you can see that queue-proxy and activator adds 2-3 millisecond overhead, but you get the advantage of autoscaling and
smart load balancing. For this example we do not see much effect or benefits because the request takes only 1-2 ms to process.

### Results on KFServing with TFServing Flower Example
- Create InferenceService
```bash
kubectl apply -f ../docs/samples/tensorflow/tensorflow.yaml
```

- Create the input vegeta configmap
```bash
kubectl apply -f ./tf_vegeta_cfg.yaml
```
- Create the benchmark job
```bash
kubectl create -f ./tf_benchmark.yaml
```

#### CC=0
- Create InferenceService with default Container Concurrency set to 0, so activator does not do smart load balancing based on
queue size.

| QPS/Replicas | mean | p50 | p95 | p99 | Success Rate |
| --- | --- | --- | --- | --- | --- |
| 1/s minReplicas=1 | 487.044ms | 488.311ms | 491.165ms | 492.091ms | 100% |
| 5/s minReplicas=1 | 1.043s | 515.479ms | 1.823s | 4.539s | 100% |
| 5/s minReplicas=1 | 1.748s | 515.565ms | 3.191s | 11.883s | 99.78% |

#### CC=1
- Create InferenceService with Container Concurrency set to 1, so activator respect container queue limit 1 and requests do
not get queued on user pods.

```yaml
apiVersion: "serving.kubeflow.org/v1alpha2"
kind: "InferenceService"
metadata:
  name: "flowers-sample"
spec:
  default:
    predictor:
      parallelism: 1 #CC=1
      tensorflow:
        storageUri: "gs://kfserving-samples/models/tensorflow/flowers
```

| QPS/Replicas | mean | p50 | p95 | p99 | Success Rate |
| --- | --- | --- | --- | --- | --- |
| 1/s minReplicas=1 | 479.772ms | 481.576ms | 485.584ms | 488.489ms | 100% |
| 3/s minReplicas=1 | 775.227ms | 460.035ms | 936.037ms | 3.748s | 100% |
| 5/s minReplicas=1 | 1.347s | 478.923ms | 3.357s | 6.48s | 100% |

So here you can see that with CC=1, when you send one request at a time the latency does not make much different with CC=0 or CC=1.
However when you send many concurrent requests you will notice pronounced result when CC=1 because each request takes ~500ms to process.
You will see better tail latency at p95 and p99. 

#### Raw Kubernetes Service(Without queue proxy and activator)

| QPS/Replicas | mean | p50 | p95 | p99 | Success Rate |
| --- | --- | --- | --- | --- | --- |
| 1/s Replicas=1 | 458.5ms | 429.096ms | 516.948ms | 522.311ms | 100% |
| 3/s Replicas=1 | 15.353s | 14.889s | 30s | 30s | 93.89% |
| 5/s Replicas=1 | 28.394s | 30s | 30s | 30s | 11.22% |

With raw kubernetes service you can see that without Autoscaling, raw kubernetes deployment can not keep up the load and results in
a lot of timeouts.