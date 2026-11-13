# Copyright 2026 The KServe Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

from unittest.mock import patch

import pytest

from e2e.common import utils


@pytest.mark.parametrize(
    "model_format, prefix",
    [("gguf", ""), ("vLLM", ""), ("huggingface", "/openai")],
)
def test_openai_endpoint_prefix(model_format, prefix):
    isvc = {"spec": {"predictor": {"model": {"modelFormat": {"name": model_format}}}}}
    with (
        patch.object(utils, "KServeClient") as kserve_client,
        patch.object(
            utils,
            "get_isvc_endpoint",
            return_value=("http", "localhost:8080", "model.example.com", "/route"),
        ),
    ):
        kserve_client.return_value.get.return_value = isvc
        url, host = utils._get_openai_endpoint_and_host(
            "model", "v1/chat/completions", namespace="test"
        )

    assert url == f"http://localhost:8080/route{prefix}/v1/chat/completions"
    assert host == "model.example.com"
