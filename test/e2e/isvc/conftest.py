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

import uuid

import pytest

from ..common.utils import KSERVE_TEST_NAMESPACE
from .fixtures import ISVC_READY_TIMEOUT_S, _make_vllm_isvc, _safe_delete


@pytest.fixture(scope="module")
def traced_vllm_isvc(kserve_client):
    service_name = f"trace-vllm-{uuid.uuid4().hex[:8]}"
    kserve_client.create(_make_vllm_isvc(service_name))
    try:
        kserve_client.wait_isvc_ready(
            service_name,
            namespace=KSERVE_TEST_NAMESPACE,
            timeout_seconds=ISVC_READY_TIMEOUT_S,
        )
        yield service_name
    finally:
        _safe_delete(kserve_client, service_name)
