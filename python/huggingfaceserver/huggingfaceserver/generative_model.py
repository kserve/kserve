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
import pathlib
import queue
import time
import json
from threading import Thread
from typing import (
    Any,
    Dict,
    Iterable,
    Optional,
    TypedDict,
    Union,
    cast,
    AsyncGenerator,
    List,
    Tuple,
)

import torch
from accelerate import init_empty_weights
from kserve.logging import logger
from kserve.protocol.rest.openai import (
    ChatPrompt,
    OpenAIChatAdapterModel,
)


from kserve.protocol.rest.openai.types import (
    CompletionRequest,
    UsageInfo,
    ChatCompletionMessageParam,
    Completion,
    CompletionChoice,
    CompletionChunk,
    CompletionChunkChoice,
    ErrorResponse,
    ChatCompletionRequest,
    ChatCompletionContentPartParam,
    ConversationMessage,
    ChatCompletionToolMessageParam,
    ChatCompletionContentPartTextParam,
    ChatCompletionAssistantMessageParam,
)

from kserve.protocol.rest.openai.errors import OpenAIError
from kserve.utils.utils import generate_uuid
from kserve.constants.constants import LLM_STATS_KEY
from transformers import (
    AutoConfig,
    AutoTokenizer,
    GenerationConfig,
    PreTrainedModel,
    PreTrainedTokenizerBase,
    PretrainedConfig,
    StoppingCriteriaList,
    TensorType,
    TextIteratorStreamer,
    set_seed,
    PreTrainedTokenizer,
    PreTrainedTokenizerFast,
)
from kserve.metrics import LLMStats
from .request_logger import RequestLogger

from .stop_sequence_stopping_criteria import StopSequenceStoppingCriteria
from .task import (
    MLTask,
    is_generative_task,
    get_model_class_for_task,
    infer_task_from_model_architecture,
)
from .utils import _get_and_verify_max_len

from fastapi import Request

AnyTokenizer = Union[PreTrainedTokenizer, PreTrainedTokenizerFast]


class _GenerateRequest(TypedDict):
    kwargs: Dict[str, Any]
    request: CompletionRequest
    response_queue: asyncio.Queue
    loop: asyncio.AbstractEventLoop
    context: Dict[str, Any]


class CompletionStreamer:
    def __init__(
        self,
        request: CompletionRequest,
        generate_queue: asyncio.Queue,
        stop_sequence_stopping_criteria: Optional[StopSequenceStoppingCriteria] = None,
        system_fingerprint: Optional[str] = None,
    ):
        self.request = request
        self.generate_queue = generate_queue
        self.index = 0
        self.id = generate_uuid()
        self.system_fingerprint = system_fingerprint
        self.stop_sequence_stopping_criteria = stop_sequence_stopping_criteria

    def __aiter__(self):
        return self

    async def __anext__(self):
        text = await self.generate_queue.get()
        if text is None:
            raise StopAsyncIteration()
        if (
            self.stop_sequence_stopping_criteria
            and self.stop_sequence_stopping_criteria.triggered
        ):
            finish_reason = "stop"
        else:
            finish_reason = "length"
        choices = [
            CompletionChunkChoice(
                finish_reason=finish_reason, index=self.index, text=text, logprobs=None
            )
        ]
        return CompletionChunk(
            id=self.id,
            created=int(time.time()),
            model=self.request.model,
            choices=choices,
            object="text_completion",
            system_fingerprint=self.system_fingerprint,
        )


class HuggingfaceGenerativeModel(
    OpenAIChatAdapterModel
):  # pylint:disable=c-extension-no-member
    model_config: PretrainedConfig
    model_id_or_path: Union[pathlib.Path, str]
    task: MLTask
    do_lower_case: bool
    max_length: Optional[int]
    model_revision: Optional[str]
    tokenizer_revision: Optional[str]
    trust_remote_code: bool
    system_fingerprint: Optional[str] = None
    ready: bool = False
    _tokenizer: PreTrainedTokenizerBase
    _model: PreTrainedModel
    _device: torch.device
    _request_queue: queue.Queue[Optional[_GenerateRequest]]

    def __init__(
        self,
        name: str,
        model_id_or_path: Union[pathlib.Path, str],
        task: Optional[MLTask] = None,
        model_config: Optional[PretrainedConfig] = None,
        do_lower_case: bool = False,
        max_length: Optional[int] = None,
        dtype: torch.dtype = torch.float16,
        model_revision: Optional[str] = None,
        tokenizer_revision: Optional[str] = None,
        trust_remote_code: bool = False,
        system_fingerprint: Optional[str] = None,
        request_logger: Optional[RequestLogger] = None,
    ):
        super().__init__(name)
        self.model_config = model_config
        self.model_id_or_path = model_id_or_path
        self.model_revision = model_revision
        self.tokenizer_revision = tokenizer_revision
        self.do_lower_case = do_lower_case
        self.max_length = max_length
        self.dtype = dtype
        self.system_fingerprint = system_fingerprint
        self.trust_remote_code = trust_remote_code
        self._device = torch.device("cuda" if torch.cuda.is_available() else "cpu")
        self._request_queue = queue.Queue()
        self.request_logger = request_logger

        if model_config:
            self.model_config = model_config
        else:
            self.model_config = AutoConfig.from_pretrained(
                self.model_id_or_path, trust_remote_code=self.trust_remote_code
            )
        if task:
            self.task = task
        else:
            self.task = infer_task_from_model_architecture(self.model_config)
        if not is_generative_task(self.task):
            raise OpenAIError(
                f"Generative model does not support encoder-only task: {self.task.name}"
            )

    def load(self) -> bool:
        model_id_or_path = self.model_id_or_path

        self.max_length = _get_and_verify_max_len(self.model_config, self.max_length)
        model_cls = get_model_class_for_task(self.task)

        # device_map = "auto" enables model parallelism but all model architecture don't support it.
        # For pre-check we initialize the model class without weights to check the `_no_split_modules`
        # device_map = "auto" for models that support this else set to either cuda/cpu
        with init_empty_weights():
            self._model = model_cls.from_config(
                self.model_config, trust_remote_code=self.trust_remote_code
            )

        device_map = self._device

        if self._model._no_split_modules:
            device_map = "auto"

        tokenizer_kwargs = {}
        model_kwargs = {}

        if not self.model_config.is_encoder_decoder:
            # Pad left for decode-only architecture models.
            # https://github.com/huggingface/transformers/issues/18388#issuecomment-1204369688
            # https://github.com/Vision-CAIR/MiniGPT-4/issues/129
            # https://github.com/huggingface/transformers/blob/1248f0925234f97da9eee98da2aa22f7b8dbeda1/src/transformers/generation/utils.py#L1376-L1388
            logger.info("Decoder-only model detected. Setting padding side to left.")
            tokenizer_kwargs["padding_side"] = "left"

        if self.trust_remote_code:
            model_kwargs["trust_remote_code"] = True
            tokenizer_kwargs["trust_remote_code"] = True

        model_kwargs["torch_dtype"] = self.dtype

        # load huggingface tokenizer
        self._tokenizer = AutoTokenizer.from_pretrained(
            str(model_id_or_path),
            revision=self.tokenizer_revision,
            do_lower_case=self.do_lower_case,
            **tokenizer_kwargs,
        )

        logger.info("Successfully loaded tokenizer")
        # load huggingface model using from_pretrained for inference mode
        self._model = model_cls.from_pretrained(
            model_id_or_path,
            revision=self.model_revision,
            device_map=device_map,
            **model_kwargs,
        )
        self._model.eval()
        if not self._tokenizer.pad_token:
            pad_token_str = "[PAD]"
            logger.warning(
                f"Tokenizer does not have a padding token defined. Adding fall back pad token `{pad_token_str}`"
            )
            # Add fallback pad token [PAD]
            self._tokenizer.add_special_tokens({"pad_token": pad_token_str})
            # When adding new tokens to the vocabulary, we should make sure to also resize the token embedding
            # matrix of the model so that its embedding matrix matches the tokenizer.
            self._model.resize_token_embeddings(len(self._tokenizer))
        logger.info(
            f"Successfully loaded huggingface model from path {model_id_or_path}"
        )
        Thread(target=self._process_requests).start()
        self.ready = True
        return self.ready

    def stop(self):
        # Signal to the background thread that it should shut down
        self._request_queue.put(None)
        self.ready = False

    @property
    def is_encoder_decoder(self) -> bool:
        return self.task in {
            MLTask.table_question_answering,
            MLTask.question_answering,
            MLTask.text2text_generation,
        }

    def _handle_request(self, req: _GenerateRequest):
        """
        Handle a single generation request
        """

        response_queue, kwargs, request, loop, context = (
            req["response_queue"],
            req["kwargs"],
            req["request"],
            req["loop"],
            req["context"],
        )

        def queue_put(outputs):
            loop.call_soon_threadsafe(response_queue.put_nowait, outputs)

        if request.seed is not None:
            set_seed(request.seed)

        echo = bool(request.echo)

        if request.stream:
            streamer = TextIteratorStreamer(
                cast(AutoTokenizer, self._tokenizer),
                skip_prompt=not echo,
            )
            thread = Thread(
                target=self._model.generate, kwargs={**kwargs, "streamer": streamer}
            )
            thread.start()
            # Consume the tokens one by one and add them to the queue
            for output in streamer:
                if output != "":
                    queue_put(output)
            # Put None to indicate we are finished
            queue_put(None)
        else:
            # Encoder-decoder models do not include the input tokens in the output
            output_start = (
                0 if echo or self.is_encoder_decoder else kwargs["input_ids"].shape[-1]
            )
            outputs = self._model.generate(**kwargs)
            stats: LLMStats = context[LLM_STATS_KEY]
            stats.num_generation_tokens = (
                outputs.shape[-1] * outputs.shape[0]
                if self.is_encoder_decoder
                else outputs[:, kwargs["input_ids"].shape[-1] :].shape[-1]
                * outputs.shape[0]
            )
            outputs = self._tokenizer.batch_decode(
                outputs[:, output_start:], skip_special_tokens=True
            )
            queue_put(outputs)

    @torch.no_grad()
    def _process_requests(self):
        """
        Process requests from the request queue in a background thread.
        This ensures we don't block the event loop while running generation.
        """
        while True:
            req = self._request_queue.get()

            # If request is None we should stop processing
            if not req:
                break

            self._handle_request(req)

    def _submit_request(
        self,
        kwargs: Dict[str, Any],
        request: CompletionRequest,
        context: Dict[str, Any],
    ) -> asyncio.Queue:
        """
        Add a request to the request queue to be processed. Results for this request
        will be pushed to the returned async queue.
        """
        req = _GenerateRequest(
            kwargs=kwargs,
            request=request,
            response_queue=asyncio.Queue(),
            loop=asyncio.get_running_loop(),
            context=context,
        )
        self._request_queue.put(req)
        return req["response_queue"]

    def validate_supported_completion_params(self, request: CompletionRequest):
        """
        Check that only support params have been provided
        """
        if request.frequency_penalty is not None and request.frequency_penalty > 0:
            raise OpenAIError("'frequency_penalty' is not supported")
        if request.best_of is not None and request.best_of > 1:
            raise OpenAIError("'best_of' > 1 is not supported")
        if request.n is not None and request.n > 1:
            # TODO: support 'n' by using num
            raise OpenAIError("'n' > 1 is not supported")
        if request.echo and self.is_encoder_decoder:
            raise OpenAIError("'echo' is not supported by encoder-decoder models")

    def build_generation_config(self, request: CompletionRequest) -> GenerationConfig:
        kwargs = {
            "max_new_tokens": request.max_tokens,
            "top_p": request.top_p,
            "temperature": request.temperature,
            "pad_token_id": self._tokenizer.pad_token_id,
        }
        if request.presence_penalty and request.presence_penalty > 0:
            kwargs["repetition_penalty"] = request.presence_penalty
        if request.logit_bias is not None:
            # transformers accepts a dict of token tuple to bias (i.e. Dict[Tuple, float])
            kwargs["sequence_bias"] = {
                tuple(token): bias for token, bias in request.logit_bias.items()
            }
        return GenerationConfig(**kwargs)

    def _parse_chat_message_content_parts(
        self,
        role: str,
        parts: Iterable[ChatCompletionContentPartParam],
    ) -> List[ConversationMessage]:  # TODO: only support text input
        texts: List[str] = []

        for part in parts:
            part_type = part["type"]
            if part_type == "text":
                text = cast(ChatCompletionContentPartTextParam, part)["text"]
                texts.append(text)
            else:
                raise OpenAIError(f"Unknown part type: {part_type}")

        text_prompt = "\n".join(texts)
        return [ConversationMessage(role=role, content=text_prompt)]

    def _parse_chat_message_content(
        self,
        message: ChatCompletionMessageParam,
    ) -> List[ConversationMessage]:
        role = message["role"]
        content = message.get("content")

        if content is None:
            content = []
        elif isinstance(content, str):
            content = [ChatCompletionContentPartTextParam(type="text", text=content)]

        result = self._parse_chat_message_content_parts(
            role,
            content,  # type: ignore
        )

        for result_msg in result:
            if role == "assistant":
                parsed_msg = cast(ChatCompletionAssistantMessageParam, message)
                if "tool_calls" in parsed_msg:
                    result_msg["tool_calls"] = list(parsed_msg["tool_calls"])
            elif role == "tool":
                parsed_msg = cast(ChatCompletionToolMessageParam, message)
                if "tool_call_id" in parsed_msg:
                    result_msg["tool_call_id"] = parsed_msg["tool_call_id"]

            if "name" in message and isinstance(message["name"], str):
                result_msg["name"] = message["name"]

        return result

    def _postprocess_messages(self, messages: List[ConversationMessage]) -> None:
        # per the Transformers docs & maintainers, tool call arguments in
        # assistant-role messages with tool_calls need to be dicts not JSON str -
        # this is how tool-use chat templates will expect them moving forwards
        # so, for messages that have tool_calls, parse the string (which we get
        # from openAI format) to dict
        for message in messages:
            if (
                message["role"] == "assistant"
                and "tool_calls" in message
                and isinstance(message["tool_calls"], list)
            ):

                for item in message["tool_calls"]:
                    item["function"]["arguments"] = json.loads(
                        item["function"]["arguments"]
                    )

    def parse_chat_messages_futures(
        self,
        messages: List[ChatCompletionMessageParam],
    ) -> Tuple[List[ConversationMessage]]:  # TODO: Does not support multimodal
        conversation: List[ConversationMessage] = []

        for msg in messages:
            sub_messages = self._parse_chat_message_content(msg)

            conversation.extend(sub_messages)

        self._postprocess_messages(conversation)

        return conversation

    def apply_hf_chat_template(
        self,
        tokenizer: Union[PreTrainedTokenizer, PreTrainedTokenizerFast],
        conversation: List[ConversationMessage],
        chat_template: Optional[str],
        *,
        tokenize: bool = False,  # Different from HF's default
        **kwargs: Any,
    ) -> str:
        if chat_template is None and tokenizer.chat_template is None:
            raise OpenAIError(
                "As of transformers v4.44, default chat template is no longer "
                "allowed, so you must provide a chat template if the tokenizer "
                "does not define one."
            )

        return tokenizer.apply_chat_template(
            conversation=conversation,  # type: ignore[arg-type]
            chat_template=chat_template,
            tokenize=tokenize,
            **kwargs,
        )

    def apply_chat_template(
        self,
        request: ChatCompletionRequest,
    ) -> (
        ChatPrompt
    ):  # TODO: Does not supprot multi-modal, also does not solve mistral tokenizer issue.
        """
        Given a list of chat completion messages, convert them to a prompt.
        """
        conversation = self.parse_chat_messages_futures(request.messages)
        tool_dicts = (
            None
            if request.tools is None
            else [tool.model_dump() for tool in request.tools]
        )
        prompt = self.apply_hf_chat_template(
            self._tokenizer,
            conversation=conversation,
            chat_template=request.chat_template,
            add_generation_prompt=request.add_generation_prompt,
            continue_final_message=request.continue_final_message,
            tools=tool_dicts,
            documents=request.documents,
            **(request.chat_template_kwargs or {}),
        )

        return ChatPrompt(prompt=prompt)

    async def create_completion(
        self,
        request: CompletionRequest,
        raw_request: Optional[Request] = None,
        context: Optional[Dict[str, Any]] = None,
    ) -> Union[AsyncGenerator[str, None], Completion, ErrorResponse]:
        self._log_request(request, raw_request)
        if request.prompt is None:
            raise OpenAIError("prompt is required")
        stats = LLMStats()
        context = {LLM_STATS_KEY: stats}
        prompt = request.prompt
        prompts = (
            prompt
            if isinstance(prompt, list) and not isinstance(prompt[0], int)
            else [prompt]
        )
        if isinstance(prompts[0][0], int):
            inputs = {
                "input_ids": torch.tensor(prompts, dtype=torch.int64).to(self._device)
            }
        else:
            inputs = self._tokenizer(
                prompts, padding=True, return_tensors=TensorType.PYTORCH
            ).to(self._device)
        num_input_tokens_per_prompt = inputs["input_ids"].shape[-1]
        num_input_tokens = num_input_tokens_per_prompt * inputs["input_ids"].shape[0]
        stats.num_prompt_tokens = num_input_tokens
        if request.max_tokens is None:
            request.max_tokens = self.max_length - num_input_tokens_per_prompt
        if num_input_tokens_per_prompt + request.max_tokens > self.max_length:
            raise OpenAIError(
                f"This model's maximum context length is {self.max_length} tokens. "
                f"However, you requested {request.max_tokens + num_input_tokens_per_prompt} tokens "
                f"({num_input_tokens_per_prompt} in the messages, "
                f"{request.max_tokens} in the completion). "
                f"Please reduce the length of the messages or completion.",
            )

        self.validate_supported_completion_params(request)
        generation_config = self.build_generation_config(request)
        stopping_criteria = None
        stop_sequence_stopping_criteria = None
        if request.stop is not None:
            stop = request.stop if isinstance(request.stop, list) else [request.stop]
            stop_sequences = [
                self._tokenizer.encode(
                    seq, return_tensors=TensorType.PYTORCH, add_special_tokens=False
                )[0]
                for seq in stop
            ]
            stop_sequence_stopping_criteria = StopSequenceStoppingCriteria(
                # Encoder-decoder models do not include input tokens in output
                input_length=(
                    0 if self.is_encoder_decoder else inputs["input_ids"].shape[-1]
                ),
                stop_sequences=stop_sequences,
            )
            stopping_criteria = StoppingCriteriaList([stop_sequence_stopping_criteria])

        response_queue = self._submit_request(
            {
                **inputs,
                "stopping_criteria": stopping_criteria,
                "generation_config": generation_config,
            },
            request,
            context,
        )
        if request.stream:
            completion = CompletionStreamer(
                request=request,
                generate_queue=response_queue,
                system_fingerprint=self.system_fingerprint,
                stop_sequence_stopping_criteria=stop_sequence_stopping_criteria,
            )

            async def stream_results() -> AsyncGenerator[str, None]:
                async for partial_completion in completion:
                    yield f"data: {partial_completion.model_dump_json()}\n\n"
                yield "data: [DONE]\n\n"

            return stream_results()

        else:
            outputs = await response_queue.get()
            if (
                stop_sequence_stopping_criteria is not None
                and stop_sequence_stopping_criteria.triggered
            ):
                finish_reason = "stop"
            else:
                finish_reason = "length"
            choices = [
                CompletionChoice(
                    finish_reason=finish_reason, index=i, text=o, logprobs=None
                )
                for i, o in enumerate(outputs)
            ]
            return Completion(
                id=generate_uuid(),
                choices=choices,
                created=int(time.time()),
                object="text_completion",
                model=request.model,
                system_fingerprint=self.system_fingerprint,
                usage=UsageInfo(
                    prompt_tokens=stats.num_prompt_tokens,
                    completion_tokens=stats.num_generation_tokens,
                    total_tokens=stats.num_prompt_tokens + stats.num_generation_tokens,
                ),
            )

    def _log_request(
        self, request: CompletionRequest, raw_request: Optional[Request] = None
    ) -> None:
        is_prompt_token = isinstance(request.prompt, list) and (
            isinstance(request.prompt[0], int)
            or isinstance(request.prompt[0], list)
            and isinstance(request.prompt[0][0], int)
        )
        if self.request_logger:
            if not raw_request:
                request_id = None
            else:
                request_id = raw_request.headers.get("x-request-id", None)
            self.request_logger.log_inputs(
                request_id,
                prompt=request.prompt if not is_prompt_token else None,
                prompt_token_ids=request.prompt if is_prompt_token else None,
                params=request.model_dump(exclude={"prompt"}),
            )
