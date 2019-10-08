## Creating your own model and testing the PyTorch server.

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

Then, we can run the PyTorch server using the trained model and test for predictions. Models can be on local filesystem, S3 compatible object storage, Azure Blob Storage, or Google Cloud Storage. 

Note: Currently KFServing supports PyTorch models saved using [state_dict method](https://pytorch.org/tutorials/beginner/saving_loading_models.html#saving-loading-model-for-inference), PyTorch's recommended way of saving models for inference. The KFServing interface for PyTorch expects users to upload the model_class_file in same location as the PyTorch model, and accepts an optional model_class_name to be passed in as a runtime input. If model class name is not specified, we use 'PyTorchModel' as the default class name. The current interface may undergo changes as we evolve this to support PyTorch models saved using other methods as well.

```shell
python -m pytorchserver --model_dir ./ --model_name pytorchmodel --model_class_name Net
```

We can also use the inbuilt PyTorch support for sample datasets and do some simple predictions

```python
import torch
import torchvision
import torchvision.transforms as transforms
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
res = requests.post('http://localhost:8080/models/pytorchmodel:predict', json=formData)
print(res)
print(res.text)
```

# Predict on a InferenceService using PyTorch

## Setup
1. Your ~/.kube/config should point to a cluster with [KFServing installed](https://github.com/kubeflow/kfserving/blob/master/docs/DEVELOPER_GUIDE.md#deploy-kfserving).
2. Your cluster's Istio Ingress gateway must be network accessible.
3. Your cluster's Istio Egresss gateway must [allow Google Cloud Storage](https://knative.dev/docs/serving/outbound-network-access/)

## Create the InferenceService

Apply the CRD
```
kubectl apply -f pytorch.yaml
```

Expected Output
```
$ inferenceservice.serving.kubeflow.org/pytorch-cifar10 created
```

## Run a prediction

```
MODEL_NAME=pytorch-cifar10
INPUT_PATH=@./input.json
CLUSTER_IP=$(kubectl -n istio-system get service istio-ingressgateway -o jsonpath='{.status.loadBalancer.ingress[0].ip}')

SERVICE_HOSTNAME=$(kubectl get inferenceservice pytorch-cifar10 -o jsonpath='{.status.url}' |sed 's/.*:\/\///g')

curl -v -H "Host: ${SERVICE_HOSTNAME}" -d $INPUT_PATH http://$CLUSTER_IP/v1/models/$MODEL_NAME:predict
```

You should see an output similar to the one below:

```
> POST /models/pytorch-cifar10:predict HTTP/1.1
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
