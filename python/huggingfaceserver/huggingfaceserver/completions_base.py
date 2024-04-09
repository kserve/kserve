import asyncio
import json
from http import HTTPStatus
from typing import Dict, List, Optional, Union
from vllm.engine.async_llm_engine import AsyncLLMEngine
from kserve.protocol.rest.openai.types.openapi import (
    CreateCompletionRequest,
    ErrorResponse,
    Error,
    Logprobs,
)
from vllm.transformers_utils.tokenizer import get_tokenizer


class OpenAIServing:

    def __init__(self, engine: AsyncLLMEngine, served_model: str):
        self.engine = engine
        self.served_model = served_model

        self.max_model_len = 0
        self.tokenizer = None

        try:
            event_loop = asyncio.get_running_loop()
        except RuntimeError:
            event_loop = None

        if (
            event_loop is not None and event_loop.is_running()
        ):  # If the current is instanced by Ray Serve, there is already a running event loop
            event_loop.create_task(self._post_init())
        else:  # When using single vLLM without engine_use_ray
            loop = asyncio.new_event_loop()
            loop.run_until_complete(self._post_init())
            loop.close()

    async def _post_init(self):
        engine_model_config = await self.engine.get_model_config()
        self.max_model_len = engine_model_config.max_model_len

        # A separate tokenizer to map token IDs to strings.
        self.tokenizer = get_tokenizer(
            engine_model_config.tokenizer,
            tokenizer_mode=engine_model_config.tokenizer_mode,
            trust_remote_code=engine_model_config.trust_remote_code,
        )

    def create_error_response(
        self,
        message: str,
        err_type: str = "BadRequestError",
        status_code: HTTPStatus = HTTPStatus.BAD_REQUEST,
    ) -> ErrorResponse:

        error = Error(
            message=message, type=err_type, param="", code=str(status_code.value)
        )
        return ErrorResponse(error=error)

    def create_streaming_error_response(
        self,
        message: str,
        err_type: str = "BadRequestError",
        status_code: HTTPStatus = HTTPStatus.BAD_REQUEST,
    ) -> ErrorResponse:
        return self.create_error_response(
            message=message, err_type=err_type, status_code=status_code
        )

    async def _check_model(self, request) -> Optional[ErrorResponse]:
        if request.model == self.served_model:
            return
        return self.create_error_response(
            message=f"The model `{request.model}` does not exist.",
            err_type="NotFoundError",
            status_code=HTTPStatus.NOT_FOUND,
        )

    def _validate_prompt_and_tokenize(
        self,
        request: Union[CreateCompletionRequest],
        prompt: Optional[str] = None,
        prompt_ids: Optional[List[int]] = None,
    ) -> List[int]:
        if not (prompt or prompt_ids):
            raise ValueError("Either prompt or prompt_ids should be provided.")
        if prompt and prompt_ids:
            raise ValueError("Only one of prompt or prompt_ids should be provided.")

        input_ids = (
            prompt_ids if prompt_ids is not None else self.tokenizer(prompt).input_ids
        )
        token_num = len(input_ids)

        if request.max_tokens is None:
            request.max_tokens = self.max_model_len - token_num

        if token_num + request.max_tokens > self.max_model_len:
            raise ValueError(
                f"This model's maximum context length is "
                f"{self.max_model_len} tokens. However, you requested "
                f"{request.max_tokens + token_num} tokens "
                f"({token_num} in the messages, "
                f"{request.max_tokens} in the completion). "
                f"Please reduce the length of the messages or completion.",
            )
        else:
            return input_ids
