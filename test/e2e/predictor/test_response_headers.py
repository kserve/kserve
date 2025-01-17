import os

import pytest
import requests
import logging
import time
import json

from kserve import (
    KServeClient,
    V1beta1InferenceService,
    V1beta1InferenceServiceSpec,
    V1beta1PredictorSpec,
    V1beta1TransformerSpec,
    constants,
)
from kubernetes.client import V1ResourceRequirements
from kubernetes import client
from kubernetes.client import V1Container, V1ContainerPort
from ..common.utils import KSERVE_TEST_NAMESPACE, get_isvc_endpoint


@pytest.mark.transformer
def test_predictor_headers_v1():
    service_name = "isvc-custom-model-transformer-v1"
    model_name = "custom-model"
    input_json = "./data/custom_model_input.json"

    predictor = V1beta1PredictorSpec(
        containers=[
            V1Container(
                name="kserve-container",
                image=os.environ.get("CUSTOM_MODEL_GRPC_IMG_TAG"),
                # Override the entrypoint to run the custom model rest server
                command=["python", "-m", "custom_model.model"],
                resources=V1ResourceRequirements(
                    requests={"cpu": "50m", "memory": "128Mi"},
                    limits={"cpu": "100m", "memory": "2Gi"},
                ),
                ports=[V1ContainerPort(container_port=8080, protocol="TCP")],
                args=["--model_name", model_name],
            )
        ]
    )

    transformer = V1beta1TransformerSpec(
        containers=[
            V1Container(
                name="kserve-container",
                image="kserve/custom-image-transformer-grpc:"
                + os.environ.get("GITHUB_SHA"),
                # Override the entrypoint to run the custom transformer rest server
                command=["python", "-m", "custom_transformer.model"],
                resources=V1ResourceRequirements(
                    requests={"cpu": "50m", "memory": "128Mi"},
                    limits={"cpu": "100m", "memory": "1Gi"},
                ),
                args=["--model_name", model_name, "--predictor_protocol", "v1"],
            )
        ]
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor, transformer=transformer),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

    isvc = kserve_client.get(
        service_name,
        namespace=KSERVE_TEST_NAMESPACE,
        version=constants.KSERVE_V1BETA1_VERSION,
    )
    scheme, cluster_ip, host, path = get_isvc_endpoint(isvc)
    headers = {"Host": host, "Content-Type": "application/json"}

    if model_name is None:
        model_name = service_name

    url = f"{scheme}://{cluster_ip}{path}/v1/models/{model_name}:predict"

    time.sleep(10)
    with open(input_json) as json_file:
        data = json.load(json_file)
        response = requests.post(url, json.dumps(data), headers=headers)
        logging.info(
            "Got response code %s, content %s", response.status_code, response.content
        )
        if response.status_code == 200:
            res_data = json.loads(response.content.decode("utf-8"))
        else:
            response.raise_for_status()

    assert "prediction-time-latency" in response.headers
    points = ["%.3f" % (point) for point in list(res_data["predictions"])]
    assert points == ["14.976", "14.037", "13.966", "12.252", "12.086"]
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.transformer
def test_predictor_headers_v2():
    service_name = "isvc-custom-model-transformer-v2"
    model_name = "custom-model"
    input_json = "./data/custom_model_input_v2.json"

    predictor = V1beta1PredictorSpec(
        containers=[
            V1Container(
                name="kserve-container",
                image=os.environ.get("CUSTOM_MODEL_GRPC_IMG_TAG"),
                # Override the entrypoint to run the custom model rest server
                command=["python", "-m", "custom_model.model"],
                resources=V1ResourceRequirements(
                    requests={"cpu": "50m", "memory": "128Mi"},
                    limits={"cpu": "100m", "memory": "2Gi"},
                ),
                ports=[V1ContainerPort(container_port=8080, protocol="TCP")],
                args=["--model_name", model_name],
            )
        ]
    )

    transformer = V1beta1TransformerSpec(
        containers=[
            V1Container(
                name="kserve-container",
                image="kserve/custom-image-transformer-grpc:"
                + os.environ.get("GITHUB_SHA"),
                # Override the entrypoint to run the custom transformer rest server
                command=["python", "-m", "custom_transformer.model"],
                resources=V1ResourceRequirements(
                    requests={"cpu": "50m", "memory": "128Mi"},
                    limits={"cpu": "100m", "memory": "1Gi"},
                ),
                args=["--model_name", model_name, "--predictor_protocol", "v2"],
            )
        ]
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor, transformer=transformer),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)
    isvc = kserve_client.get(
        service_name,
        namespace=KSERVE_TEST_NAMESPACE,
        version=constants.KSERVE_V1BETA1_VERSION,
    )
    scheme, cluster_ip, host, path = get_isvc_endpoint(isvc)
    headers = {"Host": host, "Content-Type": "application/json"}

    if model_name is None:
        model_name = service_name

    url = f"{scheme}://{cluster_ip}{path}/v2/models/{model_name}/infer"

    time.sleep(10)
    with open(input_json) as json_file:
        data = json.load(json_file)
        response = requests.post(url, json.dumps(data), headers=headers)
        logging.info(
            "Got response code %s, content %s", response.status_code, response.content
        )
        if response.status_code == 200:
            res_data = json.loads(response.content.decode("utf-8"))
        else:
            response.raise_for_status()

    assert "prediction-time-latency" in response.headers
    points = ["%.3f" % (point) for point in list(res_data["outputs"][0]["data"])]
    assert points == ["14.976", "14.037", "13.966", "12.252", "12.086"]
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
