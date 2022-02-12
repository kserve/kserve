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
from kserve import V1beta1TorchServeSpec
from kserve import V1beta1InferenceServiceSpec
from kserve import V1beta1InferenceService
from kubernetes.client import V1ResourceRequirements
from ..common.utils import predict
from ..common.utils import KSERVE_TEST_NAMESPACE
from concurrent import futures

kserve_client = KServeClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))


def test_batcher():
    service_name = 'isvc-pytorch-batcher'
    predictor = V1beta1PredictorSpec(
        batcher=V1beta1Batcher(
            max_batch_size=32,
            max_latency=5000,
        ),
        min_replicas=1,
        pytorch=V1beta1TorchServeSpec(
            storage_uri="gs://kfserving-examples/models/torchserve/image_classifier/v1",
            resources=V1ResourceRequirements(
                requests={'cpu': '1', 'memory': '4Gi'},
                limits={'cpu': '1', 'memory': '4Gi'}
            )
        )
    )

    isvc = V1beta1InferenceService(api_version=constants.KSERVE_V1BETA1,
                                   kind=constants.KSERVE_KIND,
                                   metadata=client.V1ObjectMeta(
                                       name=service_name,
                                       namespace=KSERVE_TEST_NAMESPACE
                                   ),
                                   spec=V1beta1InferenceServiceSpec(predictor=predictor))
    kserve_client.create(isvc)
    try:
        kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)
    except RuntimeError as e:
        print(kserve_client.api_instance.get_namespaced_custom_object("serving.knative.dev", "v1",
                                                                      KSERVE_TEST_NAMESPACE,
                                                                      "services", service_name + "-predictor-default"))
        pods = kserve_client.core_api.list_namespaced_pod(KSERVE_TEST_NAMESPACE,
                                                          label_selector='serving.kserve.io/inferenceservice={}'.
                                                          format(service_name))
        for pod in pods.items:
            print(pod)
        raise e
    with futures.ThreadPoolExecutor(max_workers=4) as executor:
        future_res = [
            executor.submit(lambda: predict(service_name, './data/torchserve_batch_input.json')) for _ in range(4)
        ]
    results = [
        f.result()["batchId"] for f in future_res
    ]
    assert (all(x == results[0] for x in results))
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
