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

from typing import Dict
import os
import numpy as np
from paddle import inference
import kserve


class PaddleModel(kserve.Model):

    def __init__(self, name: str, model_dir: str):
        super().__init__(name)
        self.name = name
        self.model_dir = model_dir
        self.ready = False
        self.predictor = None
        self.input_tensor = None
        self.output_tensor = None

    def load(self) -> bool:
        def get_model_files(ext: str) -> str:
            file_list = []
            for filename in os.listdir(model_path):
                if filename.endswith(ext):
                    file_list.append(filename)
            if len(file_list) == 0:
                raise Exception("Missing {} model file".format(ext))
            if len(file_list) > 1:
                raise Exception("More than one {} model file".format(ext))
            return os.path.join(model_path, file_list[0])

        model_path = kserve.Storage.download(self.model_dir)
        config = inference.Config(get_model_files('.pdmodel'), get_model_files('.pdiparams'))
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

    def predict(self, request: Dict) -> Dict:
        instances = request["instances"]
        try:
            inputs = np.array(instances, dtype='float32')
        except Exception as e:
            raise Exception("Failed to initialize NumPy array from inputs:%s, %s"
                            % (e, instances)) from e

        try:
            self.input_tensor.copy_from_cpu(inputs)
            self.predictor.run()
            return {"predictions": self.output_tensor.copy_to_cpu().tolist()}
        except Exception as e:
            raise Exception("Failed to predict %s" % e) from e
