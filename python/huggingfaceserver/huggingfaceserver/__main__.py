# Copyright 2024 The KServe Authors.
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

from . import HuggingfaceModel, HuggingfaceModelRepository
import kserve
from kserve.errors import ModelMissingError

parser = argparse.ArgumentParser(parents=[kserve.model_server.parser])

parser.add_argument('--model_dir', required=False, default="/mnt/models",
                    help='A URI pointer to the model binary')
parser.add_argument('--model_id', required=False,
                    help='Huggingface model id')
parser.add_argument('--tensor_parallel_degree', type=int, default=-1,
                    help='tensor parallel degree')
parser.add_argument('--max_length', type=int, default=None,
                    help='max sequence length for the tokenizer')
parser.add_argument('--do_lower_case', type=bool, default=True,
                    help='do lower case for the tokenizer')
parser.add_argument('--add_special_tokens', type=bool, default=True,
                    help='the sequences will be encoded with the special tokens relative to their model')
parser.add_argument('--task', required=False,  help="The ML task name")

try:
    from vllm.engine.arg_utils import AsyncEngineArgs
    parser = AsyncEngineArgs.add_cli_args(parser)
    _vllm = True
except ImportError:
    _vllm = False
args, _ = parser.parse_known_args()

if __name__ == "__main__":
    engine_args = AsyncEngineArgs.from_cli_args(args) if _vllm else None
    model = HuggingfaceModel(args.model_name, vars(args), engine_args)
    try:
        model.load()
    except ModelMissingError:
        logging.error(f"fail to locate model file for model {args.model_name} under dir {args.model_dir},"
                      f"trying loading from model repository.")
    if not args.model_id:
        kserve.ModelServer(registered_models=HuggingfaceModelRepository(args.model_dir)).start(
            [model] if model.ready else [])
    else:
        kserve.ModelServer().start([model] if model.ready else [])
