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

import argparse
import base64
import io
from typing import Dict, Union

import numpy

from PIL import Image
from torchvision import transforms
from kserve.grpc.grpc_predict_v2_pb2 import ModelInferRequest, ModelInferResponse
from kserve import Model, ModelServer, model_server, InferInput, InferRequest
from kserve.model import PredictorProtocol


def image_transform(instance):
    """converts the input image of Bytes Array into Tensor
    Args:
        instance: The input image bytes.
    Returns:
        numpy.array: Returns the numpy array after the image preprocessing.
    """
    preprocess = transforms.Compose([
        transforms.Resize(256),
        transforms.CenterCrop(224),
        transforms.ToTensor(),
        transforms.Normalize(mean=[0.485, 0.456, 0.406],
                             std=[0.229, 0.224, 0.225]),
    ])
    byte_array = base64.b64decode(instance["image"]["b64"])
    image = Image.open(io.BytesIO(byte_array))
    tensor = preprocess(image).numpy()
    return tensor


class ImageTransformer(Model):
    def __init__(self, name: str, predictor_host: str, protocol: str):
        super().__init__(name)
        self.predictor_host = predictor_host
        self.protocol = protocol
        self.ready = True

    def preprocess(self, payload: Dict, headers: Dict[str, str] = None) -> Union[Dict, ModelInferRequest]:
        # Input follows the Tensorflow V1 HTTP API for binary values
        # https://www.tensorflow.org/tfx/serving/api_rest#encoding_binary_values
        input_tensors = [image_transform(instance) for instance in payload["instances"]]
        input_tensors = numpy.asarray(input_tensors)
        infer_inputs = [InferInput(name="INPUT__0", datatype='FP32', shape=input_tensors.shape,
                                   data=input_tensors)]
        infer_request = InferRequest(infer_inputs)

        # Transform to KServe v1/v2 inference protocol
        if self.protocol == PredictorProtocol.REST_V1.value:
            inputs = [{"data": input_tensor.tolist()} for input_tensor in input_tensors]
            payload = {"instances": inputs}
            return payload
        else:
            return infer_request

    def postprocess(self, infer_response: Union[Dict, ModelInferResponse], headers: Dict[str, str] = None) -> Dict:
        if self.protocol == PredictorProtocol.GRPC_V2.value:
            res = super().postprocess(infer_response, headers)
            return {"predictions": res["outputs"][0]["data"]}
        elif self.protocol == PredictorProtocol.REST_V2.value:
            return {"predictions": infer_response["outputs"][0]["data"]}
        else:
            return infer_response


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
    ModelServer().start([model])
