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

from kserve.protocol.infer_type import InferOutput, InferRequest, InferResponse
from kserve.utils.numpy_codec import from_np_dtype
from kserve.utils.utils import generate_uuid, get_predict_input, get_predict_response


class PaddleModel(Model):
    def __init__(self, name: str, model_dir: str):
        super().__init__(name)
        self.name = name
        self.model_dir = model_dir
        self.ready = False
        self.predictor = None
        self.input_tensors = {}
        self.output_tensors = {}

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

        self.input_tensors = {
            name: self.predictor.get_input_handle(name)
            for name in self.predictor.get_input_names()
        }
        self.output_tensors = {
            name: self.predictor.get_output_handle(name)
            for name in self.predictor.get_output_names()
        }

        self.ready = True
        return self.ready

    def _validate_input_names(self, names):
        expected = set(self.input_tensors)
        if len(names) != len(set(names)) or set(names) != expected:
            raise ValueError(
                f"Expected inputs {sorted(expected)}, received {sorted(names)}"
            )

    def _get_inputs(self, payload):
        if isinstance(payload, InferRequest):
            # Existing single-input clients may use a generic name such as inputs.
            if len(self.input_tensors) == 1 and len(payload.inputs) == 1:
                return {next(iter(self.input_tensors)): payload.inputs[0].as_numpy()}
            self._validate_input_names([item.name for item in payload.inputs])
            return {item.name: item.as_numpy() for item in payload.inputs}

        if "inputs" in payload and isinstance(payload["inputs"], dict):
            values = payload["inputs"]
            self._validate_input_names(list(values))
        elif len(self.input_tensors) == 1:
            values = {next(iter(self.input_tensors)): get_predict_input(payload)}
        else:
            instances = payload.get("instances", [])
            if not isinstance(instances, list) or not instances:
                raise ValueError("Multi-input models require named inputs or instances")
            for instance in instances:
                if not isinstance(instance, dict):
                    raise ValueError(
                        "Each instance must be a dictionary of named inputs"
                    )
                self._validate_input_names(list(instance))
            values = {
                name: [instance[name] for instance in instances]
                for name in self.input_tensors
            }
        # JSON carries no dtype. Use the model's input types instead of forcing
        # integer token IDs (and other inputs) to float32.
        return {
            name: np.asarray(value, dtype=self.input_tensors[name].type().name.lower())
            for name, value in values.items()
        }

    def _get_response(self, payload, results):
        if len(results) == 1:
            # Preserve the legacy output-0 name and V1 predictions structure.
            return get_predict_response(
                payload, next(iter(results.values())), self.name
            )
        if isinstance(payload, dict):
            batch_sizes = {value.shape[0] for value in results.values() if value.ndim}
            if len(batch_sizes) != 1 or any(
                value.ndim == 0 for value in results.values()
            ):
                raise ValueError(
                    "V1 outputs must share a batch dimension; use V2 otherwise"
                )
            return {
                "predictions": [
                    {name: value[index].tolist() for name, value in results.items()}
                    for index in range(next(iter(batch_sizes)))
                ]
            }
        outputs = []
        for name, value in results.items():
            output = InferOutput(name, list(value.shape), from_np_dtype(value.dtype))
            output.set_data_from_numpy(value, binary_data=payload.use_binary_outputs)
            outputs.append(output)
        return InferResponse(
            model_name=self.name,
            infer_outputs=outputs,
            response_id=payload.id if payload.id else generate_uuid(),
            use_binary_outputs=payload.use_binary_outputs,
            requested_outputs=payload.request_outputs,
        )

    def predict(
        self, payload: Union[Dict, InferRequest], headers: Dict[str, str] = None
    ) -> Union[Dict, InferResponse]:
        try:
            inputs = self._get_inputs(payload)
            for name, value in inputs.items():
                self.input_tensors[name].copy_from_cpu(value)
            self.predictor.run()
            results = {
                name: tensor.copy_to_cpu()
                for name, tensor in self.output_tensors.items()
            }
            return self._get_response(payload, results)
        except Exception as e:
            raise InferenceError(str(e))
