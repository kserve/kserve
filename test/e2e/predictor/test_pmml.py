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

from kfserving import KFServingClient
from kfserving import V1alpha2EndpointSpec
from kfserving import V1alpha2InferenceService
from kfserving import V1alpha2InferenceServiceSpec
from kfserving import V1alpha2PMMLSpec
from kfserving import V1alpha2PredictorSpec
from kfserving import constants
from kubernetes import client
from kubernetes.client import V1ResourceRequirements

from ..common.utils import KFSERVING_TEST_NAMESPACE
from ..common.utils import predict

api_version = constants.KFSERVING_GROUP + '/' + constants.KFSERVING_VERSION
KFServing = KFServingClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))


def test_pmml_kfserving():
    service_name = 'isvc-pmml'
    default_endpoint_spec = V1alpha2EndpointSpec(
        predictor=V1alpha2PredictorSpec(
            min_replicas=1,
            pmml=V1alpha2PMMLSpec(
                storage_uri='gs://kfserving-examples/models/pmml',
                resources=V1ResourceRequirements(
                    requests={'cpu': '100m', 'memory': '256Mi'},
                    limits={'cpu': '100m', 'memory': '256Mi'}))))

    isvc = V1alpha2InferenceService(api_version=api_version,
                                    kind=constants.KFSERVING_KIND,
                                    metadata=client.V1ObjectMeta(
                                        name=service_name, namespace=KFSERVING_TEST_NAMESPACE),
                                    spec=V1alpha2InferenceServiceSpec(default=default_endpoint_spec))

    KFServing.create(isvc)
    KFServing.wait_isvc_ready(service_name, namespace=KFSERVING_TEST_NAMESPACE)
    res = predict(service_name, './data/pmml_input.json')
    assert (res["predictions"] == [{'Species': 'setosa',
                                    'Probability_setosa': 1.0,
                                    'Probability_versicolor': 0.0,
                                    'Probability_virginica': 0.0,
                                    'Node_Id': '2'}])
    KFServing.delete(service_name, KFSERVING_TEST_NAMESPACE)
