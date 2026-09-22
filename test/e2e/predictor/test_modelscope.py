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

import os

import pytest
from kubernetes import client
from kubernetes.client import V1ResourceRequirements

from kserve import (
    KServeClient,
    V1beta1InferenceService,
    V1beta1InferenceServiceSpec,
    V1beta1PredictorSpec,
    V1beta1ModelSpec,
    V1beta1ModelFormat,
)
from kserve.constants import constants

from ..common.utils import (
    KSERVE_TEST_NAMESPACE,
    generate,
)

# Real, publicly-readable ModelScope model. Mirrors the HuggingFace e2e tests,
# which serve "Qwen/Qwen2-0.5B-Instruct" via the huggingface runtime; here the
# same model family is sourced from ModelScope storage instead, exercising
# Storage._download_ms end to end. The modelscope:// URI form is
# "modelscope://<owner>/<model>[:revision]" (see Storage._download_ms).
MODELSCOPE_QWEN_URI = "modelscope://Qwen/Qwen2.5-0.5B-Instruct"

# Cold model loads + ModelScope pulls can outrun the 600s default on contended
# CI runners. Increase the timeout and Knative progress deadline accordingly.
ISVC_READY_TIMEOUT_S = 900
ISVC_ANNOTATIONS = {"serving.knative.dev/progress-deadline": "20m"}


# TODO(#5330): Unskip once a ModelScope-reachable CI runner is available.
# ModelScope's CDN is not reliably reachable from the default (overseas) GitHub
# Actions runners, which would make this test flaky in CI. The test is complete
# and passes when run from an environment with ModelScope network access.
@pytest.mark.skip(
    reason="ModelScope CDN not reliably reachable from CI runners; see TODO(#5330)."
)
@pytest.mark.llm
def test_modelscope_qwen_chat_completions():
    """Download Qwen2.5-0.5B-Instruct from ModelScope and serve it.

    Mirrors test_huggingface_openai_chat_completions but sources the model from
    ModelScope storage (modelscope:// URI) instead of HuggingFace, exercising
    Storage._download_ms end to end. With a storage_uri the huggingface runtime
    loads the model from the downloaded directory, so no --model_id is passed.

    If the model requires authentication, the download reads the token from the
    MODELSCOPE_API_TOKEN env var (surfaced via the ms credentials secret as
    ms.MSTokenKey). A public model needs no token.
    """
    service_name = "isvc-modelscope-qwen-chat"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="huggingface",
            ),
            args=[
                "--model_name",
                "qwen-chat",
                "--backend",
                "huggingface",
                "--max_model_len",
                "512",
                "--dtype",
                "bfloat16",
            ],
            resources=V1ResourceRequirements(
                requests={"cpu": "1", "memory": "2Gi"},
                limits={"cpu": "1", "memory": "4Gi"},
            ),
            storage_uri=MODELSCOPE_QWEN_URI,
        ),
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name,
            namespace=KSERVE_TEST_NAMESPACE,
            annotations=ISVC_ANNOTATIONS,
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(
        service_name,
        namespace=KSERVE_TEST_NAMESPACE,
        timeout_seconds=ISVC_READY_TIMEOUT_S,
    )

    res = generate(service_name, "./data/qwen_input_chat.json")
    assert res["choices"][0]["message"]["content"] == "The result of 2 + 2 is 4."

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
