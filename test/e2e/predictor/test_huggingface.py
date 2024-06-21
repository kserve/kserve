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
from ..common.utils import KSERVE_TEST_NAMESPACE, generate, predict_isvc
from .test_output import huggingface_text_embedding_expected_output


@pytest.mark.llm
def test_huggingface_openai_chat_completions():
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
                "--backend",
                "huggingface",
                "--max_length",
                "512",
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
        res["choices"][0]["message"]["content"]
        == "I'm not sure if this is a good idea, but I'm not sure if I should be"
    )

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.llm
@pytest.mark.asyncio(scope="session")
async def test_huggingface_v2_sequence_classification(rest_v2_client):
    service_name = "hf-bert-sequence-v2"
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
                "textattack/bert-base-uncased-yelp-polarity",
                "--model_revision",
                "a4d0a85ea6c1d5bb944dcc12ea5c918863e469a4",
                "--tokenizer_revision",
                "a4d0a85ea6c1d5bb944dcc12ea5c918863e469a4",
                "--backend",
                "huggingface",
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

    res = await predict_isvc(
        rest_v2_client,
        service_name,
        "./data/bert_sequence_classification_v2.json",
    )
    assert res.outputs[0].data == [1]

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.llm
@pytest.mark.asyncio(scope="session")
async def test_huggingface_v1_fill_mask(rest_v1_client):
    service_name = "hf-bert-fill-mask-v1"
    protocol_version = "v1"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="huggingface",
            ),
            protocol_version=protocol_version,
            args=[
                "--model_id",
                "bert-base-uncased",
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

    res = await predict_isvc(
        rest_v1_client,
        service_name,
        "./data/bert_fill_mask_v1.json",
    )
    assert res["predictions"] == ["paris", "france"]

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.llm
@pytest.mark.asyncio(scope="session")
async def test_huggingface_v2_token_classification(rest_v2_client):
    service_name = "hf-bert-token-classification-v2"
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
                "dbmdz/bert-large-cased-finetuned-conll03-english",
                "--model_revision",
                "4c534963167c08d4b8ff1f88733cf2930f86add0",
                "--tokenizer_revision",
                "4c534963167c08d4b8ff1f88733cf2930f86add0",
                "--disable_special_tokens",
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

    res = await predict_isvc(
        rest_v2_client,
        service_name,
        "./data/bert_token_classification_v2.json",
    )
    assert res.outputs[0].data == [0, 6, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.llm
def test_huggingface_openai_text_2_text():
    service_name = "hf-t5-small"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="huggingface",
            ),
            args=[
                "--model_id",
                "t5-small",
                "--backend",
                "huggingface",
                "--max_length",
                "512",
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

    res = generate(
        service_name, "./data/t5_small_generate.json", chat_completions=False
    )
    assert res["choices"][0]["text"] == "Das ist für Deutschland"

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.llm
def test_huggingface_v2_text_embedding():
    service_name = "hf-text-embedding-v2"
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
                "sentence-transformers/all-MiniLM-L6-v2",
                "--model_revision",
                "8b3219a92973c328a8e22fadcfa821b5dc75636a",
                "--tokenizer_revision",
                "8b3219a92973c328a8e22fadcfa821b5dc75636a",
                "--backend",
                "huggingface",
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
        service_name,
        "./data/text_embedding_input_v2.json",
        protocol_version=protocol_version,
    )
    assert res["outputs"][0]["data"] == huggingface_text_embedding_expected_output

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
