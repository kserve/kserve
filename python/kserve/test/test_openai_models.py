# Copyright 2026 The KServe Authors.
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

import pytest

from kserve.model_repository import ModelRepository
from kserve.protocol.rest.openai import OpenAIChatAdapterModel
from kserve.protocol.rest.openai.dataplane import OpenAIDataPlane
from kserve.protocol.rest.openai.endpoints import OpenAIEndpoints


class NamedModel(OpenAIChatAdapterModel):
    def __init__(self, name: str):
        self.name = name

    def apply_chat_template(self, *args, **kwargs):
        raise NotImplementedError

    async def create_completion(self, *args, **kwargs):
        raise NotImplementedError


@pytest.mark.asyncio
async def test_models_lists_each_registered_name_once():
    repository = ModelRepository()
    model = NamedModel("qwen")
    repository.update(model)
    repository.update(model, "alias-b")

    response = await OpenAIEndpoints(OpenAIDataPlane(repository)).models()

    assert [m.id for m in response.data] == ["qwen", "alias-b"]
