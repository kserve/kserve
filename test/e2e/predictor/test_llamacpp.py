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

import json
import os

from kubernetes import client
import pytest

from kserve import (
    KServeClient,
    V1beta1InferenceService,
    V1beta1InferenceServiceSpec,
    V1beta1ModelFormat,
    V1beta1ModelSpec,
    V1beta1PredictorSpec,
)
from kserve.constants import constants

from ..common.utils import chat_completion_stream, generate


@pytest.mark.llamacpp_runtime
@pytest.mark.parametrize(
    "quantization, runtime", [("Q4_K_M", "kserve-llamacpp"), ("Q8_0", None)]
)
def test_llamacpp_cpu_openai(
    test_namespace, tmp_path, quantization, runtime, network_layer
):
    service_name = f"llamacpp-smollm2-{quantization.lower().replace('_', '-')}"
    filename = f"SmolLM2-135M-Instruct.{quantization}.gguf"
    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(name=service_name, namespace=test_namespace),
        spec=V1beta1InferenceServiceSpec(
            predictor=V1beta1PredictorSpec(
                min_replicas=1,
                model=V1beta1ModelSpec(
                    model_format=V1beta1ModelFormat(name="gguf"),
                    runtime=runtime,
                    storage_uri=f"hf://QuantFactory/SmolLM2-135M-Instruct-GGUF/{filename}",
                    args=["--model", f"/mnt/models/{filename}", "--threads", "2"],
                    resources=client.V1ResourceRequirements(
                        requests={"cpu": "2", "memory": "2Gi"},
                        limits={"cpu": "2", "memory": "2Gi"},
                    ),
                ),
            )
        ),
    )
    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(
        service_name, namespace=test_namespace, timeout_seconds=900
    )

    payload = {
        "model": service_name,
        "messages": [{"role": "user", "content": "Say hello."}],
        "max_tokens": 16,
        "temperature": 0,
    }
    input_path = tmp_path / "chat.json"
    input_path.write_text(json.dumps(payload))
    response = generate(
        service_name,
        str(input_path),
        namespace=test_namespace,
        network_layer=network_layer,
    )
    assert response["model"] == service_name
    assert response["choices"][0]["message"]["content"].strip()
    assert response["usage"]["completion_tokens"] > 0

    payload["stream"] = True
    input_path.write_text(json.dumps(payload))
    content, chunks = chat_completion_stream(
        service_name,
        str(input_path),
        namespace=test_namespace,
        network_layer=network_layer,
    )
    assert chunks
    assert content.strip()

    payload.pop("messages")
    payload["prompt"] = "Hello, my name is"
    payload["stream"] = False
    input_path.write_text(json.dumps(payload))
    response = generate(
        service_name,
        str(input_path),
        chat_completions=False,
        namespace=test_namespace,
        network_layer=network_layer,
    )
    assert response["model"] == service_name
    assert response["choices"][0]["text"].strip()
    assert response["usage"]["completion_tokens"] > 0
