# Predict on a KFService using PyTorch Server and Transformer

PyTorch server expects tensor in and tensor out and often the time it requires transforming from the raw inputs to tensors as
a preprocess step before making the predition call. The example demonstrates the capability of KFService to automatically wire up the call graph
for the user between transformer and predictor with provided user code for preprocess/postprocess.


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
> Host: pytorch-cifar10.default.svc.cluster.local
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

{"predictions": [[-0.8955065011978149, -1.4453213214874268, 0.1515328735113144, 2.638284683227539, -1.00240159034729, 2.270702600479126, 0.22645258903503418, -0.880557119846344, 0.08783778548240662, -1.5551214218139648]]
```

## Notebook

you can also try this example on [notebook](./kfserving_sdk_transformer.ipynb)