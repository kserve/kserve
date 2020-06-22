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
- Create `InferenceService`
```bash
kubectl apply -f ./sklearn.yaml
```
- Create the input vegeta configmap
```bash
kubectl apply -f ./sklearn_vegeta_cfg.yaml
```
- Create the benchmark job
```bash
kubectl create -f ./sk_benchmark.yaml
```

#### CC=8 With queue proxy and activator on the request path
Create an `InferenceService` with `ContainerCurrency`(cc) set to 8 which equals to the number of codes on the node.
```yaml
apiVersion: "serving.kubeflow.org/v1alpha2"
kind: "InferenceService"
metadata:
  name: "sklearn-iris"
spec:
  default:
    parallelism: 8 # CC=8
    predictor:
      sklearn:
        storageUri: "gs://kfserving-samples/models/sklearn/iris"
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
apiVersion: "serving.kubeflow.org/v1alpha2"
kind: "InferenceService"
metadata:
  name: "sklearn-iris"
  annotations:
    autoscaling.knative.dev/class: hpa.autoscaling.knative.dev
    autoscaling.knative.dev/metric: cpu
    autoscaling.knative.dev/target: "80"
spec:
  default:
    predictor:
      sklearn:
        storageUri: "gs://kfserving-samples/models/sklearn/iris"
```
```bash
kubectl apply -f ./sklearn_hpa.yaml
```
- Setup virtual service to go directly to the private service to bypass the Knative Activator and queue-proxy, change the benchmark
test target url host to `sklearn-iris-raw.default.svc.cluster.local`.
```yaml
apiVersion: v1
kind: Service
metadata:
  name: sklearn-iris-raw
spec:
  externalName: cluster-local-gateway.istio-system.svc.cluster.local
  sessionAffinity: None
  type: ExternalName
---
apiVersion: networking.istio.io/v1alpha3
kind: VirtualService
metadata:
  name: sklearn-iris-raw
spec:
  gateways:
  - knative-serving/cluster-local-gateway
  hosts:
  - sklearn-iris-raw.default.svc.cluster.local
  http:
  - match:
    - authority:
        regex: ^sklearn-iris-raw\.default(\.svc(\.cluster\.local)?)?(?::\d{1,5})?$
      gateways:
      - knative-serving/cluster-local-gateway
      uri:
        regex: ^/v1/models/[\w-]+(:predict)?
    route:
    - destination:
        host: sklearn-iris-predictor-default-xt264-private.default.svc.cluster.local #this is the private service to user container
        port:
          number: 80
      weight: 100
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
- Create the benchmark job
```bash
kubectl create -f ./tf_benchmark.yaml
```

#### CC=0
- Create `InferenceService` with default `ContainerConcurrency` set to 0 which is unlimited concurrency, activator in this case does not do smart load balancing based on
queue limit.
```yaml
apiVersion: "serving.kubeflow.org/v1alpha2"
kind: "InferenceService"
metadata:
  name: "flowers-sample"
spec:
  default:
    predictor:
      tensorflow:
        storageUri: "gs://kfserving-samples/models/tensorflow/flowers
```

```bash
kubectl apply -f ./tf_flowers.yaml
```

| QPS/Replicas | mean | p50 | p95 | p99 | Success Rate |
| --- | --- | --- | --- | --- | --- |
| 1/s minReplicas=1 | 487.044ms | 488.311ms | 491.165ms | 492.091ms | 100% |
| 5/s minReplicas=1 | 1.043s | 515.479ms | 1.823s | 4.539s | 100% |
| 5/s minReplicas=1 | 1.748s | 515.565ms | 3.191s | 11.883s | 99.78% |

#### CC=1
- Create `InferenceService` with `ContainerConcurrency` set to 1, so activator respect container queue limit 1 and requests do
not get queued on user pods and requests can go to the pods which have capacity.

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
However when you send many concurrent requests you will notice pronounced result when CC=1 because each request takes ~500ms to process and you 
will observe better tail latency at p95 and p99 thanks to Knative activator smarter load balancing than random. 

#### Raw Kubernetes Service(Without queue proxy and activator)
```yaml
apiVersion: "serving.kubeflow.org/v1alpha2"
kind: "InferenceService"
metadata:
  name: "flowers-sample"
  annotations:
    autoscaling.knative.dev/class: hpa.autoscaling.knative.dev
    autoscaling.knative.dev/metric: cpu
    autoscaling.knative.dev/target: "80"
spec:
  default:
    predictor:
      tensorflow:
        storageUri: "gs://kfserving-samples/models/tensorflow/flowers
```

| QPS/Replicas | mean | p50 | p95 | p99 | Success Rate |
| --- | --- | --- | --- | --- | --- |
| 1/s Replicas=1 | 458.5ms | 429.096ms | 516.948ms | 522.311ms | 100% |
| 3/s Replicas=1 | 9.867s | 8.35s | 22.906s | 28.907s | 95.74% |
| 5/s Replicas=1 | 28.394s | 30s | 30s | 30s | 30.44% |

This experiment runs the `InferenceService` using HPA with average target utilization 80% of CPU and calls directly to Kubernetes Service bypassing
the Knative queue proxy and activator. You can see that KPA reacts faster with the load and performs better than HPA for both low latency and high latency 
requests.
