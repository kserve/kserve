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

from kserve.protocol.rest.v2_datamodels import GenerateRequest
from .AsyncGenerateOutput import AsyncGenerateStream
from .task import ARCHITECTURES_2_TASK, MLTask
from kserve.logging import logger
import pathlib
from typing import Dict, Union, Any, AsyncIterator

from kserve.errors import InferenceError, InvalidInput
from kserve.storage import Storage

from kserve.protocol.infer_type import InferRequest, InferResponse, InferInput
from kserve.utils.utils import get_predict_response
from kserve import Model
import torch

try:
    from vllm.outputs import RequestOutput
    from vllm.sampling_params import SamplingParams
    from vllm.engine.arg_utils import AsyncEngineArgs
    from vllm.vllm_async_engine import AsyncLLMEngine

    _vllm = True
except ImportError:
    _vllm = False
from transformers import AutoModelForCausalLM, AutoModelForSeq2SeqLM, AutoTokenizer, \
    AutoConfig, \
    AutoModelForSequenceClassification, AutoModelForTokenClassification, AutoModelForQuestionAnswering, \
    AutoModelForMaskedLM, BatchEncoding

torch_dtype_to_oip_dtype_dict = {
    torch.bool: "BOOL",
    torch.uint8: "UINT8",
    torch.int8: "INT8",
    torch.int16: "INT16",
    torch.int32: "INT32",
    torch.int64: "INT64",
    torch.float16: "FP16",
    torch.float32: "FP32",
    torch.float64: "FP64",
}


class HuggingfaceModel(Model):  # pylint:disable=c-extension-no-member
    def __init__(self, model_name, kwargs, engine_args=None):
        super().__init__(model_name)
        if kwargs is None:
            kwargs = {}
        self.kwargs = {}
        tp_degree = kwargs.get('tensor_parallel_degree', -1)
        self.device = torch.device(
            "cuda" if torch.cuda.is_available() else "cpu"
        )
        if "device_map" in kwargs:
            self.kwargs["device_map"] = kwargs['device_map']
        elif tp_degree > 0:
            self.kwargs["device_map"] = "auto"
            world_size = torch.cuda.device_count()
            assert world_size == tp_degree, f"TP degree ({tp_degree}) doesn't match available GPUs ({world_size})"
        self.model_id = kwargs.get('model_id', None)
        self.model_dir = kwargs.get('model_dir', None)
        self.predictor_host = kwargs.get('predictor_host', None)
        self.do_lower_case = kwargs.get('do_lower_case', True)
        self.add_special_tokens = kwargs.get('add_special_tokens', True)
        self.max_length = kwargs.get('max_length', None)
        self.enable_streaming = kwargs.get('enable_streaming', False)
        self.task = kwargs.get('task', None)
        self.tokenizer = None
        self.model = None
        self.mapping = None
        self.engine = AsyncLLMEngine.from_engine_args(engine_args) if _vllm else None
        self.ready = False

    @staticmethod
    def infer_task_from_model_architecture(model_config_path: str):
        model_config = AutoConfig.from_pretrained(model_config_path)
        architecture = model_config.architectures[0]
        task = None
        for arch_options in ARCHITECTURES_2_TASK:
            if architecture.endswith(arch_options):
                return ARCHITECTURES_2_TASK[arch_options]

        if task is None:
            raise ValueError(f"Task couldn't be inferred from {architecture}. Please manually set `task` option.")

    def load(self) -> bool:
        model_id_or_path = self.model_id
        if self.model_dir:
            model_id_or_path = pathlib.Path(Storage.download(self.model_dir))
            # TODO Read the mapping file, index to object name
        if not self.task:
            self.task = self.infer_task_from_model_architecture(model_id_or_path)
        # load huggingface tokenizer
        self.tokenizer = AutoTokenizer.from_pretrained(
            model_id_or_path, do_lower_case=self.do_lower_case)
        logger.info(f"successfully loaded tokenizer for task: {self.task}")
        # load huggingface model using from_pretrained for inference mode
        if not self.predictor_host:
            if self.task == MLTask.sequence_classification.value:
                self.model = AutoModelForSequenceClassification.from_pretrained(
                    model_id_or_path)
            elif self.task == MLTask.question_answering.value:
                self.model = AutoModelForQuestionAnswering.from_pretrained(model_id_or_path)
            elif self.task == MLTask.token_classification.value:
                self.model = AutoModelForTokenClassification.from_pretrained(model_id_or_path)
            elif self.task == MLTask.fill_mask.value:
                self.model = AutoModelForMaskedLM.from_pretrained(model_id_or_path)
            elif self.task == MLTask.text_generation.value:
                self.model = AutoModelForCausalLM.from_pretrained(model_id_or_path)
            elif self.task == MLTask.text2text_generation.value:
                self.model = AutoModelForSeq2SeqLM.from_pretrained(model_id_or_path)
            else:
                raise ValueError(f"Unsupported task {self.task}. Please check the supported `task` option.")
            self.model.eval()
            logger.info(f"successfully loaded huggingface model from path {model_id_or_path}")
        self.ready = True
        return self.ready

    def preprocess(self, payload: Union[Dict, InferRequest, GenerateRequest], context: Dict[str, Any] = None) -> \
            Union[BatchEncoding, InferRequest]:
        text_input = payload.text_input
        inputs = self.tokenizer(
            text_input,
            max_length=self.max_length,
            add_special_tokens=self.add_special_tokens,
            return_tensors="pt",
        )
        context["payload"] = payload
        context["input_ids"] = inputs["input_ids"]
        # Serialize to tensor
        if self.predictor_host:
            infer_inputs = []
            for key, input_tensor in inputs.items():
                infer_input = InferInput(name=key, datatype=torch_dtype_to_oip_dtype_dict.get(input_tensor.dtype, None),
                                         shape=list(input_tensor.shape), data=input_tensor.numpy())
                infer_inputs.append(infer_input)
            infer_request = InferRequest(infer_inputs=infer_inputs, model_name=self.name)
            return infer_request
        else:
            return inputs

    async def generate(self, input_batch: BatchEncoding, context: Dict[str, Any] = None) \
            -> Union[Tensor, AsyncIterator[Any]]:
        parameters = context["payload"].parameters
        prompt = context["payload"].text_input
        request_id = str(uuid.uuid4())
        if _vllm:
            sampling_params = SamplingParams(**parameters)
            results_generator = self.engine.generate(prompt, sampling_params=sampling_params,
                                                     prompt_token_ids=input_batch["input_ids"],
                                                     request_id=request_id)
            return results_generator
        else:
            if context.get("streaming", "false") == "true":
                streamer = AsyncGenerateStream(self.tokenizer)
                generation_kwargs = dict(**input_batch, streamer=streamer)
                thread = Thread(target=self.model.generate, kwargs=generation_kwargs)
                thread.start()
                return streamer
            else:
                return self.model.generate(**input_batch)

    async def predict(self, input_batch: Union[BatchEncoding, InferRequest], context: Dict[str, Any] = None) \
            -> Union[Tensor, InferResponse]:
        if self.predictor_host:
            # when predictor_host is provided, serialize the tensor and send to optimized model serving runtime
            # like NVIDIA triton inference server
            return await super().predict(input_batch, context)
        else:
            try:
                with torch.no_grad():
                    if self.task == MLTask.text2text_generation.value:
                        outputs = self.model.generate(**input_batch)
                    elif self.task == MLTask.text_generation:
                        raise InvalidInput(f"text generation is not supported for predict endpoint, "
                                           f"use generate endpoint instead")
                    else:
                        outputs = self.model(**input_batch).logits
                    return outputs
            except Exception as e:
                raise InferenceError(str(e))

    def postprocess(self, outputs: Union[Tensor, InferResponse], context: Dict[str, Any] = None) \
            -> Union[Dict, InferResponse]:
        inferences = []
        if self.task == MLTask.sequence_classification.value:
            num_rows, num_cols = outputs.shape
            for i in range(num_rows):
                out = outputs[i].unsqueeze(0)
                predicted_idx = out.argmax().item()
                inferences.append(predicted_idx)
        elif self.task == MLTask.fill_mask.value:
            num_rows = outputs.shape[0]
            for i in range(num_rows):
                mask_pos = (context["input_ids"] == self.tokenizer.mask_token_id)[i]
                mask_token_index = mask_pos.nonzero(as_tuple=True)[0]
                predicted_token_id = outputs[i, mask_token_index].argmax(axis=-1)
                inferences.append(self.tokenizer.decode(predicted_token_id))
        elif self.task == MLTask.token_classification.value:
            num_rows = outputs.shape[0]
            for i in range(num_rows):
                output = outputs[i].unsqueeze(0)
                predictions = torch.argmax(output, dim=2)
                inferences.append(predictions.tolist())
        elif self.task == MLTask.text_generation.value or self.task == MLTask.text2text_generation.value:
            if context.get("streaming", "false") == "true":
                return outputs
            else:
                inferences = self.tokenizer.batch_decode(outputs, skip_special_tokens=True)
        else:
            raise ValueError(f"Unsupported task {self.task}. Please check the supported `task` option.")
        return get_predict_response(context["payload"], inferences, self.name)
