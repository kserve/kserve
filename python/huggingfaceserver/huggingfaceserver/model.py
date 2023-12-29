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

import json
import os

from torch import Tensor

from kserve.logging import logger
import pathlib
from typing import Dict, Union, Tuple, Any

from kserve.errors import InferenceError
from kserve.storage import Storage

from kserve.protocol.infer_type import InferRequest, InferResponse, InferInput
from kserve.utils.utils import get_predict_input, get_predict_response
from kserve import Model
import torch
from transformers import AutoModelForCausalLM, AutoModelForSeq2SeqLM, AutoTokenizer, \
    AutoConfig, \
    AutoModelForSequenceClassification, AutoModelForTokenClassification, AutoModelForQuestionAnswering, \
    AutoModelForMaskedLM, BatchEncoding

ARCHITECTURES_2_TASK = {
    "TapasForQuestionAnswering": "table-question-answering",
    "ForQuestionAnswering": "question-answering",
    "ForTokenClassification": "token-classification",
    "ForSequenceClassification": "sequence-classification",
    "ForMultipleChoice": "multiple-choice",
    "ForMaskedLM": "fill-mask",
    "ForCausalLM": "text-generation",
    "ForConditionalGeneration": "text2text-generation",
    "MTModel": "text2text-generation",
    "EncoderDecoderModel": "text2text-generation",
    # Model specific task for backward comp
    "GPT2LMHeadModel": "text-generation",
    "T5WithLMHeadModel": "text2text-generation",
    "BloomModel": "text-generation",
}


def from_torch_dtype(torch_dtype):
    if torch_dtype == torch.bool:
        return "BOOL"
    elif torch_dtype == torch.int8:
        return "INT8"
    elif torch_dtype == torch.int16:
        return "INT16"
    elif torch_dtype == torch.int32:
        return "INT32"
    elif torch_dtype == torch.int64:
        return "INT64"
    elif torch_dtype == torch.uint8:
        return "UINT8"
    elif torch_dtype == torch.uint16:
        return "UINT16"
    elif torch_dtype == torch.uint32:
        return "UINT32"
    elif torch_dtype == torch.uint64:
        return "UINT64"
    elif torch_dtype == torch.float16:
        return "FP16"
    elif torch_dtype == torch.float32:
        return "FP32"
    elif torch_dtype == torch.float64:
        return "FP64"
    return None


class HuggingfaceModel(Model):  # pylint:disable=c-extension-no-member
    def __init__(self, model_name, kwargs):
        super().__init__(model_name)
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
        self.ready = False

    @staticmethod
    def infer_task_from_model_architecture(model_config_path: str):
        model_config = AutoConfig.from_pretrained(model_config_path)
        architecture = model_config.architectures[0]
        task = None
        for arch_options in ARCHITECTURES_2_TASK:
            if architecture.endswith(arch_options):
                task = ARCHITECTURES_2_TASK[arch_options]

        if task is None:
            raise ValueError(f"Task couldn't be inferred from {architecture}. Please manually set `task` option.")
        return task

    def load(self) -> bool:
        model_id_or_path = self.model_id
        if self.model_dir:
            model_id_or_path = pathlib.Path(Storage.download(self.model_dir))
            # Read the mapping file, index to object name
            mapping_file_path = os.path.join(self.model_dir, "index_to_name.json")
            # Question answering does not need the index_to_name.json file.
            if self.task == "question_answering" or self.task == "text_generation":
                if os.path.isfile(mapping_file_path):
                    with open(mapping_file_path) as f:
                        self.mapping = json.load(f)
                else:
                    logger.warning("Missing the index_to_name.json file.")
        if not self.task:
            self.task = self.infer_task_from_model_architecture(model_id_or_path)
        logger.info(f"create inference pipeline for task: {self.task}")
        # load huggingface tokenizer
        self.tokenizer = AutoTokenizer.from_pretrained(
            model_id_or_path, do_lower_case=self.do_lower_case)
        # load huggingface model using from_pretrained for inference mode
        if not self.predictor_host:
            if self.task == "sequence-classification":
                self.model = AutoModelForSequenceClassification.from_pretrained(
                    model_id_or_path)
            elif self.task == "question-answering":
                self.model = AutoModelForQuestionAnswering.from_pretrained(model_id_or_path)
            elif self.task == "token-classification":
                self.model = AutoModelForTokenClassification.from_pretrained(model_id_or_path)
            elif self.task == "fill-mask":
                self.model = AutoModelForMaskedLM.from_pretrained(model_id_or_path)
            elif self.task == "text-generation":
                self.model = AutoModelForCausalLM.from_pretrained(model_id_or_path)
            elif self.task == "text2text-generation":
                self.model = AutoModelForSeq2SeqLM.from_pretrained(model_id_or_path)
            else:
                raise ValueError(f"Unsupported task {self.task}. Please check the supported`task` option.")
            self.model.eval()
            logger.info("Transformer model from path %s loaded successfully", model_id_or_path)
        self.ready = True
        return self.ready

    def preprocess(self, payload: Union[Dict, InferRequest], context: Dict[str, Any] = None) -> \
            Union[BatchEncoding, InferRequest]:
        context["payload"] = payload
        text_inputs = get_predict_input(payload)
        inputs = self.tokenizer(
            text_inputs,
            max_length=self.max_length,
            add_special_tokens=self.add_special_tokens,
            return_tensors="pt",
        )
        context["input_ids"] = inputs["input_ids"]
        # Serialize to tensor
        if self.predictor_host:
            infer_inputs = []
            for key, input_tensor in inputs.items():
                infer_input = InferInput(name=key, datatype=from_torch_dtype(input_tensor.dtype),
                                         shape=list(input_tensor.shape), data=input_tensor.numpy())
                print(infer_input.data)
                infer_inputs.append(infer_input)
            infer_request = InferRequest(infer_inputs=infer_inputs, model_name=self.name)
            return infer_request
        else:
            return inputs

    async def predict(self, input_batch: Union[BatchEncoding, InferRequest], context: Dict[str, Any] = None) \
            -> Union[Tensor, InferResponse]:
        if self.predictor_host:
            # when predictor_host is provided, serialize the tensor and send to optimized model serving runtime
            # like NVIDIA triton inference server
            return await super().predict(input_batch, context)
        else:
            outputs = None
            try:
                with torch.no_grad():
                    if self.task == "sequence-classification":
                        outputs = self.model(**input_batch).logits
                    elif self.task == "fill-mask":
                        outputs = self.model(**input_batch).logits
                    elif self.task == "token-classification":
                        outputs = self.model(**input_batch).logits
                    elif self.task == "text-generation" or self.task == "text2text-generation":
                        # TODO implement with more efficient backend vllm and use the generate handler instead
                        outputs = self.model.generate(**input_batch)
                    else:
                        raise ValueError(f"Unsupported task {self.task}. Please check the supported`task` option.")
            except Exception as e:
                raise InferenceError(str(e))
            return outputs

    def postprocess(self, outputs: Union[Tensor, InferResponse], context: Dict[str, Any] = None) \
            -> Union[Dict, InferResponse]:
        inferences = []
        if self.task == "sequence-classification":
            num_rows, num_cols = outputs.shape
            for i in range(num_rows):
                out = outputs[i].unsqueeze(0)
                predicted_idx = out.argmax().item()
                inferences.append(predicted_idx)
        elif self.task == "fill-mask":
            num_rows = outputs.shape[0]
            for i in range(num_rows):
                mask_pos = (context["input_ids"] == self.tokenizer.mask_token_id)[i]
                mask_token_index = mask_pos.nonzero(as_tuple=True)[0]
                predicted_token_id = outputs[i, mask_token_index].argmax(axis=-1)
                inferences.append(self.tokenizer.decode(predicted_token_id))
        elif self.task == "token-classification":
            num_rows = outputs.shape[0]
            for i in range(num_rows):
                output = outputs[i].unsqueeze(0)
                predictions = torch.argmax(output, dim=2)
                inferences.append(predictions.tolist())
        elif self.task == "text-generation" or self.task == "text2text-generation":
            inferences = self.tokenizer.batch_decode(outputs, skip_special_tokens=True)
        else:
            raise ValueError(f"Unsupported task {self.task}. Please check the supported`task` option.")
        return get_predict_response(context["payload"], inferences, self.name)
