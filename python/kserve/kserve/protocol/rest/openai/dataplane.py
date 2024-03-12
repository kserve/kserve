from typing import AsyncIterator, Dict, Optional, Tuple, Union

from openai.types import Completion, CompletionCreateParams
from openai.types.chat import ChatCompletion, ChatCompletionChunk
from openai.types.chat import \
    CompletionCreateParams as ChatCompletionCreateParams

from ...dataplane import DataPlane
from .openai_model import OpenAIModel


class OpenAIDataPlane(DataPlane):
    """OpenAI DataPlane"""

    async def create_completion(
        self,
        model_name: str,
        request: CompletionCreateParams,
        headers: Optional[Dict[str, str]] = None,
    ) -> Union[Completion, AsyncIterator[Completion]]:
        """Generate the text with the provided text prompt.

        Args:
            model_name (str): Model name.
            request (bytes|GenerateRequest|ChatCompletionRequest): Generate Request / ChatCompletion Request body data.
            headers: (Optional[Dict[str, str]]): Request headers.

        Returns:
            response: The generated output or output stream.
            response_headers: Headers to construct the HTTP response.

        Raises:
            InvalidInput: An error when the body bytes can't be decoded as JSON.
        """
        model = self.get_model(model_name)
        if not isinstance(model, OpenAIModel):
            raise RuntimeError(f"Model {model_name} does not support completion")
        return await model.create_completion(request)

    async def create_chat_completion(
        self,
        model_name: str,
        request: ChatCompletionCreateParams,
        headers: Optional[Dict[str, str]] = None,
    ) -> Union[ChatCompletion, AsyncIterator[ChatCompletionChunk]]:
        """Generate the text with the provided text prompt.

        Args:
            model_name (str): Model name.
            request (bytes|GenerateRequest|ChatCompletionRequest): Generate Request / ChatCompletion Request body data.
            headers: (Optional[Dict[str, str]]): Request headers.

        Returns:
            response: The generated output or output stream.
            response_headers: Headers to construct the HTTP response.

        Raises:
            InvalidInput: An error when the body bytes can't be decoded as JSON.
        """
        model = self.get_model(model_name)
        if not isinstance(model, OpenAIModel):
            raise RuntimeError(f"Model {model_name} does not support completion")
        return await model.create_chat_completion(request)
