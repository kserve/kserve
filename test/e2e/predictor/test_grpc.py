# Copyright 2022 The KServe Authors.
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
import base64
import json
import os

import pytest
from kserve import (
    KServeClient,
    V1beta1InferenceService,
    V1beta1InferenceServiceSpec,
    V1beta1PredictorSpec,
    V1beta1LoggerSpec,
    constants,
)
from kubernetes.client import V1ResourceRequirements
from kubernetes import client
from kubernetes.client import V1Container, V1ContainerPort
from ..common.utils import KSERVE_TEST_NAMESPACE, predict_grpc

pytest.skip("Not testable in ODH at the moment", allow_module_level=True)


@pytest.mark.grpc
@pytest.mark.predictor
@pytest.mark.asyncio(scope="session")
async def test_custom_model_grpc():
    service_name = "custom-grpc-logger"
    model_name = "custom-model"

    msg_dumper = "message-dumper-grpc"
    logger_predictor = V1beta1PredictorSpec(
        min_replicas=1,
        containers=[
            V1Container(
                name="kserve-container",
                image="gcr.io/knative-releases/knative.dev/eventing-contrib/cmd/event_display",
                resources=V1ResourceRequirements(
                    requests={"cpu": "10m", "memory": "128Mi"},
                    limits={"cpu": "100m", "memory": "256Mi"},
                ),
            )
        ],
    )

    logger_isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(name=msg_dumper, namespace=KSERVE_TEST_NAMESPACE),
        spec=V1beta1InferenceServiceSpec(predictor=logger_predictor),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(logger_isvc)
    kserve_client.wait_isvc_ready(msg_dumper, namespace=KSERVE_TEST_NAMESPACE)

    predictor = V1beta1PredictorSpec(
        logger=V1beta1LoggerSpec(
            mode="all",
            url=f"http://{msg_dumper}." + KSERVE_TEST_NAMESPACE + ".svc.cluster.local",
        ),
        containers=[
            V1Container(
                name="kserve-container",
                image=os.environ.get("CUSTOM_MODEL_GRPC_IMG_TAG"),
                resources=V1ResourceRequirements(
                    requests={"cpu": "50m", "memory": "128Mi"},
                    limits={"cpu": "100m", "memory": "1Gi"},
                ),
                ports=[
                    V1ContainerPort(container_port=8081, name="h2c", protocol="TCP")
                ],
                args=["--model_name", model_name],
            )
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

    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

    json_file = open("./data/custom_model_input.json")
    data = json.load(json_file)
    payload = [
        {
            "name": "input-0",
            "shape": [],
            "datatype": "BYTES",
            "contents": {
                "bytes_contents": [
                    base64.b64decode(data["instances"][0]["image"]["b64"])
                ]
            },
        }
    ]
    response = await predict_grpc(
        service_name=service_name, payload=payload, model_name=model_name
    )
    fields = response.outputs[0].data
    points = ["%.3f" % (point) for point in fields]
    assert points == ["14.976", "14.037", "13.966", "12.252", "12.086"]

    pods = kserve_client.core_api.list_namespaced_pod(
        KSERVE_TEST_NAMESPACE,
        label_selector="serving.kserve.io/inferenceservice={}".format(msg_dumper),
    )
    await asyncio.sleep(5)
    log = ""
    for pod in pods.items:
        log += kserve_client.core_api.read_namespaced_pod_log(
            name=pod.metadata.name,
            namespace=pod.metadata.namespace,
            container="kserve-container",
        )
        print(log)
    assert "org.kubeflow.serving.inference.request" in log
    assert "org.kubeflow.serving.inference.response" in log
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
    kserve_client.delete(msg_dumper, KSERVE_TEST_NAMESPACE)
