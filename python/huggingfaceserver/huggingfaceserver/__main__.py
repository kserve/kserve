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
from pathlib import Path

import kserve
from kserve.logging import logger
from kserve.model import PredictorConfig
from kserve.storage import Storage

from transformers import AutoConfig

from huggingfaceserver.task import (
    MLTask,
    infer_task_from_model_architecture,
    is_generative_task,
)

from . import (
    HuggingfaceGenerativeModel,
    HuggingfaceEncoderModel,
    Backend,
)
from .vllm.utils import (
    build_vllm_engine_args,
    infer_vllm_supported_from_model_architecture,
    maybe_add_vllm_cli_parser,
    vllm_available,
)


def list_of_strings(arg):
    return arg.split(",")


parser = argparse.ArgumentParser(parents=[kserve.model_server.parser])

parser.add_argument(
    "--model_dir",
    required=False,
    default=None,
    help="A URI pointer to the model binary",
)
parser.add_argument("--model_id", required=False, help="Huggingface model id")
parser.add_argument(
    "--model_revision", required=False, default=None, help="Huggingface model revision"
)
parser.add_argument(
    "--tokenizer_revision",
    required=False,
    default=None,
    help="Huggingface tokenizer revision",
)
parser.add_argument(
    "--max_length",
    type=int,
    required=False,
    help="max sequence length for the tokenizer",
)
parser.add_argument(
    "--disable_lower_case",
    action="store_true",
    help="do not use lower case for the tokenizer",
)
parser.add_argument(
    "--disable_special_tokens",
    action="store_true",
    help="the sequences will not be encoded with the special tokens relative to their model",
)
parser.add_argument(
    "--trust_remote_code",
    action="store_true",
    default=False,
    help="allow loading of models and tokenizers with custom code",
)
parser.add_argument(
    "--tensor_input_names",
    type=list_of_strings,
    default=None,
    help="the tensor input names passed to the model",
)
parser.add_argument("--task", required=False, help="The ML task name")
available_backends = ", ".join(f"'{b.name}'" for b in Backend)
parser.add_argument(
    "--backend",
    default="auto",
    type=lambda t: Backend[t],
    help=f"the backend to use to load the model. Can be one of {available_backends}",
)
parser.add_argument(
    "--return_token_type_ids", action="store_true", help="Return token type ids"
)

parser = maybe_add_vllm_cli_parser(parser)

args, _ = parser.parse_known_args()

if __name__ == "__main__":
    engine_args = None
    if args.model_dir:
        model_id_or_path = Path(Storage.download(args.model_dir))
    else:
        model_id_or_path = args.model_id

    if model_id_or_path is None:
        raise ValueError("You must provide a model_id or model_dir")

    if args.backend == Backend.vllm and not vllm_available():
        raise RuntimeError("Backend is set to 'vllm' but vLLM is not available")

    if (
        (args.backend == Backend.vllm or args.backend == Backend.auto)
        and vllm_available()
        and infer_vllm_supported_from_model_architecture(model_id_or_path) is not None
    ):
        from .vllm.vllm_model import VLLMModel

        args.model = args.model_dir or args.model_id
        args.revision = args.model_revision
        engine_args = build_vllm_engine_args(args)
        model = VLLMModel(args.model_name, engine_args)

    else:
        kwargs = vars(args)
        model_config = AutoConfig.from_pretrained(
            str(model_id_or_path), revision=kwargs.get("model_revision", None)
        )
        if kwargs.get("task", None) is not None:
            try:
                task = MLTask[kwargs["task"]]
            except KeyError:
                raise ValueError(f"Unsupported task: {kwargs['task']}")
        else:
            task = infer_task_from_model_architecture(model_config)

        if is_generative_task(task):
            logger.info(f"Loading generative model for task '{task.name}'")
            model = HuggingfaceGenerativeModel(
                args.model_name,
                model_id_or_path=model_id_or_path,
                task=task,
                model_config=model_config,
                model_revision=kwargs.get("model_revision", None),
                tokenizer_revision=kwargs.get("tokenizer_revision", None),
                do_lower_case=not kwargs.get("disable_lower_case", False),
                max_length=kwargs["max_length"],
                trust_remote_code=kwargs["trust_remote_code"],
            )
        else:
            predictor_config = PredictorConfig(
                args.predictor_host,
                args.predictor_protocol,
                args.predictor_use_ssl,
                args.predictor_request_timeout_seconds,
            )
            logger.info(f"Loading encoder model for task '{task.name}'")
            model = HuggingfaceEncoderModel(
                model_name=args.model_name,
                model_id_or_path=model_id_or_path,
                task=task,
                model_config=model_config,
                model_revision=kwargs.get("model_revision", None),
                tokenizer_revision=kwargs.get("tokenizer_revision", None),
                do_lower_case=not kwargs.get("disable_lower_case", False),
                add_special_tokens=not kwargs.get("disable_special_tokens", False),
                max_length=kwargs["max_length"],
                trust_remote_code=kwargs["trust_remote_code"],
                tensor_input_names=kwargs.get("tensor_input_names", None),
                return_token_type_ids=kwargs.get("return_token_type_ids", None),
            )
    model.load()
    kserve.ModelServer().start([model] if model.ready else [])
