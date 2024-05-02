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

import uuid
from threading import Thread

from torch import Tensor

from kserve.model import PredictorConfig
from kserve.protocol.rest.v2_datamodels import (
    GenerateRequest,
    GenerateResponse,
    Token,
    Details,
)
from .async_generate_stream import AsyncGenerateStream
from .task import ARCHITECTURES_2_TASK, MLTask
from kserve.logging import logger
import pathlib
from typing import Dict, Union, Any, AsyncIterator, Optional

from kserve.errors import InferenceError
from kserve.storage import Storage

from kserve.protocol.infer_type import InferRequest, InferResponse, InferInput
from kserve.utils.utils import get_predict_response, get_predict_input, from_np_dtype
from kserve import Model
import torch
from accelerate import init_empty_weights

try:
    from vllm.sampling_params import SamplingParams
    from vllm.engine.async_llm_engine import AsyncLLMEngine
    from vllm.model_executor.models import ModelRegistry

    _vllm = True
except ImportError:
    _vllm = False

from transformers import (
    AutoModelForCausalLM,
    AutoModelForSeq2SeqLM,
    AutoTokenizer,
    AutoConfig,
    AutoModel,
    AutoModelForSequenceClassification,
    AutoModelForTokenClassification,
    AutoModelForQuestionAnswering,
    AutoModelForMaskedLM,
    BatchEncoding,
    TensorType,
)

VLLM_USE_GENERATE_ENDPOINT_ERROR = "Use /generate endpoint for vllm runtime"


class HuggingfaceModel(Model):  # pylint:disable=c-extension-no-member
    def __init__(
        self,
        model_name: str,
        kwargs,
        engine_args=None,
        predictor_config: Optional[PredictorConfig] = None,
    ):
        super().__init__(model_name, predictor_config)
        if kwargs is None:
            kwargs = {}
        self.kwargs = {}
        self.device = torch.device("cuda" if torch.cuda.is_available() else "cpu")
        self.device_map = "cuda" if torch.cuda.is_available() else "cpu"
        self.model_id = kwargs.get("model_id", None)
        self.model_dir = kwargs.get("model_dir", None)
        if not self.model_id and not self.model_dir:
            self.model_dir = "/mnt/models"
        self.model_revision = kwargs.get("model_revision", None)
        self.tokenizer_revision = kwargs.get("tokenizer_revision", None)
        self.do_lower_case = not kwargs.get("disable_lower_case", False)
        self.add_special_tokens = not kwargs.get("disable_special_tokens", False)
        self.max_length = kwargs.get("max_length", None)
        self.tensor_input_names = kwargs.get("tensor_input_names", None)
        self.return_token_type_ids = kwargs.get("return_token_type_ids", None)
        self.task = kwargs.get("task", None)
        self.tokenizer = None
        self.model = None
        self.mapping = None
        self.vllm_engine = None
        self.vllm_engine_args = engine_args
        self.use_vllm = not kwargs.get("disable_vllm", False) if _vllm else False
        self.ready = False
        self.dtype = kwargs.get(
            "dtype", "float16"
        )  # This parameter is used both by HF and vLLM runtimes. This will ensure consistency b/w the two.

    @staticmethod
    def infer_task_from_model_architecture(model_config: str):
        architecture = model_config.architectures[0]
        task = None
        for arch_options in ARCHITECTURES_2_TASK:
            if architecture.endswith(arch_options):
                return ARCHITECTURES_2_TASK[arch_options]

        if task is None:
            raise ValueError(
                f"Task couldn't be inferred from {architecture}. Please manually set `task` option."
            )

    @staticmethod
    def infer_vllm_supported_from_model_architecture(model_config: str):
        architecture = model_config.architectures[0]
        model_cls = ModelRegistry.load_model_cls(architecture)
        if model_cls is None:
            logger.info("not a supported model by vLLM")
        return model_cls

    def load(self) -> bool:
        model_id_or_path = self.model_id
        revision = self.model_revision
        tokenizer_revision = self.tokenizer_revision
        if self.model_dir:
            model_id_or_path = pathlib.Path(Storage.download(self.model_dir))
            # TODO Read the mapping file, index to object name

        model_config = AutoConfig.from_pretrained(model_id_or_path, revision=revision)

        if self.use_vllm and self.device == torch.device("cuda"):  # vllm needs gpu
            if self.infer_vllm_supported_from_model_architecture(model_config):
                logger.info("supported model by vLLM")
                self.vllm_engine_args.tensor_parallel_size = torch.cuda.device_count()
                self.vllm_engine = AsyncLLMEngine.from_engine_args(
                    self.vllm_engine_args
                )
                self.ready = True
                return self.ready

        if not self.task:
            self.task = self.infer_task_from_model_architecture(model_config)

        # device_map = "auto" enables model parallelism but all model architcture dont support it.
        # For pre-check we initialize the model class without weights to check the `_no_split_modules`
        # device_map = "auto" for models that support this else set to either cuda/cpu
        with init_empty_weights():
            self.model = AutoModel.from_config(model_config)

        if self.model._no_split_modules:
            self.device_map = "auto"
        # load huggingface tokenizer
        if not model_config.is_encoder_decoder:
            # Pad left for decode-only architecture models.
            # https://github.com/huggingface/transformers/issues/18388#issuecomment-1204369688
            # https://github.com/Vision-CAIR/MiniGPT-4/issues/129
            # https://github.com/huggingface/transformers/blob/1248f0925234f97da9eee98da2aa22f7b8dbeda1/src/transformers/generation/utils.py#L1376-L1388
            self.tokenizer = AutoTokenizer.from_pretrained(
                model_id_or_path,
                revision=tokenizer_revision,
                do_lower_case=self.do_lower_case,
                device_map=self.device_map,
                padding_side="left",
            )
        else:
            self.tokenizer = AutoTokenizer.from_pretrained(
                model_id_or_path,
                revision=tokenizer_revision,
                do_lower_case=self.do_lower_case,
                device_map=self.device_map,
            )

        if not self.tokenizer.pad_token:
            self.tokenizer.add_special_tokens({"pad_token": "[PAD]"})
        logger.info(f"successfully loaded tokenizer for task: {self.task}")

        # load huggingface model using from_pretrained for inference mode
        # Convert dtype from string to torch type for HF
        if not self.predictor_host:
            hf_dtype_map = {
                "float32": torch.float32,
                "float16": torch.float16,
                "bfloat16": torch.bfloat16,
            }
            self.dtype = hf_dtype_map[self.dtype]
            if self.task == MLTask.sequence_classification.value:
                self.model = AutoModelForSequenceClassification.from_pretrained(
                    model_id_or_path,
                    revision=revision,
                    device_map=self.device_map,
                    torch_dtype=self.dtype,
                )
            elif self.task == MLTask.question_answering.value:
                self.model = AutoModelForQuestionAnswering.from_pretrained(
                    model_id_or_path,
                    revision=revision,
                    device_map=self.device_map,
                    torch_dtype=self.dtype,
                )
            elif self.task == MLTask.token_classification.value:
                self.model = AutoModelForTokenClassification.from_pretrained(
                    model_id_or_path,
                    revision=revision,
                    device_map=self.device_map,
                    torch_dtype=self.dtype,
                )
            elif self.task == MLTask.fill_mask.value:
                self.model = AutoModelForMaskedLM.from_pretrained(
                    model_id_or_path,
                    revision=revision,
                    device_map=self.device_map,
                    torch_dtype=self.dtype,
                )
            elif self.task == MLTask.text_generation.value:
                self.model = AutoModelForCausalLM.from_pretrained(
                    model_id_or_path,
                    revision=revision,
                    device_map=self.device_map,
                    torch_dtype=self.dtype,
                )
            elif self.task == MLTask.text2text_generation.value:
                self.model = AutoModelForSeq2SeqLM.from_pretrained(
                    model_id_or_path,
                    revision=revision,
                    device_map=self.device_map,
                    torch_dtype=self.dtype,
                )
            else:
                raise ValueError(
                    f"Unsupported task {self.task}. Please check the supported `task` option."
                )
            self.model.eval()
            logger.info(
                f"successfully loaded huggingface model from path {model_id_or_path}"
            )
        self.ready = True
        return self.ready

    def preprocess(
        self, payload: Union[Dict, InferRequest], context: Dict[str, Any] = None
    ) -> Union[BatchEncoding, InferRequest]:
        instances = get_predict_input(payload)

        if self.vllm_engine:
            raise InferenceError(VLLM_USE_GENERATE_ENDPOINT_ERROR)

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

    async def generate(
        self, generate_request: GenerateRequest, headers: Dict[str, str] = None
    ) -> Union[GenerateResponse, AsyncIterator[Any]]:
        parameters = generate_request.parameters or {}
        prompt = generate_request.text_input
        request_id = str(uuid.uuid4())
        if self.vllm_engine:
            sampling_params = SamplingParams(**parameters)
            results_generator = self.vllm_engine.generate(
                prompt, sampling_params=sampling_params, request_id=request_id
            )
            return results_generator
        else:
            input_batch = self.tokenizer(
                prompt,
                max_length=self.max_length,
                add_special_tokens=self.add_special_tokens,
                return_tensors=TensorType.PYTORCH,
            )
            input_batch = input_batch.to(self.device)
            if headers.get("streaming", "false") == "true":
                streamer = AsyncGenerateStream(self.tokenizer)
                generation_kwargs = dict(**input_batch, streamer=streamer)
                if parameters:
                    generation_kwargs = dict(
                        **input_batch, **parameters, streamer=streamer
                    )
                # TODO change to use thread pool executor
                thread = Thread(target=self.model.generate, kwargs=generation_kwargs)
                thread.start()
                return streamer
            else:
                if parameters:
                    output_ids = self.model.generate(**input_batch, **parameters)
                else:
                    output_ids = self.model.generate(**input_batch)
                outputs = self.tokenizer.batch_decode(
                    output_ids, skip_special_tokens=True
                )
                token_outputs = [
                    Token(
                        id=output_id,
                        special=False,
                        logprob=0,  # TODO set logprob
                        text=self.tokenizer.decode(output_id, skip_special_tokens=True),
                    )
                    for output_id in output_ids[0]
                ]
                generate_details = Details(
                    finish_reason="length", logprobs=token_outputs
                )
                return GenerateResponse(
                    text_output=outputs[0],
                    model_name=self.name,
                    details=generate_details,
                )

    async def predict(
        self,
        input_batch: Union[BatchEncoding, InferRequest],
        context: Dict[str, Any] = None,
    ) -> Union[Tensor, InferResponse]:
        if self.vllm_engine:
            raise InferenceError(VLLM_USE_GENERATE_ENDPOINT_ERROR)

        if self.predictor_host:
            # when predictor_host is provided, serialize the tensor and send to optimized model serving runtime
            # like NVIDIA triton inference server
            return await super().predict(input_batch, context)
        else:
            input_batch = input_batch.to(self.device)
            try:
                with torch.no_grad():
                    if (
                        self.task == MLTask.text2text_generation.value
                        or self.task == MLTask.text_generation
                    ):
                        outputs = self.model.generate(**input_batch)
                    else:
                        outputs = self.model(**input_batch).logits
                    return outputs
            except Exception as e:
                raise InferenceError(str(e))

    def postprocess(
        self, outputs: Union[Tensor, InferResponse], context: Dict[str, Any] = None
    ) -> Union[Dict, InferResponse]:
        input_ids = context["input_ids"]
        request = context["payload"]
        if isinstance(outputs, InferResponse):
            shape = torch.Size(outputs.outputs[0].shape)
            data = torch.Tensor(outputs.outputs[0].data)
            outputs = data.view(shape)
            input_ids = torch.Tensor(input_ids)
        inferences = []
        if self.task == MLTask.sequence_classification.value:
            num_rows, num_cols = outputs.shape
            for i in range(num_rows):
                out = outputs[i].unsqueeze(0)
                predicted_idx = out.argmax().item()
                inferences.append(predicted_idx)
            return get_predict_response(request, inferences, self.name)
        elif self.task == MLTask.fill_mask.value:
            num_rows = outputs.shape[0]
            for i in range(num_rows):
                mask_pos = (input_ids == self.tokenizer.mask_token_id)[i]
                mask_token_index = mask_pos.nonzero(as_tuple=True)[0]
                predicted_token_id = outputs[i, mask_token_index].argmax(axis=-1)
                inferences.append(self.tokenizer.decode(predicted_token_id))
            return get_predict_response(request, inferences, self.name)
        elif self.task == MLTask.token_classification.value:
            num_rows = outputs.shape[0]
            for i in range(num_rows):
                output = outputs[i].unsqueeze(0)
                predictions = torch.argmax(output, dim=2)
                inferences.append(predictions.tolist())
            return get_predict_response(request, inferences, self.name)
        elif (
            self.task == MLTask.text_generation.value
            or self.task == MLTask.text2text_generation.value
        ):
            outputs = self.tokenizer.batch_decode(outputs, skip_special_tokens=True)
            return get_predict_response(request, outputs, self.name)
        else:
            raise ValueError(
                f"Unsupported task {self.task}. Please check the supported `task` option."
            )
