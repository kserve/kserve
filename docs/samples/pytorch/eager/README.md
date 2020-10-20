## Creating your own model and testing the PyTorch Server locally.

To test the [PyTorch](https://pytorch.org/) server, first we need to generate a simple cifar10 model using PyTorch. 

```shell
python cifar10.py
```
You should see an output similar to this

```shell
Downloading https://www.cs.toronto.edu/~kriz/cifar-10-python.tar.gz to ./data/cifar-10-python.tar.gz
Failed download. Trying https -> http instead. Downloading http://www.cs.toronto.edu/~kriz/cifar-10-python.tar.gz to ./data/cifar-10-python.tar.gz
100.0%Files already downloaded and verified
[1,  2000] loss: 2.232
[1,  4000] loss: 1.913
[1,  6000] loss: 1.675
[1,  8000] loss: 1.555
[1, 10000] loss: 1.492
[1, 12000] loss: 1.488
[2,  2000] loss: 1.412
[2,  4000] loss: 1.358
[2,  6000] loss: 1.362
[2,  8000] loss: 1.338
[2, 10000] loss: 1.315
[2, 12000] loss: 1.278
Finished Training
```

Then, we can install and run the [PyTorch Server](../../../python/pytorchserver) using the trained model and test for predictions. Models can be on local filesystem, S3 compatible object storage, Azure Blob Storage, or Google Cloud Storage. 

Note: Currently KFServing supports PyTorch models saved using [state_dict method](https://pytorch.org/tutorials/beginner/saving_loading_models.html#saving-loading-model-for-inference), PyTorch's recommended way of saving models for inference. The KFServing interface for PyTorch expects users to upload the model_class_file in same location as the PyTorch model, and accepts an optional model_class_name to be passed in as a runtime input. If model class name is not specified, we use 'PyTorchModel' as the default class name. The current interface may undergo changes as we evolve this to support PyTorch models saved using other methods as well.

```shell
python -m pytorchserver --model_dir `pwd` --model_name pytorchmodel --model_class_name Net
```

We can also use the inbuilt PyTorch support for sample datasets and do some simple predictions

```python
import torch
import torchvision
import torchvision.transforms as transforms
import requests

transform = transforms.Compose([transforms.ToTensor(),
                                transforms.Normalize((0.5, 0.5, 0.5), (0.5, 0.5, 0.5))])
testset = torchvision.datasets.CIFAR10(root='./data', train=False,
                                       download=True, transform=transform)
testloader = torch.utils.data.DataLoader(testset, batch_size=4,
                                         shuffle=False, num_workers=2)
dataiter = iter(testloader)
images, labels = dataiter.next()
formData = {
    'instances': images[0:1].tolist()
}
res = requests.post('http://localhost:8080/v1/models/pytorchmodel:predict', json=formData)
print(res)
print(res.text)
```

# Predict on a InferenceService using PyTorch Server

## Setup
1. Your ~/.kube/config should point to a cluster with [KFServing installed](https://github.com/kubeflow/kfserving/#install-kfserving).
2. Your cluster's Istio Ingress gateway must be [network accessible](https://istio.io/latest/docs/tasks/traffic-management/ingress/ingress-control/).

## Create the InferenceService

Apply the `InferenceService`
```
# For v1alpha2
kubectl apply -f pytorch.yaml

# For v1beta1
kubectl apply -f pytorch_v1beta1.yaml
```

Expected Output
```
$ inferenceservice.serving.kubeflow.org/pytorch-cifar10 created
```

> :warning: **Setting OMP_NUM_THREADS env is critical for performance**: 
OMP_NUM_THREADS is commonly used in numpy, PyTorch, and Tensorflow to perform multi-threaded linear algebra. 
We want one thread per worker instead of many threads per worker to avoid contention.

## Run a prediction
The first step is to [determine the ingress IP and ports](../../../README.md#determine-the-ingress-ip-and-ports) and set `INGRESS_HOST` and `INGRESS_PORT`

```
MODEL_NAME=pytorch-cifar10
INPUT_PATH=@./input.json
SERVICE_HOSTNAME=$(kubectl get inferenceservice pytorch-cifar10 -o jsonpath='{.status.url}' | cut -d "/" -f 3)

curl -v -H "Host: ${SERVICE_HOSTNAME}" -d $INPUT_PATH http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/$MODEL_NAME:predict
```

You should see an output similar to the one below:

```
*   Trying 10.51.242.87...
* Connected to pytorch-cifar10.default.svc.cluster.local (10.51.242.87) port 80 (#0)
> POST /v1/models/pytorch-cifar10:predict HTTP/1.1
> Host: pytorch-cifar10.default.svc.cluster.local
> User-Agent: curl/7.47.0
> Accept: */*
> Content-Length: 110681
> Content-Type: application/x-www-form-urlencoded
> Expect: 100-continue
> 
< HTTP/1.1 100 Continue
* We are completely uploaded and fine
< HTTP/1.1 200 OK
< content-length: 227
< content-type: application/json; charset=UTF-8
< date: Sun, 11 Oct 2020 21:17:04 GMT
< server: istio-envoy
< x-envoy-upstream-service-time: 11
< 
* Connection #0 to host pytorch-cifar10.default.svc.cluster.local left intact
{"predictions": [[-1.6099599599838257, -2.6461076736450195, 0.32844460010528564, 2.4825072288513184, 0.43524596095085144, 2.3108041286468506, 1.0005676746368408, -0.42327630519866943, -0.5100945234298706, -1.7978392839431763]]}
```

## Run a performance test
QPS rate `--rate` can be changed in the [perf.yaml](./perf.yaml).
```
kubectl create -f perf.yaml

Requests      [total, rate, throughput]         6000, 100.02, 100.01
Duration      [total, attack, wait]             59.996s, 59.99s, 6.392ms
Latencies     [min, mean, 50, 90, 95, 99, max]  4.792ms, 6.185ms, 5.948ms, 7.184ms, 7.64ms, 9.996ms, 38.185ms
Bytes In      [total, mean]                     1362000, 227.00
Bytes Out     [total, mean]                     429870000, 71645.00
Success       [ratio]                           100.00%
Status Codes  [code:count]                      200:6000
Error Set:
```