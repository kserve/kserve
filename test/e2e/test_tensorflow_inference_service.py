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

import time
from kubernetes import client

from kfserving import KFServingClient
from kfserving import constants
from kfserving import V1alpha2EndpointSpec
from kfserving import V1alpha2PredictorSpec
from kfserving import V1alpha2TensorflowSpec
from kfserving import V1alpha2InferenceServiceSpec
from kfserving import V1alpha2InferenceService
from kubernetes.client import V1ResourceRequirements

from utils import wait_for_kfservice_ready

api_version = constants.KFSERVING_GROUP + '/' + constants.KFSERVING_VERSION
KFServing = KFServingClient(config_file="~/.kube/config")


def test_tensorflow_kfserving():
    default_endpoint_spec = V1alpha2EndpointSpec(
        predictor=V1alpha2PredictorSpec(
            tensorflow=V1alpha2TensorflowSpec(
                storage_uri='gs://kfserving-samples/models/tensorflow/flowers',
                resources=V1ResourceRequirements(
                    requests={'cpu': '100m', 'memory': '200Mi'},
                    limits={'cpu': '1', 'memory': '1Gi'}))))

    isvc = V1alpha2InferenceService(api_version=api_version,
                                    kind=constants.KFSERVING_KIND,
                                    metadata=client.V1ObjectMeta(
                                        name='isvc-tensorflow-test', namespace='kfserving-ci-e2e-test'),
                                    spec=V1alpha2InferenceServiceSpec(default=default_endpoint_spec))

    KFServing.create(isvc)
    wait_for_kfservice_ready('isvc-tensorflow-test')
