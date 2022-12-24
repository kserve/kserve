# Copyright 2021 The KServe Authors.
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
import kserve
from torchvision import models, transforms
from typing import Dict
import torch
from PIL import Image
import base64
import io
import numpy as np
from kserve.utils.utils import generate_uuid


# This custom predictor example implements the custom model following KServe REST v1/v2 protocol,
# the input can be raw image base64 encoded bytes or image tensor which is pre-processed by transformer
# and then passed to predictor, the output is the prediction response.
class AlexNetModel(kserve.Model):
    def __init__(self, name: str):
        super().__init__(name)
        self.name = name
        self.load()
        self.model = None
        self.ready = False

    def load(self):
        self.model = models.alexnet(pretrained=True)
        self.model.eval()
        self.ready = True

    def preprocess(self, payload: Dict, headers: Dict[str, str] = None) -> torch.Tensor:
        raw_img_data = None
        if "instances" in payload:
            headers["request-type"] = "v1"
            if "data" in payload["instances"][0]:
                # assume the data is already preprocessed in transformer
                input_tensor = torch.Tensor(payload["instances"][0]["data"])
                return torch.Tensor(input_tensor)
            elif "image" in payload["instances"][0]:
                # Input follows the Tensorflow V1 HTTP API for binary values
                # https://www.tensorflow.org/tfx/serving/api_rest#encoding_binary_values
                data = payload["instances"][0]["image"]["b64"]
                raw_img_data = base64.b64decode(data)
        elif "inputs" in payload:
            headers["request-type"] = "v2"
            inputs = payload["inputs"]
            data = inputs[0]["data"][0]
            if inputs[0]["datatype"] == "BYTES":
                raw_img_data = base64.b64decode(data)
            elif inputs[0]["datatype"] == "FP32":
                # assume the data is already preprocessed in transformer
                input_tensor = torch.Tensor(np.asarray(data))
                return input_tensor.unsqueeze(0)

        input_image = Image.open(io.BytesIO(raw_img_data))
        preprocess = transforms.Compose([
            transforms.Resize(256),
            transforms.CenterCrop(224),
            transforms.ToTensor(),
            transforms.Normalize(mean=[0.485, 0.456, 0.406],
                                 std=[0.229, 0.224, 0.225]),
        ])
        input_tensor = preprocess(input_image)
        return input_tensor.unsqueeze(0)

    def predict(self, input_tensor: torch.Tensor, headers: Dict[str, str] = None) -> Dict:
        output = self.model(input_tensor)
        torch.nn.functional.softmax(output, dim=1)
        values, top_5 = torch.topk(output, 5)
        if headers["request-type"] == "v1":
            return {"predictions": values.tolist()}
        else:
            result = values.tolist()
            response_id = generate_uuid()
            response = {
                "id": response_id,
                "model_name": "custom-model",
                "outputs": [
                    {
                        "data": result,
                        "datatype": "FP32",
                        "name": "output-0",
                        "shape": list(values.shape)
                    }
                ]}
            return response


if __name__ == "__main__":
    model = AlexNetModel("custom-model")
    model.load()
    kserve.ModelServer().start([model])
