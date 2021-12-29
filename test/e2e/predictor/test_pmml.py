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

from kserve import KServeClient
from kserve import V1beta1InferenceService
from kserve import V1beta1InferenceServiceSpec
from kserve import V1beta1PMMLSpec
from kserve import V1beta1PredictorSpec
from kserve import V1beta1ModelSpec, V1beta1ModelFormat
from kserve import constants
from kubernetes import client
from kubernetes.client import V1ResourceRequirements

from ..common.utils import KSERVE_TEST_NAMESPACE
from ..common.utils import predict


def test_pmml_kserve():
    service_name = 'isvc-pmml'
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        pmml=V1beta1PMMLSpec(
            storage_uri='gs://kfserving-examples/models/pmml',
            resources=V1ResourceRequirements(
                requests={'cpu': '100m', 'memory': '256Mi'},
                limits={'cpu': '100m', 'memory': '256Mi'}
            )
        )
    )

    isvc = V1beta1InferenceService(api_version=constants.KSERVE_V1BETA1,
                                   kind=constants.KSERVE_KIND,
                                   metadata=client.V1ObjectMeta(
                                        name=service_name, namespace=KSERVE_TEST_NAMESPACE),
                                   spec=V1beta1InferenceServiceSpec(predictor=predictor))

    kserve_client = KServeClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)
    res = predict(service_name, './data/pmml_input.json')
    assert (res["predictions"] == [{'Species': 'setosa',
                                    'Probability_setosa': 1.0,
                                    'Probability_versicolor': 0.0,
                                    'Probability_virginica': 0.0,
                                    'Node_Id': '2'}])
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


def test_pmml_runtime_kserve():
    service_name = 'isvc-pmml-runtime'
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="pmml",
            ),
            storage_uri='gs://kfserving-examples/models/pmml',
            resources=V1ResourceRequirements(
                requests={'cpu': '100m', 'memory': '256Mi'},
                limits={'cpu': '100m', 'memory': '256Mi'}
            )
        )
    )

    isvc = V1beta1InferenceService(api_version=constants.KSERVE_V1BETA1,
                                   kind=constants.KSERVE_KIND,
                                   metadata=client.V1ObjectMeta(
                                        name=service_name, namespace=KSERVE_TEST_NAMESPACE),
                                   spec=V1beta1InferenceServiceSpec(predictor=predictor))

    kserve_client = KServeClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)
    res = predict(service_name, './data/pmml_input.json')
    assert (res["predictions"] == [{'Species': 'setosa',
                                    'Probability_setosa': 1.0,
                                    'Probability_versicolor': 0.0,
                                    'Probability_virginica': 0.0,
                                    'Node_Id': '2'}])
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
