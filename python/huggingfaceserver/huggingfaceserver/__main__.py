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
from pathlib import Path
from typing import cast

import torch
import kserve
from kserve.logging import logger
from kserve.model import PredictorConfig
from kserve.storage import Storage

from transformers import AutoConfig

from huggingfaceserver.task import (
    MLTask,
    infer_task_from_model_architecture,
    is_generative_task,
    SUPPORTED_TASKS,
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
    default="/mnt/models",
    help="A URI pointer to the model binary",
)
parser.add_argument(
    "--model_id", required=False, default=None, help="Huggingface model id"
)
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
parser.add_argument(
    "--return_probabilities",
    action="store_true",
    help="Return all probabilities",
)

parser = maybe_add_vllm_cli_parser(parser)

default_dtype = "float16" if torch.cuda.is_available() else "float32"
if not vllm_available():
    dtype_choices = ["auto", "float16", "float32", "bfloat16", "float", "half"]
    parser.add_argument(
        "--dtype",
        required=False,
        default="auto",
        choices=dtype_choices,
        help=f"data type to load the weights in. One of {dtype_choices}. Defaults to float16 for GPU and float32 for CPU systems",
    )

args, _ = parser.parse_known_args()

# auto for vLLM uses FP16 even for an FP32 model while HF uses FP32 causing inconsistency.
# To ensure consistency b/w vLLM and HF, we use FP16 for auto on GPU instances
# auto would use FP32 for CPU only instances.
# FP16, BF16 and FP32 if explicitly mentioned would use those data types
if "dtype" in args and args.dtype == "auto":
    args.dtype = default_dtype


def load_model():
    engine_args = None
    # If --model_id is specified then pass model_id to HF API, otherwise load the model from /mnt/models
    if args.model_id:
        model_id_or_path = cast(str, args.model_id)
    else:
        model_id_or_path = Path(Storage.download(args.model_dir))

    if model_id_or_path is None:
        raise ValueError("You must provide a model_id or model_dir")

    if args.backend == Backend.vllm and not vllm_available():
        raise RuntimeError("Backend is set to 'vllm' but vLLM is not available")

    if (
        (args.backend == Backend.vllm or args.backend == Backend.auto)
        and vllm_available()
        and infer_vllm_supported_from_model_architecture(model_id_or_path)
    ):
        from .vllm.vllm_model import VLLMModel

        args.model = args.model_dir or args.model_id
        args.revision = args.model_revision
        engine_args = build_vllm_engine_args(args)
        model = VLLMModel(args.model_name, engine_args)

    else:
        kwargs = vars(args)
        hf_dtype_map = {
            "float32": torch.float32,
            "float16": torch.float16,
            "bfloat16": torch.bfloat16,
            "half": torch.float16,
            "float": torch.float32,
        }

        model_config = AutoConfig.from_pretrained(
            str(model_id_or_path), revision=kwargs.get("model_revision", None)
        )
        if kwargs.get("task", None):
            try:
                task = MLTask[kwargs["task"]]
                if task not in SUPPORTED_TASKS:
                    raise ValueError(f"Task not supported: {task.name}")
            except (KeyError, ValueError):
                tasks_str = ", ".join(t.name for t in SUPPORTED_TASKS)
                raise ValueError(
                    f"Unsupported task: {kwargs['task']}. "
                    f"Currently supported tasks are: {tasks_str}"
                )
        else:
            task = infer_task_from_model_architecture(model_config)

        if is_generative_task(task):
            # Convert dtype from string to torch dtype. Default to float16
            dtype = kwargs.get("dtype", default_dtype)
            dtype = hf_dtype_map[dtype]
            logger.debug(f"Loading model in {dtype}")

            logger.info(f"Loading generative model for task '{task.name}' in {dtype}")

            model = HuggingfaceGenerativeModel(
                args.model_name,
                model_id_or_path=model_id_or_path,
                task=task,
                model_config=model_config,
                model_revision=kwargs.get("model_revision", None),
                tokenizer_revision=kwargs.get("tokenizer_revision", None),
                do_lower_case=not kwargs.get("disable_lower_case", False),
                max_length=kwargs["max_length"],
                dtype=dtype,
                trust_remote_code=kwargs["trust_remote_code"],
            )
        else:
            # Convert dtype from string to torch dtype. Default to float32
            dtype = kwargs.get("dtype", default_dtype)
            dtype = hf_dtype_map[dtype]

            predictor_config = PredictorConfig(
                args.predictor_host,
                args.predictor_protocol,
                args.predictor_use_ssl,
                args.predictor_request_timeout_seconds,
            )
            logger.info(f"Loading encoder model for task '{task.name}' in {dtype}")
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
                dtype=dtype,
                trust_remote_code=kwargs["trust_remote_code"],
                tensor_input_names=kwargs.get("tensor_input_names", None),
                return_token_type_ids=kwargs.get("return_token_type_ids", None),
                predictor_config=predictor_config,
            )
    model.load()
    return model


if __name__ == "__main__":
    try:
        model = load_model()
        kserve.ModelServer().start([model] if model.ready else [])
    except Exception as e:
        import sys

        logger.error(f"Failed to start model server: {e}")
        sys.exit(1)
