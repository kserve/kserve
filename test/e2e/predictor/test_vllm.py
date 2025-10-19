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
import ast

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
    predict_isvc,
    chat_completion_stream,
    completion_stream,
)
from .test_output import (
    huggingface_text_embedding_expected_output,
    huggingface_sequence_classification_with_raw_logits_expected_output,
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
                "vllm-qwen-chat",
                "--max-model-len",
                "512",
                "--dtype",
                "bfloat16",
            ],
            resources=V1ResourceRequirements(
                requests={"cpu": "2", "memory": "8Gi", "nvidia.com/gpu": "1"},
                limits={"cpu": "4", "memory": "16Gi", "nvidia.com/gpu": "1"},
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
                "vllm-qwen-chat-stream",
                "--max-model-len",
                "512",
                "--dtype",
                "bfloat16",
            ],
            resources=V1ResourceRequirements(
                requests={"cpu": "2", "memory": "8Gi", "nvidia.com/gpu": "1"},
                limits={"cpu": "4", "memory": "16Gi", "nvidia.com/gpu": "1"},
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
                "vllm-qwen-cmpl",
                "--max-model-len",
                "512",
                "--dtype",
                "bfloat16",
            ],
            resources=V1ResourceRequirements(
                requests={"cpu": "2", "memory": "8Gi", "nvidia.com/gpu": "1"},
                limits={"cpu": "4", "memory": "16Gi", "nvidia.com/gpu": "1"},
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
                "vllm-qwen-cmpl-stream",
                "--max-model-len",
                "512",
                "--dtype",
                "bfloat16",
            ],
            resources=V1ResourceRequirements(
                requests={"cpu": "2", "memory": "8Gi", "nvidia.com/gpu": "1"},
                limits={"cpu": "4", "memory": "16Gi", "nvidia.com/gpu": "1"},
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
@pytest.mark.asyncio(scope="session")
async def test_vllm_v2_sequence_classification(rest_v2_client):
    service_name = "vllm-bert-sequence-v2"
    protocol_version = "v2"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="vllm",
            ),
            protocol_version=protocol_version,
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
                "vllm-bert-sequence-v2",
            ],
            resources=V1ResourceRequirements(
                requests={"cpu": "2", "memory": "8Gi", "nvidia.com/gpu": "1"},
                limits={"cpu": "4", "memory": "16Gi", "nvidia.com/gpu": "1"},
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

    res = await predict_isvc(
        rest_v2_client,
        service_name,
        "./data/bert_sequence_classification_v2.json",
    )
    assert res.outputs[0].data == [1]

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.llm
@pytest.mark.asyncio(scope="session")
async def test_vllm_v1_fill_mask(rest_v1_client):
    service_name = "vllm-bert-fill-mask-v1"
    protocol_version = "v1"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="vllm",
            ),
            protocol_version=protocol_version,
            env=[
                client.V1EnvVar(
                    name="MODEL_ID",
                    value="bert-base-uncased",
                ),
            ],
            args=[
                "--served-model-name",
                "vllm-bert-fill-mask-v1",
            ],
            resources=V1ResourceRequirements(
                requests={"cpu": "2", "memory": "8Gi", "nvidia.com/gpu": "1"},
                limits={"cpu": "4", "memory": "16Gi", "nvidia.com/gpu": "1"},
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

    res = await predict_isvc(
        rest_v1_client,
        service_name,
        "./data/bert_fill_mask_v1.json",
    )
    assert res["predictions"] == ["paris", "france"]

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.llm
@pytest.mark.asyncio(scope="session")
async def test_vllm_v2_token_classification(rest_v2_client):
    service_name = "vllm-bert-token-classification-v2"
    protocol_version = "v2"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="vllm",
            ),
            protocol_version=protocol_version,
            env=[
                client.V1EnvVar(
                    name="MODEL_ID",
                    value="dbmdz/bert-large-cased-finetuned-conll03-english",
                ),
            ],
            args=[
                "--revision",
                "4c534963167c08d4b8ff1f88733cf2930f86add0",
                "--tokenizer_revision",
                "4c534963167c08d4b8ff1f88733cf2930f86add0",
                "--disable-special-tokens",
                "--served-model-name",
                "vllm-bert-token-classification-v2",
            ],
            resources=V1ResourceRequirements(
                requests={"cpu": "2", "memory": "8Gi", "nvidia.com/gpu": "1"},
                limits={"cpu": "4", "memory": "16Gi", "nvidia.com/gpu": "1"},
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

    res = await predict_isvc(
        rest_v2_client,
        service_name,
        "./data/bert_token_classification_v2.json",
    )
    assert res.outputs[0].data == [0, 6, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.llm
def test_vllm_openai_text_2_text():
    service_name = "vllm-t5-small"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="vllm",
            ),
            env=[
                client.V1EnvVar(
                    name="MODEL_ID",
                    value="t5-small",
                ),
            ],
            args=[
                "--served-model-name",
                "vllm-t5-small",
                "--max-model-len",
                "512",
            ],
            resources=V1ResourceRequirements(
                requests={"cpu": "2", "memory": "8Gi", "nvidia.com/gpu": "1"},
                limits={"cpu": "4", "memory": "16Gi", "nvidia.com/gpu": "1"},
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

    res = generate(
        service_name, "./data/t5_small_generate.json", chat_completions=False
    )
    assert res["choices"][0]["text"] == "Das ist für Deutschland"

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.llm
@pytest.mark.asyncio(scope="session")
async def test_vllm_v2_text_embedding(rest_v2_client):
    service_name = "vllm-text-embedding-v2"
    protocol_version = "v2"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="vllm",
            ),
            protocol_version=protocol_version,
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
                # This model will fail with "Task couldn't be inferred from BertModel"
                # if the task is not specified.
                "--task",
                "text_embedding",
                "--served-model-name",
                "vllm-text-embedding-v2",
            ],
            resources=V1ResourceRequirements(
                requests={"cpu": "2", "memory": "8Gi", "nvidia.com/gpu": "1"},
                limits={"cpu": "4", "memory": "16Gi", "nvidia.com/gpu": "1"},
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

    res = await predict_isvc(
        rest_v2_client, service_name, "./data/text_embedding_input_v2.json"
    )
    assert res.outputs[0].data == huggingface_text_embedding_expected_output

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
                "text_embedding",
                "--served-model-name",
                "vllm-text-embedding-openai",
            ],
            resources=V1ResourceRequirements(
                requests={"cpu": "2", "memory": "8Gi", "nvidia.com/gpu": "1"},
                limits={"cpu": "4", "memory": "16Gi", "nvidia.com/gpu": "1"},
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
    assert res["data"][0]["embedding"] == huggingface_text_embedding_expected_output

    # Validate base64 output. Decoded as the OpenAI library:
    # https://github.com/openai/openai-python/blob/v1.59.7/src/openai/resources/embeddings.py#L118-L120
    res = embed(service_name, "./data/text_embedding_input_openai_base64.json")
    embedding = np.frombuffer(
        base64.b64decode(res["data"][0]["embedding"]), dtype="float32"
    ).tolist()
    assert len(res["data"]) == 1
    assert embedding == huggingface_text_embedding_expected_output

    # Validate Token count
    assert res["usage"]["prompt_tokens"] == 8
    assert res["usage"]["total_tokens"] == 8

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.llm
@pytest.mark.asyncio(scope="session")
async def test_vllm_v2_sequence_classification_with_raw_logits(
    rest_v2_client,
):
    service_name = "vllm-bert-sequence-v2-prob"
    protocol_version = "v2"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="vllm",
            ),
            protocol_version=protocol_version,
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
                "vllm-bert-sequence-v2-prob",
            ],
            resources=V1ResourceRequirements(
                requests={"cpu": "2", "memory": "8Gi", "nvidia.com/gpu": "1"},
                limits={"cpu": "4", "memory": "16Gi", "nvidia.com/gpu": "1"},
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

    res = await predict_isvc(
        rest_v2_client,
        service_name,
        "./data/bert_sequence_classification_v2.json",
    )

    result = res.outputs[0].data[0]
    temp_dict = eval(result, {"np": np})
    converted = {k: float(v) for k, v in temp_dict.items()}
    res.outputs[0].data[0] = str(converted)

    parsed_output = [ast.literal_eval(res.outputs[0].data[0])]
    assert (
        parsed_output
        == huggingface_sequence_classification_with_raw_logits_expected_output
    )

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.llm
@pytest.mark.asyncio(scope="session")
async def test_vllm_v2_sequence_classification_with_probabilities(
    rest_v2_client,
):
    service_name = "vllm-bert-sequence-v2-logits"
    protocol_version = "v2"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="vllm",
            ),
            protocol_version=protocol_version,
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
                "vllm-bert-sequence-v2-logits",
            ],
            resources=V1ResourceRequirements(
                requests={"cpu": "2", "memory": "8Gi", "nvidia.com/gpu": "1"},
                limits={"cpu": "4", "memory": "16Gi", "nvidia.com/gpu": "1"},
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

    res = await predict_isvc(
        rest_v2_client,
        service_name,
        "./data/bert_sequence_classification_v2.json",
    )
    output = ast.literal_eval(res.outputs[0].data[0])
    assert output == {0: 0.0094, 1: 0.9906}

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
