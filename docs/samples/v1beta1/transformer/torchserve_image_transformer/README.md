# Predict on a InferenceService with transformer using Torchserve

Transformer is an `InferenceService` component which does pre/post processing alongside with model inference. It usually takes raw input and transforms them to the
input tensors model server expects. In this example we demonstrate an example of running inference with `Transformer` and `TorchServe` predictor.

## Setup

1. Your ~/.kube/config should point to a cluster with [KServe installed](https://github.com/kserve/kserve#installation).
2. Your cluster's Istio Ingress gateway must be [network accessible](https://istio.io/latest/docs/tasks/traffic-management/ingress/ingress-control/).

## Build Transformer image

`KServe.Model` base class mainly defines three handlers `preprocess`, `predict` and `postprocess`, these handlers are executed in sequence, the output of the `preprocess` is passed to `predict` as the input, when `predictor_host` is passed the `predict` handler by default makes a HTTP call to the predictor url and gets back a response which then passes to `postproces` handler. KServe automatically fills in the `predictor_host` for `Transformer` and handle the call to the `Predictor`, for gRPC predictor currently you would need to overwrite the `predict` handler to make the gRPC call.

To implement a `Transformer` you can derive from the base `Model` class and then overwrite the `preprocess` and `postprocess` handler to have your own
customized transformation logic.

### Extend Model and implement pre/post processing functions

```python
import kserve
from typing import List, Dict
from PIL import Image
import torchvision.transforms as transforms
import logging
import io
import numpy as np
import base64

logging.basicConfig(level=kserve.constants.KSERVE_LOGLEVEL)

transform = transforms.Compose(
        [transforms.ToTensor(),
         transforms.Normalize((0.5, 0.5, 0.5), (0.5, 0.5, 0.5))])


def image_transform(instance):
    byte_array = base64.b64decode(instance['image_bytes']['b64'])
    image = Image.open(io.BytesIO(byte_array))
    a = np.asarray(image)
    im = Image.fromarray(a)
    res = transform(im)
    logging.info(res)
    return res.tolist()


class ImageTransformer(kserve.Model):
    def __init__(self, name: str, predictor_host: str):
        super().__init__(name)
        self.predictor_host = predictor_host

    def preprocess(self, inputs: Dict) -> Dict:
        return {'instances': [image_transform(instance) for instance in inputs['instances']]}

    def postprocess(self, inputs: Dict) -> Dict:
        return inputs
```

Please see the code example [here](./image_transformer)

## Build Transformer docker image

```bash
docker build -t {username}/image-transformer:latest -f transformer.Dockerfile .

docker push {username}/image-transformer:latest
```

## Create the InferenceService

Please use the [YAML file](./transformer.yaml) to create the `InferenceService`, which includes a Transformer and a PyTorch Predictor.

By default `InferenceService` uses `TorchServe` to serve the PyTorch models and the models are loaded from a model repository in KServe example gcs bucket according to `TorchServe` model repository layout.
The model repository contains a mnist model but you can store more than one models there. In the `Transformer` image you can create a transformer class for all the models in the repository if they can share the same transformer or maintain a map from model name to transformer classes so KServe knows to use the transformer for the corresponding model.  

```yaml
apiVersion: serving.kserve.io/v1beta1
kind: InferenceService
metadata:
  name: torchserve-transformer
spec:
  transformer:
    containers:
    - image: kfserving/torchserve-image-transformer:latest
      name: kserve-container
      env:
        - name: STORAGE_URI
          value: gs://kfserving-examples/models/torchserve/image_classifier
  predictor:
    pytorch:
      storageUri: gs://kfserving-examples/models/torchserve/image_classifier
```

Note that `STORAGE_URI` environment variable is a build-in env to inject the storage initializer for custom container just like `StorageURI` field for prepackaged predictors
and the downloaded artifacts are stored under `/mnt/models`.

Apply the CRD

```bash
kubectl apply -f transformer.yaml
```

Expected Output

```bash
inferenceservice.serving.kserve.io/torchserve-transformer created
```

## Run a prediction

The first step is to [determine the ingress IP and ports](../../../../../README.md#determine-the-ingress-ip-and-ports) and set `INGRESS_HOST` and `INGRESS_PORT`

```bash
SERVICE_NAME=torchserve-transformer
MODEL_NAME=mnist
INPUT_PATH=@./input.json
SERVICE_HOSTNAME=$(kubectl get inferenceservice $SERVICE_NAME -o jsonpath='{.status.url}' | cut -d "/" -f 3)

curl -v -H "Host: ${SERVICE_HOSTNAME}" -d $INPUT_PATH http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/$MODEL_NAME:predict
```

Expected Output

```bash
> POST /v1/models/mnist:predict HTTP/1.1
> Host: torchserve-transformer.default.example.com
> User-Agent: curl/7.73.0
> Accept: */*
> Content-Length: 401
> Content-Type: application/x-www-form-urlencoded
> 
* upload completely sent off: 401 out of 401 bytes
Handling connection for 8080
* Mark bundle as not supporting multiuse
< HTTP/1.1 200 OK
< content-length: 20
< content-type: application/json; charset=UTF-8
< date: Tue, 12 Jan 2021 09:52:30 GMT
< server: istio-envoy
< x-envoy-upstream-service-time: 83
< 
* Connection #0 to host localhost left intact
{"predictions": [2]}
```
