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

from argparse import ArgumentParser
from typing import Any, Union
from pathlib import Path

from kserve.logging import logger

try:
    from vllm.engine.arg_utils import AsyncEngineArgs, nullable_str
    from vllm.model_executor.models import ModelRegistry

    _vllm = True
except ImportError:
    AsyncEngineArgs = Any
    _vllm = False

from transformers import AutoConfig


def vllm_available() -> bool:
    return _vllm


def infer_vllm_supported_from_model_architecture(
    model_config_path: Union[Path, str],
    trust_remote_code: bool = False,
) -> bool:
    if not _vllm:
        return False

    model_config = AutoConfig.from_pretrained(
        model_config_path, trust_remote_code=trust_remote_code
    )
    for architecture in model_config.architectures:
        if architecture not in ModelRegistry.get_supported_archs():
            logger.info("not a supported model by vLLM")
            return False
    return True


def maybe_add_vllm_cli_parser(parser: ArgumentParser) -> ArgumentParser:
    if not _vllm:
        return parser

    parser.add_argument(
        "--enable-auto-tool-choice",
        action="store_true",
        default=False,
        help="Enable auto tool choice for supported models. Use --tool-call-parser"
        "to specify which parser to use",
    )

    parser.add_argument(
        "--tool-call-parser",
        type=str,
        default=None,
        help="Select the tool call parser depending on the model that you're using."
        " This is used to parse the model-generated tool call into OpenAI API "
        "format. Required for --enable-auto-tool-choice.",
    )

    parser.add_argument(
        "--tool-parser-plugin",
        type=str,
        default="",
        help="Special the tool parser plugin write to parse the model-generated tool"
        " into OpenAI API format, the name register in this plugin can be used "
        "in --tool-call-parser.",
    )

    parser.add_argument(
        "--chat-template",
        type=nullable_str,
        default=None,
        help="The file path to the chat template, "
        "or the template in single-line form "
        "for the specified model",
    )
    return AsyncEngineArgs.add_cli_args(parser)


def build_vllm_engine_args(args) -> "AsyncEngineArgs":
    if not _vllm:
        return None
    return AsyncEngineArgs.from_cli_args(args)
