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

import asyncio
from argparse import Namespace
from http import HTTPStatus
from typing import Any, AsyncGenerator, Dict, List, Optional, Union

import torch
from fastapi import Request

from kserve.logging import logger
from kserve.protocol.rest.openai.errors import create_error_response
from kserve.protocol.rest.openai import (
    OpenAIEncoderModel,
    OpenAIGenerativeModel,
)
from kserve.protocol.rest.openai.types import (
    Completion,
    ChatCompletion,
    CompletionRequest,
    ChatCompletionRequest,
    EmbeddingRequest,
    Embedding,
    ErrorResponse,
)

from huggingfaceserver.request_logger import RequestLogger
from .utils import build_sglang_server_args

try:
    import sglang
    from sglang.srt.server_args import ServerArgs as SGLangServerArgs
    from sglang.srt.client import SRTClient
    from sglang.srt.openai_protocol import (
        OpenAIServingChat,
        OpenAIServingCompletion,
        OpenAIServingEmbedding,
    )
    _sglang = True
except ImportError:
    SGLangServerArgs = Any
    SRTClient = Any
    OpenAIServingChat = Any
    OpenAIServingCompletion = Any
    OpenAIServingEmbedding = Any
    _sglang = False


class SGLangModel(OpenAIEncoderModel, OpenAIGenerativeModel):
    """
    SGLang model implementation for KServe.
    """
    
    sglang_server_args: SGLangServerArgs = None
    args: Namespace = None
    ready: bool = False
    client: Optional[SRTClient] = None
    openai_serving_chat: Optional[OpenAIServingChat] = None
    openai_serving_completion: Optional[OpenAIServingCompletion] = None
    openai_serving_embedding: Optional[OpenAIServingEmbedding] = None
    
    def __init__(
        self,
        model_name: str,
        args: Namespace,
        request_logger: Optional[RequestLogger] = None,
    ):
        super().__init__(model_name)
        self.args = args
        self.sglang_server_args = build_sglang_server_args(args)
        self.request_logger = request_logger
        self.model_name = model_name
        
        # Initialize SGLang server and client
        self._initialize_sglang()
    
    def _initialize_sglang(self):
        """
        Initialize SGLang server and client.
        """
        if not _sglang:
            raise ImportError("SGLang is not available. Please install it with `pip install sglang`.")
        
        # In a real implementation, we would start the SGLang server here
        # For now, we'll just create a client that connects to a server
        # that would be started separately
        
        # The server would be started with something like:
        # import subprocess
        # cmd = ["python", "-m", "sglang.launch_server"]
        # cmd.extend(["--model-path", self.args.model_id or self.args.model_dir])
        # # Add other arguments as needed
        # self.server_process = subprocess.Popen(cmd)
        
        # For now, we'll just create a client that connects to a server
        # that would be started separately
        self.client = SRTClient(
            server_addr=f"http://localhost:30000",  # Default SGLang port
            model=self.model_name,
        )
        
        # Initialize OpenAI API handlers
        self.openai_serving_chat = OpenAIServingChat(self.client)
        self.openai_serving_completion = OpenAIServingCompletion(self.client)
        
        # Initialize embedding if needed
        if self.args.is_embedding:
            self.openai_serving_embedding = OpenAIServingEmbedding(self.client)
        
        self.ready = True
    
    def load(self) -> bool:
        """
        Load the model.
        """
        # The model is loaded when the server starts
        return True
    
    def start(self):
        """
        Start the model.
        """
        pass
    
    async def healthy(self) -> bool:
        """
        Check if the model is healthy.
        """
        # In a real implementation, we would check the health of the SGLang server
        return self.ready
    
    async def create_completion(
        self,
        request: CompletionRequest,
        raw_request: Optional[Request] = None,
        context: Optional[Dict[str, Any]] = None,
    ) -> Union[AsyncGenerator[str, None], Completion, ErrorResponse]:
        """
        Create a completion.
        """
        if self.openai_serving_completion is None:
            return create_error_response(
                message="The model does not support Completions API",
                status_code=HTTPStatus.BAD_REQUEST,
            )
        
        try:
            response = await self.openai_serving_completion.create_completion(request)
            return response
        except Exception as e:
            logger.error(f"Error in create_completion: {e}")
            return create_error_response(
                message=str(e),
                status_code=HTTPStatus.INTERNAL_SERVER_ERROR,
            )
    
    async def create_chat_completion(
        self,
        request: ChatCompletionRequest,
        raw_request: Optional[Request] = None,
        context: Optional[Dict[str, Any]] = None,
    ) -> Union[AsyncGenerator[str, None], ChatCompletion, ErrorResponse]:
        """
        Create a chat completion.
        """
        if self.openai_serving_chat is None:
            return create_error_response(
                message="The model does not support Chat Completions API",
                status_code=HTTPStatus.BAD_REQUEST,
            )
        
        try:
            response = await self.openai_serving_chat.create_chat_completion(request)
            return response
        except Exception as e:
            logger.error(f"Error in create_chat_completion: {e}")
            return create_error_response(
                message=str(e),
                status_code=HTTPStatus.INTERNAL_SERVER_ERROR,
            )
    
    async def create_embedding(
        self,
        request: EmbeddingRequest,
        raw_request: Optional[Request] = None,
        context: Optional[Dict[str, Any]] = None,
    ) -> Union[Embedding, ErrorResponse]:
        """
        Create an embedding.
        """
        if self.openai_serving_embedding is None:
            return create_error_response(
                message="The model does not support Embeddings API",
                status_code=HTTPStatus.BAD_REQUEST,
            )
        
        try:
            response = await self.openai_serving_embedding.create_embedding(request)
            return response
        except Exception as e:
            logger.error(f"Error in create_embedding: {e}")
            return create_error_response(
                message=str(e),
                status_code=HTTPStatus.INTERNAL_SERVER_ERROR,
            )
