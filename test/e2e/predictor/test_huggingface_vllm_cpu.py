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
    KSERVE_TEST_NAMESPACE,
    generate,
    rerank,
    chat_completion_stream,
    completion_stream,
)


@pytest.mark.vllm
def test_huggingface_vllm_cpu_openai_chat_completions():
    service_name = "hf-qwen-chat-vllm"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="huggingface",
            ),
            args=[
                "--model_id",
                "Qwen/Qwen2-0.5B-Instruct",
                "--model_name",
                "hf-qwen-chat",
                "--backend",
                "vllm",
                "--max_model_len",
                "512",
                "--dtype",
                "bfloat16",
            ],
            resources=V1ResourceRequirements(
                requests={"cpu": "2", "memory": "6Gi"},
                limits={"cpu": "2", "memory": "6Gi"},
            ),
        ),
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

    res = generate(service_name, "./data/qwen_input_chat.json")
    assert res["choices"][0]["message"]["content"] == "The result of 2 + 2 is 4."

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.vllm
def test_huggingface_vllm_cpu_text_completion_streaming():
    service_name = "hf-qwen-cmpl-stream-vllm"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="huggingface",
            ),
            args=[
                "--model_id",
                "Qwen/Qwen2-0.5B",
                "--model_name",
                "hf-qwen-cmpl-stream",
                "--backend",
                "vllm",
                "--max_model_len",
                "512",
                "--dtype",
                "bfloat16",
            ],
            resources=V1ResourceRequirements(
                requests={"cpu": "2", "memory": "6Gi"},
                limits={"cpu": "2", "memory": "6Gi"},
            ),
        ),
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

    full_response, _ = completion_stream(
        service_name, "./data/qwen_input_cmpl_stream.json"
    )
    assert full_response.strip() == "The result of 2 + 2 is 4."

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.vllm
def test_huggingface_vllm_cpu_openai_completions():
    service_name = "hf-qwen-cmpl-vllm"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="huggingface",
            ),
            args=[
                "--model_id",
                "Qwen/Qwen2-0.5B",
                "--model_name",
                "hf-qwen-cmpl",
                "--backend",
                "vllm",
                "--max_model_len",
                "512",
                "--dtype",
                "bfloat16",
            ],
            resources=V1ResourceRequirements(
                requests={"cpu": "2", "memory": "6Gi"},
                limits={"cpu": "2", "memory": "6Gi"},
            ),
        ),
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)
    res = generate(service_name, "./data/qwen_input_cmpl.json", chat_completions=False)
    assert res["choices"][0]["text"].strip() == "The result of 2 + 2 is 4."

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.vllm
def test_huggingface_vllm_openai_chat_completions_streaming():
    service_name = "hf-qwen-chat-stream-vllm"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="huggingface",
            ),
            args=[
                "--model_id",
                "Qwen/Qwen2-0.5B-Instruct",
                "--model_name",
                "hf-qwen-chat-stream",
                "--backend",
                "vllm",
                "--max_model_len",
                "512",
                "--dtype",
                "bfloat16",
            ],
            resources=V1ResourceRequirements(
                requests={"cpu": "2", "memory": "6Gi"},
                limits={"cpu": "2", "memory": "6Gi"},
            ),
        ),
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

    full_response, _ = chat_completion_stream(
        service_name, "./data/qwen_input_chat_stream.json"
    )
    assert full_response.strip() == "The result of 2 + 2 is 4."

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.vllm
def test_huggingface_vllm_cpu_rerank():
    service_name = "bge-reranker-base"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="huggingface",
            ),
            args=[
                "--model_id",
                "BAAI/bge-reranker-base",
                "--backend",
                "vllm",
                "--model_revision",
                "2cfc18c9415c912f9d8155881c133215df768a70",
                "--tokenizer_revision",
                "2cfc18c9415c912f9d8155881c133215df768a70",
                "--max-model-len",
                "100",
                "--dtype",
                "bfloat16",
                "--enforce-eager",
            ],
            resources=V1ResourceRequirements(
                requests={"cpu": "2", "memory": "6Gi"},
                limits={"cpu": "2", "memory": "6Gi"},
            ),
        ),
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

    res = rerank(service_name, "./data/bge-reranker-base.json")
    assert res["results"][0]["index"] == 1
    assert res["results"][0]["relevance_score"] == 1.0
    assert res["results"][0]["document"]["text"] == "The capital of France is Paris."
    assert res["results"][1]["index"] == 0
    assert res["results"][1]["relevance_score"] == 0.00058746337890625
    assert res["results"][1]["document"]["text"] == "The capital of Brazil is Brasilia."

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
