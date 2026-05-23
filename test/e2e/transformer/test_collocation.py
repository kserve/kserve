# Copyright 2023 The KServe Authors.
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
import uuid
from kserve.models.v1beta1_model_format import V1beta1ModelFormat
from kserve.models.v1beta1_model_spec import V1beta1ModelSpec
from kubernetes import client

from kserve import KServeClient
from kserve import constants
from kserve import V1beta1PredictorSpec
from kserve import V1beta1InferenceServiceSpec
from kserve import V1beta1InferenceService
from kubernetes.client import V1ResourceRequirements
from kubernetes.client import V1Container
from kubernetes.client import V1EnvVar
from kubernetes.client import V1ContainerPort
import pytest
from ..common.utils import is_model_ready, predict_isvc
from ..common.utils import (
    KSERVE_TEST_NAMESPACE,
    INFERENCESERVICE_CONTAINER,
    TRANSFORMER_CONTAINER,
    STORAGE_URI_ENV,
)

SENTIMENT_TRANSFORMER_ARGS = [
    "--tokenizer_name",
    "optimum/distilbert-base-uncased-finetuned-sst-2-english",
    "--sentiment_labels",
    "negative,positive",
    "--output_name",
    "logits",
]

SENTIMENT_STORAGE_URI = "hf://optimum/distilbert-base-uncased-finetuned-sst-2-english"


def assert_sentiment_predictions(res, expected_count=2):
    """Common assertion logic for sentiment prediction responses."""
    assert "predictions" in res
    predictions = res["predictions"]
    assert len(predictions) == expected_count
    for pred in predictions:
        assert pred["sentiment"] in ["positive", "negative"]
        assert 0.0 <= pred["confidence"] <= 1.0


@pytest.mark.collocation
@pytest.mark.asyncio(scope="session")
async def test_transformer_collocation(rest_v1_client, network_layer):
    service_name = "custom-model-transformer-collocation"
    model_name = "sentiment-analysis"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        containers=[
            V1Container(
                name=INFERENCESERVICE_CONTAINER,
                image="openvino/model_server:2024.5",
                env=[
                    V1EnvVar(
                        name=STORAGE_URI_ENV,
                        value=SENTIMENT_STORAGE_URI,
                    ),
                ],
                args=[
                    f"--model_name={model_name}",
                    "--model_path=/mnt/models",
                    "--port=9000",
                    "--rest_port=8085",
                    "--file_system_poll_wait_seconds=0",
                ],
                resources=V1ResourceRequirements(
                    requests={"cpu": "100m", "memory": "512Mi"},
                    limits={"cpu": "1", "memory": "2Gi"},
                ),
            ),
            V1Container(
                name=TRANSFORMER_CONTAINER,
                image=os.environ.get("IMAGE_TRANSFORMER_IMG_TAG"),
                args=[
                    f"--model_name={model_name}",
                    "--http_port=8080",
                    "--grpc_port=8081",
                    "--predictor_host=localhost:8085",
                    "--predictor_protocol=v2",
                    "--enable_predictor_health_check",
                ]
                + SENTIMENT_TRANSFORMER_ARGS,
                ports=[V1ContainerPort(container_port=8080, protocol="TCP")],
                resources=V1ResourceRequirements(
                    requests={"cpu": "10m", "memory": "128Mi"},
                    limits={"cpu": "100m", "memory": "1Gi"},
                ),
                readiness_probe=client.V1Probe(
                    http_get=client.V1HTTPGetAction(
                        path=f"/v1/models/{model_name}", port=8080
                    )
                ),
            ),
        ],
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
    try:
        kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)
    except RuntimeError as e:
        print(
            kserve_client.api_instance.get_namespaced_custom_object(
                "serving.knative.dev",
                "v1",
                KSERVE_TEST_NAMESPACE,
                "services",
                service_name + "-predictor",
            )
        )
        pods = kserve_client.core_api.list_namespaced_pod(
            KSERVE_TEST_NAMESPACE,
            label_selector="serving.kserve.io/inferenceservice={}".format(service_name),
        )
        for pod in pods.items:
            print(pod)
        raise e
    is_ready = await is_model_ready(rest_v1_client, service_name, model_name) is True
    assert is_ready is True
    res = await predict_isvc(
        rest_v1_client,
        service_name,
        "./data/sentiment.json",
        model_name=model_name,
        network_layer=network_layer,
    )
    assert_sentiment_predictions(res)
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.collocation
@pytest.mark.asyncio(scope="session")
async def test_transformer_collocation_runtime(rest_v1_client, network_layer):
    service_name = "custom-model-trans-collocation-runtime"
    model_name = "sentiment-analysis"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="onnx",
            ),
            storage_uri=SENTIMENT_STORAGE_URI,
            protocol_version="v2",
            resources=V1ResourceRequirements(
                requests={"cpu": "100m", "memory": "512Mi"},
                limits={"cpu": "1", "memory": "2Gi"},
            ),
        ),
        containers=[
            V1Container(
                name=TRANSFORMER_CONTAINER,
                image=os.environ.get("IMAGE_TRANSFORMER_IMG_TAG"),
                args=[
                    f"--model_name={model_name}",
                    "--http_port=8090",
                    "--grpc_port=8091",
                    "--predictor_host=localhost:8085",
                    "--predictor_protocol=v2",
                    "--enable_predictor_health_check",
                ]
                + SENTIMENT_TRANSFORMER_ARGS,
                ports=[V1ContainerPort(container_port=8090, protocol="TCP")],
                resources=V1ResourceRequirements(
                    requests={"cpu": "10m", "memory": "128Mi"},
                    limits={"cpu": "100m", "memory": "1Gi"},
                ),
                readiness_probe=client.V1Probe(
                    http_get=client.V1HTTPGetAction(
                        path=f"/v1/models/{model_name}", port=8090
                    )
                ),
            ),
        ],
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
    try:
        kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)
    except RuntimeError as e:
        print(
            kserve_client.api_instance.get_namespaced_custom_object(
                "serving.knative.dev",
                "v1",
                KSERVE_TEST_NAMESPACE,
                "services",
                service_name + "-predictor",
            )
        )
        pods = kserve_client.core_api.list_namespaced_pod(
            KSERVE_TEST_NAMESPACE,
            label_selector="serving.kserve.io/inferenceservice={}".format(service_name),
        )
        for pod in pods.items:
            print(pod)
        raise e
    is_ready = await is_model_ready(rest_v1_client, service_name, model_name) is True
    assert is_ready is True
    res = await predict_isvc(
        rest_v1_client,
        service_name,
        "./data/sentiment.json",
        model_name=model_name,
        network_layer=network_layer,
    )
    assert_sentiment_predictions(res)
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.raw
@pytest.mark.asyncio(scope="session")
async def test_raw_transformer_collocation(rest_v1_client, network_layer):
    suffix = str(uuid.uuid4())[1:6]
    service_name = "raw-custom-model-collocation-" + suffix
    model_name = "sentiment-analysis"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        containers=[
            V1Container(
                name=INFERENCESERVICE_CONTAINER,
                image="openvino/model_server:2024.5",
                env=[
                    V1EnvVar(
                        name=STORAGE_URI_ENV,
                        value=SENTIMENT_STORAGE_URI,
                    ),
                ],
                args=[
                    f"--model_name={model_name}",
                    "--model_path=/mnt/models",
                    "--port=9000",
                    "--rest_port=8085",
                    "--file_system_poll_wait_seconds=0",
                ],
                resources=V1ResourceRequirements(
                    requests={"cpu": "100m", "memory": "512Mi"},
                    limits={"cpu": "1", "memory": "2Gi"},
                ),
            ),
            V1Container(
                name=TRANSFORMER_CONTAINER,
                image=os.environ.get("IMAGE_TRANSFORMER_IMG_TAG"),
                args=[
                    f"--model_name={model_name}",
                    "--http_port=8080",
                    "--grpc_port=8081",
                    "--predictor_host=localhost:8085",
                    "--predictor_protocol=v2",
                    "--enable_predictor_health_check",
                ]
                + SENTIMENT_TRANSFORMER_ARGS,
                ports=[
                    V1ContainerPort(name="http", container_port=8080, protocol="TCP"),
                    V1ContainerPort(name="grpc", container_port=8081, protocol="TCP"),
                ],
                resources=V1ResourceRequirements(
                    requests={"cpu": "10m", "memory": "128Mi"},
                    limits={"cpu": "100m", "memory": "1Gi"},
                ),
            ),
        ],
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name,
            namespace=KSERVE_TEST_NAMESPACE,
            annotations={"serving.kserve.io/deploymentMode": "Standard"},
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(isvc)
    try:
        kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)
    except RuntimeError as e:
        print(
            kserve_client.api_instance.get_namespaced_custom_object(
                "serving.knative.dev",
                "v1",
                KSERVE_TEST_NAMESPACE,
                "services",
                service_name + "-predictor",
            )
        )
        pods = kserve_client.core_api.list_namespaced_pod(
            KSERVE_TEST_NAMESPACE,
            label_selector="serving.kserve.io/inferenceservice={}".format(service_name),
        )
        for pod in pods.items:
            print(pod)
        raise e
    is_ready = (
        await is_model_ready(
            rest_v1_client, service_name, model_name, network_layer=network_layer
        )
        is True
    )
    assert is_ready is True
    res = await predict_isvc(
        rest_v1_client,
        service_name,
        "./data/sentiment.json",
        model_name=model_name,
        network_layer=network_layer,
    )
    assert_sentiment_predictions(res)
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.raw
@pytest.mark.asyncio(scope="session")
async def test_raw_transformer_collocation_runtime(rest_v1_client, network_layer):
    suffix = str(uuid.uuid4())[1:5]
    service_name = "raw-custom-pred-collocation-" + suffix
    model_name = "sentiment-analysis"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="onnx",
            ),
            storage_uri=SENTIMENT_STORAGE_URI,
            protocol_version="v2",
            resources=V1ResourceRequirements(
                requests={"cpu": "100m", "memory": "512Mi"},
                limits={"cpu": "1", "memory": "2Gi"},
            ),
        ),
        containers=[
            V1Container(
                name=TRANSFORMER_CONTAINER,
                image=os.environ.get("IMAGE_TRANSFORMER_IMG_TAG"),
                args=[
                    f"--model_name={model_name}",
                    "--http_port=8090",
                    "--grpc_port=8091",
                    "--predictor_host=localhost:8085",
                    "--predictor_protocol=v2",
                    "--enable_predictor_health_check",
                ]
                + SENTIMENT_TRANSFORMER_ARGS,
                ports=[V1ContainerPort(container_port=8090, protocol="TCP")],
                resources=V1ResourceRequirements(
                    requests={"cpu": "10m", "memory": "128Mi"},
                    limits={"cpu": "100m", "memory": "1Gi"},
                ),
                readiness_probe=client.V1Probe(
                    http_get=client.V1HTTPGetAction(
                        path=f"/v1/models/{model_name}", port=8090
                    )
                ),
            ),
        ],
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name,
            namespace=KSERVE_TEST_NAMESPACE,
            annotations={"serving.kserve.io/deploymentMode": "Standard"},
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(isvc)
    try:
        kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)
    except RuntimeError as e:
        print(
            kserve_client.api_instance.get_namespaced_custom_object(
                "serving.knative.dev",
                "v1",
                KSERVE_TEST_NAMESPACE,
                "services",
                service_name + "-predictor",
            )
        )
        pods = kserve_client.core_api.list_namespaced_pod(
            KSERVE_TEST_NAMESPACE,
            label_selector="serving.kserve.io/inferenceservice={}".format(service_name),
        )
        for pod in pods.items:
            print(pod)
        raise e
    is_ready = (
        await is_model_ready(
            rest_v1_client, service_name, model_name, network_layer=network_layer
        )
        is True
    )
    assert is_ready is True
    res = await predict_isvc(
        rest_v1_client,
        service_name,
        "./data/sentiment.json",
        model_name=model_name,
        network_layer=network_layer,
    )
    assert_sentiment_predictions(res)
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
