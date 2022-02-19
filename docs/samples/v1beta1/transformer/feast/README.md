# Predict on an InferenceService with transformer using Feast online feature store 
Transformer is an `InferenceService` component which does pre/post processing alongside with model inference. In this example, instead of typical input transformation of raw data to tensors, we demonstrate a use case of online feature augmentation as part of preprocessing. We use a [Feast](https://github.com/feast-dev/feast) `Transformer` to gather online features, run inference with a `SKLearn` predictor, and leave post processing as pass-through.

## Setup

1. Your ~/.kube/config should point to a cluster with [KServe installed](https://github.com/kserve/kserve/#installation).
2. Your cluster's Istio Ingress gateway must be [network accessible](https://istio.io/latest/docs/tasks/traffic-management/ingress/ingress-control/).
3. Your Feast online store is populated with driver [data](https://github.com/tedhtchang/populate_feast_online_store/blob/main/driver_stats.parquet), instructions available [here](https://github.com/tedhtchang/populate_feast_online_store), and network accessible.

## Build Transformer image
`KServe.Model` base class mainly defines three handlers `preprocess`, `predict` and `postprocess`, these handlers are executed
in sequence, the output of the `preprocess` is passed to `predict` as the input, when `predictor_host` is passed the `predict` handler by default makes a HTTP call to the predictor url 
and gets back a response which then passes to `postproces` handler. KServe automatically fills in the `predictor_host` for `Transformer` and handle the call to the `Predictor`, for gRPC
predictor currently you would need to overwrite the `predict` handler to make the gRPC call.

To implement a `Transformer` you can derive from the base `Model` class and then overwrite the `preprocess` and `postprocess` handler to have your own
customized transformation logic.

### Extend Model and implement pre/post processing functions
We created a class, DriverTransformer, which extends Model for this driver ranking example. It takes additional arguments for the transformer to interact with Feast:
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

In the Feast Transformer image we packaged the driver transformer class so KServe knows to use the preprocess implementation to augment inputs with online features before making model inference requests. Then the `InferenceService` uses `SKLearn` to serve the [driver ranking model](https://github.com/feast-dev/feast-driver-ranking-tutorial), which is trained with Feast offline features, available in a gcs bucket specified under `storageUri`.

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
< content-length: 119
< content-type: application/json; charset=UTF-8
< date: Thu, 02 Sep 2021 19:27:55 GMT
< server: istio-envoy
< x-envoy-upstream-service-time: 30
<
* Connection #0 to host 1.2.3.4 left intact
{"predictions": [1.3320522732903406, -0.49981088917615324, -0.17008354122857838, 0.8017473264530217, 1.2042992134934583]}
```

