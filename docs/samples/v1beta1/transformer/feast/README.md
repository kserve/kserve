# Predict on an InferenceService with transformer using Feast online feature store 
Transformer is an `InferenceService` component which does pre/post processing alongside with model inference. In this example, instead of typical input transformation of raw data to tensors, we demonstrate a use case of online feature augmentation as part of preprocessing. We use a Feast `Transformer` to gather online features, run inference with a `SKLearn` predictor, and leave post processing as pass-through.

## Setup

1. Your ~/.kube/config should point to a cluster with [KFServing installed](https://github.com/kubeflow/kfserving/#install-kfserving).
2. Your cluster's Istio Ingress gateway must be [network accessible](https://istio.io/latest/docs/tasks/traffic-management/ingress/ingress-control/).
3. Your Feast online store is populated with driver data and network accessible.

## Build Transformer image
`KFServing.KFModel` base class mainly defines three handlers `preprocess`, `predict` and `postprocess`, these handlers are executed
in sequence, the output of the `preprocess` is passed to `predict` as the input, when `predictor_host` is passed the `predict` handler by default makes a HTTP call to the predictor url 
and gets back a response which then passes to `postproces` handler. KFServing automatically fills in the `predictor_host` for `Transformer` and handle the call to the `Predictor`, for gRPC
predictor currently you would need to overwrite the `predict` handler to make the gRPC call.

To implement a `Transformer` you can derive from the base `KFModel` class and then overwrite the `preprocess` and `postprocess` handler to have your own
customized transformation logic.

### Extend KFModel and implement pre/post processing functions
We created a class, DriverTransformer, which extends KFModel for this driver ranking example. It takes additional arguments for the transformer to interact with Feast:
* feast_serving_url: The Feast serving URL, in the form of `<host_name_or_ip:port>`
* entity_ids: The entity IDs for which to retrieve features from the Feast feature store
* feature_refs: The feature references for the features to be retrieved

Please see the code example [here](./driver_transformer)

## Build Transformer docker image

```bash
docker build -t {username}/driver-transformer:latest -f driver_transformer.Dockerfile .

docker push {username}/driver-transformer:latest
```

## Create the InferenceService
Please use the [YAML file](./driver_transformer.yaml) and update the `feast_serving_url` argument to create the `InferenceService`, which includes a Feast Transformer and a SKLearn Predictor.

In the Feast Transformer image we packaged the driver tranformer class so KFServing knows to use the preprocess implementation to augment inputs with online features before making model inference requests. Then the `InferenceService` uses `SKLearn` to serve the driver ranking model, which is trained with Feast offline features, available in a gcs bucket specified under `storageUri`.

Apply the CRD
```
kubectl apply -f driver_transformer.yaml
```

Expected Output
```
$ inferenceservice.serving.kubeflow.org/driver-transformer created
```

## Run a prediction
The first step is to [determine the ingress IP and ports](../../../../../README.md#determine-the-ingress-ip-and-ports) and set `INGRESS_HOST` and `INGRESS_PORT`

```
SERVICE_NAME=sklearn-driver-transformer
MODEL_NAME=sklearn-driver-transformer
INPUT_PATH=@./driver-input.json
SERVICE_HOSTNAME=$(kubectl get inferenceservice $SERVICE_NAME -o jsonpath='{.status.url}' | cut -d "/" -f 3)

curl -v -H "Host: ${SERVICE_HOSTNAME}" -d $INPUT_PATH http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/$MODEL_NAME:predict
```

Expected Output
```
> POST /v1/models/sklearn-driver-transformer:predict HTTP/1.1
> Host: sklearn-driver-transformer.default.example.com
> User-Agent: curl/7.58.0
> Accept: */*
> Content-Length: 57
> Content-Type: application/x-www-form-urlencoded
>
* upload completely sent off: 57 out of 57 bytes
< HTTP/1.1 200 OK
< content-length: 117
< content-type: application/json; charset=UTF-8
< date: Thu, 13 May 2021 00:44:01 GMT
< server: istio-envoy
< x-envoy-upstream-service-time: 47
<
* Connection #0 to host 169.62.78.106 left intact
{"predictions": [1.8440737040128852, 1.7381656744054226, 3.6771303027855993, 2.241143189554492, 0.06753551272342406]}
```

