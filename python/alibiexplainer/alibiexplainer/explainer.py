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
import json
import logging
import asyncio
from enum import Enum
from typing import List, Any, Mapping, Union, Dict

import kserve
import numpy as np
import struct
from alibiexplainer.anchor_images import AnchorImages
from alibiexplainer.anchor_tabular import AnchorTabular
from alibiexplainer.anchor_text import AnchorText
from alibiexplainer.explainer_wrapper import ExplainerWrapper
from kserve.model import PredictorProtocol
from tritonclient.grpc import service_pb2 as pb

import nest_asyncio
nest_asyncio.apply()

logging.basicConfig(level=kserve.constants.KSERVE_LOGLEVEL)


def deserialize_bytes_tensor(encoded_tensor):
    strs = list()
    offset = 0
    while offset < len(encoded_tensor):
        val = struct.unpack_from("<f", encoded_tensor, offset)[0]
        offset += 4
        strs.append(val)
    return np.array(strs, dtype=np.float32)


class ExplainerMethod(Enum):
    anchor_tabular = "AnchorTabular"
    anchor_images = "AnchorImages"
    anchor_text = "AnchorText"

    def __str__(self):
        return self.value


class AlibiExplainer(kserve.Model):
    def __init__(  # pylint:disable=too-many-arguments
        self,
        name: str,
        protocol: str,
        predictor_host: str,
        method: ExplainerMethod,
        config: Mapping,
        explainer: object = None,
    ):
        super().__init__(name)
        self.protocol = protocol
        self.predictor_host = predictor_host
        logging.info("Predict URL set to %s", self.predictor_host)
        self.method = method

        if self.method is ExplainerMethod.anchor_tabular:
            self.wrapper: ExplainerWrapper = AnchorTabular(
                self._predict_fn, explainer, **config
            )
        elif self.method is ExplainerMethod.anchor_images:
            self.wrapper = AnchorImages(self._predict_fn, explainer, **config)
        elif self.method is ExplainerMethod.anchor_text:
            self.wrapper = AnchorText(self._predict_fn, explainer, **config)
        else:
            raise NotImplementedError

    def load(self) -> bool:
        pass

    def _predict_fn(self, arr: Union[np.ndarray, List]) -> np.ndarray:
        instances = []
        for req_data in arr:
            if isinstance(req_data, np.ndarray):
                instances.append(req_data.tolist())
            else:
                instances.append(req_data)
        shape = np.shape(instances)
        loop = asyncio.get_running_loop()  # type: ignore

        # Prepare the request beased on the predictor protocol
        if self.protocol == PredictorProtocol.GRPC_V2.value:
            data = np.array(instances, dtype=np.float32).flatten()
            tensor_contents = pb.InferTensorContents(fp32_contents=data)
            inputs = pb.ModelInferRequest().InferInputTensor(
                    name="input_1",
                    shape=list(shape),
                    datatype="FP32",
                    contents=tensor_contents
            )
            predict_req = pb.ModelInferRequest(model_name=self.name, inputs=[inputs])
        elif self.protocol == PredictorProtocol.REST_V2.value:
            predict_req = {
                "inputs": [
                    {
                        "name": "input_1",
                        "shape": list(shape),
                        "datatype": "FP32",
                        "data": instances
                    }
                ]
            }
        else:
            predict_req = {"instances": instances}

        resp = loop.run_until_complete(self.predict(predict_req))

        # Process the request beased on the predictor protocol
        if self.protocol == PredictorProtocol.GRPC_V2.value:
            shape = []
            for value in resp.outputs[0].shape:
                shape.append(value)
            rst = deserialize_bytes_tensor(resp.raw_output_contents[0])
            outputs = np.resize(rst, shape)
        elif self.protocol == PredictorProtocol.REST_V2.value:
            outputs_shape = resp["outputs"][0]["shape"]
            outputs = np.reshape(np.array(resp["outputs"][0]["data"]), outputs_shape)
        else:
            outputs = np.array(resp["predictions"])

        return outputs

    def explain(self, payload: Dict, headers: Dict[str, str] = None) -> Any:
        if (
                self.method is ExplainerMethod.anchor_tabular
                or self.method is ExplainerMethod.anchor_images
                or self.method is ExplainerMethod.anchor_text
        ):
            explanation = self.wrapper.explain(payload["instances"])
            explanationAsJsonStr = explanation.to_json()
            logging.info("Explanation: %s", explanationAsJsonStr)
            return json.loads(explanationAsJsonStr)

        raise NotImplementedError
