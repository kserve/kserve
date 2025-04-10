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

import numpy as np
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
from ..common.utils import KSERVE_TEST_NAMESPACE, embed, generate, transcribe
from .expected_outputs import (
    huggingface_text_embedding_expected_output,
    whisper_winning_call_transcription_expected_output,
)


@pytest.mark.vllm
def test_huggingface_vllm_cpu_openai_chat_completions():
    service_name = "hf-opt-125m-chat"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="huggingface",
            ),
            args=[
                "--model_id",
                "facebook/opt-125m",
                "--model_revision",
                "27dcfa74d334bc871f3234de431e71c6eeba5dd6",
                "--tokenizer_revision",
                "27dcfa74d334bc871f3234de431e71c6eeba5dd6",
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

    res = generate(service_name, "./data/opt_125m_input_generate.json")
    assert (
        res["choices"][0]["message"]["content"]
        == "I'm not sure if this is a good idea, but I'm going to try to get a"
    )

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.vllm
def test_huggingface_vllm_cpu_openai_completions():
    service_name = "hf-opt-125m-cmpl"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="huggingface",
            ),
            args=[
                "--model_id",
                "facebook/opt-125m",
                "--model_revision",
                "27dcfa74d334bc871f3234de431e71c6eeba5dd6",
                "--tokenizer_revision",
                "27dcfa74d334bc871f3234de431e71c6eeba5dd6",
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
    res = generate(
        service_name, "./data/opt_125m_completion_input.json", chat_completions=False
    )
    assert res["choices"][0]["text"] == "\nI think it's a mod that allows you"

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.vllm
def test_vllm_cpu_openai_text_embedding():
    service_name = "hf-text-embedding-openai"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="huggingface",
            ),
            args=[
                "--model_id",
                "sentence-transformers/all-MiniLM-L6-v2",
                "--model_revision",
                "8b3219a92973c328a8e22fadcfa821b5dc75636a",
                "--tokenizer_revision",
                "8b3219a92973c328a8e22fadcfa821b5dc75636a",
                "--task",
                "embedding",
                "--dtype",
                "float32",
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


@pytest.mark.vllm
def test_huggingface_vllm_cpu_openai_transcriptions():
    service_name = "vllm-whisper-tiny"
    model_name = "whisper-tiny"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="huggingface",
            ),
            args=[
                "--model_id",
                "openai/whisper-tiny",
                "--model_name",
                f"{model_name}",
                "--dtype",
                "bfloat16",
                "--task",
                "transcription",
            ],
            env=[
                client.V1EnvVar(
                    name="VLLM_USE_V1",
                    value="0",
                ),
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

    res = transcribe(service_name, "./data/winning_call.ogg", model_name=model_name)
    assert res["text"] == whisper_winning_call_transcription_expected_output

    # Validate Stream
    transcript = transcribe(
        service_name,
        "./data/winning_call.ogg",
        stream=True,
        model_name=model_name,
    )
    assert transcript == whisper_winning_call_transcription_expected_output

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
