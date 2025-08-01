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

from typing import Optional, List, Union, Any

from kserve.logging import trace_logger


class RequestLogger:

    def __init__(self, *, max_log_len: Optional[int]) -> None:
        super().__init__()

        self.max_log_len = max_log_len

    def log_inputs(
        self,
        request_id: str,
        prompt: Optional[Union[str, List[str]]] = None,
        prompt_token_ids: Optional[List[int]] = None,
        prompt_embeds: Optional[Any] = None,
        params: Optional[Any] = None,
        lora_request: Optional[Any] = None,
        prompt_adapter_request: Optional[Any] = None,
    ) -> None:
        max_log_len = self.max_log_len
        if max_log_len is not None:
            if prompt is not None:
                if isinstance(prompt, list):
                    for i, p in enumerate(prompt):
                        prompt[i] = p[:max_log_len]
                prompt = prompt[:max_log_len]

            if prompt_token_ids is not None:
                if isinstance(prompt_token_ids, list):
                    for i, p in enumerate(prompt_token_ids):
                        prompt_token_ids[i] = p[:max_log_len]
                prompt_token_ids = prompt_token_ids[:max_log_len]

        trace_logger.info(
            "Received request: %s, prompt: %r, prompt_embeds: %s, "
            "params: %s, prompt_token_ids: %s, "
            "lora_request: %s, prompt_adapter_request: %s.",
            request_id,
            prompt,
            prompt_embeds,
            params,
            prompt_token_ids,
            lora_request,
            prompt_adapter_request,
        )
