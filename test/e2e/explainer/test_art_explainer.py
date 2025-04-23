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


import logging
import os
import uuid

from kubernetes import client
from kubernetes.client import V1ResourceRequirements
import pytest

from kserve import KServeClient
from kserve import constants
from kserve import V1beta1PredictorSpec
from kserve import V1beta1InferenceServiceSpec
from kserve import V1beta1ExplainerSpec
from kserve import V1beta1SKLearnSpec
from kserve import V1beta1ARTExplainerSpec
from kserve import V1beta1InferenceService

from ..common.utils import predict_isvc
from ..common.utils import explain_art
from ..common.utils import KSERVE_TEST_NAMESPACE

kserve_client = KServeClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))

pytest.skip("ODH does not support art explainer at the moment", allow_module_level=True)


@pytest.mark.path_based_routing
@pytest.mark.explainer
@pytest.mark.asyncio(scope="session")
async def test_tabular_explainer(rest_v1_client):
    service_name = "art-explainer"
    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(
            predictor=V1beta1PredictorSpec(
                sklearn=V1beta1SKLearnSpec(
                    storage_uri="gs://kfserving-examples/models/sklearn/mnist/art",
                    resources=V1ResourceRequirements(
                        requests={"cpu": "10m", "memory": "128Mi"},
                        limits={"cpu": "100m", "memory": "256Mi"},
                    ),
                ),
                timeout=180,
            ),
            explainer=V1beta1ExplainerSpec(
                min_replicas=1,
                art=V1beta1ARTExplainerSpec(
                    type="SquareAttack",
                    name="explainer",
                    resources=V1ResourceRequirements(
                        requests={"cpu": "10m", "memory": "256Mi"},
                        limits={"cpu": "100m", "memory": "512Mi"},
                    ),
                    config={"nb_classes": "10"},
                ),
                timeout=180,
            ),
        ),
    )

    kserve_client.create(isvc)
    try:
        kserve_client.wait_isvc_ready(
            service_name, namespace=KSERVE_TEST_NAMESPACE, timeout_seconds=720
        )
    except RuntimeError as e:
        logging.info(
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
            logging.info(pod)
        raise e

    res = await predict_isvc(
        rest_v1_client, service_name, "./data/mnist_input_bw_flat.json"
    )
    assert res["predictions"] == [3]

    adv_prediction = await explain_art(
        rest_v1_client, service_name, "./data/mnist_input_bw.json"
    )
    assert adv_prediction != 3
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.raw
@pytest.mark.asyncio(scope="session")
async def test_raw_tabular_explainer(rest_v1_client, network_layer):
    suffix = str(uuid.uuid4())[1:6]
    service_name = "art-explainer-raw-" + suffix
    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name,
            namespace=KSERVE_TEST_NAMESPACE,
            annotations={"serving.kserve.io/deploymentMode": "RawDeployment"},
            labels={"networking.kserve.io/visibility": "exposed"},
        ),
        spec=V1beta1InferenceServiceSpec(
            predictor=V1beta1PredictorSpec(
                sklearn=V1beta1SKLearnSpec(
                    storage_uri="gs://kfserving-examples/models/sklearn/mnist/art",
                    resources=V1ResourceRequirements(
                        requests={"cpu": "10m", "memory": "128Mi"},
                        limits={"cpu": "100m", "memory": "256Mi"},
                    ),
                ),
                timeout=180,
            ),
            explainer=V1beta1ExplainerSpec(
                min_replicas=1,
                art=V1beta1ARTExplainerSpec(
                    type="SquareAttack",
                    name="explainer",
                    resources=V1ResourceRequirements(
                        requests={"cpu": "10m", "memory": "256Mi"},
                        limits={"cpu": "100m", "memory": "512Mi"},
                    ),
                    config={"nb_classes": "10"},
                ),
                timeout=180,
            ),
        ),
    )

    kserve_client.create(isvc)
    try:
        kserve_client.wait_isvc_ready(
            service_name, namespace=KSERVE_TEST_NAMESPACE, timeout_seconds=720
        )
    except RuntimeError as e:
        logging.info(
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
            logging.info(pod)
        raise e

    res = await predict_isvc(
        rest_v1_client,
        service_name,
        "./data/mnist_input_bw_flat.json",
        network_layer=network_layer,
    )
    assert res["predictions"] == [3]

    adv_prediction = await explain_art(
        rest_v1_client,
        service_name,
        "./data/mnist_input_bw.json",
        network_layer=network_layer,
    )
    assert adv_prediction != 3
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
