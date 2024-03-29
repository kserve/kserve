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

from kserve.model import PredictorConfig
from . import HuggingfaceModel, HuggingfaceModelRepository
import kserve
from kserve.errors import ModelMissingError


def list_of_strings(arg):
    return arg.split(',')


parser = argparse.ArgumentParser(parents=[kserve.model_server.parser])

parser.add_argument('--model_dir', required=False, default=None,
                    help='A local path to the model binary')
parser.add_argument('--model_id', required=False,
                    help='Huggingface model id')
parser.add_argument('--model_revision', required=False, default=None,
                    help='Huggingface model revision')
parser.add_argument('--tokenizer_revision', required=False, default=None,
                    help='Huggingface tokenizer revision')
parser.add_argument('--max_length', type=int, default=None,
                    help='max sequence length for the tokenizer')
parser.add_argument('--disable_lower_case', action='store_true',
                    help='do not use lower case for the tokenizer')
parser.add_argument('--disable_special_tokens', action='store_true',
                    help='the sequences will not be encoded with the special tokens relative to their model')
parser.add_argument('--tensor_input_names', type=list_of_strings, default=None,
                    help='the tensor input names passed to the model')
parser.add_argument('--task', required=False, help="The ML task name")
parser.add_argument('--disable_vllm', action='store_true', help="Do not use vllm as the default runtime")
parser.add_argument('--return_token_type_ids', action="store_true", help="Return token type ids")

try:
    from vllm.engine.arg_utils import AsyncEngineArgs

    parser = AsyncEngineArgs.add_cli_args(parser)
    _vllm = True
except ImportError:
    _vllm = False

args, _ = parser.parse_known_args()

if __name__ == "__main__":
    engine_args = None
    if _vllm and not args.disable_vllm:
        args.model = args.model_dir or args.model_id
        args.revision = args.model_revision
        engine_args = AsyncEngineArgs.from_cli_args(args)
    predictor_config = PredictorConfig(args.predictor_host, args.predictor_protocol,
                                       args.predictor_use_ssl,
                                       args.predictor_request_timeout_seconds)
    model = HuggingfaceModel(args.model_name,
                             engine_args=engine_args,
                             predictor_config=predictor_config,
                             kwargs=vars(args))
    try:
        model.load()
        kserve.ModelServer().start([model] if model.ready else [])
    except ModelMissingError:
        logging.error(f"fail to locate model file for model {args.model_name} under dir {args.model_dir},"
                      f"trying loading from model repository.")
        kserve.ModelServer(registered_models=HuggingfaceModelRepository(args.model_dir)).start(
            [model] if model.ready else [])
