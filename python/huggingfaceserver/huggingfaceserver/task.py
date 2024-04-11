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

from enum import auto, Enum


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

    @classmethod
    def _missing_(cls, value: str):
        value = value.lower()
        for member in cls:
            if member.value == value:
                return member
        return None


ARCHITECTURES_2_TASK = {
    "TapasForQuestionAnswering": MLTask.table_question_answering.value,
    "ForQuestionAnswering": MLTask.question_answering.value,
    "ForTokenClassification": MLTask.token_classification.value,
    "ForSequenceClassification": MLTask.sequence_classification.value,
    "ForMultipleChoice": MLTask.multiple_choice.value,
    "ForMaskedLM": MLTask.fill_mask.value,
    "ForCausalLM": MLTask.text_generation.value,
    "ForConditionalGeneration": MLTask.text2text_generation.value,
    "MTModel": MLTask.text2text_generation.value,
    "EncoderDecoderModel": MLTask.text2text_generation.value,
}
