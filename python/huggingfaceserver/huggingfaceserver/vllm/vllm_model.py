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

from typing import Any, AsyncIterator, Iterable, Optional, Union

import torch
from kserve import Model
from kserve.model import PredictorConfig
from kserve.protocol.rest.openai import (
    ChatCompletionRequestMessage,
    ChatPrompt,
    CompletionRequest,
    OpenAIChatAdapterModel,
)
from kserve.protocol.rest.openai.types import Completion
from vllm.engine.async_llm_engine import AsyncLLMEngine

from .vllm_completions import OpenAIServingCompletion


class VLLMModel(Model, OpenAIChatAdapterModel):  # pylint:disable=c-extension-no-member
    vllm_engine: AsyncLLMEngine
    vllm_engine_args: Any = None
    ready: bool = False

    def __init__(
        self,
        model_name: str,
        engine_args=None,
        predictor_config: Optional[PredictorConfig] = None,
    ):
        super().__init__(model_name, predictor_config)
        self.vllm_engine_args = engine_args

    def load(self) -> bool:
        if torch.cuda.is_available():
            self.vllm_engine_args.tensor_parallel_size = torch.cuda.device_count()
        self.vllm_engine = AsyncLLMEngine.from_engine_args(self.vllm_engine_args)
        self.openai_serving_completion = OpenAIServingCompletion(self.vllm_engine)
        self.ready = True
        return self.ready

    def apply_chat_template(
        self,
        messages: Iterable[ChatCompletionRequestMessage,],
    ) -> ChatPrompt:
        """
        Given a list of chat completion messages, convert them to a prompt.
        """
        return ChatPrompt(
            prompt=self.openai_serving_completion.apply_chat_template(messages)
        )

    async def create_completion(
        self, request: CompletionRequest
    ) -> Union[Completion, AsyncIterator[Completion]]:
        return await self.openai_serving_completion.create_completion(request)
