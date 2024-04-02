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

from typing import List, Dict, Union, Optional

from prometheus_client import Histogram, Counter, Gauge
from pydantic import BaseModel

PROM_LABELS = ['model_name']
PRE_HIST_TIME = Histogram('request_preprocess_seconds', 'pre-process request latency', PROM_LABELS)
POST_HIST_TIME = Histogram('request_postprocess_seconds', 'post-process request latency', PROM_LABELS)
PREDICT_HIST_TIME = Histogram('request_predict_seconds', 'predict request latency', PROM_LABELS)
EXPLAIN_HIST_TIME = Histogram('request_explain_seconds', 'explain request latency', PROM_LABELS)
GENERATE_HIST_TIME = Histogram('request_generate_secondes', 'generate request latency', PROM_LABELS)
GENERATE_STREAM_HIST_TIME = Histogram('request_generate_stream_seconds', 'generate stream request latency', PROM_LABELS)

# LLM Metrics
# Available only for non-streaming endpoints
GENERATION_TOKENS_COUNTER = Counter(
    name="kserve_generation_tokens_total",
    documentation="Number of generation tokens processed.",
    labelnames=PROM_LABELS)
TIME_PER_OUTPUT_TOKEN_HIST = Histogram(
    name="kserve_time_per_output_token_seconds",
    documentation="Histogram of time per output token in seconds.",
    labelnames=PROM_LABELS)
AVG_GENERATION_THROUGHPUT_GAUGE = Gauge(
    name="kserve_avg_generation_throughput_toks_per_s",
    documentation="Average generation throughput in tokens/s.",
    labelnames=PROM_LABELS)

# Available for both streaming and non-streaming endpoints
PROMPT_TOKENS_COUNTER = Counter(
    name="kserve_prompt_tokens_total",
    documentation="Number of prefill tokens processed.",
    labelnames=PROM_LABELS)
TOTAL_TOKENS_COUNTER = Counter(
    name="kserve_total_tokens",
    documentation="Total number of tokens processed.",
    labelnames=PROM_LABELS)
AVG_PROMPT_THROUGHPUT_GAUGE = Gauge(
    name="kserve:avg_prompt_throughput_toks_per_s",
    documentation="Average prefill throughput in tokens/s.",
    labelnames=PROM_LABELS)
AVG_TOTAL_TOKENS_THROUGHPUT_GAUGE = Gauge(
    name="kserve_avg_total_tokens_throughput_toks_per_s",
    documentation="Average total tokens throughput in tokens/s.",
    labelnames=PROM_LABELS)

# Only available for streaming endpoint
TIME_TO_FIRST_TOKEN_HIST = Histogram(
    name="kserve_time_to_first_token_seconds",
    documentation="Histogram of time to first token in seconds.",
    labelnames=PROM_LABELS)


class LLMStats(BaseModel):
    """LLM metrics data class."""

    req_start_time: Optional[float] = None  # Used for calculating time to first token metric
    num_prompt_tokens: List[int] = []
    num_generation_tokens: List[int] = []
    prompt_token_time_taken: Optional[float] = None
    generation_token_time_taken: Optional[float] = None
    time_to_first_token: Optional[float] = None


def export_llm_metrics(stats: LLMStats, labels: Dict):
    """
    Exports all the llm metrics except time to first token to prometheus.

    :param stats: LLM stats as LLMStats object
    :param labels: Label sets as dict
    """
    total_prompt_tokens = 0
    total_generation_tokens = 0
    if stats.num_prompt_tokens:
        total_prompt_tokens = sum(stats.num_prompt_tokens)
        PROMPT_TOKENS_COUNTER.labels(**labels).inc(total_prompt_tokens)
        if stats.prompt_token_time_taken is not None:
            AVG_PROMPT_THROUGHPUT_GAUGE.labels(**labels).set(total_prompt_tokens / stats.prompt_token_time_taken)
    if stats.num_generation_tokens:
        total_generation_tokens = sum(stats.num_generation_tokens)
        GENERATION_TOKENS_COUNTER.labels(**labels).inc(total_generation_tokens)
        if stats.generation_token_time_taken is not None:
            AVG_GENERATION_THROUGHPUT_GAUGE.labels(**labels).set(
                total_generation_tokens / stats.generation_token_time_taken)
            if len(stats.num_prompt_tokens) == 1:
                TIME_PER_OUTPUT_TOKEN_HIST.labels(**labels).observe(stats.generation_token_time_taken)

    total_num_tokens = total_prompt_tokens + total_generation_tokens
    TOTAL_TOKENS_COUNTER.labels(**labels).inc(total_num_tokens)
    if (stats.prompt_token_time_taken is not None and stats.generation_token_time_taken is not None
            and total_num_tokens != 0):
        total_time = stats.prompt_token_time_taken + stats.generation_token_time_taken
        AVG_TOTAL_TOKENS_THROUGHPUT_GAUGE.labels(**labels).set(total_num_tokens / total_time)


def get_labels(model_name):
    return {PROM_LABELS[0]: model_name}


def add_llm_stats_header(headers: Dict) -> Dict:
    headers["stats"] = LLMStats()
    return headers


def get_llm_stats_header(headers: Dict) -> Union[LLMStats, None]:
    return headers.get("stats", None)


def remove_stats_header(headers: Dict) -> Dict:
    if "stats" in headers:
        del headers["stats"]
    return headers
