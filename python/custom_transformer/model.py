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
from torchvision import transforms
from typing import Dict
from PIL import Image
import base64
import io
import argparse
from tritonclient.grpc.service_pb2 import ModelInferRequest, ModelInferResponse
from tritonclient.grpc import InferResult, InferInput


class ImageTransformer(kserve.Model):
    def __init__(self, name: str, predictor_host: str, protocol: str):
        super().__init__(name)
        self.predictor_host = predictor_host
        self.protocol = protocol
        self.ready = True

    def preprocess(self, request: Dict) -> ModelInferRequest:
        instances = request["instances"]
        # Input follows the Tensorflow V1 HTTP API for binary values
        # https://www.tensorflow.org/tfx/serving/api_rest#encoding_binary_values
        data = instances[0]["image"]["b64"]

        raw_img_data = base64.b64decode(data)
        input_image = Image.open(io.BytesIO(raw_img_data))
        preprocess = transforms.Compose([
            transforms.ToTensor(),
            transforms.Normalize((0.1307,), (0.3081,))
        ])

        input_tensor = preprocess(input_image).numpy()
        sp = [1]
        for p in input_tensor.shape:
            sp.append(p)
        input_0 = InferInput(
            "INPUT__0", sp, "FP32"
        )
        input_0.set_data_from_numpy(input_tensor.reshape(sp))
        inputs = [input_0]
        request = ModelInferRequest()
        request.model_name = self.name
        for infer_input in inputs:
            request.inputs.extend([infer_input._get_tensor()])
            if infer_input._get_content() is not None:
                request.raw_input_contents.extend([infer_input._get_content()])
        return request

    def postprocess(self, infer_response: ModelInferResponse) -> Dict:
        response = InferResult(infer_response)
        return {"predictions": response.as_numpy("OUTPUT__0").tolist()}


parser = argparse.ArgumentParser(parents=[kserve.model_server.parser])
parser.add_argument(
    "--predictor_host", help="The URL for the model predict function", required=True
)
parser.add_argument(
    "--protocol", help="The protocol for the predictor", required=True
)
parser.add_argument(
    "--model_name", help="The name that the model is served under."
)
args, _ = parser.parse_known_args()

if __name__ == "__main__":
    model = ImageTransformer(args.model_name, predictor_host=args.predictor_host,
                             protocol=args.protocol)
    kserve.ModelServer(workers=1).start([model])
