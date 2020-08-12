# Predict on a InferenceService using PyTorch Server and Transformer

Most of the model servers expect tensors as input data, so a pre-processing step is needed before making the prediction call if the user is sending in raw input format. Transformer is a service we orchestrated from InferenceService spec for user implemented pre/post processing code. In the [pytorch](../../pytorch/README.md) example we call the prediction endpoint with tensor inputs, and in this example we add additional pre-processing step to allow the user send raw image data.

## Setup
1. Your ~/.kube/config should point to a cluster with [KFServing installed](https://github.com/kubeflow/kfserving/#install-kfserving).
2. Your cluster's Istio Ingress gateway must be [network accessible](https://istio.io/latest/docs/tasks/traffic-management/ingress/ingress-control/).

##  Build Transformer image

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

### Build Transformer docker image
This step can be part of your CI/CD pipeline to continuously build the transformer image version. 
```shell
docker build -t gcr.io/kubeflow-ci/kfserving/image-transformer:latest -f transformer.Dockerfile .
```

## Create the InferenceService
Please use the [YAML file](./image_transformer.yaml) to create the InferenceService, which includes a Transformer and a Predictor.

Apply the CRD
```
kubectl apply -f image_transformer.yaml
```

Expected Output
```
$ inferenceservice.serving.kubeflow.org/transformer-cifar10 created
```

## Run a prediction
The first step is to [determine the ingress IP and ports](../../../../README.md#determine-the-ingress-ip-and-ports) and set `INGRESS_HOST` and `INGRESS_PORT`

```
MODEL_NAME=transformer-cifar10
INPUT_PATH=@./input.json
SERVICE_HOSTNAME=$(kubectl get inferenceservice transformer-cifar10 -o jsonpath='{.status.url}' | cut -d "/" -f 3)

curl -v -H "Host: ${SERVICE_HOSTNAME}" -d $INPUT_PATH http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/$MODEL_NAME:predict
```

You should see an output similar to the one below:

```
> POST /models/transformer-cifar10:predict HTTP/1.1
> Host: pytorch-cifar10.default.example.com
> User-Agent: curl/7.54.0
> Accept: */*
> Content-Length: 110681
> Content-Type: application/x-www-form-urlencoded
> Expect: 100-continue
> 
< HTTP/1.1 100 Continue
* We are completely uploaded and fine
< HTTP/1.1 200 OK
< content-length: 221
< content-type: application/json; charset=UTF-8
< date: Fri, 21 Jun 2019 04:05:39 GMT
< server: istio-envoy
< x-envoy-upstream-service-time: 35292
< 

{"predictions": [[-1.6099601984024048, -2.6461076736450195, 0.32844462990760803, 2.4825074672698975, 0.43524616956710815, 2.3108043670654297, 1.00056791305542, -0.4232763648033142, -0.5100948214530945, -1.7978394031524658]]}
```

## Notebook

You can also try this example on the [notebook](./kfserving_sdk_transformer.ipynb)
