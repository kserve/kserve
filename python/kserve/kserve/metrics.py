# Copyright 2022 The KServe Authors.
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

from prometheus_client import Histogram
from pydantic import BaseModel

PROM_LABELS = ["model_name"]
PRE_HIST_TIME = Histogram(
    "request_preprocess_seconds", "pre-process request latency", PROM_LABELS
)
POST_HIST_TIME = Histogram(
    "request_postprocess_seconds", "post-process request latency", PROM_LABELS
)
PREDICT_HIST_TIME = Histogram(
    "request_predict_seconds", "predict request latency", PROM_LABELS
)
EXPLAIN_HIST_TIME = Histogram(
    "request_explain_seconds", "explain request latency", PROM_LABELS
)


class LLMStats(BaseModel):
    """LLM metrics data class."""

    num_prompt_tokens: int = 0
    num_generation_tokens: int = 0


def get_labels(model_name):
    return {PROM_LABELS[0]: model_name}
