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
from kubernetes import client

from kserve import KServeClient
from kserve import constants
from kserve import V1beta1PredictorSpec
from kserve import V1beta1SKLearnSpec
from kserve import V1beta1InferenceServiceSpec
from kserve import V1beta1InferenceService
from kserve import V1beta1LoggerSpec
from kubernetes.client import V1ResourceRequirements
from kubernetes.client import V1Container
import pytest
from ..common.utils import predict_isvc
from ..common.utils import KSERVE_TEST_NAMESPACE

kserve_client = KServeClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))


@pytest.mark.predictor
@pytest.mark.path_based_routing
@pytest.mark.asyncio(scope="session")
async def test_kserve_logger(rest_v1_client):
    msg_dumper = "message-dumper"
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
        metadata=client.V1ObjectMeta(name=msg_dumper, namespace=KSERVE_TEST_NAMESPACE),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(msg_dumper, namespace=KSERVE_TEST_NAMESPACE)

    service_name = "isvc-logger"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        logger=V1beta1LoggerSpec(
            mode="all",
            url=f"http://{msg_dumper}." + KSERVE_TEST_NAMESPACE + ".svc.cluster.local",
        ),
        sklearn=V1beta1SKLearnSpec(
            storage_uri="gs://kfserving-examples/models/sklearn/1.0/model",
            resources=V1ResourceRequirements(
                requests={"cpu": "10m", "memory": "128Mi"},
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

    res = await predict_isvc(rest_v1_client, service_name, "./data/iris_input.json")
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
