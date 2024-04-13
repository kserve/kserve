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
from threading import Thread
from typing import (Any, AsyncIterator, Dict, Iterable, Optional, TypedDict,
                    Union, cast)

import torch
from accelerate import init_empty_weights
from fastapi import Response
from kserve import Model
from kserve.errors import InferenceError
from kserve.logging import logger
from kserve.model import PredictorConfig
from kserve.protocol.infer_type import InferInput, InferRequest, InferResponse
from kserve.protocol.rest.openai import (ChatPrompt, CompletionRequest,
                                         OpenAIChatAdapterModel)
from kserve.protocol.rest.openai.types import (ChatCompletionRequestMessage,
                                               Completion, CompletionChoice,
                                               CreateCompletionRequest)
from kserve.utils.utils import (from_np_dtype, generate_uuid,
                                get_predict_input, get_predict_response)
from torch import Tensor
from transformers import (AutoConfig, AutoModel, AutoTokenizer, BatchEncoding,
                          GenerationConfig, PreTrainedModel,
                          PreTrainedTokenizerBase, StoppingCriteriaList,
                          TensorType, TextIteratorStreamer, set_seed)

from .stop_sequence_stopping_criteria import StopSequenceStoppingCriteria
from .task import (MLTask, get_model_class_for_task,
                   infer_task_from_model_architecture)


class _GenerateRequest(TypedDict):
    kwargs: Dict[str, Any]
    request: CompletionRequest
    response_queue: asyncio.Queue


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
            CompletionChoice(
                finish_reason=finish_reason, index=self.index, text=text, logprobs=None
            )
        ]
        return Completion(
            id=self.id,
            created=int(time.time()),
            model=self.request.params.model,
            choices=choices,
            object="text_completion",
            system_fingerprint=self.system_fingerprint,
        )


class HuggingfaceModel(
    Model, OpenAIChatAdapterModel
):  # pylint:disable=c-extension-no-member
    tokenizer: PreTrainedTokenizerBase
    model: PreTrainedModel
    device: torch.device
    model_id_or_path: Union[pathlib.Path, str]
    max_length: Optional[int]
    do_lower_case: bool
    add_special_tokens: bool
    tensor_input_names: Optional[str]
    return_token_type_ids: Optional[bool]
    system_fingerprint: Optional[str] = None
    trust_remote_code: bool
    task: Optional[MLTask]
    ready: bool = False
    _request_queue: queue.Queue[Optional[_GenerateRequest]]
    _loop: asyncio.AbstractEventLoop

    def __init__(
        self,
        model_name: str,
        model_id_or_path: Union[pathlib.Path, str],
        do_lower_case: bool = False,
        add_special_tokens: bool = True,
        max_length: Optional[int] = None,
        tensor_input_names: Optional[str] = None,
        return_token_type_ids: Optional[bool] = None,
        model_revision: Optional[str] = None,
        tokenizer_revision: Optional[str] = None,
        trust_remote_code: bool = False,
        system_fingerprint: Optional[str] = None,
        task: Optional[MLTask] = None,
        predictor_config: Optional[PredictorConfig] = None,
    ):
        super().__init__(model_name, predictor_config)
        self.device = torch.device("cuda" if torch.cuda.is_available() else "cpu")
        self.model_id_or_path = model_id_or_path
        self.model_revision = model_revision
        self.tokenizer_revision = tokenizer_revision
        self.do_lower_case = do_lower_case
        self.add_special_tokens = add_special_tokens
        self.max_length = max_length
        self.tensor_input_names = tensor_input_names
        self.return_token_type_ids = return_token_type_ids
        self.system_fingerprint = system_fingerprint
        self.trust_remote_code = trust_remote_code
        self.task = task
        self._request_queue = queue.Queue()

    def load(self) -> bool:
        model_id_or_path = self.model_id_or_path
        model_config = AutoConfig.from_pretrained(
            str(model_id_or_path), revision=self.model_revision
        )
        if not self.task:
            self.task = infer_task_from_model_architecture(model_config)

        if self.max_length is None:
            self.max_length = model_config.max_length

        # device_map = "auto" enables model parallelism but all model architcture dont support it.
        # For pre-check we initialize the model class without weights to check the `_no_split_modules`
        # device_map = "auto" for models that support this else set to either cuda/cpu
        with init_empty_weights():
            self.model = AutoModel.from_config(model_config)

        device_map = self.device

        if self.model._no_split_modules:
            device_map = "auto"

        tokenizer_kwargs = {}
        model_kwargs = {}

        if not model_config.is_encoder_decoder:
            # Pad left for decode-only architecture models.
            # https://github.com/huggingface/transformers/issues/18388#issuecomment-1204369688
            # https://github.com/Vision-CAIR/MiniGPT-4/issues/129
            # https://github.com/huggingface/transformers/blob/1248f0925234f97da9eee98da2aa22f7b8dbeda1/src/transformers/generation/utils.py#L1376-L1388
            tokenizer_kwargs["padding_side"] = "left"

        if self.trust_remote_code:
            model_kwargs["trust_remote_code"] = True
            tokenizer_kwargs["trust_remote_code"] = True

        # load huggingface tokenizer
        self.tokenizer = AutoTokenizer.from_pretrained(
            str(model_id_or_path),
            revision=self.tokenizer_revision,
            do_lower_case=self.do_lower_case,
            **tokenizer_kwargs,
        )

        if not self.tokenizer.pad_token:
            self.tokenizer.add_special_tokens({"pad_token": "[PAD]"})

        logger.info(f"Successfully loaded tokenizer for task: {self.task}")
        # load huggingface model using from_pretrained for inference mode
        if not self.predictor_host:
            model_cls = get_model_class_for_task(self.task)
            self.model = model_cls.from_pretrained(
                model_id_or_path,
                revision=self.model_revision,
                device_map=device_map,
                **model_kwargs,
            )
            self.model.eval()
            self.model.to(self.device)
            logger.info(
                f"Successfully loaded huggingface model from path {model_id_or_path}"
            )
        Thread(target=self._process_requests).start()
        self.ready = True
        return self.ready

    def unload(self):
        # Signal to the background thread that it should shut down
        self._request_queue.put(None)

    def preprocess(
        self,
        payload: Union[Dict, InferRequest],
        context: Dict[str, Any],
    ) -> Union[BatchEncoding, InferRequest]:
        instances = get_predict_input(payload)

        # Serialize to tensor
        if self.predictor_host:
            inputs = self.tokenizer(
                instances,
                max_length=self.max_length,
                add_special_tokens=self.add_special_tokens,
                return_tensors=TensorType.NUMPY,
                return_token_type_ids=self.return_token_type_ids,
                padding=True,
                truncation=True,
            )
            context["payload"] = payload
            context["input_ids"] = inputs["input_ids"]
            infer_inputs = []
            for key, input_tensor in inputs.items():
                if (not self.tensor_input_names) or (key in self.tensor_input_names):
                    infer_input = InferInput(
                        name=key,
                        datatype=from_np_dtype(input_tensor.dtype),
                        shape=list(input_tensor.shape),
                        data=input_tensor,
                    )
                    infer_inputs.append(infer_input)
            infer_request = InferRequest(
                infer_inputs=infer_inputs, model_name=self.name
            )
            return infer_request
        else:
            inputs = self.tokenizer(
                instances,
                max_length=self.max_length,
                add_special_tokens=self.add_special_tokens,
                return_tensors=TensorType.PYTORCH,
                return_token_type_ids=self.return_token_type_ids,
                padding=True,
                truncation=True,
            )
            context["payload"] = payload
            context["input_ids"] = inputs["input_ids"]
            return inputs

    async def predict(
        self,
        input_batch: Union[BatchEncoding, InferRequest],
        context: Dict[str, Any],
    ) -> Union[Tensor, InferResponse]:
        if self.predictor_host:
            # when predictor_host is provided, serialize the tensor and send to optimized model serving runtime
            # like NVIDIA triton inference server
            return await super().predict(input_batch, context)
        else:
            input_batch = input_batch.to(self.device)
            try:
                with torch.no_grad():
                    if (
                        self.task == MLTask.text2text_generation
                        or self.task == MLTask.text_generation
                    ):
                        outputs = self.model.generate(**input_batch)
                    else:
                        outputs = self.model(**input_batch).logits
                    return outputs
            except Exception as e:
                raise InferenceError(str(e))

    def postprocess(
        self, outputs: Union[Tensor, InferResponse], context: Dict[str, Any]
    ) -> Union[Dict, InferResponse]:
        input_ids = context["input_ids"]
        request = context["payload"]
        if isinstance(outputs, InferResponse):
            shape = torch.Size(outputs.outputs[0].shape)
            data = torch.Tensor(outputs.outputs[0].data)
            outputs = data.view(shape)
            input_ids = torch.Tensor(input_ids)
        inferences = []
        if self.task == MLTask.sequence_classification:
            num_rows, num_cols = outputs.shape
            for i in range(num_rows):
                out = outputs[i].unsqueeze(0)
                predicted_idx = out.argmax().item()
                inferences.append(predicted_idx)
            return get_predict_response(request, inferences, self.name)
        elif self.task == MLTask.fill_mask:
            num_rows = outputs.shape[0]
            for i in range(num_rows):
                mask_pos = (input_ids == self.tokenizer.mask_token_id)[i]
                mask_token_index = mask_pos.nonzero(as_tuple=True)[0]
                predicted_token_id = outputs[i, mask_token_index].argmax(axis=-1)
                inferences.append(self.tokenizer.decode(predicted_token_id))
            return get_predict_response(request, inferences, self.name)
        elif self.task == MLTask.token_classification:
            num_rows = outputs.shape[0]
            for i in range(num_rows):
                output = outputs[i].unsqueeze(0)
                predictions = torch.argmax(output, dim=2)
                inferences.append(predictions.tolist())
            return get_predict_response(request, inferences, self.name)
        elif (
            self.task == MLTask.text_generation
            or self.task == MLTask.text2text_generation
        ):
            outputs = self.tokenizer.batch_decode(outputs, skip_special_tokens=True)
            return get_predict_response(request, outputs, self.name)
        else:
            raise ValueError(
                f"Unsupported task {self.task}. Please check the supported `task` option."
            )

    def _handle_request(self, req: _GenerateRequest):
        """
        Handle a single generation request
        """

        response_queue, kwargs, request = (
            req["response_queue"],
            req["kwargs"],
            req["request"],
        )

        def queue_put(outputs):
            self._loop.call_soon_threadsafe(response_queue.put_nowait, outputs)

        if request.params.seed is not None:
            set_seed(request.params.seed)

        echo = bool(cast(CreateCompletionRequest, request.params).echo)

        if request.params.stream:
            streamer = TextIteratorStreamer(
                self.tokenizer,
                skip_prompt=not echo,
            )
            thread = Thread(
                target=self.model.generate, kwargs={**kwargs, "streamer": streamer}
            )
            thread.start()
            # Consume the tokens one by one and add them to the queue
            for output in streamer:
                if output != "":
                    queue_put(output)
            # Put None to indicate we are finished
            queue_put(None)
        else:
            output_start = 0 if echo else kwargs["input_ids"].shape[-1]
            outputs = self.model.generate(**kwargs)
            outputs = self.tokenizer.batch_decode(
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
        self, kwargs: Dict[str, Any], request: CompletionRequest
    ) -> asyncio.Queue:
        """
        Add a request to the request queue to be processed. Results for this request
        will be pushed to the returned async queue.
        """
        if not hasattr(self, "_loop"):
            self._loop = asyncio.get_running_loop()
        req = _GenerateRequest(
            kwargs=kwargs, request=request, response_queue=asyncio.Queue()
        )
        self._request_queue.put(req)
        return req["response_queue"]

    def validate_supported_completion_params(self, params: CreateCompletionRequest):
        """
        Check that only support params have been provided
        """
        if params.frequency_penalty is not None and params.frequency_penalty > 0:
            raise ValueError("'frequency_penalty' is not supported")
        if params.best_of is not None and params.best_of > 1:
            raise ValueError("'best_of' > 1 is not supported")
        if params.n is not None and params.n > 1:
            # TODO: support 'n' by using num
            raise ValueError("'n' > 1 is not supported")

    def build_generation_config(
        self, params: CreateCompletionRequest
    ) -> GenerationConfig:
        kwargs = {}
        kwargs["max_new_tokens"] = params.max_tokens
        if params.presence_penalty and params.presence_penalty > 0:
            kwargs["repetition_penalty"] = params.presence_penalty
        if params.logit_bias is not None:
            # transformers accepts a dict of token tuple to bias (i.e. Dict[Tuple, float])
            kwargs["sequence_bias"] = {
                tuple(token): bias for token, bias in params.logit_bias.items()
            }
        kwargs["top_p"] = params.top_p
        kwargs["temperature"] = params.temperature

        return GenerationConfig(**kwargs)

    def apply_chat_template(
        self, messages: Iterable[ChatCompletionRequestMessage]
    ) -> ChatPrompt:
        """
        Given a list of chat completion messages, convert them to a prompt.
        """
        return ChatPrompt(
            prompt=self.tokenizer.apply_chat_template(messages, tokenize=False)
        )

    async def create_completion(
        self, request: CompletionRequest
    ) -> Union[Completion, AsyncIterator[Completion]]:
        params = cast(CreateCompletionRequest, request.params)
        prompt = params.prompt
        prompts = (
            prompt
            if isinstance(prompt, list) and not isinstance(prompt[0], int)
            else [prompt]
        )
        if isinstance(prompts[0][0], int):
            inputs = {"input_ids": torch.tensor(prompts, dtype=torch.int64)}
        else:
            inputs = self.tokenizer(
                prompts, padding=True, return_tensors=TensorType.PYTORCH
            )
        num_input_tokens = len(inputs["input_ids"])
        if params.max_tokens is None:
            params.max_tokens = self.max_length - num_input_tokens
        if num_input_tokens + params.max_tokens > self.max_length:
            raise ValueError(
                f"This model's maximum context length is {self.max_length} tokens. "
                f"However, you requested {params.max_tokens + num_input_tokens} tokens "
                f"({num_input_tokens} in the messages, "
                f"{params.max_tokens} in the completion). "
                f"Please reduce the length of the messages or completion.",
            )

        self.validate_supported_completion_params(params)
        generation_config = self.build_generation_config(params)
        stopping_criteria = None
        stop_sequence_stopping_criteria = None
        if params.stop is not None:
            stop = params.stop if isinstance(params.stop, list) else [params.stop]
            stop_sequences = [
                self.tokenizer.encode(
                    seq, return_tensors=TensorType.PYTORCH, add_special_tokens=False
                )[0]
                for seq in stop
            ]
            stop_sequence_stopping_criteria = StopSequenceStoppingCriteria(
                input_length=inputs["input_ids"].shape[-1],
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
        )
        if params.stream:
            return CompletionStreamer(
                request=request,
                generate_queue=response_queue,
                system_fingerprint=self.system_fingerprint,
            )
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
                model=params.model,
                system_fingerprint=self.system_fingerprint,
            )
