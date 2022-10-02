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

import asyncio
import base64
import io
from typing import Dict, Union

import kserve
import torch
from kserve.grpc.grpc_predict_v2_pb2 import (ModelInferRequest,
                                             ModelInferResponse)
from kserve.utils.utils import generate_uuid
from PIL import Image
from torchvision import models, transforms


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

    def predict(
        self,
        payload: Union[Dict, ModelInferRequest],
        headers: Dict[str, str] = None
    ) -> Union[Dict, ModelInferResponse]:
        raw_img_data = ""
        if isinstance(payload, Dict):
            inputs = payload["instances"]
            # Input follows the Tensorflow V1 HTTP API for binary values
            # https://www.tensorflow.org/tfx/serving/api_rest#encoding_binary_values
            data = inputs[0]["image"]["b64"]
            raw_img_data = base64.b64decode(data)
        elif isinstance(payload, ModelInferRequest):
            req = payload.inputs[0]
            fields = req.contents.ListFields()
            _, field_value = fields[0]
            points = list(field_value)
            raw_img_data = points[0]

        input_image = Image.open(io.BytesIO(raw_img_data))
        preprocess = transforms.Compose([
            transforms.Resize(256),
            transforms.CenterCrop(224),
            transforms.ToTensor(),
            transforms.Normalize(mean=[0.485, 0.456, 0.406],
                                 std=[0.229, 0.224, 0.225]),
        ])

        input_tensor = preprocess(input_image)
        input_batch = input_tensor.unsqueeze(0)

        output = self.model(input_batch)

        torch.nn.functional.softmax(output, dim=1)
        values, top_5 = torch.topk(output, 5)
        result = values.tolist()
        if isinstance(payload, Dict):
            return {"predictions": values.tolist()}
        else:
            return {
                "id": generate_uuid(),
                "model_name": payload.model_name,
                "outputs": [
                  {
                    "contents": {
                        "fp32_contents": result[0],
                    },
                    "datatype": "FP32",
                    "name": "output-0",
                    "shape": list(values.shape)
                  }
                ]
            }


if __name__ == "__main__":
    model = AlexNetModel("custom-model")
    model.load()
    asyncio.run(
        kserve.ModelServer(workers=1).start([model])
    )
