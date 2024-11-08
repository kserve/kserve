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

from typing import Optional, Union, AsyncGenerator
import asyncio
import torch
from argparse import Namespace
from fastapi import Request  # TODO: Double check if it's installed here

from kserve import Model
from kserve.errors import ModelNotReady
from kserve.model import PredictorConfig
from kserve.protocol.rest.openai import OpenAIModel
from kserve.protocol.rest.openai.types import (
    Completion,
    ChatCompletion,
    CompletionRequest,
)

from vllm import AsyncEngineArgs
from vllm.entrypoints.logger import RequestLogger
from vllm.engine.protocol import EngineClient
from vllm.entrypoints.openai.cli_args import validate_parsed_serve_args
from vllm.entrypoints.openai.serving_completion import OpenAIServingCompletion
from vllm.entrypoints.openai.serving_chat import OpenAIServingChat
from vllm.entrypoints.openai.serving_embedding import OpenAIServingEmbedding
from vllm.entrypoints.openai.serving_tokenization import OpenAIServingTokenization
from vllm.entrypoints.openai.protocol import ErrorResponse
from vllm.entrypoints.openai.serving_engine import BaseModelPath
from vllm.entrypoints.openai.api_server import build_async_engine_client

from .utils import build_vllm_engine_args


class VLLMModel(Model, OpenAIModel):  # pylint:disable=c-extension-no-member
    engine_client: EngineClient
    vllm_engine_args: AsyncEngineArgs = None
    args: Namespace = None
    ready: bool = False  # TODO: check members here

    def __init__(
        self,
        args: Namespace,
        predictor_config: Optional[PredictorConfig] = None,
        request_logger: Optional[RequestLogger] = None,
    ):
        validate_parsed_serve_args(args)
        super().__init__(
            args.model_name, predictor_config
        )  # TODO: where is model_name?
        self.args = args
        engine_args = build_vllm_engine_args(args)  # TODO: should be enough
        self.vllm_engine_args = engine_args
        self.request_logger = request_logger

    async def setup_engine(self):
        if self.args.served_model_name is not None:
            served_model_names = self.args.served_model_name
        else:
            served_model_names = [self.args.model]

        base_model_paths = [
            BaseModelPath(name=name, model_path=self.args.model)
            for name in served_model_names
        ]
        async with build_async_engine_client(self.vllm_engine_args) as engine_client:
            self.engine_client = engine_client
            self.log_stats = not self.vllm_engine_args.disable_log_stats
            self.model_config = await engine_client.get_model_config()

            self.openai_serving_chat = OpenAIServingChat(
                self.engine_client,
                self.model_config,
                base_model_paths,
                self.args.response_role,
                lora_modules=self.args.lora_modules,
                prompt_adapters=self.args.prompt_adapters,
                request_logger=self.request_logger,
                chat_template=self.args.chat_template,
                return_tokens_as_token_ids=self.args.return_tokens_as_token_ids,
                enable_auto_tools=self.args.enable_auto_tool_choice,
                tool_parser=self.args.tool_call_parser,
            )

            self.openai_serving_completion = OpenAIServingCompletion(
                self.engine_client,
                self.model_config,
                base_model_paths,
                lora_modules=self.args.lora_modules,
                prompt_adapters=self.args.prompt_adapters,
                request_logger=self.request_logger,
                return_tokens_as_token_ids=self.args.return_tokens_as_token_ids,
            )
            # TODO: how to use embedding and tokenization
            self.openai_serving_embedding = OpenAIServingEmbedding(
                self.engine_client,
                self.model_config,
                base_model_paths,
                request_logger=self.request_logger,
            )
            self.openai_serving_tokenization = OpenAIServingTokenization(
                self.engine_client,
                self.model_config,
                base_model_paths,
                lora_modules=self.args.lora_modules,
                request_logger=self.request_logger,
                chat_template=self.args.chat_template,
            )

    def load(self) -> bool:
        if torch.cuda.is_available():
            self.vllm_engine_args.tensor_parallel_size = torch.cuda.device_count()
        asyncio.run(self.setup_engine())
        self.ready = True
        return self.ready

    async def healthy(self) -> bool:
        try:
            await self.engine_client.check_health()
        except Exception as e:
            raise ModelNotReady(self.name) from e
        return True

    async def create_completion(
        self,
        request: CompletionRequest,
        raw_request: Optional[Request] = None,
    ) -> Union[AsyncGenerator[str, None], Completion, ErrorResponse]:
        return await self.openai_serving_completion.create_completion(
            request, raw_request
        )

    async def create_chat_completion(
        self,
        request: CompletionRequest,
        raw_request: Optional[Request] = None,
    ) -> Union[AsyncGenerator[str, None], ChatCompletion, ErrorResponse]:
        return await self.openai_serving_chat.create_chat_completion(
            request, raw_request
        )
