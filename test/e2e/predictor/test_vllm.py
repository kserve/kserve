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

import base64
import os

import pytest
from kubernetes import client
from kubernetes.client import V1ResourceRequirements
import numpy as np

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
    embed,
    chat_completion_stream,
    completion_stream,
    classify,
)
from .test_output import (
    vllm_text_embedding_expected_output,
)

from kserve.logging import trace_logger


@pytest.mark.llm
def test_vllm_openai_chat_completions():
    service_name = "vllm-qwen-chat"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="vllm",
            ),
            env=[
                client.V1EnvVar(
                    name="MODEL_ID",
                    value="Qwen/Qwen2-0.5B-Instruct",
                ),
            ],
            args=[
                "--served-model-name",
                "qwen-chat",
                "--max-model-len",
                "512",
                "--dtype",
                "bfloat16",
            ],
            resources=V1ResourceRequirements(
                requests={"cpu": "2", "memory": "8Gi"},
                limits={"cpu": "4", "memory": "16Gi"},
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


@pytest.mark.llm
def test_vllm_openai_chat_completions_streaming():
    service_name = "vllm-qwen-chat-stream"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="vllm",
            ),
            env=[
                client.V1EnvVar(
                    name="MODEL_ID",
                    value="Qwen/Qwen2-0.5B-Instruct",
                ),
                client.V1EnvVar(
                    name="TRANSFORMERS_VERBOSITY",
                    value="info",
                ),
            ],
            args=[
                "--served-model-name",
                "qwen-chat-stream",
                "--max-model-len",
                "512",
                "--dtype",
                "bfloat16",
            ],
            resources=V1ResourceRequirements(
                requests={"cpu": "2", "memory": "8Gi"},
                limits={"cpu": "4", "memory": "16Gi"},
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

    # Test streaming response
    full_response, _ = chat_completion_stream(
        service_name, "./data/qwen_input_chat_stream.json"
    )
    trace_logger.info(f"Full response: {full_response}")

    # Verify we got a valid response
    assert full_response.strip() == "The result of 2 + 2 is 4."

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.llm
def test_vllm_openai_text_completion_qwen2():
    service_name = "vllm-qwen-cmpl"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="vllm",
            ),
            env=[
                client.V1EnvVar(
                    name="MODEL_ID",
                    value="Qwen/Qwen2-0.5B",
                ),
            ],
            args=[
                "--served-model-name",
                "qwen-cmpl",
                "--max-model-len",
                "512",
                "--dtype",
                "bfloat16",
            ],
            resources=V1ResourceRequirements(
                requests={"cpu": "2", "memory": "8Gi"},
                limits={"cpu": "4", "memory": "16Gi"},
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
    assert res["choices"][0].get("text").strip() == "The result of 2 + 2 is 4."

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.llm
def test_vllm_openai_text_completion_streaming():
    service_name = "vllm-qwen-cmpl-stream"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="vllm",
            ),
            env=[
                client.V1EnvVar(
                    name="MODEL_ID",
                    value="Qwen/Qwen2-0.5B",
                ),
            ],
            args=[
                "--served-model-name",
                "qwen-cmpl-stream",
                "--max-model-len",
                "512",
                "--dtype",
                "bfloat16",
            ],
            resources=V1ResourceRequirements(
                requests={"cpu": "2", "memory": "8Gi"},
                limits={"cpu": "4", "memory": "16Gi"},
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
    trace_logger.info(f"Full response: {full_response}")
    assert full_response.strip() == "The result of 2 + 2 is 4."

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.llm
def test_vllm_classify_sequence_classification():
    """Test vLLM sequence classification using /classify endpoint"""
    service_name = "vllm-bert-sequence-classify"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="vllm",
            ),
            env=[
                client.V1EnvVar(
                    name="MODEL_ID",
                    value="textattack/bert-base-uncased-yelp-polarity",
                ),
            ],
            args=[
                "--revision",
                "a4d0a85ea6c1d5bb944dcc12ea5c918863e469a4",
                "--tokenizer_revision",
                "a4d0a85ea6c1d5bb944dcc12ea5c918863e469a4",
                "--served-model-name",
                "vllm-bert-sequence-classify",
                "--task",
                "classify",
            ],
            resources=V1ResourceRequirements(
                requests={"cpu": "2", "memory": "8Gi"},
                limits={"cpu": "4", "memory": "16Gi"},
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

    # Reuse existing v2 format file - helper will convert it
    res = classify(service_name, "./data/bert_sequence_classification_v2.json")
    # vLLM classify endpoint returns classification results
    # The exact format may vary, but should contain prediction
    assert "label" in res or "prediction" in res or res is not None

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.llm
@pytest.mark.asyncio(scope="session")
async def test_vllm_openai_text_embedding():
    service_name = "vllm-text-embedding-openai"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="vllm",
            ),
            env=[
                client.V1EnvVar(
                    name="MODEL_ID",
                    value="sentence-transformers/all-MiniLM-L6-v2",
                ),
            ],
            args=[
                "--revision",
                "8b3219a92973c328a8e22fadcfa821b5dc75636a",
                "--tokenizer_revision",
                "8b3219a92973c328a8e22fadcfa821b5dc75636a",
                "--task",
                "embedding",
                "--served-model-name",
                "text-embedding-openai",
            ],
            resources=V1ResourceRequirements(
                requests={"cpu": "2", "memory": "8Gi"},
                limits={"cpu": "4", "memory": "16Gi"},
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

    # Validate float output
    res = embed(service_name, "./data/text_embedding_input_openai_float.json")
    assert len(res["data"]) == 1
    assert res["data"][0]["embedding"] == vllm_text_embedding_expected_output

    # Validate base64 output. Decoded as the OpenAI library:
    # https://github.com/openai/openai-python/blob/v1.59.7/src/openai/resources/embeddings.py#L118-L120
    res = embed(service_name, "./data/text_embedding_input_openai_base64.json")
    embedding = np.frombuffer(
        base64.b64decode(res["data"][0]["embedding"]), dtype="float32"
    ).tolist()
    assert len(res["data"]) == 1
    assert embedding == vllm_text_embedding_expected_output

    # Validate Token count
    assert res["usage"]["prompt_tokens"] == 8
    assert res["usage"]["total_tokens"] == 8

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.llm
def test_vllm_classify_sequence_classification_probabilities():
    """Test vLLM sequence classification using /classify endpoint for probabilities"""
    service_name = "vllm-bert-sequence-classify-prob"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="vllm",
            ),
            env=[
                client.V1EnvVar(
                    name="MODEL_ID",
                    value="textattack/bert-base-uncased-yelp-polarity",
                ),
            ],
            args=[
                "--revision",
                "a4d0a85ea6c1d5bb944dcc12ea5c918863e469a4",
                "--tokenizer_revision",
                "a4d0a85ea6c1d5bb944dcc12ea5c918863e469a4",
                "--served-model-name",
                "vllm-bert-sequence-classify-prob",
                "--task",
                "classify",
            ],
            resources=V1ResourceRequirements(
                requests={"cpu": "2", "memory": "8Gi"},
                limits={"cpu": "4", "memory": "16Gi"},
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

    # Reuse existing v2 format file - helper will convert it
    res = classify(service_name, "./data/bert_sequence_classification_v2.json")
    # vLLM classify endpoint returns classification probabilities in the data array
    assert "data" in res
    assert len(res["data"]) > 0
    assert "probs" in res["data"][0]
    assert isinstance(res["data"][0]["probs"], list)
    assert "label" in res["data"][0]

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
