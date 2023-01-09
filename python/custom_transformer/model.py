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
from kserve.protocol.grpc.grpc_predict_v2_pb2 import ModelInferResponse
from kserve import Model, ModelServer, model_server, InferInput, InferRequest, InferResponse
from kserve.model import PredictorProtocol


def image_transform(model_name, data):
    """converts the input image of Bytes Array into Tensor
    Args:
        model_name: The model name
        data: The input image bytes.
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
    if model_name == "mnist" or model_name == "cifar10":
        preprocess = transforms.Compose([
            transforms.ToTensor(),
            transforms.Normalize((0.1307,), (0.3081,))
        ])
    byte_array = base64.b64decode(data)
    image = Image.open(io.BytesIO(byte_array))
    tensor = preprocess(image).numpy()
    return tensor


class ImageTransformer(Model):
    def __init__(self, name: str, predictor_host: str, protocol: str):
        super().__init__(name)
        self.predictor_host = predictor_host
        self.protocol = protocol
        self.ready = True

    def preprocess(self, payload: Union[Dict, InferRequest], headers: Dict[str, str] = None) \
            -> Union[Dict, InferRequest]:
        if isinstance(payload, InferRequest):
            input_tensors = [image_transform(self.name, instance) for instance in payload.inputs[0].data]
        else:
            headers["request-type"] = "v1"
            # Input follows the Tensorflow V1 HTTP API for binary values
            # https://www.tensorflow.org/tfx/serving/api_rest#encoding_binary_values
            input_tensors = [image_transform(self.name, instance["image"]["b64"]) for instance in payload["instances"]]
        input_tensors = numpy.asarray(input_tensors)
        infer_inputs = [InferInput(name="INPUT__0", datatype='FP32', shape=list(input_tensors.shape),
                                   data=input_tensors)]
        infer_request = InferRequest(model_name=self.name, infer_inputs=infer_inputs)

        # Transform to KServe v1/v2 inference protocol
        if self.protocol == PredictorProtocol.REST_V1.value:
            inputs = [{"data": input_tensor.tolist()} for input_tensor in input_tensors]
            payload = {"instances": inputs}
            return payload
        else:
            return infer_request

    def postprocess(self, infer_response: Union[Dict, ModelInferResponse], headers: Dict[str, str] = None) \
            -> Union[Dict, InferResponse]:
        if "request-type" in headers and headers["request-type"] == "v1":
            if self.protocol == PredictorProtocol.REST_V1.value:
                return infer_response
            else:
                res = super().postprocess(infer_response, headers)
                return {"predictions": res["outputs"][0]["data"]}
        else:
            return super().postprocess(infer_response, headers)


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
