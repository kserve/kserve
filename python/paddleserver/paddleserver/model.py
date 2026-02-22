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


import os
import numpy as np
from paddle import inference
from kserve import Model
from kserve.errors import InferenceError
from kserve_storage import Storage
from typing import Dict, Union

from kserve.protocol.infer_type import InferRequest, InferResponse
from kserve.utils.utils import get_predict_input, get_predict_response


class PaddleModel(Model):
    def __init__(self, name: str, model_dir: str):
        super().__init__(name)
        self.name = name
        self.model_dir = model_dir
        self.ready = False
        self.predictor = None
        self.input_tensor = None
        self.output_tensor = None

    def load(self) -> bool:
        def get_model_file(primary_ext: str, fallback_ext: str = None) -> str:
            def find_file_with_ext(ext):
                matches = [f for f in os.listdir(model_path) if f.endswith(ext)]
                if len(matches) == 1:
                    return os.path.join(model_path, matches[0])
                elif len(matches) > 1:
                    raise Exception(f"More than one {ext} model file found.")
                return None

            file_path = find_file_with_ext(primary_ext)
            if file_path:
                return file_path

            if fallback_ext:
                file_path = find_file_with_ext(fallback_ext)
                if file_path:
                    return file_path

            raise Exception(
                f"Missing model file with extension '{primary_ext}'"
                + (f" or '{fallback_ext}'" if fallback_ext else "")
            )

        model_path = Storage.download(self.model_dir)
        config = inference.Config(
            get_model_file(".pdmodel", ".json"), get_model_file(".pdiparams")
        )
        # TODO: add GPU support
        config.disable_gpu()

        self.predictor = inference.create_predictor(config)

        # TODO: add support for multiple input_names/output_names
        input_names = self.predictor.get_input_names()
        self.input_tensor = self.predictor.get_input_handle(input_names[0])
        output_names = self.predictor.get_output_names()
        self.output_tensor = self.predictor.get_output_handle(output_names[0])

        self.ready = True
        return self.ready

    def predict(
        self, payload: Union[Dict, InferRequest], headers: Dict[str, str] = None
    ) -> Union[Dict, InferResponse]:
        try:
            instances = get_predict_input(payload)
            np_array_input = np.array(instances, dtype="float32")
            self.input_tensor.copy_from_cpu(np_array_input)
            self.predictor.run()
            result = self.output_tensor.copy_to_cpu()
            return get_predict_response(payload, result, self.name)
        except Exception as e:
            raise InferenceError(str(e))
