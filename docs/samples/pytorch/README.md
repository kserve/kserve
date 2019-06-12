## Creating your own model and testing the Scikit-Learn server.

To test the [PyTorch](https://pytorch.org/) server, first we need to generate a simple cifar10 model using PyTorch.

```shell
python cifar10.py
```

Then, we can run the PyTorch server using the generated model and test for prediction. Models can be on local filesystem, S3 compatible object storage or Google Cloud Storage.

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
