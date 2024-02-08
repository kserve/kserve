import os

import pytest
from kserve import (
    KServeClient,
    V1beta1InferenceService,
    V1beta1InferenceServiceSpec,
    V1beta1PredictorSpec,
    V1beta1TransformerSpec,
    constants
)
from kubernetes.client import V1ResourceRequirements
from kubernetes import client
from kubernetes.client import V1Container, V1ContainerPort
from ..common.utils import KSERVE_TEST_NAMESPACE, predict


@pytest.mark.transformer
def test_predictor_headers_v1():
    service_name = "isvc-custom-model-transformer-v1"
    model_name = "custom-model"

    predictor = V1beta1PredictorSpec(
        containers=[
            V1Container(
                name="kserve-container",
                image="kserve/custom-model-rest:"
                      + os.environ.get("GITHUB_SHA"),
                resources=V1ResourceRequirements(
                    requests={"cpu": "50m", "memory": "128Mi"},
                    limits={"cpu": "100m", "memory": "1Gi"}),
                ports=[
                    V1ContainerPort(
                        container_port=8080,
                        protocol="TCP"
                    )],
                args=["--model_name", model_name]
            )
        ]
    )

    transformer = V1beta1TransformerSpec(
        containers=[
            V1Container(
                name="kserve-container",
                image="kserve/custom-image-transformer-rest:"
                      + os.environ.get("GITHUB_SHA"),
                resources=V1ResourceRequirements(
                    requests={"cpu": "50m", "memory": "128Mi"},
                    limits={"cpu": "100m", "memory": "1Gi"}),
                args=["--model_name", model_name, "--predictor_protocol", "v1"]
            )
        ]
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(
            predictor=predictor, transformer=transformer),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(
        service_name, namespace=KSERVE_TEST_NAMESPACE)

    res = predict(service_name, "./data/custom_model_input.json",
                  protocol_version="v1", model_name=model_name)
    response_headers = res["headers"]
    assert response_headers["my-header"] == "test_header"
    points = ['%.3f' % (point) for point in list(res["predictions"])]
    assert points == ["14.976", "14.037", "13.966", "12.252", "12.086"]
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.transformer
def test_predictor_headers_v2():
    service_name = "isvc-custom-model-transformer-v2"
    model_name = "custom-model"

    predictor = V1beta1PredictorSpec(
        containers=[
            V1Container(
                name="kserve-container",
                image="kserve/custom-model-rest:"
                      + os.environ.get("GITHUB_SHA"),
                resources=V1ResourceRequirements(
                    requests={"cpu": "50m", "memory": "128Mi"},
                    limits={"cpu": "100m", "memory": "1Gi"}),
                ports=[
                    V1ContainerPort(
                        container_port=8080,
                        protocol="TCP"
                    )],
                args=["--model_name", model_name]
            )
        ]
    )

    transformer = V1beta1TransformerSpec(
        containers=[
            V1Container(
                name="kserve-container",
                image="kserve/custom-image-transformer-rest:"
                      + os.environ.get("GITHUB_SHA"),
                resources=V1ResourceRequirements(
                    requests={"cpu": "50m", "memory": "128Mi"},
                    limits={"cpu": "100m", "memory": "1Gi"}),
                args=["--model_name", model_name, "--predictor_protocol", "v2"]
            )
        ]
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(
            predictor=predictor, transformer=transformer),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(
        service_name, namespace=KSERVE_TEST_NAMESPACE)

    res = predict(service_name, "./data/custom_model_input_v2.json",
                  protocol_version="v2", model_name=model_name)
    response_headers = res["headers"]
    assert response_headers["my-header"] == "test_header"
    points = ['%.3f' % (point) for point in list(res["outputs"][0]["data"])]
    assert points == ["14.976", "14.037", "13.966", "12.252", "12.086"]
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
