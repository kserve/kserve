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

from enum import Enum, auto
from typing import Type

from transformers import (
    AutoModel,
    AutoModelForCausalLM,
    AutoModelForMaskedLM,
    AutoModelForMultipleChoice,
    AutoModelForQuestionAnswering,
    AutoModelForSeq2SeqLM,
    AutoModelForSequenceClassification,
    AutoModelForTableQuestionAnswering,
    AutoModelForTokenClassification,
    PretrainedConfig,
)


class MLTask(str, Enum):
    """
    Task defines the common ML tasks using Huggingface Transformer Models
    """

    table_question_answering = auto()
    question_answering = auto()
    token_classification = auto()
    sequence_classification = auto()
    fill_mask = auto()
    text_generation = auto()
    text2text_generation = auto()
    multiple_choice = auto()
    text_embedding = auto()
    time_series_forecast = auto()

    @classmethod
    def _missing_(cls, value: str):
        value = value.lower()
        for member in cls:
            if member.value == value:
                return member
        return None


ARCHITECTURES_2_TASK = {
    "TapasForQuestionAnswering": MLTask.table_question_answering,
    "ForQuestionAnswering": MLTask.question_answering,
    "ForTokenClassification": MLTask.token_classification,
    "ForSequenceClassification": MLTask.sequence_classification,
    "ForMultipleChoice": MLTask.multiple_choice,
    "ForMaskedLM": MLTask.fill_mask,
    "ForCausalLM": MLTask.text_generation,
    "ForConditionalGeneration": MLTask.text2text_generation,
    "MTModel": MLTask.text2text_generation,
    "EncoderDecoderModel": MLTask.text2text_generation,
    "ForPrediction": MLTask.time_series_forecast,
}

TASK_2_CLS = {
    MLTask.sequence_classification: AutoModelForSequenceClassification,
    MLTask.question_answering: AutoModelForQuestionAnswering,
    MLTask.table_question_answering: AutoModelForTableQuestionAnswering,
    MLTask.token_classification: AutoModelForTokenClassification,
    MLTask.fill_mask: AutoModelForMaskedLM,
    MLTask.text_generation: AutoModelForCausalLM,
    MLTask.text2text_generation: AutoModelForSeq2SeqLM,
    MLTask.multiple_choice: AutoModelForMultipleChoice,
    MLTask.text_embedding: AutoModel,
    MLTask.time_series_forecast: AutoModel,
}

SUPPORTED_TASKS = {
    MLTask.sequence_classification,
    MLTask.token_classification,
    MLTask.fill_mask,
    MLTask.text_generation,
    MLTask.text2text_generation,
    MLTask.text_embedding,
    MLTask.time_series_forecast,
}


def infer_task_from_model_architecture(
    model_config: PretrainedConfig,
) -> MLTask:
    architecture = model_config.architectures[0]
    task = None
    for arch_options in ARCHITECTURES_2_TASK:
        if architecture.endswith(arch_options):
            task = ARCHITECTURES_2_TASK[arch_options]
            break

    if task is None:
        raise ValueError(
            f"Task couldn't be inferred from {architecture}. Please manually set `task` option. "
        )
    elif task not in SUPPORTED_TASKS:
        tasks_str = ", ".join(t.name for t in SUPPORTED_TASKS)
        raise ValueError(
            f"Task {task.name} is not supported. Currently supported tasks are: {tasks_str}."
        )
    return task


def is_generative_task(task: MLTask) -> bool:
    return task in {
        MLTask.text_generation,
        MLTask.text2text_generation,
        MLTask.time_series_forecast,
    }


def get_model_class_for_task(task: MLTask) -> Type[AutoModel]:
    return TASK_2_CLS[task]
