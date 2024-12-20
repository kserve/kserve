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
from kubernetes import client

from kserve import KServeClient
from kserve import constants
from kserve import V1beta1PredictorSpec
from kserve import V1beta1Batcher
from kserve import V1beta1SKLearnSpec
from kserve import V1beta1InferenceServiceSpec
from kserve import V1beta1InferenceService

from kubernetes.client import V1ResourceRequirements, V1ContainerPort
import pytest
from ..common.utils import predict_isvc
from ..common.utils import KSERVE_TEST_NAMESPACE

kserve_client = KServeClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))


@pytest.mark.predictor
@pytest.mark.asyncio(scope="session")
async def test_batcher_custom_port(rest_v1_client):
    service_name = "isvc-sklearn-batcher-custom"

    predictor = V1beta1PredictorSpec(
        batcher=V1beta1Batcher(
            max_batch_size=32,
            max_latency=5000,
        ),
        min_replicas=1,
        sklearn=V1beta1SKLearnSpec(
            args=["--http_port=5000"],
            storage_uri="gs://kfserving-examples/models/sklearn/1.0/model",
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "256Mi"},
            ),
            ports=[V1ContainerPort(container_port=5000, protocol="TCP")],
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
    results = await predict_isvc(
        rest_v1_client, service_name, "./data/iris_batch_input.json", is_batch=True
    )
    assert all(x == results[0] for x in results)
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
