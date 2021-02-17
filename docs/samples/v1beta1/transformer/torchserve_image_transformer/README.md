# Predict on a InferenceService with transformer using Torchserve

## Setup

1. Your ~/.kube/config should point to a cluster with [KFServing installed](https://github.com/kubeflow/kfserving/#install-kfserving).
2. Your cluster's Istio Ingress gateway must be [network accessible](https://istio.io/latest/docs/tasks/traffic-management/ingress/ingress-control/).

## Build Transformer image

### Extend KFModel and implement pre/post processing functions

```python
import kfserving
from typing import List, Dict
from PIL import Image
import torchvision.transforms as transforms
import logging
import io
import numpy as np
import base64

logging.basicConfig(level=kfserving.constants.KFSERVING_LOGLEVEL)

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


class ImageTransformer(kfserving.KFModel):
    def __init__(self, name: str, predictor_host: str):
        super().__init__(name)
        self.predictor_host = predictor_host

    def preprocess(self, inputs: Dict) -> Dict:
        return {'instances': [image_transform(instance) for instance in inputs['instances']]}

    def postprocess(self, inputs: List) -> List:
        return inputs
```

## Build Transformer docker image

```bash
docker build -t {username}/image-transformer:latest -f transformer.Dockerfile .

docker push {username}/image-transformer:latest
```

## Create the InferenceService
Please use the [YAML file](./transformer.yaml) to create the InferenceService, which includes a Transformer and a Predictor.

Apply the CRD
```
kubectl apply -f image_transformer.yaml
```

Expected Output
```
$ inferenceservice.serving.kubeflow.org/torchserve-transformer created
```

## Run a prediction
The first step is to [determine the ingress IP and ports](../../../../../README.md#determine-the-ingress-ip-and-ports) and set `INGRESS_HOST` and `INGRESS_PORT`

```
SERVICE_NAME=torchserve-transformer
MODEL_NAME=mnist
INPUT_PATH=@./input.json
SERVICE_HOSTNAME=$(kubectl get inferenceservice $SERVICE_NAME -o jsonpath='{.status.url}' | cut -d "/" -f 3)

curl -v -H "Host: ${SERVICE_HOSTNAME}" -d $INPUT_PATH http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/$MODEL_NAME:predict
```

Expected Output
```
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


