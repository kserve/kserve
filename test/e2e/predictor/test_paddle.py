# Copyright 2021 kubeflow.org.
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
import numpy as np

from kubernetes.client import (
    V1ResourceRequirements,
    V1ObjectMeta,
)

from kfserving import (
    constants,
    KFServingClient,
    V1beta1PredictorSpec,
    V1beta1InferenceService,
    V1beta1InferenceServiceSpec,
    V1beta1PaddleServerSpec,
)
from ..common.utils import KFSERVING_TEST_NAMESPACE, predict

logging.basicConfig(level=logging.INFO)
KFServing = KFServingClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))


def test_paddle():
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        paddle=V1beta1PaddleServerSpec(
            storage_uri="https://zhouti-mcp-edge.cdn.bcebos.com/resnet50.tar.gz",
            resources=V1ResourceRequirements(
                requests={"cpu": "200m", "memory": "4Gi"},
                limits={"cpu": "200m", "memory": "4Gi"},
            )
        )
    )

    service_name = 'isvc-paddle'
    isvc = V1beta1InferenceService(
        api_version=constants.KFSERVING_V1BETA1,
        kind=constants.KFSERVING_KIND,
        metadata=V1ObjectMeta(
            name=service_name, namespace=KFSERVING_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor)
    )

    KFServing.create(isvc)
    try:
        KFServing.wait_isvc_ready(service_name, namespace=KFSERVING_TEST_NAMESPACE, timeout_seconds=720)
    except RuntimeError as e:
        pods = KFServing.core_api.list_namespaced_pod(KFSERVING_TEST_NAMESPACE,
                                                      label_selector='serving.kubeflow.org/inferenceservice={}'.format(
                                                          service_name))
        for pod in pods.items:
            logging.info(pod)
        raise e

    res = predict(service_name, './data/jay.json')
    assert np.argmax(res["predictions"][0]) == 17

    KFServing.delete(service_name, KFSERVING_TEST_NAMESPACE)
