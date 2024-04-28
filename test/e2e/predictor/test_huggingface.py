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
from ..common.utils import KSERVE_TEST_NAMESPACE, predict, generate


@pytest.mark.llm
def test_huggingface_v1():
    service_name = "hf-opt-125m-v1"
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
                "--disable_vllm",
            ],
            resources=V1ResourceRequirements(
                requests={"cpu": "1", "memory": "2Gi"},
                limits={"cpu": "1", "memory": "4Gi"},
            ),
        ),
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND,
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

    res = predict(service_name, "./data/opt_125m_input_v1.json")
    assert res["predictions"] == [
        "What are we having for dinner?\nA nice dinner with a friend.\nI'm not"
    ]

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.llm
def test_huggingface_v2():
    service_name = "hf-opt-125m-v2"
    protocol_version = "v2"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="huggingface",
            ),
            protocol_version=protocol_version,
            args=[
                "--model_id",
                "facebook/opt-125m",
                "--model_revision",
                "27dcfa74d334bc871f3234de431e71c6eeba5dd6",
                "--tokenizer_revision",
                "27dcfa74d334bc871f3234de431e71c6eeba5dd6",
                "--disable_vllm",
            ],
            resources=V1ResourceRequirements(
                requests={"cpu": "1", "memory": "2Gi"},
                limits={"cpu": "1", "memory": "4Gi"},
            ),
        ),
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND,
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

    res = predict(
        service_name, "./data/opt_125m_input_v2.json", protocol_version=protocol_version
    )
    assert res["outputs"][0]["data"] == [
        "What are we having for dinner?\n\nWe're having a dinner with a couple of friends."
    ]

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.llm
def test_huggingface_v2_generate():
    service_name = "hf-opt-125m-v2-generate"
    protocol_version = "v2"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="huggingface",
            ),
            protocol_version=protocol_version,
            args=[
                "--model_id",
                "facebook/opt-125m",
                "--model_revision",
                "27dcfa74d334bc871f3234de431e71c6eeba5dd6",
                "--tokenizer_revision",
                "27dcfa74d334bc871f3234de431e71c6eeba5dd6",
                "--disable_vllm",
            ],
            resources=V1ResourceRequirements(
                requests={"cpu": "1", "memory": "2Gi"},
                limits={"cpu": "1", "memory": "4Gi"},
            ),
        ),
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND,
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
        res["text_output"]
        == "What are we having for dinner?\nA nice dinner with a friend.\nI'm not"
    )

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
