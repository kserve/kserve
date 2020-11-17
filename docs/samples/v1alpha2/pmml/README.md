## Creating your own model and testing the PMML Server locally.

To test the [PMMLServer](http://dmg.org/pmml/pmml_examples/#Iris) server, first we need to download model from [single_iris_dectree](http://dmg.org/pmml/pmml_examples/KNIME_PMML_4.1_Examples/single_iris_dectree.xml)

# Predict on a InferenceService using PMMLServer

## Setup
1. Your ~/.kube/config should point to a cluster with [KFServing installed](https://github.com/kubeflow/kfserving/#install-kfserving).
2. Your cluster's Istio Ingress gateway must be [network accessible](https://istio.io/latest/docs/tasks/traffic-management/ingress/ingress-control/).

## Create the InferenceService

Apply the CRD
```
kubectl apply -f pmml.yaml
```

Expected Output
```
$ inferenceservice.serving.kubeflow.org/pmml-demo created
```
## Run a prediction
The first step is to [determine the ingress IP and ports](../../../README.md#determine-the-ingress-ip-and-ports) and set `INGRESS_HOST` and `INGRESS_PORT`

```
MODEL_NAME=pmml-demo
INPUT_PATH=@./pmml-input.json
SERVICE_HOSTNAME=$(kubectl get inferenceservice pmml-demo -o jsonpath='{.status.url}' | cut -d "/" -f 3)
curl -v -H "Host: ${SERVICE_HOSTNAME}" http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/$MODEL_NAME:predict -d $INPUT_PATH
```

Expected Output

```
* TCP_NODELAY set
* Connected to localhost (::1) port 8081 (#0)
> POST /v1/models/pmml-demo:predict HTTP/1.1
> Host: pmml-demo.default.example.com
> User-Agent: curl/7.64.1
> Accept: */*
> Content-Length: 45
> Content-Type: application/x-www-form-urlencoded
>
* upload completely sent off: 45 out of 45 bytes
< HTTP/1.1 200 OK
< content-length: 39
< content-type: application/json; charset=UTF-8
< date: Sun, 18 Oct 2020 15:50:02 GMT
< server: istio-envoy
< x-envoy-upstream-service-time: 12
<
* Connection #0 to host localhost left intact
{"predictions": [[1.0, 0.0, 0.0, "2"]]}* Closing connection 0
```

