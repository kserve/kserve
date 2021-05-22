# Copyright 2019 kubeflow.org.
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
import numpy as np
from kubernetes import client

from kfserving import KFServingClient
from kfserving import constants
from kfserving import V1beta1PredictorSpec
from kfserving import V1beta1TransformerSpec
from kfserving import V1beta1TorchServeSpec
from kfserving import V1beta1InferenceServiceSpec
from kfserving import V1beta1InferenceService
from kubernetes.client import V1ResourceRequirements
from kubernetes.client import V1Container
from ..common.utils import predict
from ..common.utils import KFSERVING_TEST_NAMESPACE

api_version = constants.KFSERVING_GROUP + '/' + constants.KFSERVING_V1BETA1_VERSION
KFServing = KFServingClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))


def test_transformer():
    service_name = 'isvc-transformer'
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        pytorch=V1beta1TorchServeSpec(
            storage_uri='gs://kfserving-samples/models/pytorch/cifar10',
            model_class_name="Net",
            resources=V1ResourceRequirements(
                requests={'cpu': '100m', 'memory': '256Mi'},
                limits={'cpu': '100m', 'memory': '256Mi'}
            )
        ),
    )
    transformer = V1beta1TransformerSpec(
        min_replicas=1,
        containers=[V1Container(
                      image='809251082950.dkr.ecr.us-west-2.amazonaws.com/kfserving/image-transformer:latest',
                      name='kfserving-container',
                      resources=V1ResourceRequirements(
                          requests={'cpu': '100m', 'memory': '256Mi'},
                          limits={'cpu': '100m', 'memory': '256Mi'}))]
    )

    isvc = V1beta1InferenceService(api_version=api_version,
                                   kind=constants.KFSERVING_KIND,
                                   metadata=client.V1ObjectMeta(
                                       name=service_name, namespace=KFSERVING_TEST_NAMESPACE),
                                   spec=V1beta1InferenceServiceSpec(predictor=predictor, transformer=transformer))

    KFServing.create(isvc)
    try:
        KFServing.wait_isvc_ready(service_name, namespace=KFSERVING_TEST_NAMESPACE)
    except RuntimeError as e:
        print(KFServing.api_instance.get_namespaced_custom_object("serving.knative.dev", "v1", KFSERVING_TEST_NAMESPACE,
                                                                  "services", service_name + "-predictor-default"))
        pods = KFServing.core_api.list_namespaced_pod(KFSERVING_TEST_NAMESPACE,
                                                      label_selector='serving.kubeflow.org/inferenceservice={}'
                                                      .format(service_name))
        for pod in pods.items:
            print(pod)
        raise e
    res = predict(service_name, './data/transformer.json')
    assert(np.argmax(res["predictions"]) == 3)
    KFServing.delete(service_name, KFSERVING_TEST_NAMESPACE)
