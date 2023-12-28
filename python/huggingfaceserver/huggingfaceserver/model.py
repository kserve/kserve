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
from torch import Tensor

from kserve.logging import logger
import pathlib
from typing import Dict, Union

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

PEFT_MODEL_TASK_TO_CLS = {
    "SEQ_CLS": AutoModelForSequenceClassification,
    "SEQ_2_SEQ_LM": AutoModelForSeq2SeqLM,
    "CAUSAL_LM": AutoModelForCausalLM,
    "TOKEN_CLS": AutoModelForTokenClassification,
    "QUESTION_ANS": AutoModelForQuestionAnswering,
}


class HuggingfaceModel(Model):  # pylint:disable=c-extension-no-member
    def __init__(self, model_name, **kwargs):
        self.hf_pipeline = None
        super().__init__(model_name)
        self.kwargs = {}
        tp_degree = kwargs.get('tensor_parallel_degree', -1)
        self.device_id = kwargs.get('device_id')
        self.device = torch.device(
            "cuda:" + str(self.device_id)
            if torch.cuda.is_available() and self.device_id is not None
            else "cpu"
        )
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
        self.do_lower_case = kwargs['do_lower_case']
        self.max_length = kwargs['max_length']
        self.enable_streaming = kwargs['enable_streaming']
        self.task = kwargs['task']
        self.tokenizer = None
        self.model = None
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
        model_id_or_path = None
        if self.model_id:
            model_id_or_path = self.model_id

        if self.model_dir:
            model_id_or_path = pathlib.Path(Storage.download(self.model_dir))
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

    def preprocess(self, payload: InferRequest, headers: Dict[str, str] = None) -> Union[
        (Tensor, Tensor), InferRequest]:
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

    def predict(self, input_batch: Union[(Tensor, Tensor), InferRequest], headers: Dict[str, str] = None) -> \
            Union[Dict, InferResponse]:
        inferences = []
        if self.predictor_host:
            inferences = super().predict(input_batch, headers)
        else:
            input_ids_batch, attention_mask_batch = input_batch
            try:
                if self.task == "sequence_classification":
                    predictions = self.model(input_ids_batch, attention_mask_batch)
                    num_rows, num_cols = predictions[0].shape
                    for i in range(num_rows):
                        out = predictions[0][i].unsqueeze(0)
                        y_hat = out.argmax(1).item()
                        predicted_idx = str(y_hat)
                        inferences.append(self.mapping[predicted_idx])
                elif self.task == "token_classification":
                    outputs = self.model(input_ids_batch, attention_mask_batch)[0]
                    print(
                        "This the output size from the token classification model",
                        outputs.size(),
                    )
                    print("This the output from the token classification model", outputs)
                    num_rows = outputs.shape[0]
                    for i in range(num_rows):
                        output = outputs[i].unsqueeze(0)
                        predictions = torch.argmax(output, dim=2)
                        tokens = self.tokenizer.tokenize(
                            self.tokenizer.decode(input_ids_batch[i])
                        )
                        if self.mapping:
                            label_list = self.mapping["label_list"]
                        label_list = label_list.strip("][").split(", ")
                        prediction = [
                            (token, label_list[prediction])
                            for token, prediction in zip(tokens, predictions[0].tolist())
                        ]
                        inferences.append(prediction)
                elif self.task == "text_generation":
                    if self.setup_config["model_parallel"]:
                        # Need to move the first device, as the transformer model has been placed there
                        # https://github.com/huggingface/transformers/blob/v4.17.0/src/transformers/models/gpt2/modeling_gpt2.py#L970
                        input_ids_batch = input_ids_batch.to("cuda:0")
                    outputs = self.model.generate(
                        input_ids_batch, max_length=50, do_sample=True, top_p=0.95, top_k=60
                    )
                    for i, x in enumerate(outputs):
                        inferences.append(
                            self.tokenizer.decode(outputs[i], skip_special_tokens=True)
                        )

                    logger.info("Generated text: '%s'", inferences)
            except Exception as e:
                raise InferenceError(str(e))
        return inferences
