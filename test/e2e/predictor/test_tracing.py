# Copyright 2026 The KServe Authors.
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

import json
import os

import pytest
from kserve import (
    KServeClient,
    V1beta1InferenceService,
    V1beta1InferenceServiceSpec,
    V1beta1PredictorSpec,
    V1beta1SKLearnSpec,
    constants,
)
from kserve.models.v1beta1_tracing_spec import V1beta1TracingSpec
from kubernetes import client
from kubernetes.client import V1ContainerPort, V1ResourceRequirements

from ..common.utils import (
    KSERVE_TEST_NAMESPACE,
    predict_grpc,
    predict_isvc,
    wait_for_pod_logs,
)


@pytest.mark.predictor
@pytest.mark.tracing
@pytest.mark.asyncio(scope="session")
async def test_sklearn_traces_rest_inference_in_knative_mode(
    rest_v2_client, network_layer
):
    service_name = "sklearn-tracing-rest"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        sklearn=V1beta1SKLearnSpec(
            storage_uri="gs://kfserving-examples/models/sklearn/1.0/model",
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "256Mi"},
            ),
        ),
    )
    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(
            predictor=predictor,
            tracing=V1beta1TracingSpec(exporter="console", sampler="always_on"),
        ),
    )
    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    try:
        kserve_client.create(isvc)
        kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

        response = await predict_isvc(
            rest_v2_client,
            service_name,
            "./data/iris_input_v2.json",
            network_layer=network_layer,
        )
        assert response.outputs[0].data == [1, 1]

        pods = kserve_client.core_api.list_namespaced_pod(
            KSERVE_TEST_NAMESPACE,
            label_selector=f"serving.kserve.io/inferenceservice={service_name}",
        )
        assert len(pods.items) == 1
        logs = await wait_for_pod_logs(
            kserve_client.core_api,
            pods.items[0].metadata.name,
            KSERVE_TEST_NAMESPACE,
            expected_substring=f"POST /v2/models/{service_name}/infer",
        )
        assert f"POST /v2/models/{service_name}/infer" in logs
    finally:
        kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


# The dual-protocol suite runs in Standard mode with a Gateway API provider,
# which is required to expose both the REST and gRPC ports.
@pytest.mark.tracing
@pytest.mark.dual_protocol
@pytest.mark.asyncio(scope="session")
async def test_sklearn_traces_rest_and_grpc_inference(rest_v2_client, network_layer):
    service_name = "sklearn-tracing"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        sklearn=V1beta1SKLearnSpec(
            storage_uri="gs://kfserving-examples/models/sklearn/1.0/model",
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "256Mi"},
            ),
            ports=[
                V1ContainerPort(container_port=8080, name="http-rest", protocol="TCP"),
                V1ContainerPort(container_port=8081, name="grpc-port", protocol="TCP"),
            ],
        ),
    )
    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(
            predictor=predictor,
            tracing=V1beta1TracingSpec(
                exporter="console",
                sampler="always_on",
            ),
        ),
    )
    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    try:
        kserve_client.create(isvc)
        kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

        rest_response = await predict_isvc(
            rest_v2_client,
            service_name,
            "./data/iris_input_v2.json",
            network_layer=network_layer,
        )
        assert rest_response.outputs[0].data == [1, 1]

        with open("./data/iris_input_v2_grpc.json") as input_file:
            grpc_inputs = json.load(input_file)["inputs"]
        grpc_response = await predict_grpc(
            service_name=service_name,
            payload=grpc_inputs,
            model_name=service_name,
            network_layer=network_layer,
        )
        assert grpc_response.outputs[0].data == [1, 1]

        pods = kserve_client.core_api.list_namespaced_pod(
            KSERVE_TEST_NAMESPACE,
            label_selector=f"serving.kserve.io/inferenceservice={service_name}",
        )
        assert len(pods.items) == 1
        logs = await wait_for_pod_logs(
            kserve_client.core_api,
            pods.items[0].metadata.name,
            KSERVE_TEST_NAMESPACE,
            expected_substring="/inference.GRPCInferenceService/ModelInfer",
        )
        assert f"POST /v2/models/{service_name}/infer" in logs
    finally:
        kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
