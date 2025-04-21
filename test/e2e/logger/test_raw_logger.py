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
import uuid
from kubernetes import client

from kserve import (
    KServeClient,
    constants,
    V1beta1PredictorSpec,
    V1beta1SKLearnSpec,
    V1beta1InferenceServiceSpec,
    V1beta1InferenceService,
    V1beta1LoggerSpec,
)
from kubernetes.client import V1ResourceRequirements
from kubernetes.client import V1Container
import pytest
from ..common.utils import predict_isvc
from ..common.utils import KSERVE_TEST_NAMESPACE


kserve_client = KServeClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))
annotations = {"serving.kserve.io/deploymentMode": "RawDeployment"}
labels = {"networking.kserve.io/visibility": "exposed"}


@pytest.mark.raw
@pytest.mark.asyncio(scope="session")
async def test_kserve_logger(rest_v1_client, network_layer):
    suffix = str(uuid.uuid4())[1:6]
    msg_dumper = "message-dumper-raw-" + suffix
    before(msg_dumper)

    service_name = "isvc-logger-raw-" + suffix
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        logger=V1beta1LoggerSpec(
            mode="all",
            url="http://"
            + msg_dumper
            + "-predictor"
            + "."
            + KSERVE_TEST_NAMESPACE
            + ".svc.cluster.local",
        ),
        sklearn=V1beta1SKLearnSpec(
            storage_uri="gs://kfserving-examples/models/sklearn/1.0/model",
            resources=V1ResourceRequirements(
                requests={"cpu": "10m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "256Mi"},
            ),
        ),
    )
    await base_test(msg_dumper, service_name, predictor, rest_v1_client, network_layer)


@pytest.mark.rawcipn
async def test_kserve_logger_cipn(rest_v1_client, network_layer):
    msg_dumper = "message-dumper-raw-cipn"
    before(msg_dumper)

    service_name = "isvc-logger-raw-cipn"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        logger=V1beta1LoggerSpec(
            mode="all",
            url="http://"
            + msg_dumper
            + "-predictor"
            + "."
            + KSERVE_TEST_NAMESPACE
            + ".svc.cluster.local:8080",
        ),
        sklearn=V1beta1SKLearnSpec(
            storage_uri="gs://kfserving-examples/models/sklearn/1.0/model",
            resources=V1ResourceRequirements(
                requests={"cpu": "10m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "256Mi"},
            ),
        ),
    )
    await base_test(msg_dumper, service_name, predictor, rest_v1_client, network_layer)


def before(msg_dumper):
    predictor = V1beta1PredictorSpec(
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

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=msg_dumper,
            namespace=KSERVE_TEST_NAMESPACE,
            annotations=annotations,
            labels=labels,
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(msg_dumper, namespace=KSERVE_TEST_NAMESPACE)


async def base_test(msg_dumper, service_name, predictor, rest_v1_client, network_layer):
    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name,
            namespace=KSERVE_TEST_NAMESPACE,
            annotations=annotations,
            labels=labels,
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client.create(isvc)
    try:
        kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)
    except RuntimeError:
        pods = kserve_client.core_api.list_namespaced_pod(
            KSERVE_TEST_NAMESPACE,
            label_selector="serving.kserve.io/inferenceservice={}".format(service_name),
        )
        for pod in pods.items:
            print(pod)

    res = await predict_isvc(
        rest_v1_client,
        service_name,
        "./data/iris_input.json",
        network_layer=network_layer,
    )
    assert res["predictions"] == [1, 1]
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
