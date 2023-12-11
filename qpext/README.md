# Queue Proxy Extension

## What qpext does
This directory handles extending the Knative queue-proxy sidecar container.

The qpext creates a new port in the queue-proxy container that scrapes Prometheus metrics from both `queue-proxy` and `kserve-container`. 
When the new aggregate metrics endpoint is hit, the response will contain metrics from both `queue-proxy` and `kserve-container`. 
The qpext adds the functionality to emit metrics from both containers on a single endpoint. 

## Why qpext is needed
If an InferenceService uses Knative, then it has at least two containers in one pod, `queue-proxy` and `kserve-container`. A limitation of using Prometheus is that it supports scraping only one endpoint in the pod. 
When there are multiple containers in a pod that emit Prometheus metrics, this becomes an issue (see [Prometheus for multiple port annotations issue #3756](https://github.com/prometheus/prometheus/issues/3756) for the 
full discussion on this topic). In an attempt to make an easy-to-use solution, the queue-proxy is extended to handle this use case. 


see also: [KServe Issue #2645](https://github.com/kserve/kserve/issues/2465), 

## How to use 

Save this file as qpext_image_patch.yaml, update the tag if needed.
```yaml
data:
  queue-sidecar-image: kserve/qpext:latest
```

Run the following command to patch the deployment config in the appropriate knative namespace.
```shell
kubectl patch configmaps -n knative-serving config-deployment --patch-file qpext_image_patch.yaml
```

## Configs

The qpext relies on pod annotations to be set in the InferenceService YAML. If these annotations are set to true, then environment variables will be added to the queue-proxy container. 
The qpext uses the environment variables to configure which port/path to expose metrics on and which port/path to scrape metrics from in `queue-proxy` and `kserve-container`.

| Annotation                                           | Default | Description |
|------------------------------------------------------|---------|-------------|
| serving.kserve.io/enable-metric-aggregation          | false | If true, enables metric aggregation in queue-proxy by setting env vars in the queue proxy container to configure scraping ports. |
| serving.kserve.io/enable-prometheus-scraping | false | If true, sets the prometheus annotations in the pod. If true and "serving.kserve.io/enable-metric-aggregation" is false, the prometheus port will be set as the default queue-proxy port. If both are true, the prometheus port annotation will be set as the aggregate metric port.  | 


| Queue Proxy Env Vars                     | Default  | Description                                                                                                                                                                     |
|------------------------------------------|----------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| AGGREGATE_PROMETHEUS_METRICS_PORT        | 9088     | The metrics aggregation port in queue-proxy that is added in the qpext.                                                                                                         | 
| KSERVE_CONTAINER_PROMETHEUS_METRICS_PORT | 8080     | The default metrics port for the `kserve-container`. If present, the default ClusterServingRuntime overrides this value with each runtime's default prometheus port.            |
| KSERVE_CONTAINER_PROMETHEUS_METRICS_PATH | /metrics | The default metrics path for the `kserve-container`. If present, the default ClusterServingRuntime annotation overrides this value with each runtime's default prometheus path. |   

To implement this feature, configure the InferenceService YAML annotations. 

```yaml
apiVersion: "serving.kserve.io/v1beta1"
kind: "InferenceService"
metadata:
  name: "sklearn-irisv2"
  annotations:
    serving.kserve.io/enable-metric-aggregation: "true"
    serving.kserve.io/enable-prometheus-scraping: "true"
spec:
  predictor:
    sklearn:
      protocolVersion: v2
      storageUri: "gs://seldon-models/sklearn/iris"
```

To view the runtime specific defaults for the `kserve-container` prometheus port and path, view the spec annotations in `kserve/config/runtimes`.
These values can be overriden in the InferenceService YAML annotations.

```yaml
apiVersion: "serving.kserve.io/v1beta1"
kind: "InferenceService"
metadata:
  name: "sklearn-irisv2"
  annotations:
    serving.kserve.io/enable-metric-aggregation: "true"
    serving.kserve.io/enable-prometheus-scraping: "true"
    prometheus.kserve.io/port: '8081'
    prometheus.kserve.io/path: "/other/metrics"
spec:
  predictor:
    sklearn:
      protocolVersion: v2
      storageUri: "gs://seldon-models/sklearn/iris"
```
The default port for sklearn runtime is `8080`, and the default path is `/metrics`. 
By setting the annotations in the InferenceService YAML, the default runtime configurations are overridden.

**KServe Developer's Note:** If the qpext is implemented in the cluster and you wish to set the default annotation values to `true`, 
the defaults in the configMap can be overridden via patching the configMap or setting up a webhook to override the values.
To check the default values in your cluster, run 

```shell
kubectl get configmaps inferenceservice-config -n kserve -oyaml
```

the values are in the output of the YAML like

```yaml
    metricsAggregator: |-
      {
        "enableMetricAggregation": "false",
        "enablePrometheusScraping" : "false"
      }
```

If these values are overridden to default to `true` 

```yaml
    metricsAggregator: |-
      {
        "enableMetricAggregation": "true",
        "enablePrometheusScraping" : "true"
      }
```
then the annotations should be inserted into the YAML with `false` values when
an InferenceService does not want to aggregate metrics and/or set the prometheus 
scraping port annotation. 

## Developer's guide

Changes can be made in the qpext and tested via unit tests, e2e tests, and interactively in a cluster. 

### Note on dependencies

The controller reads the `serving.kserve.io/enable-metric-aggregation` and `serving.kserve.io/enable-prometheus-scraping`
annotations and then adds prometheus annotations to the pod and/or environment variables to the queue-proxy container if specified. 
This code is found in `kserve/pkg/webhook/admission/pod/metrics_aggregate_injector.go`. 

The specific runtime default configurations are annotations in the YAML files in `kserve/config/runtimes`. 

### Test

In kserve/qpext, run `go test -v ./... -cover` to run the unit tests and get the total coverage.
The e2e tests are defined in `kserve/test/qpext`. To add an e2e test, create a python test in this directory.

### Build
The qpext code can be interactively tested by building the image with any changes, 
pushing the image to dockerhub/container registry, and patching the knative deploy config to use
the test image. The pods will then pick up the new configuration.


(1) To build the qpext image in the kserve/qpext directory (as an example, `some_docker_repo` in dockerhub), run 
```shell
make docker-build-push-qpext
```

Alternatively, build and push the image step by step yourself.
```shell
cd kserve/qpext 
export QPEXT_IMG={some_docker_repo}/qpext
docker build -t ${QPEXT_IMG} -f qpext.Dockerfile .
```

Next push the image to a container registry, 
```shell
docker push {some_docker_repo}/qpext:latest
```

(2) Save this file as qpext_image_patch.yaml, update the tag if needed.
```yaml
data:
  queue-sidecar-image: kserve/qpext:latest
```

(3) Run the following command to patch the deployment config in the appropriate knative namespace.
```shell
kubectl patch configmaps -n knative-serving config-deployment --patch-file qpext_image_patch.yaml
```

(4) Confirm the config-deployment updated
```shell
kubectl get configmaps -n knative-serving config-deployment -oyaml
```

(5) Deploy an InferenceService and check that the change works. 

For example, using the sklearn example above saved as `sklearn.yaml`
```shell
kubectl apply -f sklearn.yaml 
```

To check that the configs were applied as env vars in the queue-proxy container 
and annotations on the pod, check the Pod output.
```shell
kubectl get pod {name_of_pod} -oyaml
```

To check that the metrics are aggregated, use the KServe [Getting Started](https://kserve.github.io/website/latest/get_started/first_isvc/#4-determine-the-ingress-ip-and-ports) 
documentation as a guide to send a request to the pod. Next, send a request to the metrics endpoint. 

For example, port-forward the pod prometheus aggregate metrics port to localhost. 
```shell
kubectl port-forward pods/{pod_name} 9088:9088
```
Next, cURL the port to see the metrics output.
```shell
curl localhost:9088/metrics
```
