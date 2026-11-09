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
import sys
from typing import Any, AsyncIterator, Optional, Union
from pathlib import Path
from contextlib import asynccontextmanager

from kserve.logging import logger

try:
    from vllm.engine.arg_utils import AsyncEngineArgs
    from vllm.engine.protocol import EngineClient
    from vllm.entrypoints.openai.cli_args import make_arg_parser
    from vllm.model_executor.models import ModelRegistry
    from vllm.usage.usage_lib import UsageContext

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
    return make_arg_parser(parser)


def cli_flag_given(names, argv=None) -> bool:
    """Return True if any of the CLI flags in ``names`` appears in ``argv``.

    ``-`` and ``_`` are treated alike, as vLLM's parser accepts both spellings.
    """
    argv = sys.argv[1:] if argv is None else argv
    wanted = {name.replace("_", "-") for name in names}
    for arg in argv:
        flag = arg.split("=", 1)[0].replace("_", "-")
        if flag in wanted:
            return True
    return False


def resolve_tensor_parallel_size(
    tensor_parallel_size: int,
    pipeline_parallel_size: int,
    device_count: int,
    argv=None,
) -> int:
    """Keep an explicit --tensor-parallel-size; otherwise use the GPUs left per pipeline stage."""
    if cli_flag_given(("--tensor-parallel-size", "-tp"), argv):
        return tensor_parallel_size
    return max(1, device_count // max(1, pipeline_parallel_size))


def build_vllm_engine_args(args) -> "AsyncEngineArgs":
    if not _vllm:
        return None
    return AsyncEngineArgs.from_cli_args(args)


@asynccontextmanager
async def build_async_engine_client_from_engine_args(
    engine_args: AsyncEngineArgs,
) -> AsyncIterator[EngineClient]:
    """
    Create V1 AsyncLLM EngineClient.

    Returns the Client or None if the creation failed.
    """

    usage_context = UsageContext.OPENAI_API_SERVER
    vllm_config = engine_args.create_engine_config(usage_context=usage_context)

    from vllm.v1.engine.async_llm import AsyncLLM

    async_llm: Optional[AsyncLLM] = None
    try:
        async_llm = AsyncLLM.from_vllm_config(
            vllm_config=vllm_config,
            usage_context=usage_context,
            enable_log_requests=engine_args.enable_log_requests,
            disable_log_stats=engine_args.disable_log_stats,
        )
        yield async_llm
    finally:
        logger.info("V1 AsyncLLM build complete")
