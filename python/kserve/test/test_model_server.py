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

import pytest
from kserve import ModelServer

UNKNOWN_MODEL_TYPE_ERR_MESSAGE = "Unknown model collection type"


def test_model_server_start_no_models():
    server = ModelServer()

    with pytest.raises(RuntimeError) as exc:
        server.start(models=None)

    assert exc.value.args[0] == UNKNOWN_MODEL_TYPE_ERR_MESSAGE
