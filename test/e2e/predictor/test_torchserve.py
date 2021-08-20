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
from kserve import (
    constants,
    KServeClient,
    V1beta1InferenceService,
    V1beta1InferenceServiceSpec,
    V1beta1PredictorSpec,
    V1beta1TorchServeSpec,
)
from kubernetes.client import V1ResourceRequirements

from ..common.utils import predict
from ..common.utils import KSERVE_TEST_NAMESPACE

kserve_client = KServeClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))


def test_torchserve_kserve():
    service_name = "mnist"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        pytorch=V1beta1TorchServeSpec(
            storage_uri="gs://kfserving-examples/models/torchserve/image_classifier",
            protocol_version="v1",
            resources=V1ResourceRequirements(
                requests={"cpu": "1", "memory": "4Gi"},
                limits={"cpu": "1", "memory": "4Gi"},
            ),
        ),
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

    res = predict(service_name, "./data/torchserve_input.json")
    assert(res.get("predictions")[0] == 2)
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)

def test_torchserve_v2_kfserving():
    service_name = "mnist"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        pytorch=V1beta1TorchServeSpec(
            storage_uri="gs://kfserving-examples/models/torchserve/image_classifier",
            protocol_version="v2",
            resources=V1ResourceRequirements(
                requests={"cpu": "1", "memory": "4Gi"},
                limits={"cpu": "1", "memory": "4Gi"},
            ),
        ),
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KFSERVING_V1BETA1,
        kind=constants.KFSERVING_KIND,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

    res = predict(service_name, "./data/torchserve_input_v2.json")
    assert(res.get("outputs")[0]["data"] == [1])
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
