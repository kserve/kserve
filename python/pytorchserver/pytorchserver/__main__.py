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
import argparse

from pytorchserver import PyTorchModel

DEFAULT_MODEL_NAME = "model"
DEFAULT_LOCAL_MODEL_DIR = "/tmp/model"
DEFAULT_MODEL_CLASS_NAME = "PyTorchModel"

parser = argparse.ArgumentParser(parents=[kserve.model_server.parser])
parser.add_argument('--model_dir', required=True,
                    help='A URI pointer to the model directory')
parser.add_argument('--model_name', default=DEFAULT_MODEL_NAME,
                    help='The name that the model is served under.')
parser.add_argument('--model_class_name', default=DEFAULT_MODEL_CLASS_NAME,
                    help='The class name for the model.')
args, _ = parser.parse_known_args()

if __name__ == "__main__":
    model = PyTorchModel(args.model_name, args.model_class_name, args.model_dir)
    model.load()
    kserve.ModelServer().start([model])
