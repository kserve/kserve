import os

import pytest
from kubernetes import client
from kubernetes.client import V1ResourceRequirements

from kserve import (
    KServeClient,
    V1beta1InferenceService,
    V1beta1InferenceServiceSpec,
    V1beta1ModelFormat,
    V1beta1ModelSpec,
    V1beta1PredictorSpec,
    constants,
)
from ..common.utils import KSERVE_TEST_NAMESPACE, predict_isvc

AUTOGLOUON_STORAGE_URI_ENV = "KSERVE_TEST_AUTOGLOUON_STORAGE_URI"
AUTOGLOUON_V1_INPUT_ENV = "KSERVE_TEST_AUTOGLOUON_V1_INPUT"
AUTOGLOUON_V2_INPUT_ENV = "KSERVE_TEST_AUTOGLOUON_V2_INPUT"


def _get_autogluon_storage_uri() -> str:
    storage_uri = os.getenv(AUTOGLOUON_STORAGE_URI_ENV, "")
    if not storage_uri:
        pytest.skip(f"{AUTOGLOUON_STORAGE_URI_ENV} is not set")
    return storage_uri


@pytest.mark.predictor
@pytest.mark.asyncio(scope="session")
async def test_autogluon_runtime_kserve_v1(rest_v1_client):
    service_name = "isvc-autogluon-v1"
    storage_uri = _get_autogluon_storage_uri()
    payload_path = os.getenv(AUTOGLOUON_V1_INPUT_ENV, "./data/autogluon_iris_input.json")

    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(name="autogluon"),
            runtime="kserve-autogluonserver",
            storage_uri=storage_uri,
            resources=V1ResourceRequirements(
                requests={"cpu": "100m", "memory": "1Gi"},
                limits={"cpu": "1", "memory": "2Gi"},
            ),
        ),
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(name=service_name, namespace=KSERVE_TEST_NAMESPACE),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

    res = await predict_isvc(rest_v1_client, service_name, payload_path)
    assert "predictions" in res
    assert len(res["predictions"]) > 0

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.predictor
@pytest.mark.asyncio(scope="session")
async def test_autogluon_runtime_kserve_v2(rest_v2_client):
    service_name = "isvc-autogluon-v2"
    storage_uri = _get_autogluon_storage_uri()
    payload_path = os.getenv(
        AUTOGLOUON_V2_INPUT_ENV, "./data/autogluon_iris_input_v2.json"
    )

    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(name="autogluon"),
            runtime="kserve-autogluonserver",
            protocol_version="v2",
            storage_uri=storage_uri,
            resources=V1ResourceRequirements(
                requests={"cpu": "100m", "memory": "1Gi"},
                limits={"cpu": "1", "memory": "2Gi"},
            ),
            readiness_probe=client.V1Probe(
                http_get=client.V1HTTPGetAction(
                    path=f"/v2/models/{service_name}/ready", port=8080
                ),
                initial_delay_seconds=30,
            ),
        ),
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(name=service_name, namespace=KSERVE_TEST_NAMESPACE),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

    res = await predict_isvc(rest_v2_client, service_name, payload_path)
    assert len(res.outputs) > 0
    assert len(res.outputs[0].data) > 0

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
