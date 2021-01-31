# Predict on a Triton InferenceService with TorchScript model
While Python is a suitable and preferred language for many scenarios requiring dynamism and ease of iteration,
there are equally many situations where precisely these properties of Python are unfavorable. One environment in which
the latter often applies is production – the land of low latencies and strict deployment requirements. For production scenarios,
C++ is very often the language of choice, The following example will outline the path PyTorch provides to go from an existing Python model
to a serialized representation that can be loaded and executed purely from C++ like Triton Inference Server, with no dependency on Python.

## Setup
1. Your ~/.kube/config should point to a cluster with [KFServing 0.5 installed](https://github.com/kubeflow/kfserving/#install-kfserving).
2. Your cluster's Istio Ingress gateway must be [network accessible](https://istio.io/latest/docs/tasks/traffic-management/ingress/ingress-control/).
3. Skip [tag resolution](https://knative.dev/docs/serving/tag-resolution/) for `nvcr.io` which requires auth to resolve triton inference server image digest
```bash
kubectl patch cm config-deployment --patch '{"data":{"registriesSkippingTagResolving":"nvcr.io"}}' -n knative-serving
```
4. Increase progress deadline since pulling triton image and big bert model may longer than default timeout for 120s, this setting requires knative 0.15.0+
```bash
kubectl patch cm config-deployment --patch '{"data":{"progressDeadline": "600s"}}' -n knative-serving
```

## Train a Pytorch Model
Train the [cifar pytorch model](../eager/cifar10.py).

## Export as Torchscript Model
A PyTorch model’s journey from Python to C++ is enabled by [Torch Script](https://pytorch.org/docs/master/jit.html), a representation of a PyTorch model
that can be understood, compiled and serialized by the Torch Script compiler. If you are starting out from an existing PyTorch model written in the vanilla `eager` API,
you must first convert your model to Torch Script.

Convert the above model via Tracing and serialize the script module to a file
```python
import torch
# Use torch.jit.trace to generate a torch.jit.ScriptModule via tracing.
example = torch.rand(1, 3, 32, 32)
traced_script_module = torch.jit.trace(net, example)
traced_script_module.save("model.pt")
```

## Store your trained model on GCS in a Model Repository
Once the model is exported as Torchscript model file, the next step is to upload the model to a GCS bucket.
Triton supports loading multiple models so it expects a model repository which follows a required layout in the bucket.
```
<model-repository-path>/
  <model-name>/
    [config.pbtxt]
    [<output-labels-file> ...]
    <version>/
      <model-definition-file>
    <version>/
      <model-definition-file>
    ...
  <model-name>/
    [config.pbtxt]
    [<output-labels-file> ...]
    <version>/
      <model-definition-file>
    <version>/
      <model-definition-file>
```
For example in your model repository bucket `gs://kfserving-examples/models/torchscript`, the layout can be
```
torchscript/
  cifar/
    config.pbtxt
    1/
      model.pt
```
The config.pbtxt defines a model configuration that provides the required and optional information for the model.
A minimal model configuration must specify name, platform, max_batch_size, input, and output. Due to the absence of names 
for inputs and outputs in a TorchScript model, the `name` attribute of both the inputs and outputs in the configuration must
follow a specific naming convention i.e. “<name>__<index>”. Where <name> can be any string and <index> refers to the position of the corresponding
input/output. This means if there are two inputs and two outputs they must be named as: `INPUT__0`, `INPUT__1` and `OUTPUT__0`, `OUTPUT__1` such that `INPUT__0`
refers to first input and INPUT__1 refers to the second input, etc.
```
name: "cifar"
platform: "pytorch_libtorch"
max_batch_size: 1
input [
  {
    name: "INPUT__0"
    data_type: TYPE_FP32
    dims: [3,32,32]
  }
]
output [
  {
    name: "OUTPUT__0"
    data_type: TYPE_FP32
    dims: [10]
  }
]

instance_group [
    {
        count: 1
        kind: KIND_CPU
    }
]
```

To schedule the model on GPU you would need to change the `instance_group` with GPU kind
```
instance_group [
    {
        count: 1
        kind: KIND_GPU
    }
]
```


## Inference with HTTP endpoint

### Create the InferenceService
Create the inference service yaml with the above specified model repository uri.
```
kubectl apply -f torchscript.yaml
```

```yaml
apiVersion: serving.kubeflow.org/v1beta1
kind: InferenceService
metadata:
  name: torchscript-cifar10
spec:
  predictor:
    triton:
      storageUri: gs://kfserving-examples/models/torchscript
      runtimeVersion: 20.10-py3
      env:
      - name: OMP_NUM_THREADS
        value: "1"
```

> :warning: **Setting OMP_NUM_THREADS env is critical for performance**: 
OMP_NUM_THREADS is commonly used in numpy, PyTorch, and Tensorflow to perform multi-threaded linear algebra. 
We want one thread per worker instead of many threads per worker to avoid contention.


Expected Output and check the readiness of the `InferenceService`
```
$ inferenceservice.serving.kubeflow.org/torchscript-cifar10 created
```

```bash
kubectl get inferenceservices torchscript-demo
```

### Run a prediction with curl
The first step is to [determine the ingress IP and ports](../../../README.md#determine-the-ingress-ip-and-ports) and set `INGRESS_HOST` and `INGRESS_PORT`

The latest Triton Inference Server already switched to use KFServing [prediction V2 protocol](https://github.com/kubeflow/kfserving/tree/master/docs/predict-api/v2), so 
the input request needs to follow the V2 schema with the specified data type, shape.
```bash
MODEL_NAME=cifar10
INPUT_PATH=@./input.json
SERVICE_HOSTNAME=$(kubectl get inferenceservice torchscript-cifar10 -o jsonpath='{.status.url}' | cut -d "/" -f 3)
curl -v -X -H "Host: ${SERVICE_HOSTNAME}" POST https://${INGRESS_HOST}:${INGRESS_PORT}/v2/models/$MODEL_NAME/infer -d $INPUT_PATH
```
expected output
```bash
* Connected to torchscript-cifar.default.svc.cluster.local (10.51.242.87) port 80 (#0)
> POST /v2/models/cifar10/infer HTTP/1.1
> Host: torchscript-cifar.default.svc.cluster.local
> User-Agent: curl/7.47.0
> Accept: */*
> Content-Length: 110765
> Content-Type: application/x-www-form-urlencoded
> Expect: 100-continue
> 
< HTTP/1.1 100 Continue
* We are completely uploaded and fine
< HTTP/1.1 200 OK
< content-length: 315
< content-type: application/json
< date: Sun, 11 Oct 2020 21:26:51 GMT
< x-envoy-upstream-service-time: 8
< server: istio-envoy
< 
* Connection #0 to host torchscript-cifar.default.svc.cluster.local left intact
{"model_name":"cifar10","model_version":"1","outputs":[{"name":"OUTPUT__0","datatype":"FP32","shape":[1,10],"data":[-2.0964810848236086,-0.13700756430625916,-0.5095657706260681,2.795621395111084,-0.5605481863021851,1.9934231042861939,1.1288187503814698,-1.4043136835098267,0.6004879474639893,-2.1237082481384279]}]}
```

## Inference with gRPC endpoint

### Create the InferenceService
Create the inference service yaml and expose the gRPC port, currently only one port is allowed to expose either HTTP or gRPC port and by
default HTTP port is exposed.
```
kubectl apply -f torchscript.yaml
```

```yaml
apiVersion: serving.kubeflow.org/v1beta1
kind: InferenceService
metadata:
  name: torchscript-cifar10
spec:
  predictor:
    triton:
      storageUri: gs://kfserving-examples/models/torchscript
      runtimeVersion: 20.10-py3
      ports:
      - containerPort: 9000
        name: h2c
        protocol: TCP
      env:
      - name: OMP_NUM_THREADS
        value: "1"
```



## Run a performance test
QPS rate `--rate` can be changed in the [perf.yaml](./perf.yaml).
```
kubectl create -f perf.yaml

Requests      [total, rate, throughput]         6000, 100.02, 100.01
Duration      [total, attack, wait]             59.995s, 59.99s, 4.961ms
Latencies     [min, mean, 50, 90, 95, 99, max]  4.222ms, 5.7ms, 5.548ms, 6.384ms, 6.743ms, 9.286ms, 25.85ms
Bytes In      [total, mean]                     1890000, 315.00
Bytes Out     [total, mean]                     665874000, 110979.00
Success       [ratio]                           100.00%
Status Codes  [code:count]                      200:6000
Error Set:
```

## Add Transformer on the InferenceService

`Triton Inference Server` expects tensors as input data, often the time a pre-processing step is required before making the prediction call
when the user is sending in request with raw input format. Transformer component can be specified on InferenceService spec for user implemented pre/post processing code.
User is responsible to create a python class which extends from KFServing `KFModel` base class which implements `preprocess` handler to transform raw input
format to tensor format according to V2 prediction protocol, `postprocess` handle is to convert raw prediction response to a more user friendly response.

### Implement pre/post processing functions
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
        return {
           'inputs': [
               {
                 'name': 'INPUT__0',
                 'shape': [1, 3, 32, 32],
                 'datatype': "FP32",
                 'data': [image_transform(instance) for instance in inputs['instances']]
               }
            ]
        }

    def postprocess(self, results: Dict) -> Dict:
        # Here we reshape the data because triton always returns the flatten 1D array as json if not explicitly requesting binary
        # since we are not using the triton python client library which takes care of the reshape it is up to user to reshape the returned tensor.
        return {output["name"] : np.array(output["data"]).reshape(output["shape"]) for output in results["outputs"]} 
```

### Build Transformer docker image
```
docker build -t $DOCKER_USER/image-transformer-v2:latest -f transformer.Dockerfile . --rm
```

### Create the InferenceService with Transformer
Please use the [YAML file](./torch_transformer.yaml) to create the InferenceService, which adds the image transformer component with the docker image built from above.
```yaml
apiVersion: serving.kubeflow.org/v1beta1
kind: InferenceService
metadata:
  name: torch-transfomer
spec:
  predictor:
    triton:
      storageUri: gs://kfserving-examples/models/torchscript
      runtimeVersion: 20.10-py3
      env:
      - name: OMP_NUM_THREADS
        value: "1"
  transformer:
    containers:
    - image: yuzisun/image-transformer-v2:latest
      name: kfserving-container
      command:
      - "python"
      - "-m"
      - "image_transformer_v2"
      args:
      - --model_name
      - cifar10
      - --protocol
      - v2
```

```
kubectl apply -f torch_transformer.yaml
```

Expected Output
```
$ inferenceservice.serving.kubeflow.org/torch-transfomer created
```

### Run a prediction from curl
The transformer does not enforce a specific schema like predictor but the general recommendation is to send in as a list of object(dict): 
`"instances": <value>|<list-of-objects>`
```json
{
  "instances": [
    {
      "image": { "b64": "aW1hZ2UgYnl0ZXM=" },
      "caption": "seaside"
    },
    {
      "image": { "b64": "YXdlc29tZSBpbWFnZSBieXRlcw==" },
      "caption": "mountains"
    }
  ]
}
```
```
SERVICE_NAME=torch-transfomer
MODEL_NAME=cifar10
INPUT_PATH=@./image.json

SERVICE_HOSTNAME=$(kubectl get inferenceservice $SERVICE_NAME -o jsonpath='{.status.url}' | cut -d "/" -f 3)

curl -v -X POST -H "Host: ${SERVICE_HOSTNAME}" https://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/$MODEL_NAME:predict -d $INPUT_PATH
```

You should see an output similar to the one below:

```
> POST /v2/models/cifar:predict HTTP/2
> user-agent: curl/7.71.1
> accept: */*
> content-length: 3422
> content-type: application/x-www-form-urlencoded
> 
* We are completely uploaded and fine
* TLSv1.3 (IN), TLS handshake, Newsession Ticket (4):
* TLSv1.3 (IN), TLS handshake, Newsession Ticket (4):
* old SSL session ID is stale, removing
* Connection state changed (MAX_CONCURRENT_STREAMS == 4294967295)!
< HTTP/2 200 
< content-length: 338
< content-type: application/json; charset=UTF-8
< date: Thu, 08 Oct 2020 13:15:14 GMT
< server: istio-envoy
< x-envoy-upstream-service-time: 52
< 
{"model_name": "cifar", "model_version": "1", "outputs": [{"name": "OUTPUT__0", "datatype": "FP32", "shape": [1, 10], "data": [-0.7299326062202454, -2.186835289001465, -0.029627874493598938, 2.3753483295440674, -0.3476247489452362, 1.3253062963485718, 0.5721136927604675, 0.049311548471450806, -0.3691796362400055, -1.0804035663604736]}]}
```
