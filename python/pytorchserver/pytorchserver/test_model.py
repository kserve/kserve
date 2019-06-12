# Copyright 2019 kubeflow.org.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

from pytorchserver import PyTorchModel
import os
import torch
import torchvision
import torchvision.transforms as transforms

model_dir = "../../docs/samples/pytorch"
MODEL_FILE = "model.pt"

def test_model():
    server = PyTorchModel("pytorchmodel", "Net", "cifar10.py", model_dir)
    server.load()

    transform = transforms.Compose([transforms.ToTensor(),
                                    transforms.Normalize((0.5, 0.5, 0.5), (0.5, 0.5, 0.5))])
    testset = torchvision.datasets.CIFAR10(root='./data', train=False,
                                           download=True, transform=transform)
    testloader = torch.utils.data.DataLoader(testset, batch_size=4,
                                             shuffle=False, num_workers=2)
    dataiter = iter(testloader)
    images, labels = dataiter.next()

    request = images[0:1].tolist()
    response = server.predict(request)
    assert isinstance(response[0], list)
