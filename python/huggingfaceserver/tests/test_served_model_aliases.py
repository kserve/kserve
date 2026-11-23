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

from kserve import Model, ModelServer
from kserve.model_repository import ModelRepository

from huggingfaceserver.utils import register_served_model_aliases


@pytest.mark.parametrize(
    "served_model_names,aliases",
    [
        (None, []),
        (["qwen"], []),
        (["qwen", "alias-b", "alias-c"], ["alias-b", "alias-c"]),
    ],
)
def test_register_served_model_aliases(served_model_names, aliases):
    model = Model("qwen")
    server = ModelServer(registered_models=ModelRepository())

    register_served_model_aliases(server, model, served_model_names)
    server.register_model(model)  # what ModelServer.start() does for a ready model

    repository = server.registered_models
    assert sorted(repository.get_models()) == sorted(["qwen", *aliases])
    for name in ["qwen", *aliases]:
        assert repository.get_model(name) is model
