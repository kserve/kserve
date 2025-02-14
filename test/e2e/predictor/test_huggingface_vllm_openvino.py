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
from ..common.utils import KSERVE_TEST_NAMESPACE, generate


@pytest.mark.vllm
def test_huggingface_vllm_openvino_openai_chat_completions():
    service_name = "hf-chat"
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
                "--model_name",
                "hf-opt-125m-chat",
                "--max_model_len",
                "512",
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
        == "I'm not sure if this is a good idea, but I'm not sure if I should be"
    )

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.vllm
def test_huggingface_vllm_openvino_openai_completions():
    service_name = "hf-cmpl"
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
                "--model_name",
                "hf-opt-125m-cmpl",
                "--max_model_len",
                "512",
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
