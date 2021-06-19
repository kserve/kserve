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
from kubernetes import client

from kfserving import KFServingClient
from kfserving import constants
from kfserving import V1beta1PredictorSpec
from kfserving import V1beta1TritonSpec
from kfserving import V1beta1InferenceServiceSpec
from kfserving import V1beta1InferenceService
from ..common.utils import KFSERVING_TEST_NAMESPACE

KFServing = KFServingClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))


def test_triton():
    service_name = 'isvc-triton'
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        triton=V1beta1TritonSpec(
            storage_uri='gs://kfserving-samples/models/tensorrt'
        )
    )

    isvc = V1beta1InferenceService(api_version=constants.KFSERVING_V1BETA1,
                                   kind=constants.KFSERVING_KIND,
                                   metadata=client.V1ObjectMeta(
                                       name=service_name, namespace=KFSERVING_TEST_NAMESPACE),
                                   spec=V1beta1InferenceServiceSpec(predictor=predictor))

    KFServing.create(isvc)
    try:
        KFServing.wait_isvc_ready(service_name, namespace=KFSERVING_TEST_NAMESPACE)
    except RuntimeError as e:
        print(KFServing.api_instance.get_namespaced_custom_object("serving.knative.dev", "v1", KFSERVING_TEST_NAMESPACE,
                                                                  "services", service_name + "-predictor-default"))
        deployments = KFServing.app_api. \
            list_namespaced_deployment(KFSERVING_TEST_NAMESPACE, label_selector='serving.kubeflow.org/'
                                                                                'inferenceservice={}'.
                                       format(service_name))
        for deployment in deployments.items:
            print(deployment)
        raise e
    KFServing.delete(service_name, KFSERVING_TEST_NAMESPACE)
