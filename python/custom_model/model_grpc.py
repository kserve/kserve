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
import io
from typing import Dict

import torch
from kserve import InferRequest, Model, ModelServer, logging, model_server
from kserve.utils.utils import generate_uuid
from PIL import Image
from torchvision import models, transforms


# This custom predictor example implements the custom model following KServe v2 inference gPPC protocol,
# the input can be raw image bytes or image tensor which is pre-processed by transformer
# and then passed to predictor, the output is the prediction response.
class AlexNetModel(Model):
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

    def preprocess(
            self, payload: InferRequest, headers: Dict[str, str] = None
    ) -> torch.Tensor:
        req = payload.inputs[0]
        if req.datatype == "BYTES":
            input_image = Image.open(io.BytesIO(req.data[0]))
            preprocess = transforms.Compose(
                [
                    transforms.Resize(256),
                    transforms.CenterCrop(224),
                    transforms.ToTensor(),
                    transforms.Normalize(
                        mean=[0.485, 0.456, 0.406], std=[0.229, 0.224, 0.225]
                    ),
                ]
            )

            input_tensor = preprocess(input_image)
            return input_tensor.unsqueeze(0)
        elif req.datatype == "FP32":
            np_array = payload.inputs[0].as_numpy()
            return torch.Tensor(np_array)

    def predict(
            self, input_tensor: torch.Tensor, headers: Dict[str, str] = None
    ) -> Dict:
        output = self.model(input_tensor)
        torch.nn.functional.softmax(output, dim=1)
        values, top_5 = torch.topk(output, 5)
        result = values.flatten().tolist()
        id = generate_uuid()
        response = {
            "id": id,
            "model_name": "custom-model",
            "outputs": [
                {
                    "contents": {
                        "fp32_contents": result,
                    },
                    "datatype": "FP32",
                    "name": "output-0",
                    "shape": list(values.shape),
                }
            ],
        }
        return response


parser = argparse.ArgumentParser(parents=[model_server.parser])
args, _ = parser.parse_known_args()

if __name__ == "__main__":
    logging.configure_logging(args.log_config_file)
    model = AlexNetModel("custom-model")
    model.load()
    ModelServer().start([model])
