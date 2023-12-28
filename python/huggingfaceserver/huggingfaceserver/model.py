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
from typing import Dict, Union, Tuple

from kserve.errors import InferenceError, ModelMissingError
from kserve.storage import Storage

from kserve.protocol.infer_type import InferRequest, InferResponse
from kserve.utils.utils import get_predict_input, get_predict_response
from kserve import Model
import torch
from transformers import AutoModelForCausalLM, AutoModelForSeq2SeqLM, AutoTokenizer, \
    AutoConfig, \
    AutoModelForSequenceClassification, AutoModelForTokenClassification, AutoModelForQuestionAnswering, \
    PreTrainedTokenizerBase, TensorType

ARCHITECTURES_2_TASK = {
    "TapasForQuestionAnswering": "table-question-answering",
    "ForQuestionAnswering": "question-answering",
    "ForTokenClassification": "token-classification",
    "ForSequenceClassification": "text-classification",
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


class HuggingfaceModel(Model):  # pylint:disable=c-extension-no-member
    def __init__(self, model_name, **kwargs):
        print(kwargs)
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
        self.model_id = kwargs['model_id']
        if not self.model_id:
            self.model_dir = kwargs['model_dir']
        self.do_lower_case = kwargs.get('do_lower_case', False)
        self.max_length = kwargs.get('max_length', 2048)
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
            raise ValueError(
                f"Task couldn't be inferred from {architecture}. Please manually set `task` option."
            )
        return task

    def load(self) -> bool:
        model_id_or_path = self.name
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
        task = self.task
        if not task:
            task = self.infer_task_from_model_architecture(model_id_or_path)
        logger.info(f"create inference pipeline for task: {task}")
        # load huggingface tokenizer
        self.tokenizer = AutoTokenizer.from_pretrained(
            model_id_or_path, do_lower_case=self.do_lower_case)
        # load huggingface model using from_pretrained for inference mode
        if not self.predictor_host:
            if task == "sequence-classification":
                self.model = AutoModelForSequenceClassification.from_pretrained(
                    model_id_or_path)
            elif task == "question-answering":
                self.model = AutoModelForQuestionAnswering.from_pretrained(model_id_or_path)
            elif task == "token-classification":
                self.model = AutoModelForTokenClassification.from_pretrained(model_id_or_path)
            elif task == "text-generation":
                self.model = AutoModelForCausalLM.from_pretrained(model_id_or_path)
            else:
                raise ValueError(
                    f"Unsupported task {task}. Please check the supported`task` option."
                )
            self.model.eval()
            logger.info("Transformer model from path %s loaded successfully", model_id_or_path)
        self.ready = True
        return self.ready

    def preprocess(self, payload: InferRequest, headers: Dict[str, str] = None) -> \
            Union[Tuple[Tensor, Tensor], InferRequest]:
        text_inputs = get_predict_input(payload)
        input_ids_batch = None
        attention_mask_batch = None
        for input_text in text_inputs:
            if isinstance(input_text, (bytes, bytearray)):
                input_text = input_text.decode("utf-8")

            inputs = self.tokenizer.encode_plus(
                input_text,
                max_length=int(self.max_length),
                pad_to_max_length=True,
                add_special_tokens=True,
                return_tensors="pt",
            )
            input_ids = inputs["input_ids"].to(self.device)
            attention_mask = inputs["attention_mask"].to(self.device)
            if input_ids.shape is not None:
                if input_ids_batch is None:
                    input_ids_batch = input_ids
                    attention_mask_batch = attention_mask
                else:
                    input_ids_batch = torch.cat((input_ids_batch, input_ids), 0)
                    attention_mask_batch = torch.cat(
                        (attention_mask_batch, attention_mask), 0
                    )
        if self.predictor_host:
            return
        else:
            return input_ids_batch, attention_mask_batch

    async def predict(self, input_batch: Union[Tuple[Tensor, Tensor], InferRequest], headers: Dict[str, str] = None) -> \
            Union[Dict, InferResponse]:
        if self.predictor_host:
            return await super().predict(input_batch, headers)
        else:
            input_ids_batch, attention_mask_batch = input_batch
            outputs = None
            try:
                if self.task == "sequence-classification":
                    outputs = self.model(input_ids_batch, attention_mask_batch)
                elif self.task == "token-classification":
                    outputs = self.model(input_ids_batch, attention_mask_batch)[0]
                    print(
                        "This is the output size from the token classification model", outputs.size(),
                    )
                    print("This is the output from the token classification model", outputs)
                elif self.task == "text-generation":
                    outputs = self.model.generate(
                        input_ids_batch, attention_mask_batch)

                    logger.info("Generated outputs: '%s'", outputs)
            except Exception as e:
                raise InferenceError(str(e))
            return outputs

    def postprocess(self, outputs, headers: Dict[str, str] = None) -> Union[Dict, InferResponse]:
        inferences = []
        if self.task == "sequence-classification":
            num_rows, num_cols = outputs[0].shape
            for i in range(num_rows):
                out = outputs[0][i].unsqueeze(0)
                y_hat = out.argmax(1).item()
                predicted_idx = str(y_hat)
                inferences.append(self.mapping[predicted_idx])
        elif self.task == "token-classification":
            num_rows = outputs.shape[0]
            for i in range(num_rows):
                output = outputs[i].unsqueeze(0)
                predictions = torch.argmax(output, dim=2)
                inferences.append(predictions)
        elif self.task == "text-generation":
            inferences = self.tokenizer.batch_decode(outputs, skip_special_tokens=True)
        return inferences
