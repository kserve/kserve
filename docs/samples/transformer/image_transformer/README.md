# Predict on a KFService using PyTorch Server and Transformer

Most of model servers expect tensors as input data, so a pre-process step is needed before making the prediction call if user is sending in raw input format. Transformer is
a service we orchestrated from KFService spec for user implemented pre/post process code. In the [pytorch](../../pytorch/README.md) example we send the prediction endpoint with
Most of the model servers expect tensors as input data, so a pre-processing step is needed before making the prediction call if the user is sending in raw input format. Transformer is a service we orchestrated from KFService spec for user implemented pre/post processing code. In the [pytorch](../../pytorch/README.md) example we call the prediction endpoint with tensor inputs, and in this example we add additional pre-processing step to allow the user send raw image data.

## Setup
1. Your ~/.kube/config should point to a cluster with [KFServing installed](https://github.com/kubeflow/kfserving/blob/master/docs/DEVELOPER_GUIDE.md#deploy-kfserving).
2. Your cluster's Istio Ingress gateway must be network accessible.
3. Your cluster's Istio Egresss gateway must [allow Google Cloud Storage](https://knative.dev/docs/serving/outbound-network-access/)

##  Build transformer image

### Extend Transformer and implement pre/postprocess functions
```python
from typing import List, Dict
from kfserving.transformer import Transformer
from PIL import Image
import torchvision.transforms as transforms
import logging
import io
import numpy as np
import base64

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


class ImageTransformer(Transformer):

    def preprocess(self, inputs: Dict) -> Dict:
        return {'instances': [image_transform(instance) for instance in inputs['instances']]}

    def postprocess(self, inputs: List) -> List:
        return inputs
```

### Build transformer docker image
This step can be part of your CI/CD pipeline to continuously build the transformer image version. 
```shell
docker build -t yuzisun/image-transformer:latest -f transformer.Dockerfile .
```

## Create the KFService

Apply the CRD
```
kubectl apply -f image_transformer.yaml
```

Expected Output
```
$ kfservice.serving.kubeflow.org/transformer-cifar10 created
```

## Run a prediction

```
MODEL_NAME=transformer-cifar10
INPUT_PATH=@./input.json
CLUSTER_IP=$(kubectl -n istio-system get service istio-ingressgateway -o jsonpath='{.status.loadBalancer.ingress[0].ip}')

SERVICE_HOSTNAME=$(kubectl get kfservice transformer-cifar10 -o jsonpath='{.status.url}' | sed 's/.*:\/\///g')

curl -v -H "Host: ${SERVICE_HOSTNAME}" -d $INPUT_PATH http://$CLUSTER_IP/models/$MODEL_NAME:predict
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

you can also try this example on [notebook](./kfserving_sdk_transformer.ipynb)
