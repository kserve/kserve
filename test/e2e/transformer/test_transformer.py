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
from kserve import V1beta1TransformerSpec
from kserve import V1beta1TorchServeSpec
from kserve import V1beta1InferenceServiceSpec
from kserve import V1beta1InferenceService
from kubernetes.client import V1ResourceRequirements
from kubernetes.client import V1Container
from kubernetes.client import V1EnvVar
import pytest
from ..common.utils import predict_isvc
from ..common.utils import KSERVE_TEST_NAMESPACE


@pytest.mark.transformer
@pytest.mark.asyncio(scope="session")
async def test_transformer(rest_v1_client):
    service_name = "isvc-transformer"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        pytorch=V1beta1TorchServeSpec(
            storage_uri="gs://kfserving-examples/models/torchserve/image_classifier/v1",
            protocol_version="v1",
            resources=V1ResourceRequirements(
                requests={"cpu": "10m", "memory": "128Mi"},
                limits={"cpu": "1", "memory": "1Gi"},
            ),
        ),
    )
    transformer = V1beta1TransformerSpec(
        min_replicas=1,
        containers=[
            V1Container(
                image=os.environ.get("IMAGE_TRANSFORMER_IMG_TAG"),
                name="kserve-container",
                resources=V1ResourceRequirements(
                    requests={"cpu": "10m", "memory": "128Mi"},
                    limits={"cpu": "100m", "memory": "1Gi"},
                ),
                args=["--model_name", "mnist"],
                env=[
                    V1EnvVar(
                        name="STORAGE_URI",
                        value="gs://kfserving-examples/models/torchserve/image_classifier/v1",
                    )
                ],
            )
        ],
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
    res = await predict_isvc(
        rest_v1_client, service_name, "./data/transformer.json", model_name="mnist"
    )
    assert res["predictions"][0] == 2
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
