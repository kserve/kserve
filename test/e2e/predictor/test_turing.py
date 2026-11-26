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
    V1beta1PredictorSpec,
    V1beta1ModelSpec,
    V1beta1ModelFormat,
    V1beta1InferenceService,
    V1beta1InferenceServiceSpec,
    KServeClient,
)
from kserve.constants import constants
from ..common.utils import (
    assert_answers_four,
    generate,
    chat_completion_stream,
)

ISVC_READY_TIMEOUT_S = 900

# The runtime's image is large; the first pull onto a node can outlast Knative's default 10-minute deadline.
ISVC_ANNOTATIONS = {"serving.knative.dev/progress-deadline": "20m"}


def _turing_isvc(service_name: str, namespace: str) -> V1beta1InferenceService:
    """An InferenceService on the kserve-turingserver runtime, served on the CPU (the image falls back to it
    without a GPU). As in the vLLM tests, the server downloads the model itself: the `--model` argument here
    follows the runtime's `--model=/mnt/models` and takes its place, so the storage initializer is not used."""
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="huggingface",
            ),
            runtime="kserve-turingserver",
            args=["--model", "Qwen/Qwen2.5-0.5B-Instruct"],
            resources=V1ResourceRequirements(
                requests={"cpu": "2", "memory": "8Gi"},
                limits={"cpu": "4", "memory": "16Gi"},
            ),
        ),
    )
    return V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name,
            namespace=namespace,
            annotations=ISVC_ANNOTATIONS,
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )


@pytest.mark.turing_runtime
def test_turing_openai_chat_completions(test_namespace):
    service_name = "turing-qwen-chat"
    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(_turing_isvc(service_name, test_namespace))
    kserve_client.wait_isvc_ready(
        service_name,
        namespace=test_namespace,
        timeout_seconds=ISVC_READY_TIMEOUT_S,
    )

    res = generate(
        service_name, "./data/qwen_input_chat.json", namespace=test_namespace
    )
    assert_answers_four(res["choices"][0]["message"]["content"])


@pytest.mark.turing_runtime
def test_turing_openai_chat_completions_streaming(test_namespace):
    service_name = "turing-qwen-chat-stream"
    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(_turing_isvc(service_name, test_namespace))
    kserve_client.wait_isvc_ready(
        service_name,
        namespace=test_namespace,
        timeout_seconds=ISVC_READY_TIMEOUT_S,
    )

    full_response, chunks = chat_completion_stream(
        service_name,
        "./data/qwen_input_chat_stream.json",
        namespace=test_namespace,
    )
    assert len(chunks) > 0, "expected streaming chunks, got none"
    assert_answers_four(full_response)
