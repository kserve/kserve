# Copyright 2022 The KServe Authors.
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

import argparse
from typing import Dict, Union
import numpy as np
import io
from PIL import Image
from torchvision import transforms
from kserve import Model, ModelServer, model_server, InferInput, InferRequest


def image_transform(byte_array):
    """converts the input image of Bytes Array into Tensor
    Args:
        request input instance: The request input instance for image.
    Returns:
        List: Returns the data key's value and converts that into a list
        after converting it into a tensor
    """
    preprocess = transforms.Compose([
        transforms.Resize(256),
        transforms.CenterCrop(224),
        transforms.ToTensor(),
        transforms.Normalize(mean=[0.485, 0.456, 0.406],
                             std=[0.229, 0.224, 0.225]),
    ])
    image = Image.open(io.BytesIO(byte_array))
    tensor = preprocess(image).numpy()
    return tensor


class ImageTransformer(Model):
    def __init__(self, name: str, predictor_host: str, protocol: str):
        super().__init__(name)
        self.predictor_host = predictor_host
        self.protocol = protocol
        self.model_name = name

    def preprocess(self, request: Union[Dict, InferRequest], headers: Dict[str, str] = None) -> InferRequest:
        input_tensors = None
        if isinstance(request, InferRequest):
            input_tensors = [image_transform(instance) for instance in request.inputs[0]._raw_data]
        elif headers and "application/json" in headers["content-type"]:
            input_tensors = [image_transform(instance) for instance in request["inputs"][0]["data"]]
        input_tensors = np.asarray(input_tensors)
        infer_inputs = [InferInput(name="INPUT__0", datatype='FP32', shape=input_tensors.shape,
                                   data=input_tensors)]
        infer_request = InferRequest(infer_inputs)
        return infer_request


parser = argparse.ArgumentParser(parents=[model_server.parser])
parser.add_argument(
    "--predictor_host", help="The URL for the model predict function", required=True
)
parser.add_argument(
    "--protocol", help="The protocol for the predictor", default="v1"
)
parser.add_argument(
    "--model_name", help="The name that the model is served under."
)
args, _ = parser.parse_known_args()

if __name__ == "__main__":
    model = ImageTransformer(args.model_name, predictor_host=args.predictor_host,
                             protocol=args.protocol)
    ModelServer(workers=1).start([model])
