import time
import codecs
from typing import Iterable, Dict, Any, Union, AsyncIterator
from openai.types.chat import (
    ChatCompletion, CompletionCreateParams as ChatCompletionCreateParams,
    ChatCompletionMessageParam, ChatCompletionChunk)
from openai.types import Completion, CompletionCreateParams
from abc import ABC, abstractmethod
from typing import AsyncGenerator, AsyncIterator, Optional, Union
from fastapi.requests import Request
from fastapi.responses import Response
from starlette.responses import StreamingResponse
from vllm.utils import random_uuid
from kserve.utils.utils import generate_uuid

from .openai_datamodels import (
    ChatCompletionRequest, ChatCompletionResponse,
    ChatCompletionStreamResponse, ChatCompletionResponseChoice,
    ChatCompletionResponseStreamChoice, ErrorResponse,
    ChatMessage, DeltaMessage, UsageInfo)
from v2_datamodels import GenerateRequest, GenerateResponse, Parameters
from ..dataplane import DataPlane
from ..model_repository_extension import ModelRepositoryExtension


class BaseOpenAIModel():
    @abstractmethod
    async def create_completion(self, params: CompletionCreateParams) -> Completion:
        pass

    @abstractmethod
    async def create_chat_completion(self, params: ChatCompletionCreateParams) -> ChatCompletion:
        pass

class OpenAIModel(BaseOpenAIModel):
    @abstractmethod
    async def apply_chat_template(self, messages: Iterable[ChatCompletionMessageParam]) -> str:
        pass

    @abstractmethod
    async def generate(self, payload: GenerateRequest,
                       headers: Dict[str, str] = None) -> Union[GenerateResponse, AsyncIterator[Any]]:
        pass

    async def create_completion(self, params: CompletionCreateParams) -> Completion:
        # TODO: create a prompt and form a GenerateRequest
         self.apply_chat_template()

        # generate_request = translate_request(params)
        # self.generate(generate_request)

    async def create_chat_completion(self, params: ChatCompletionCreateParams) -> Union[ChatCompletion, AsyncIterator[ChatCompletionChunk]]:
        prompt = self.apply_chat_template(params.messages)
        sampling_params = to_sampling_params(params)
        response = self.generate(GenerateRequest(text_input=prompt, parameters=sampling_params))
        if not params.stream:
            ChatCompletion(
                id=generate_uuid(),
            )


def to_sampling_params(params: Union[ChatCompletionCreateParams, CompletionCreateParams]) -> Parameters:
    return dict(
        n=params.n,
        presence_penalty=params.presence_penalty,
        frequency_penalty=params.frequency_penalty,
        repetition_penalty=params.repetition_penalty,
        temperature=params.temperature,
        top_p=params.top_p,
        min_p=params.min_p,
        seed=params.seed,
        stop=params.stop,
        stop_token_ids=params.stop_token_ids,
        max_tokens=params.max_tokens,
        best_of=params.best_of,
        top_k=params.top_k,
        ignore_eos=params.ignore_eos,
        use_beam_search=params.use_beam_search,
        early_stopping=params.early_stopping,
        skip_special_tokens=params.skip_special_tokens,
        spaces_between_special_tokens=params.spaces_between_special_tokens,
        include_stop_str_in_output=params.include_stop_str_in_output,
        length_penalty=params.length_penalty,
    )

class OpenAIEndpoints:
    """
    KServe OpenAPI Endpoints
    """
    def __init__(self, dataplane: DataPlane, model_repository_extension: Optional[ModelRepositoryExtension] = None):
        self.model_repository_extension = model_repository_extension
        self.dataplane = dataplane
    
    async def _create_generator(
        self, request: ChatCompletionRequest, raw_request: Request
    ) -> Union[ErrorResponse, AsyncGenerator[str, None],
               ChatCompletionResponse]:
        """This API mimics vLLM's Completion API, which mimics OpenAI's ChatCompletion API.
        See https://platform.openai.com/docs/api-reference/chat/create
        for the API specification.

        NOTE: Currently we do not support the following features:
            - function_call (Users should implement this by themselves)
            - logit_bias (to be supported by vLLM engine)
        """
        error_check_ret = await self._check_model(request)
        if error_check_ret is not None:
            return error_check_ret

        if request.logit_bias is not None and len(request.logit_bias) > 0:
            # TODO: support logit_bias in vLLM engine.
            return self.create_error_response(
                "logit_bias is not currently supported")

        # TODO: figure out where to pass tokenizer
        # try:
        prompt = self.tokenizer.apply_chat_template(
            conversation=request.messages,
            tokenize=False,
            add_generation_prompt=request.add_generation_prompt)
        # except Exception as e:
        #     logger.error(
        #         f"Error in applying chat template from request: {str(e)}")
        #     return self.create_error_response(str(e))

        request_id = f"cmpl-{random_uuid()}"
        try:
            token_ids = self._validate_prompt_and_tokenize(request,
                                                           prompt=prompt)
            sampling_params = request.to_sampling_params()
            lora_request = self._maybe_get_lora(request)
        except ValueError as e:
            return self.create_error_response(str(e))

        result_generator = self.engine.generate(prompt, sampling_params,
                                                request_id, token_ids,
                                                lora_request)
        # Streaming response
        if request.stream:
            return self.chat_completion_stream_generator(
                request, result_generator, request_id)
        else:
            return await self.chat_completion_full_generator(
                request, raw_request, result_generator, request_id)

    def get_chat_request_role(self, request: ChatCompletionRequest) -> str:
        if request.add_generation_prompt:
            return self.response_role
        else:
            return request.messages[-1].role

    async def _chat_completion_stream_generator(
            self, request: ChatCompletionRequest,
            result_generator: AsyncIterator[RequestOutput], request_id: str
    ) -> Union[ErrorResponse, AsyncGenerator[str, None]]:

        model_name = request.model
        created_time = int(time.monotonic())
        chunk_object_type = "chat.completion.chunk"

        # Send first response for each request.n (index) with the role
        role = self.get_chat_request_role(request)
        for i in range(request.n):
            choice_data = ChatCompletionResponseStreamChoice(
                index=i, delta=DeltaMessage(role=role), finish_reason=None)
            chunk = ChatCompletionStreamResponse(id=request_id,
                                                 object=chunk_object_type,
                                                 created=created_time,
                                                 choices=[choice_data],
                                                 model=model_name)
            data = chunk.model_dump_json(exclude_unset=True)
            yield f"data: {data}\n\n"

        # Send response to echo the input portion of the last message
        if request.echo:
            last_msg_content = ""
            if request.messages and isinstance(
                    request.messages, list) and request.messages[-1].get(
                        "content") and request.messages[-1].get(
                            "role") == role:
                last_msg_content = request.messages[-1]["content"]
            if last_msg_content:
                for i in range(request.n):
                    choice_data = ChatCompletionResponseStreamChoice(
                        index=i,
                        delta=DeltaMessage(content=last_msg_content),
                        finish_reason=None)
                    chunk = ChatCompletionStreamResponse(
                        id=request_id,
                        object=chunk_object_type,
                        created=created_time,
                        choices=[choice_data],
                        model=model_name)
                    data = chunk.model_dump_json(exclude_unset=True)
                    yield f"data: {data}\n\n"

        # Send response for each token for each request.n (index)
        previous_texts = [""] * request.n
        previous_num_tokens = [0] * request.n
        finish_reason_sent = [False] * request.n
        async for res in result_generator:
            res: RequestOutput
            for output in res.outputs:
                i = output.index

                if finish_reason_sent[i]:
                    continue

                delta_text = output.text[len(previous_texts[i]):]
                previous_texts[i] = output.text
                previous_num_tokens[i] = len(output.token_ids)

                if output.finish_reason is None:
                    # Send token-by-token response for each request.n
                    choice_data = ChatCompletionResponseStreamChoice(
                        index=i,
                        delta=DeltaMessage(content=delta_text),
                        finish_reason=None)
                    chunk = ChatCompletionStreamResponse(
                        id=request_id,
                        object=chunk_object_type,
                        created=created_time,
                        choices=[choice_data],
                        model=model_name)
                    data = chunk.model_dump_json(exclude_unset=True)
                    yield f"data: {data}\n\n"
                else:
                    # Send the finish response for each request.n only once
                    prompt_tokens = len(res.prompt_token_ids)
                    final_usage = UsageInfo(
                        prompt_tokens=prompt_tokens,
                        completion_tokens=previous_num_tokens[i],
                        total_tokens=prompt_tokens + previous_num_tokens[i],
                    )
                    choice_data = ChatCompletionResponseStreamChoice(
                        index=i,
                        delta=DeltaMessage(content=delta_text),
                        finish_reason=output.finish_reason)
                    chunk = ChatCompletionStreamResponse(
                        id=request_id,
                        object=chunk_object_type,
                        created=created_time,
                        choices=[choice_data],
                        model=model_name)
                    if final_usage is not None:
                        chunk.usage = final_usage
                    data = chunk.model_dump_json(exclude_unset=True,
                                                 exclude_none=True)
                    yield f"data: {data}\n\n"
                    finish_reason_sent[i] = True
        # Send the final done message after all response.n are finished
        yield "data: [DONE]\n\n"

    async def _chat_completion_full_generator(
            self, request: ChatCompletionRequest, raw_request: Request,
            result_generator: AsyncIterator[RequestOutput],
            request_id: str) -> Union[ErrorResponse, ChatCompletionResponse]:

        model_name = request.model
        created_time = int(time.monotonic())
        final_res: RequestOutput = None

        async for res in result_generator:
            if await raw_request.is_disconnected():
                # Abort the request if the client disconnects.
                await self.engine.abort(request_id)
                return self.create_error_response("Client disconnected")
            final_res = res
        assert final_res is not None

        choices = []
        role = self.get_chat_request_role(request)
        for output in final_res.outputs:
            choice_data = ChatCompletionResponseChoice(
                index=output.index,
                message=ChatMessage(role=role, content=output.text),
                finish_reason=output.finish_reason,
            )
            choices.append(choice_data)

        if request.echo:
            last_msg_content = ""
            if request.messages and isinstance(
                    request.messages, list) and request.messages[-1].get(
                        "content") and request.messages[-1].get(
                            "role") == role:
                last_msg_content = request.messages[-1]["content"]

            for choice in choices:
                full_message = last_msg_content + choice.message.content
                choice.message.content = full_message

        num_prompt_tokens = len(final_res.prompt_token_ids)
        num_generated_tokens = sum(
            len(output.token_ids) for output in final_res.outputs)
        usage = UsageInfo(
            prompt_tokens=num_prompt_tokens,
            completion_tokens=num_generated_tokens,
            total_tokens=num_prompt_tokens + num_generated_tokens,
        )
        response = ChatCompletionResponse(
            id=request_id,
            created=created_time,
            model=model_name,
            choices=choices,
            usage=usage,
        )

        return response

    def _load_chat_template(self, chat_template):
        if chat_template is not None:
            try:
                with open(chat_template, "r") as f:
                    self.tokenizer.chat_template = f.read()
            except OSError:
                # If opening a file fails, set chat template to be args to
                # ensure we decode so our escape are interpreted correctly
                self.tokenizer.chat_template = codecs.decode(
                    chat_template, "unicode_escape")

        #     logger.info(
        #         f"Using supplied chat template:\n{self.tokenizer.chat_template}"
        #     )
        # elif self.tokenizer.chat_template is not None:
        #     logger.info(
        #         f"Using default chat template:\n{self.tokenizer.chat_template}"
        #     )
        # else:
        #     logger.warning(
        #         "No chat template provided. Chat API will not work.")

    async def generate(self, request: ChatCompletionRequest, raw_request: Request):
        generator = await self._create_generator(
            request, raw_request)
        if isinstance(generator, ErrorResponse):
            return Response(content=generator.model_dump(),
                            status_code=generator.code)
        if request.stream:
            return StreamingResponse(content=generator,
                                     media_type="text/event-stream")
        else:
            return Response(content=generator.model_dump())

