# Deploy Custom Python Model with KFServer API
If out of the box model server does not fit your need, you can build your own model server using KFServer API and use the
following source to serving workflow to deploy your models to KFServing.

## Setup
1. Your ~/.kube/config should point to a cluster with [KFServing installed](https://github.com/kubeflow/kfserving/#install-kfserving).
2. Your cluster's Istio Ingress gateway must be [network accessible](https://istio.io/latest/docs/tasks/traffic-management/ingress/ingress-control/).
3. Install [pack CLI](https://buildpacks.io/docs/tools/pack/) to build your custom model server image.

## Create your custom Model Server by extending KFModel
`KFServing.KFModel` base class mainly defines three handlers `preprocess`, `predict` and `postprocess`, these handlers are executed
in sequence, the output of the `preprocess` is passed to `predict` as the input, the `predictor` handler should execute the
inference for your model, the `postprocess` handler then turns the raw prediction result into user-friendly inference response. There
is an additional `load` handler which is used for writing custom code to load your model into the memory from local file system or
remote model storage, a general good practice is to call the `load` handler in the model server class `__init__` function, so your model
is loaded on startup and ready to serve when user is making the prediction calls.

## Build the custom image with Buildpacks
[Buildpacks](https://buildpacks.io/) allows you to transform your inference code into images that can be deployed on KFServing without
needing to define the `Dockerfile`. Buildpacks automatically determines the python application and then the dependencies from the
`requirements.txt` file, it looks at the `Procfile` to determine how to start the model server. You can also choose to use [kpack](https://github.com/pivotal/kpack)
to allow you run the image build on the cloud and continuously build/deploy new versions from your source git repository.

### Use pack to build and push the custom model server image
```bash
pack build --builder=heroku/buildpacks:20 ${DOCKER_USER}/custom_model
docker push ${DOCKER_USER}/custom_model
```

## Parallel Inference

## Deploy and Invoke Inference
### Create the InferenceService

In the `custom.yaml` file edit the container image and replace {username} with your Docker Hub username.

Apply the CRD

```
kubectl apply -f custom.yaml
```

Expected Output

```
$ inferenceservice.serving.kubeflow.org/custom-model created
```

### Arguments and Environment Variables
You can supply additional command arguments on the container spec to configure the model server.
- `--workers`: fork the specified number of model server workers(multi-processing), the default value is 1. If you start the server after model is loaded
you need to make sure model object is fork friendly for multi-processing to work. Alternatively you can decorate your model server
class with replicas and in this case each model server is created as a python worker independent of the server.
- `--http_port`: the http port model server is listening on, the default port is 8080 
- `--max_buffer_size`: Max socker buffer size for tornado http client, the default limit is 10Mi.
- `--max_asyncio_workers`: Max number of workers to spawn for python async io loop, by default it is `min(32,cpu.limit + 4)`

### Run a prediction
The first step is to [determine the ingress IP and ports](../../../../README.md#determine-the-ingress-ip-and-ports) and set `INGRESS_HOST` and `INGRESS_PORT`

```
MODEL_NAME=custom-model
INPUT_PATH=@./input.json
SERVICE_HOSTNAME=$(kubectl get inferenceservice ${MODEL_NAME} -o jsonpath='{.status.url}' | cut -d "/" -f 3)

curl -v -H "Host: ${SERVICE_HOSTNAME}" http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/${MODEL_NAME}:predict -d $INPUT_PATH
```

Expected Output:

```
*   Trying 169.47.250.204...
* TCP_NODELAY set
* Connected to 169.47.250.204 (169.47.250.204) port 80 (#0)
> POST /v1/models/custom-model:predict HTTP/1.1
> Host: custom-model.default.example.com
> User-Agent: curl/7.64.1
> Accept: */*
> Content-Length: 105339
> Content-Type: application/x-www-form-urlencoded
> Expect: 100-continue
>
< HTTP/1.1 100 Continue
* We are completely uploaded and fine
< HTTP/1.1 200 OK
< content-length: 232
< content-type: text/html; charset=UTF-8
< date: Wed, 26 Feb 2020 15:19:15 GMT
< server: istio-envoy
< x-envoy-upstream-service-time: 213
<
* Connection #0 to host 169.47.250.204 left intact
{"predictions": {"Labrador retriever": 0.4158518612384796, "golden retriever": 0.1659165322780609, "Saluki, gazelle hound": 0.16286855936050415, "whippet": 0.028539149090647697, "Ibizan hound, Ibizan Podenco": 0.023924754932522774}}* Closing connection 0
```

### Delete the InferenceService

```
kubectl delete -f custom.yaml
```

