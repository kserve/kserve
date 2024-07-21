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

from typing import Union, Dict, Any

import tritonserver
from transformers import TensorType, AutoTokenizer

from huggingfaceserver import HuggingfaceEncoderModel
from huggingfaceserver.utils import _get_and_verify_max_len
from kserve import InferRequest, InferInput, InferResponse
from kserve.logging import logger
from kserve.triton.triton_model import TritonModel
from kserve.utils.numpy_codec import from_np_dtype
from kserve.utils.utils import get_predict_input


class TritonEncoderModel(TritonModel):
    def __init__(
        self,
        encoder_model: HuggingfaceEncoderModel,
        triton_options: tritonserver.Options = None,
    ):
        self._encoder_model = encoder_model
        super().__init__(self._encoder_model.name, triton_options)

    def load(self) -> bool:
        self._encoder_model.max_length = _get_and_verify_max_len(
            self._encoder_model.model_config, self._encoder_model.max_length
        )
        tokenizer_kwargs = {}
        if self._encoder_model.trust_remote_code:
            tokenizer_kwargs["trust_remote_code"] = True
        # load huggingface tokenizer
        self._encoder_model._tokenizer = AutoTokenizer.from_pretrained(
            str(self._encoder_model.model_id_or_path),
            revision=self._encoder_model.tokenizer_revision,
            do_lower_case=self._encoder_model.do_lower_case,
            **tokenizer_kwargs,
        )
        logger.info("Successfully loaded tokenizer")
        return super().load()

    def preprocess(
        self, payload: Union[Dict, InferRequest], context: Dict[str, Any] = None
    ) -> InferRequest:
        instances = get_predict_input(payload)
        inputs = self._encoder_model._tokenizer(
            instances,
            max_length=self._encoder_model.max_length,
            add_special_tokens=self._encoder_model.add_special_tokens,
            return_tensors=TensorType.NUMPY,
            return_token_type_ids=self._encoder_model.return_token_type_ids,
            padding=True,
            truncation=True,
        )
        context["payload"] = payload
        context["input_ids"] = inputs["input_ids"]
        infer_inputs = []
        for key, input_tensor in inputs.items():
            if (not self._encoder_model.tensor_input_names) or (
                key in self._encoder_model.tensor_input_names
            ):
                infer_input = InferInput(
                    name=key,
                    datatype=from_np_dtype(input_tensor.dtype),
                    shape=list(input_tensor.shape),
                    data=input_tensor,
                )
                infer_inputs.append(infer_input)
        infer_request = InferRequest(infer_inputs=infer_inputs, model_name=self.name)
        return infer_request

    def postprocess(
        self, result: Union[Dict, InferResponse], context: Dict[str, Any] = None
    ) -> Union[Dict, InferResponse]:
        return self._encoder_model.postprocess(result, context)
