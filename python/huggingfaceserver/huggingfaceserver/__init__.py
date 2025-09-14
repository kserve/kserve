# Copyright 2023 The KServe Authors.
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

from enum import Enum, auto as auto_value
from . import utils as utils

from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from .encoder_model import HuggingfaceEncoderModel
    from .generative_model import HuggingfaceGenerativeModel


def __getattr__(name: str):
    if name == "HuggingfaceEncoderModel":
        from .encoder_model import HuggingfaceEncoderModel

        return HuggingfaceEncoderModel
    if name == "HuggingfaceGenerativeModel":
        from .generative_model import HuggingfaceGenerativeModel

        return HuggingfaceGenerativeModel
    raise AttributeError(f"module {__name__!r} has no attribute {name!r}")


class Backend(str, Enum):
    """
    Backend defines the framework used to load a model
    """

    auto = auto_value()
    huggingface = auto_value()
    vllm = auto_value()


__all__ = [
    "Backend",
    "utils",
    "HuggingfaceEncoderModel",
    "HuggingfaceGenerativeModel",
]
