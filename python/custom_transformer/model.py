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
from kserve import (
    Model,
    ModelServer,
    model_server,
    InferInput,
    InferRequest,
    InferResponse,
    logging,
)
from kserve.model import PredictorProtocol, PredictorConfig


def image_transform(model_name, data):
    """converts the input image of Bytes Array into Tensor
    Args:
        model_name: The model name
        data: The input image bytes.
    Returns:
        numpy.array: Returns the numpy array after the image preprocessing.
    """
    preprocess = transforms.Compose(
        [
            transforms.Resize(256),
            transforms.CenterCrop(224),
            transforms.ToTensor(),
            transforms.Normalize(mean=[0.485, 0.456, 0.406], std=[0.229, 0.224, 0.225]),
        ]
    )
    if model_name == "mnist" or model_name == "cifar10":
        preprocess = transforms.Compose(
            [transforms.ToTensor(), transforms.Normalize((0.1307,), (0.3081,))]
        )
    byte_array = base64.b64decode(data)
    image = Image.open(io.BytesIO(byte_array))
    tensor = preprocess(image).numpy()
    return tensor


class ImageTransformer(Model):
    def __init__(
        self,
        name: str,
        predictor_config: PredictorConfig,
    ):
        super().__init__(
            name,
            predictor_config,
            return_response_headers=True,
        )
        self.ready = True

    def preprocess(
        self, payload: Union[Dict, InferRequest], headers: Dict[str, str] = None
    ) -> Union[Dict, InferRequest]:
        if isinstance(payload, InferRequest):
            input_tensors = [
                image_transform(self.name, instance)
                for instance in payload.inputs[0].data
            ]
        else:
            headers["request-type"] = "v1"
            # Input follows the Tensorflow V1 HTTP API for binary values
            # https://www.tensorflow.org/tfx/serving/api_rest#encoding_binary_values
            input_tensors = [
                image_transform(self.name, instance["image"]["b64"])
                for instance in payload["instances"]
            ]
        input_tensors = numpy.asarray(input_tensors)
        infer_inputs = [
            InferInput(
                name="INPUT__0",
                datatype="FP32",
                shape=list(input_tensors.shape),
                data=input_tensors,
            )
        ]
        infer_request = InferRequest(model_name=self.name, infer_inputs=infer_inputs)

        # Transform to KServe v1/v2 inference protocol
        if self.protocol == PredictorProtocol.REST_V1.value:
            inputs = [{"data": input_tensor.tolist()} for input_tensor in input_tensors]
            payload = {"instances": inputs}
            return payload
        else:
            return infer_request

    def postprocess(
        self,
        infer_response: Union[Dict, InferResponse],
        headers: Dict[str, str] = None,
        response_headers: Dict[str, str] = None,
    ) -> Union[Dict, InferResponse]:
        if "request-type" in headers and headers["request-type"] == "v1":
            if self.protocol == PredictorProtocol.REST_V1.value:
                return infer_response
            else:
                # if predictor protocol is v2 but transformer uses v1
                return {"predictions": infer_response.outputs[0].as_numpy().tolist()}
        else:
            return infer_response


parser = argparse.ArgumentParser(parents=[model_server.parser])
args, _ = parser.parse_known_args()

if __name__ == "__main__":
    if args.configure_logging:
        logging.configure_logging(args.log_config_file)
    model = ImageTransformer(
        args.model_name,
        PredictorConfig(
            args.predictor_host,
            args.predictor_protocol,
            args.predictor_use_ssl,
            args.predictor_request_timeout_seconds,
            args.predictor_request_retries,
            args.enable_predictor_health_check,
        ),
    )
    ModelServer().start([model])
