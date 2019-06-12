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

Then, we can run the PyTorch server using the trained model and test for predictions. Models can be on local filesystem, S3 compatible object storage or Google Cloud Storage.

```shell
python -m pytorchserver --model_dir ./ --model_name pytorchmodel --model_class_name Net --model_class_file cifar10.py
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
