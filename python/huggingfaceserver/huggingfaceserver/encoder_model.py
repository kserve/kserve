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

import pathlib
from typing import Any, Dict, Optional, Union

import torch
import torch.nn.functional as F
from accelerate import init_empty_weights
from kserve import Model
from kserve.errors import InferenceError
from kserve.logging import logger
from kserve.model import PredictorConfig
from kserve.protocol.infer_type import InferInput, InferRequest, InferResponse
from kserve.utils.utils import (
    from_np_dtype,
    get_predict_input,
    get_predict_response,
)
from torch import Tensor
from transformers import (
    AutoConfig,
    AutoTokenizer,
    BatchEncoding,
    PreTrainedModel,
    PreTrainedTokenizerBase,
    PretrainedConfig,
    TensorType,
)

from .request_logger import RequestLogger
from .task import (
    MLTask,
    is_generative_task,
    get_model_class_for_task,
    infer_task_from_model_architecture,
)
from .utils import _get_and_verify_max_len, _mean_pooling


class HuggingfaceEncoderModel(Model):  # pylint:disable=c-extension-no-member
    task: MLTask
    model_config: PretrainedConfig
    model_id_or_path: Union[pathlib.Path, str]
    do_lower_case: bool
    add_special_tokens: bool
    max_length: Optional[int]
    tensor_input_names: Optional[str]
    return_token_type_ids: Optional[bool]
    model_revision: Optional[str]
    tokenizer_revision: Optional[str]
    trust_remote_code: bool
    ready: bool = False
    _tokenizer: PreTrainedTokenizerBase
    _model: Optional[PreTrainedModel] = None
    _device: torch.device

    def __init__(
        self,
        model_name: str,
        model_id_or_path: Union[pathlib.Path, str],
        model_config: Optional[PretrainedConfig] = None,
        task: Optional[MLTask] = None,
        do_lower_case: bool = False,
        add_special_tokens: bool = True,
        max_length: Optional[int] = None,
        dtype: torch.dtype = torch.float32,
        tensor_input_names: Optional[str] = None,
        return_token_type_ids: Optional[bool] = None,
        model_revision: Optional[str] = None,
        tokenizer_revision: Optional[str] = None,
        trust_remote_code: bool = False,
        return_probabilities: bool = False,
        predictor_config: Optional[PredictorConfig] = None,
        request_logger: Optional[RequestLogger] = None,
    ):
        super().__init__(model_name, predictor_config)
        self._device = torch.device("cuda" if torch.cuda.is_available() else "cpu")
        self.model_id_or_path = model_id_or_path
        self.do_lower_case = do_lower_case
        self.add_special_tokens = add_special_tokens
        self.max_length = max_length
        self.dtype = dtype
        self.tensor_input_names = tensor_input_names
        self.return_token_type_ids = return_token_type_ids
        self.model_revision = model_revision
        self.tokenizer_revision = tokenizer_revision
        self.trust_remote_code = trust_remote_code
        self.return_probabilities = return_probabilities
        self.request_logger = request_logger

        if model_config:
            self.model_config = model_config
        else:
            self.model_config = AutoConfig.from_pretrained(
                self.model_id_or_path, trust_remote_code=self.trust_remote_code
            )

        if task:
            self.task = task
            try:
                inferred_task = infer_task_from_model_architecture(self.model_config)
            except ValueError:
                inferred_task = None
            if inferred_task is not None and inferred_task != task:
                logger.warning(
                    f"Inferred task is '{inferred_task.name}' but"
                    f" task is explicitly set to '{self.task.name}'"
                )
        else:
            self.task = infer_task_from_model_architecture(self.model_config)

        if is_generative_task(self.task):
            raise RuntimeError(
                f"Encoder model does not support generative task: {self.task.name}"
            )

    def load(self) -> bool:
        model_id_or_path = self.model_id_or_path

        self.max_length = _get_and_verify_max_len(self.model_config, self.max_length)
        model_cls = get_model_class_for_task(self.task)

        # device_map = "auto" enables model parallelism but all model architcture dont support it.
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
        if not self.predictor_host:
            self._model = model_cls.from_pretrained(
                model_id_or_path,
                revision=self.model_revision,
                device_map=device_map,
                **model_kwargs,
            )
            self._model.eval()
            self._model.to(self._device)
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
        self.ready = True
        return self.ready

    def preprocess(
        self,
        payload: Union[Dict, InferRequest],
        context: Dict[str, Any],
    ) -> Union[BatchEncoding, InferRequest]:
        instances = get_predict_input(payload)
        if isinstance(payload, InferRequest):
            request_id = payload.id
        else:
            request_id = "N.A."
        self._log_request(request_id, instances)
        # Serialize to tensor
        if self.predictor_host:
            inputs = self._tokenizer(
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
            if self.task == MLTask.text_embedding:
                context["attention_mask"] = inputs["attention_mask"]
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
            inputs = self._tokenizer(
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
            if self.task == MLTask.text_embedding:
                context["attention_mask"] = inputs["attention_mask"]
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
            input_batch = input_batch.to(self._device)
            try:
                with torch.no_grad():
                    outputs = self._model(**input_batch)
                    if self.task == MLTask.text_embedding.value:
                        # last_hidden_state contains all token embeddings
                        outputs = outputs.last_hidden_state
                    else:
                        outputs = outputs.logits
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
                if self.return_probabilities:
                    inferences.append(dict(enumerate(out.numpy().flatten())))
                else:
                    predicted_idx = out.argmax().item()
                    inferences.append(predicted_idx)
            return get_predict_response(request, inferences, self.name)
        elif self.task == MLTask.fill_mask:
            num_rows = outputs.shape[0]
            for i in range(num_rows):
                mask_pos = (input_ids == self._tokenizer.mask_token_id)[i]
                mask_token_index = mask_pos.nonzero(as_tuple=True)[0]
                if self.return_probabilities:
                    probabilities = torch.softmax(outputs[i, mask_token_index], dim=-1)
                    decoded_probabilities = []
                    for idx, probs in enumerate(probabilities):
                        token_probs = []
                        for token_id, prob in enumerate(probs):
                            token = self._tokenizer.decode([token_id])
                            token_probs.append({f"{token}": f"{prob.item():.4f}"})
                        decoded_probabilities.append(token_probs)
                    inferences.append(decoded_probabilities)
                else:
                    predicted_token_id = outputs[i, mask_token_index].argmax(axis=-1)
                    inferences.append(self._tokenizer.decode(predicted_token_id))
            return get_predict_response(request, inferences, self.name)
        elif self.task == MLTask.token_classification:
            num_rows = outputs.shape[0]
            for i in range(num_rows):
                output = outputs[i].unsqueeze(0)
                if self.return_probabilities:
                    for values in output.tolist():
                        res = [{k: v for k, v in enumerate(value)} for value in values]
                        inferences.append([res])
                else:
                    predictions = torch.argmax(output, dim=2)
                    inferences.append(predictions.tolist())
            return get_predict_response(request, inferences, self.name)
        elif self.task == MLTask.text_embedding:
            # Perform pooling
            outputs = _mean_pooling(outputs, context["attention_mask"])
            # Normalize embeddings
            outputs = F.normalize(outputs, p=2, dim=1)
            num_rows, _ = outputs.shape
            for i in range(num_rows):
                inferences.append(outputs[i].tolist())
            return get_predict_response(request, inferences, self.name)
        else:
            raise ValueError(
                f"Unsupported task {self.task}. Please check the supported `task` option."
            )

    def _log_request(self, request_id: str, prompt: list[str]) -> None:
        if self.request_logger:
            self.request_logger.log_inputs(
                request_id,
                prompt=prompt,
            )
