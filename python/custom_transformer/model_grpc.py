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
import base64
from typing import Dict, Union

from kserve import Model, ModelServer, model_server
from kserve.grpc.grpc_predict_v2_pb2 import ModelInferRequest


class ImageTransformer(Model):
    def __init__(self, name: str, predictor_host: str, protocol: str):
        super().__init__(name)
        self.predictor_host = predictor_host
        self.protocol = protocol
        self.model_name = name

    def preprocess(self, request: Union[Dict, ModelInferRequest], headers: Dict[str, str] = None) -> ModelInferRequest:
        if isinstance(request, ModelInferRequest):
            return request
        else:
            payload = [
                {
                    "name": "input-0",
                    "shape": [],
                    "datatype": "BYTES",
                    "contents": {
                        "bytes_contents": [base64.b64decode(request["inputs"][0]["data"][0])]
                    }
                }
            ]
            return ModelInferRequest(model_name=self.model_name, inputs=payload)


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
