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

#### Without queue proxy and activator on the request path
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

#### CC=0 without activator on the request path
| QPS/Replicas | mean | p50 | p95 | p99 |
| --- | --- | --- | --- | --- |
| 1/s minReplicas=1 | 487.044ms | 488.311ms | 491.165ms | 492.091ms |
| 5/s minReplicas=1 | 1.748s | 515.565ms | 3.191s | 9.881s |

- Create InferenceService with Container Concurrency set to 1
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

#### CC=1 with activator on the request path
| QPS/Replicas | mean | p50 | p95 | p99 |
| --- | --- | --- | --- | --- |
| 1/s minReplicas=1 | 479.772ms | 481.576ms | 485.584ms | 488.489ms |
| 5/s minReplicas=1 | 851.55ms | 431.925ms | 1.528s | 5.042s |

So here you can see that with CC=1, when you send one request at a time the latency does not make much different with CC=0 or CC=1.
However when you send many concurrent requests you will notice pronounced result when CC=1 because each request takes ~500ms to process. 