# Copyright 2021 The KServe Authors.
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

from kserve.logging import logger
import pathlib
from typing import Dict, Union

from kserve.errors import InferenceError, ModelMissingError
from kserve.storage import Storage

from kserve.protocol.infer_type import InferRequest, InferResponse
from kserve.utils.utils import get_predict_input, get_predict_response
from kserve import Model
import torch
from transformers import pipeline, Conversation, AutoModelForCausalLM, AutoModelForSeq2SeqLM, AutoTokenizer, AutoConfig

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
    def __init__(self, kwargs):
        self.hf_pipeline = None
        model_name = kwargs['model_name']
        super().__init__(kwargs['model_name'])
        self.kwargs = {}
        tp_degree = kwargs.get('tensor_parallel_degree', -1)
        self.device_id = kwargs.get('device_id', -1)
        if "device_map" in kwargs:
            self.kwargs["device_map"] = kwargs['device_map']
        elif tp_degree > 0:
            self.kwargs["device_map"] = "auto"
            world_size = torch.cuda.device_count()
            assert world_size == tp_degree, f"TP degree ({tp_degree}) doesn't match available GPUs ({world_size})"
        if "low_cpu_mem_usage" in kwargs:
            kwargs["low_cpu_mem_usage"] = kwargs.get("low_cpu_mem_usage")
        self.model_id = kwargs['model_id']
        self.model_dir = kwargs['model_dir']
        self.enable_streaming = kwargs['enable_streaming']
        self.task = kwargs['task']
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
        if self.model_id:
            model_id_or_path = self.model_id
        if self.model_dir:
            model_id_or_path = pathlib.Path(Storage.download(self.model_dir))
        if not self.task:
            task = self.infer_task_from_model_architecture(model_id_or_path)
        logger.info(f"create inference pipeline for task: {task}")
        self.hf_pipeline = self.get_pipeline(task=task,
                                             model_id_or_path=model_id_or_path,
                                             device=self.device_id,
                                             kwargs=self.kwargs)
        self.ready = True
        return self.ready

    def get_pipeline(self, task: str, device: int, model_id_or_path: str,
                     kwargs):
        # define tokenizer or feature extractor as kwargs to load it the pipeline correctly
        if task in {
            "automatic-speech-recognition",
            "image-segmentation",
            "image-classification",
            "audio-classification",
            "object-detection",
            "zero-shot-image-classification",
        }:
            kwargs["feature_extractor"] = model_id_or_path
        else:
            kwargs["tokenizer"] = model_id_or_path

        use_pipeline = True
        for element in ["load_in_8bit", "low_cpu_mem_usage"]:
            if element in kwargs:
                use_pipeline = False
        # build pipeline
        if use_pipeline:
            if "device_map" in kwargs:
                hf_pipeline = pipeline(task=task,
                                       model=model_id_or_path,
                                       **kwargs)
            else:
                hf_pipeline = pipeline(task=task,
                                       model=model_id_or_path,
                                       device=device,
                                       **kwargs)
        else:
            tokenizer = AutoTokenizer.from_pretrained(model_id_or_path)
            kwargs.pop("tokenizer", None)
            model = AutoModelForCausalLM.from_pretrained(
                model_id_or_path, **kwargs)
            hf_pipeline = pipeline(task=task, model=model, tokenizer=tokenizer)

        # wrap specific pipeline to support better ux
        if task == "conversational":
            hf_pipeline = self.wrap_conversation_pipeline(hf_pipeline)

        if task == "text-generation":
            hf_pipeline.tokenizer.padding_side = "left"
            if not hf_pipeline.tokenizer.pad_token:
                hf_pipeline.tokenizer.pad_token = hf_pipeline.tokenizer.eos_token
            hf_pipeline = self.wrap_text_generation_pipeline(hf_pipeline)

        return hf_pipeline

    @staticmethod
    def wrap_conversation_pipeline(hf_pipeline):

        def wrapped_pipeline(inputs, *args, **kwargs):
            converted_input = Conversation(
                inputs["text"],
                past_user_inputs=inputs.get("past_user_inputs", []),
                generated_responses=inputs.get("generated_responses", []),
            )
            prediction = hf_pipeline(converted_input, *args, **kwargs)
            return {
                "generated_text": prediction.generated_responses[-1],
                "conversation": {
                    "past_user_inputs": prediction.past_user_inputs,
                    "generated_responses": prediction.generated_responses,
                },
            }

        return wrapped_pipeline

    @staticmethod
    def wrap_text_generation_pipeline(hf_pipeline):

        def wrapped_pipeline(inputs, *args, **kwargs):
            model = hf_pipeline.model
            tokenizer = hf_pipeline.tokenizer
            if torch.cuda.is_available():
                input_tokens = tokenizer(inputs, padding=True,
                                         return_tensors="pt").to(torch.cuda.current_device())
            else:
                input_tokens = tokenizer(inputs, padding=True,
                                         return_tensors="pt").to("cpu")
            with torch.no_grad():
                output_tokens = model.generate(
                    *args,
                    input_ids=input_tokens.input_ids,
                    attention_mask=input_tokens.attention_mask,
                    **kwargs)
            generated_text = tokenizer.batch_decode(output_tokens,
                                                    skip_special_tokens=True)

            return [{"generated_text": s} for s in generated_text]

        return wrapped_pipeline

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

    def predict(self, payload: InferRequest, headers: Dict[str, str] = None) -> Union[Dict, InferResponse]:
        try:
            inputs = [s for l in payload.inputs[0].data for s in l]
            parameters = payload.parameters
            if self.enable_streaming:
                stream_generator = StreamingUtils.get_stream_generator(
                    "Accelerate")
                outputs.add_stream_content(
                    stream_generator(self.model, self.tokenizer, data,
                                     **parameters))
                return outputs

            result = self.hf_pipeline(inputs, **parameters)
            logger.info(result)
            return get_predict_response(payload, result, self.name)
        except Exception as e:
            raise InferenceError(str(e))
