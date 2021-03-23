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

import logging
import os
from kubernetes import client

from kfserving import KFServingClient
from kfserving import constants
from kfserving import V1alpha2EndpointSpec
from kfserving import V1alpha2PredictorSpec
from kfserving import V1alpha2SKLearnSpec
from kfserving import V1alpha2InferenceServiceSpec
from kfserving import V1alpha2ExplainerSpec
from kfserving import V1alpha2AlibiExplainerSpec
from kfserving import V1alpha2InferenceService
from kubernetes.client import V1ResourceRequirements

from ..common.utils import predict
from ..common.utils import explain
from ..common.utils import KFSERVING_TEST_NAMESPACE

logging.basicConfig(level=logging.INFO)
api_version = constants.KFSERVING_GROUP + '/' + constants.KFSERVING_VERSION
KFServing = KFServingClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))


def test_tabular_explainer():
    service_name = 'isvc-explainer-tabular'
    default_endpoint_spec = V1alpha2EndpointSpec(
        predictor=V1alpha2PredictorSpec(
            sklearn=V1alpha2SKLearnSpec(
                storage_uri='gs://seldon-models/sklearn/income/model',
                resources=V1ResourceRequirements(
                    requests={'cpu': '100m', 'memory': '1Gi'},
                    limits={'cpu': '100m', 'memory': '1Gi'}))),
        explainer=V1alpha2ExplainerSpec(
            min_replicas=1,
            alibi=V1alpha2AlibiExplainerSpec(
                type='AnchorTabular',
                storage_uri='gs://seldon-models/sklearn/income/explainer-py36-0.5.2',
                resources=V1ResourceRequirements(
                    requests={'cpu': '100m', 'memory': '1Gi'},
                    limits={'cpu': '100m', 'memory': '1Gi'}))))

    isvc = V1alpha2InferenceService(api_version=api_version,
                                    kind=constants.KFSERVING_KIND,
                                    metadata=client.V1ObjectMeta(
                                        name=service_name, namespace=KFSERVING_TEST_NAMESPACE),
                                    spec=V1alpha2InferenceServiceSpec(default=default_endpoint_spec))

    KFServing.create(isvc)
    try:
        KFServing.wait_isvc_ready(service_name, namespace=KFSERVING_TEST_NAMESPACE, timeout_seconds=300)
    except RuntimeError as e:
        logging.info(KFServing.api_instance.get_namespaced_custom_object("serving.knative.dev", "v1",
                                                                         KFSERVING_TEST_NAMESPACE, "services",
                                                                         service_name + "-predictor-default"))
        pods = KFServing.core_api.list_namespaced_pod(KFSERVING_TEST_NAMESPACE,
                                                      label_selector='serving.kubeflow.org/inferenceservice={}'.format(
                                                          service_name))
        for pod in pods.items:
            logging.info(pod)
        raise e

    res = predict(service_name, './data/income_input.json')
    assert (res["predictions"] == [0])
    precision = explain(service_name, './data/income_input.json')
    assert (precision > 0.9)
    KFServing.delete(service_name, KFSERVING_TEST_NAMESPACE)
