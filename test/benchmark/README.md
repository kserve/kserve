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

* Knative activator buffers the requests while pods are scaled down to zero and also does smart load balancing on the request
path when container concurrency is enforced.

## Environment Setup
- K8S: v1.14.10-gke.36
- Istio: 1.1.6
- Knative: 0.11.2
- KFServing: 0.3.0

## Benchmarking

### Results on KFServing SKLearn Server
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

This main container consistently takes 1ms
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

