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
from typing import Dict
import numpy as np
import io
from PIL import Image
from torchvision import transforms
from kserve import Model, ModelServer, model_server, InferInput, InferRequest, logging
from kserve.model import PredictorConfig


def image_transform(data):
    """converts the input image of Bytes Array into Tensor
    Args:
        request input instance: The request input instance for image.
    Returns:
        List: Returns the data key's value and converts that into a list
        after converting it into a tensor
    """
    preprocess = transforms.Compose(
        [
            transforms.Resize(256),
            transforms.CenterCrop(224),
            transforms.ToTensor(),
            transforms.Normalize(mean=[0.485, 0.456, 0.406], std=[0.229, 0.224, 0.225]),
        ]
    )
    image = Image.open(io.BytesIO(data))
    tensor = preprocess(image).numpy()
    return tensor


class ImageTransformer(Model):
    def __init__(
        self,
        name: str,
        predictor_config: PredictorConfig,
    ):
        super().__init__(name, predictor_config)
        self.ready = True

    def preprocess(
        self, request: InferRequest, headers: Dict[str, str] = None
    ) -> InferRequest:
        input_tensors = [
            image_transform(instance) for instance in request.inputs[0].data
        ]
        input_tensors = np.asarray(input_tensors)
        infer_inputs = [
            InferInput(
                name="INPUT__0",
                datatype="FP32",
                shape=list(input_tensors.shape),
                data=input_tensors,
            )
        ]
        infer_request = InferRequest(model_name=self.name, infer_inputs=infer_inputs)
        return infer_request


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
    ModelServer(workers=1).start([model])
