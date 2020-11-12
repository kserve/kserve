# Copyright 2020 kubeflow.org.
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

from kfserving import KFServingClient
from kfserving import constants
from kfserving import V1alpha2EndpointSpec
from kfserving import V1alpha2PredictorSpec
from kfserving import V1alpha2Batcher
from kfserving import V1alpha2PyTorchSpec
from kfserving import V1alpha2InferenceServiceSpec
from kfserving import V1alpha2InferenceService
from kubernetes.client import V1ResourceRequirements
from ..common.utils import predict
from ..common.utils import KFSERVING_TEST_NAMESPACE
from concurrent import futures

api_version = constants.KFSERVING_GROUP + '/' + constants.KFSERVING_VERSION
KFServing = KFServingClient(config_file=os.environ.get("KUBECONFIG","~/.kube/config"))


def test_batcher():
    service_name = 'isvc-pytorch-batcher'
    default_endpoint_spec = V1alpha2EndpointSpec(
        predictor=V1alpha2PredictorSpec(
            batcher=V1alpha2Batcher(
                max_batch_size=32,
                max_latency=5000,
                timeout=60
            ),
            min_replicas=1,
            pytorch=V1alpha2PyTorchSpec(
                storage_uri='gs://kfserving-samples/models/pytorch/cifar10',
                model_class_name='Net',
                resources=V1ResourceRequirements(
                    requests={'cpu': '100m', 'memory': '2Gi'},
                    limits={'cpu': '100m', 'memory': '2Gi'}))))

    isvc = V1alpha2InferenceService(api_version=api_version,
                                    kind=constants.KFSERVING_KIND,
                                    metadata=client.V1ObjectMeta(
                                        name=service_name,
                                        namespace=KFSERVING_TEST_NAMESPACE
                                    ),
                                    spec=V1alpha2InferenceServiceSpec(default=default_endpoint_spec))
    KFServing.create(isvc)
    try:
        KFServing.wait_isvc_ready(service_name, namespace=KFSERVING_TEST_NAMESPACE)
    except RuntimeError as e:
        print(KFServing.api_instance.get_namespaced_custom_object("serving.knative.dev", "v1", KFSERVING_TEST_NAMESPACE,
                                                                  "services", service_name + "-predictor"))
        pods = KFServing.core_api.list_namespaced_pod(KFSERVING_TEST_NAMESPACE,
                                                      label_selector='serving.kubeflow.org/inferenceservice={}'.
                                                      format(service_name))
        for pod in pods.items:
            print(pod)
        raise e
    with futures.ThreadPoolExecutor(max_workers=4) as executor:
        future_res = [
            executor.submit(lambda: predict(service_name, './data/cifar_input.json')) for _ in range(4)
        ]
    results = [
        f.result()["batchId"] for f in future_res
    ]
    assert(all(x == results[0] for x in results) == True)
    KFServing.delete(service_name, KFSERVING_TEST_NAMESPACE)
