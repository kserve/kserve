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

from typing import Any, Dict, Optional, Union, AsyncGenerator
import torch
from argparse import Namespace
from fastapi import Request
from http import HTTPStatus

from kserve.errors import ModelNotReady
from kserve.protocol.rest.openai.errors import create_error_response
from kserve.protocol.rest.openai import OpenAIEncoderModel, OpenAIGenerativeModel
from kserve.protocol.rest.openai.types import (
    Completion,
    ChatCompletion,
    CompletionRequest,
    ChatCompletionRequest,
    EmbeddingRequest,
    Embedding,
    ErrorResponse,
)

from vllm import AsyncEngineArgs
from vllm.entrypoints.logger import RequestLogger
from vllm.engine.protocol import EngineClient
from vllm.entrypoints.openai.serving_completion import OpenAIServingCompletion
from vllm.entrypoints.openai.serving_chat import OpenAIServingChat
from vllm.entrypoints.openai.serving_embedding import OpenAIServingEmbedding
from vllm.entrypoints.openai.tool_parsers import ToolParserManager
from vllm.entrypoints.openai.serving_models import BaseModelPath, OpenAIServingModels
from vllm.entrypoints.openai.api_server import (
    build_async_engine_client_from_engine_args,
)
from vllm.entrypoints.openai.cli_args import validate_parsed_serve_args
from vllm.entrypoints.chat_utils import load_chat_template
from vllm.entrypoints.openai.protocol import ErrorResponse as engineError
from .utils import build_vllm_engine_args


class VLLMModel(
    OpenAIEncoderModel, OpenAIGenerativeModel
):  # pylint:disable=c-extension-no-member
    engine_client: EngineClient
    vllm_engine_args: AsyncEngineArgs = None
    args: Namespace = None
    ready: bool = False

    def __init__(
        self,
        model_name: str,
        args: Namespace,
        request_logger: Optional[RequestLogger] = None,
    ):
        super().__init__(model_name)
        self.args = args
        validate_parsed_serve_args(args)
        engine_args = build_vllm_engine_args(args)
        self.vllm_engine_args = engine_args
        self.request_logger = request_logger
        self.model_name = model_name

    async def start_engine(self):
        if self.args.tool_parser_plugin and len(self.args.tool_parser_plugin) > 3:
            ToolParserManager.import_tool_parser(self.args.tool_parser_plugin)

        valide_tool_parses = ToolParserManager.tool_parsers.keys()
        if (
            self.args.enable_auto_tool_choice
            and self.args.tool_call_parser not in valide_tool_parses
        ):
            raise KeyError(
                f"invalid tool call parser: {self.args.tool_call_parser} "
                f"(chose from {{ {','.join(valide_tool_parses)} }})"
            )

        if torch.cuda.is_available():
            self.vllm_engine_args.tensor_parallel_size = torch.cuda.device_count()

        async with build_async_engine_client_from_engine_args(
            self.vllm_engine_args, disable_frontend_multiprocessing=True
        ) as engine_client:
            self.engine_client = engine_client
            if self.args.served_model_name is not None:
                served_model_names = self.args.served_model_name
            else:
                served_model_names = [self.model_name]

            self.base_model_paths = [
                BaseModelPath(name=name, model_path=self.args.model)
                for name in served_model_names
            ]

            self.log_stats = not self.args.disable_log_stats
            self.model_config = await engine_client.get_model_config()

            resolved_chat_template = load_chat_template(self.args.chat_template)

            self.openai_serving_models = OpenAIServingModels(
                engine_client=self.engine_client,
                model_config=self.model_config,
                base_model_paths=self.base_model_paths,
                lora_modules=self.args.lora_modules,
                prompt_adapters=self.args.prompt_adapters,
            )

            self.openai_serving_chat = (
                OpenAIServingChat(
                    self.engine_client,
                    self.model_config,
                    self.openai_serving_models,
                    self.args.response_role,
                    request_logger=self.request_logger,
                    chat_template=resolved_chat_template,
                    chat_template_content_format=self.args.chat_template_content_format,
                    return_tokens_as_token_ids=self.args.return_tokens_as_token_ids,
                    enable_auto_tools=self.args.enable_auto_tool_choice,
                    tool_parser=self.args.tool_call_parser,
                    enable_reasoning=self.args.enable_reasoning,
                    reasoning_parser=self.args.reasoning_parser,
                    enable_prompt_tokens_details=self.args.enable_prompt_tokens_details,
                )
                if self.model_config.runner_type == "generate"
                else None
            )

            self.openai_serving_completion = (
                OpenAIServingCompletion(
                    self.engine_client,
                    self.model_config,
                    self.openai_serving_models,
                    request_logger=self.request_logger,
                    return_tokens_as_token_ids=self.args.return_tokens_as_token_ids,
                )
                if self.model_config.runner_type == "generate"
                else None
            )

            self.openai_serving_embedding = (
                OpenAIServingEmbedding(
                    self.engine_client,
                    self.model_config,
                    self.openai_serving_models,
                    request_logger=self.request_logger,
                    chat_template=resolved_chat_template,
                    chat_template_content_format=self.args.chat_template_content_format,
                )
                if self.model_config.task == "embed"
                else None
            )

        self.ready = True
        return self.ready

    def load(self) -> bool:
        self.engine = True
        return False

    def start(self):
        pass

    def stop_engine(self):
        if hasattr(self.engine_client, "shutdown"):
            self.engine_client.shutdown()
        self.ready = False

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
        context: Optional[Dict[str, Any]] = None,
    ) -> Union[AsyncGenerator[str, None], Completion, ErrorResponse]:
        response = await self.openai_serving_completion.create_completion(
            request, raw_request
        )

        if isinstance(response, engineError):
            return create_error_response(
                message=response.message,
                err_type=response.type,
                param=response.param,
                status_code=HTTPStatus(response.code),
            )

        return response

    async def create_chat_completion(
        self,
        request: ChatCompletionRequest,
        raw_request: Optional[Request] = None,
        context: Optional[Dict[str, Any]] = None,
    ) -> Union[AsyncGenerator[str, None], ChatCompletion, ErrorResponse]:
        response = await self.openai_serving_chat.create_chat_completion(
            request, raw_request
        )

        if isinstance(response, engineError):
            return create_error_response(
                message=response.message,
                err_type=response.type,
                param=response.param,
                status_code=HTTPStatus(response.code),
            )

        return response

    async def create_embedding(
        self,
        request: EmbeddingRequest,
        raw_request: Optional[Request] = None,
        context: Optional[Dict[str, Any]] = None,
    ) -> Union[AsyncGenerator[str, None], Embedding, ErrorResponse]:
        response = await self.openai_serving_embedding.create_embedding(
            request, raw_request
        )

        if isinstance(response, engineError):
            return create_error_response(
                message=response.message,
                err_type=response.type,
                param=response.param,
                status_code=HTTPStatus(response.code),
            )

        return response
