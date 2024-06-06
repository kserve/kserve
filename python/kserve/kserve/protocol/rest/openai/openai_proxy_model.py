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

from typing import AsyncIterator, Optional, Union
import httpx
from http import HTTPStatus
from functools import partial, wraps
import orjson

from pydantic import ValidationError


from .openai_model import (
    BaseCompletionRequest,
    OpenAIModel,
    AsyncMappingIterator,
    CompletionRequest,
    ChatCompletionRequest,
)
from .types import (
    ChatCompletion,
    ChatCompletionChunk,
    Completion,
    ErrorResponse,
)
from .errors import OpenAIError, create_error_response
from ....logging import logger


COMPLETIONS_ENDPOINT = "/v1/completions"
CHAT_COMPLETIONS_ENDPOINT = "/v1/chat/completions"


def error_handler(f):
    @wraps(f)
    async def wrapper(*args, **kwargs):
        try:
            res = await f(*args, **kwargs)
            return res
        except httpx.HTTPStatusError as e:
            try:
                # Try to parse upstream error as an ErrorResponse object
                response = ErrorResponse.model_validate_json(e.response.content)
            except ValidationError:
                logger.warning(
                    f"Failed to parse error response from upstream: {e.response.content}"
                )
                response = create_error_response(
                    f"Received invalid response from upstream: {e.response.text}",
                    status_code=HTTPStatus.BAD_GATEWAY,
                    err_type="BadGateway",
                )
            raise OpenAIError(response=response)

        except httpx.TimeoutException as e:
            raise OpenAIError(
                response=create_error_response(
                    f"Timed out when communicating with upstream: {e}",
                    err_type="GatewayTimeout",
                    status_code=HTTPStatus.GATEWAY_TIMEOUT,
                )
            )
        except httpx.NetworkError as e:
            raise OpenAIError(
                response=create_error_response(
                    f"Failed to communicate with upstream: {e}",
                    err_type="ServiceUnavailableError",
                    status_code=HTTPStatus.SERVICE_UNAVAILABLE,
                )
            )
        except httpx.HTTPError as e:
            raise OpenAIError(
                response=create_error_response(
                    f"Upstream request failed: {e}",
                    err_type="InternalServerError",
                    status_code=HTTPStatus.INTERNAL_SERVER_ERROR,
                )
            )

    return wrapper


class OpenAIProxyModel(OpenAIModel):
    """
    An implementation of OpenAIModel that proxies requests to a backend server exposing Open AI endpoints.

    Users can extend this class and override any of the following methods to hook into the request/response cycle:
        - preprocess_completion_request
        - postprocess_completion
        - postprocess_completion_chunk
        - preprocess_chat_completion_request
        - postprocess_chat_completion
        - postprocess_chat_completion_chunk

    Each method takes a single parameter: the object currently being processed. Request objects may be mutated to modify
    the request sent to the downstream server and response objects (completions/completion chunks) may be mutated to
    modify the response returned to the upstream caller.

    Args:
        predictor_url (str):
            The url of the model server to send requests to. Should be fully qualified with scheme, host, and port.
            e.g. `http://my-backend:9000`
        http_client (httpx.AsyncClient|None):
            An optional instance of httpx.AsyncClient to use for sending requests to the upstream server.
    """

    predictor_url: str
    skip_upstream_validation: bool
    _http_client: httpx.AsyncClient
    _completions_endpoint: str
    _chat_completions_endpoint: str

    def __init__(
        self,
        name: str,
        predictor_url: str,
        http_client: Optional[httpx.AsyncClient] = None,
        skip_upstream_validation: bool = False,
    ):
        super().__init__(name)
        self.predictor_url = predictor_url
        self._http_client = (
            http_client
            if http_client is not None
            else httpx.AsyncClient(timeout=httpx.Timeout(timeout=5.0, read=30.0))
        )
        self._completions_endpoint = (
            f"{self.predictor_url.rstrip('/')}{COMPLETIONS_ENDPOINT}"
        )
        self._chat_completions_endpoint = (
            f"{self.predictor_url.rstrip('/')}{CHAT_COMPLETIONS_ENDPOINT}"
        )
        self.skip_upstream_validation = skip_upstream_validation
        self.ready = True

    def preprocess_completion_request(self, request: CompletionRequest):
        """Preprocess a completion request."""
        pass

    def postprocess_completion(
        self, completion: Completion, request: CompletionRequest
    ):
        """Postprocess a completion. Only called when response is not being streamed (i.e. stream=false)"""
        pass

    def postprocess_completion_chunk(
        self, completion: Completion, request: CompletionRequest
    ):
        """Postprocess a completion chunk. Only called when response is being streamed (i.e. stream=true)
        This method will be called once for each chunk that is streamed back to the user.
        """
        pass

    def preprocess_chat_completion_request(self, request: ChatCompletionRequest):
        """Preprocess a chat completion request."""
        pass

    def postprocess_chat_completion(
        self, chat_completion: ChatCompletion, request: ChatCompletionRequest
    ):
        """Postprocess a chat completion. Only called when response is not being streamed (i.e. stream=false)"""
        pass

    def postprocess_chat_completion_chunk(
        self, chat_completion_chunk: ChatCompletionChunk, request: ChatCompletionRequest
    ):
        """Postprocess a chat completion chunk. Only called when response is being streamed (i.e. stream=true)
        This method will be called once for each chunk that is streamed back to the user.
        """
        pass

    def _handle_completion_chunk(self, raw_chunk: str, request: CompletionRequest):
        # Skip empty lines
        if len(raw_chunk) == 0:
            return None
        # All chunks are prefixed with "data: "
        data = raw_chunk[6:]
        if data == "[DONE]":
            return None

        if self.skip_upstream_validation:
            obj = orjson.loads(data)
            completion_chunk = Completion.model_construct(**obj)
        else:
            completion_chunk = Completion.model_validate_json(data)
        self.postprocess_completion_chunk(completion_chunk, request)
        return completion_chunk

    def _handle_chat_completion_chunk(
        self, raw_chunk: str, request: ChatCompletionRequest
    ):
        # Skip empty lines
        if len(raw_chunk) == 0:
            return None
        # All chunks are prefixed with "data: "
        if len(raw_chunk) == 0:
            return None
        data = raw_chunk[6:]
        if data == "[DONE]":
            return None
        if self.skip_upstream_validation:
            obj = orjson.loads(data)
            chat_completion_chunk = ChatCompletionChunk.model_construct(**obj)
        else:
            chat_completion_chunk = ChatCompletionChunk.model_validate_json(data)
        self.postprocess_chat_completion_chunk(chat_completion_chunk, request)
        return chat_completion_chunk

    def _build_request(
        self, endpoint: str, request: BaseCompletionRequest
    ) -> httpx.Request:

        if request.context and "upstream_headers" in request.context:
            headers = httpx.Headers(request.context["upstream_headers"])
        else:
            headers = httpx.Headers()

        headers["Content-type"] = "application/json"

        req = self._http_client.build_request(
            "POST",
            endpoint,
            content=request.params.model_dump_json(
                exclude_unset=True, exclude_none=True
            ),
            headers=headers,
        )
        return req

    @error_handler
    async def create_completion(
        self, request: CompletionRequest
    ) -> Union[Completion, AsyncIterator[Completion]]:
        self.preprocess_completion_request(request)
        if request.params.stream:
            req = self._build_request(self._completions_endpoint, request)
            r = await self._http_client.send(req, stream=True)
            r.raise_for_status()
            it = AsyncMappingIterator(
                iterator=r.aiter_lines(),
                mapper=partial(self._handle_completion_chunk, request=request),
                close=r.aclose,
            )
            return it
        else:
            completion = await self.generate_completion(request)
            self.postprocess_completion(completion, request)
            return completion

    async def generate_completion(self, request: CompletionRequest) -> Completion:
        req = self._build_request(self._completions_endpoint, request)
        response = await self._http_client.send(req)
        response.raise_for_status()
        if self.skip_upstream_validation:
            obj = response.json()
            completion = Completion.model_construct(**obj)
        else:
            completion = Completion.model_validate_json(response.content)
        return completion

    @error_handler
    async def create_chat_completion(
        self, request: ChatCompletionRequest
    ) -> Union[ChatCompletion, AsyncIterator[ChatCompletionChunk]]:
        self.preprocess_chat_completion_request(request)
        if request.params.stream:
            req = self._build_request(self._chat_completions_endpoint, request)
            r = await self._http_client.send(req, stream=True)
            r.raise_for_status()
            it = AsyncMappingIterator(
                iterator=r.aiter_lines(),
                mapper=partial(self._handle_chat_completion_chunk, request=request),
                close=r.aclose,
            )
            return it
        else:
            chat_completion = await self.generate_chat_completion(request)
            self.postprocess_chat_completion(chat_completion, request)
            return chat_completion

    async def generate_chat_completion(self, request: ChatCompletionRequest) -> ChatCompletion:
        req = self._build_request(self._chat_completions_endpoint, request)
        response = await self._http_client.send(req)
        response.raise_for_status()
        if self.skip_upstream_validation:
            obj = response.json()
            chat_completion = ChatCompletion.model_construct(**obj)
        else:
            chat_completion = ChatCompletion.model_validate_json(response.content)
        return chat_completion
