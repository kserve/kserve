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
from typing import Any, AsyncIterator, Optional, Union
from pathlib import Path
from contextlib import asynccontextmanager

from kserve.logging import logger

try:
    import vllm.envs as envs
    from vllm.engine.arg_utils import AsyncEngineArgs
    from vllm.engine.async_llm_engine import AsyncLLMEngine
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


def build_vllm_engine_args(args) -> "AsyncEngineArgs":
    if not _vllm:
        return None
    return AsyncEngineArgs.from_cli_args(args)


@asynccontextmanager
async def build_async_engine_client_from_engine_args(
    engine_args: AsyncEngineArgs,
    disable_frontend_multiprocessing: bool = False,
) -> AsyncIterator[EngineClient]:
    """
    Create EngineClient, either:
        - V1 AsyncLLM (default)
        - V0 AsyncLLMEngine (legacy)

    Returns the Client or None if the creation failed.
    """

    # Create the EngineConfig (determines if we can use V1).
    usage_context = UsageContext.OPENAI_API_SERVER
    vllm_config = engine_args.create_engine_config(usage_context=usage_context)

    # V1 AsyncLLM.
    if envs.VLLM_USE_V1:
        if disable_frontend_multiprocessing:
            logger.warning(
                "V1 is enabled, but got --disable-frontend-multiprocessing. "
                "To disable frontend multiprocessing, set VLLM_USE_V1=0."
            )

        from vllm.v1.engine.async_llm import AsyncLLM

        async_llm: Optional[AsyncLLM] = None
        try:
            async_llm = AsyncLLM.from_vllm_config(
                vllm_config=vllm_config,
                usage_context=usage_context,
                disable_log_requests=engine_args.disable_log_requests,
                disable_log_stats=engine_args.disable_log_stats,
            )
            yield async_llm
        finally:
            logger.info("V1 AsyncLLM build complete")

    # V0 AsyncLLMEngine.
    else:
        engine_client: Optional[EngineClient] = None
        try:
            engine_client = AsyncLLMEngine.from_vllm_config(
                vllm_config=vllm_config,
                usage_context=usage_context,
                disable_log_requests=engine_args.disable_log_requests,
                disable_log_stats=engine_args.disable_log_stats,
            )
            yield engine_client
        finally:
            logger.info("V0 AsyncLLMEngine build complete")
