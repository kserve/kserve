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

from kserve import KServeClient
from kserve import constants
from kserve import V1beta1PredictorSpec
from kserve import V1beta1SKLearnSpec
from kserve import V1beta1InferenceServiceSpec
from kserve import V1beta1ExplainerSpec
from kserve import V1beta1AlibiExplainerSpec
from kserve import V1beta1InferenceService
from kubernetes.client import V1ResourceRequirements
import pytest

from ..common.utils import predict
from ..common.utils import explain
from ..common.utils import KSERVE_TEST_NAMESPACE

logging.basicConfig(level=logging.INFO)
kserve_client = KServeClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))


@pytest.mark.explainer
def test_tabular_explainer():
    service_name = 'isvc-explainer-tabular'
    predictor = V1beta1PredictorSpec(
        sklearn=V1beta1SKLearnSpec(
            storage_uri='gs://kfserving-examples/models/sklearn/1.3/income/model',
            resources=V1ResourceRequirements(
                requests={'cpu': '100m', 'memory': '256Mi'},
                limits={'cpu': '250m', 'memory': '512Mi'}
            )
        )
    )
    explainer = V1beta1ExplainerSpec(
        min_replicas=1,
        alibi=V1beta1AlibiExplainerSpec(
            name='kserve-container',
            type='AnchorTabular',
            storage_uri='gs://kfserving-examples/models/sklearn/1.3/income/explainer',
            resources=V1ResourceRequirements(
                requests={'cpu': '100m', 'memory': '256Mi'},
                limits={'cpu': '250m', 'memory': '512Mi'}
            )
        )
    )

    isvc = V1beta1InferenceService(api_version=constants.KSERVE_V1BETA1,
                                   kind=constants.KSERVE_KIND,
                                   metadata=client.V1ObjectMeta(
                                       name=service_name, namespace=KSERVE_TEST_NAMESPACE),
                                   spec=V1beta1InferenceServiceSpec(predictor=predictor, explainer=explainer))

    kserve_client.create(isvc)
    try:
        kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE, timeout_seconds=720)
    except RuntimeError as e:
        logging.info(kserve_client.api_instance.get_namespaced_custom_object("serving.knative.dev", "v1",
                                                                             KSERVE_TEST_NAMESPACE, "services",
                                                                             service_name + "-predictor-default"))
        pods = kserve_client.core_api.list_namespaced_pod(KSERVE_TEST_NAMESPACE,
                                                          label_selector='serving.kserve.io/inferenceservice={}'.format(
                                                              service_name))
        for pod in pods.items:
            logging.info(pod)
        raise e

    res = predict(service_name, './data/income_input.json')
    assert (res["predictions"] == [0])
    precision = explain(service_name, './data/income_input.json')
    assert (precision > 0.9)
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
