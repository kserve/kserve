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

from kubernetes import client

from kfserving import KFServingClient
from kfserving import constants
from kfserving import V1alpha2EndpointSpec
from kfserving import V1alpha2PredictorSpec
from kfserving import V1alpha2SKLearnSpec
from kfserving import V1alpha2InferenceServiceSpec
from kfserving import V1alpha2InferenceService
from kubernetes.client import V1ResourceRequirements


from ..common.utils import wait_for_isvc_ready, predict, get_config_map
from ..common.utils import KFSERVING_TEST_NAMESPACE
from ..common.utils import api_version
from ..common.utils import KFServing

#api_version = constants.KFSERVING_GROUP + '/' + constants.KFSERVING_VERSION
#KFServing = KFServingClient(config_file="~/.kube/config")


def test_sklearn_kfserving():
    service_name = 'isvc-sklearn'
    default_endpoint_spec = V1alpha2EndpointSpec(
        predictor=V1alpha2PredictorSpec(
            min_replicas=1,
            sklearn=V1alpha2SKLearnSpec(
                storage_uri='gs://kfserving-samples/models/sklearn/iris',
                resources=V1ResourceRequirements(
                    requests={'cpu': '100m', 'memory': '256Mi'},
                    limits={'cpu': '100m', 'memory': '256Mi'}))))

    isvc = V1alpha2InferenceService(api_version=api_version,
                                    kind=constants.KFSERVING_KIND,
                                    metadata=client.V1ObjectMeta(
                                        name=service_name, namespace=KFSERVING_TEST_NAMESPACE),
                                    spec=V1alpha2InferenceServiceSpec(default=default_endpoint_spec))

    configmap = str(get_config_map())
    print(configmap)
    #assert 'test123' in configmap
    KFServing.create(isvc)
    wait_for_isvc_ready(service_name)
    probs = predict(service_name, './data/iris_input.json')
    assert(probs == [1, 1])
    KFServing.delete(service_name, KFSERVING_TEST_NAMESPACE)


