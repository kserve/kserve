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

import asyncio
import os

import pytest
import requests
from portforward import AsyncPortForwarder
from kubernetes import client
from kserve import (
    constants,
    KServeClient,
    V1beta1InferenceService,
    V1beta1InferenceServiceSpec,
    V1beta1PredictorSpec,
    V1beta1SKLearnSpec,
)
from kserve.logging import logger
from kubernetes.client import V1ResourceRequirements

from ..common.utils import KSERVE_TEST_NAMESPACE, get_cluster_ip
from ..common.utils import predict_isvc


ENABLE_METRIC_AGG = "serving.kserve.io/enable-metric-aggregation"
METRICS_AGG_PORT = 9088
METRICS_PATH = "metrics"


@pytest.mark.asyncio(scope="session")
async def test_qpext_kserve(rest_v2_client):
    # test the qpext using the sklearn predictor
    service_name = "sklearn-v2-metrics"
    protocol_version = "v2"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        sklearn=V1beta1SKLearnSpec(
            storage_uri="gs://kfserving-examples/models/sklearn/1.0/model",
            protocol_version=protocol_version,
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "512Mi"},
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
        metadata=client.V1ObjectMeta(
            name=service_name,
            namespace=KSERVE_TEST_NAMESPACE,
            # set the metric aggregation annotation to true
            annotations={ENABLE_METRIC_AGG: "true"},
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)
    kserve_client.wait_model_ready(
        service_name,
        model_name=service_name,
        isvc_namespace=KSERVE_TEST_NAMESPACE,
        isvc_version=constants.KSERVE_V1BETA1_VERSION,
        protocol_version=protocol_version,
        cluster_ip=get_cluster_ip(),
    )

    res = await predict_isvc(
        rest_v2_client,
        service_name,
        "./data/iris_input_v2.json",
    )
    assert res.outputs[0].data == [1, 1]

    await send_metrics_request(kserve_client, service_name)
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


async def send_metrics_request(kserve_client, service_name):
    await asyncio.sleep(10)
    pods = kserve_client.core_api.list_namespaced_pod(
        KSERVE_TEST_NAMESPACE,
        label_selector="serving.kserve.io/inferenceservice={}".format(service_name),
    )
    pod_name = ""
    for pod in pods.items:
        # get a pod name
        pod_name = pod.metadata.name
        break

    url = f"http://localhost:{METRICS_AGG_PORT}/{METRICS_PATH}"
    port_forwarder = AsyncPortForwarder(
        KSERVE_TEST_NAMESPACE, pod_name, METRICS_AGG_PORT, METRICS_AGG_PORT
    )
    await port_forwarder.forward()
    logger.info(f"metrics request url: {url}")
    response = requests.get(url)
    await port_forwarder.stop()
    logger.info(f"response: {response}, content: {response.content}")
    logger.info(
        "Got response code %s, content %s", response.status_code, response.content
    )

    assert response.status_code == 200
    assert len(response.content) > 0
