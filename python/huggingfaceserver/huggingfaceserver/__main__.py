# Copyright 2023 The KServe Authors.
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
import logging

from huggingfaceserver import HuggingfaceModel, HuggingfaceModelRepository

import kserve
from kserve.errors import ModelMissingError

DEFAULT_MODEL_NAME = "model"

parser = argparse.ArgumentParser(parents=[kserve.model_server.parser])
parser.add_argument('--model_dir', required=False,
                    help='A URI pointer to the model binary')
parser.add_argument('--model_id', required=False,
                    help='Huggingface model id')
parser.add_argument('--enable_streaming', type=bool, default=False,
                    help='enable streaming mode')
parser.add_argument('--tensor_parallel_degree', type=int, default=-1,
                    help='tensor parallel degree')
parser.add_argument('--task', required=False,  help="The task name")

args, _ = parser.parse_known_args()

if __name__ == "__main__":
    print(vars(args))
    model = HuggingfaceModel(args.model_name, vars(args))
    try:
        model.load()
    except ModelMissingError:
        logging.error(f"fail to locate model file for model {args.model_name} under dir {args.model_dir},"
                      f"trying loading from model repository.")
    print(args.model_id)
    if not args.model_id:
        kserve.ModelServer(registered_models=HuggingfaceModelRepository(args.model_dir)).start(
            [model] if model.ready else [])
    else:
        kserve.ModelServer().start([model] if model.ready else [])
