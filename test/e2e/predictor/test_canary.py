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
import pytest
from kubernetes import client

from kfserving import KFServingClient
from kfserving import constants
from kfserving import V1alpha2EndpointSpec
from kfserving import V1alpha2PredictorSpec
from kfserving import V1alpha2TensorflowSpec
from kfserving import V1alpha2InferenceServiceSpec
from kfserving import V1alpha2InferenceService
from kubernetes.client import V1ResourceRequirements

from ..common.utils import predict
from ..common.utils import KFSERVING_TEST_NAMESPACE


# Setting config_file is required since SDK is running in a different cluster than KFServing
KFServing = KFServingClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))

@pytest.mark.skip(reason="No need to test this for v1beta1 version.")
def test_canary_rollout():
    service_name = 'isvc-canary'
    default_endpoint_spec = V1alpha2EndpointSpec(
        predictor=V1alpha2PredictorSpec(
            min_replicas=1,
            tensorflow=V1alpha2TensorflowSpec(
                storage_uri='gs://kfserving-samples/models/tensorflow/flowers',
                resources=V1ResourceRequirements(
                    requests={'cpu': '100m', 'memory': '256Mi'},
                    limits={'cpu': '100m', 'memory': '256Mi'}))))

    isvc = V1alpha2InferenceService(api_version=constants.KFSERVING_API_VERSION,
                                    kind=constants.KFSERVING_KIND,
                                    metadata=client.V1ObjectMeta(
                                        name=service_name, namespace=KFSERVING_TEST_NAMESPACE),
                                    spec=V1alpha2InferenceServiceSpec(default=default_endpoint_spec))

    KFServing.create(isvc)
    KFServing.wait_isvc_ready(service_name, namespace=KFSERVING_TEST_NAMESPACE)

    # define canary endpoint spec, and then rollout 10% traffic to the canary version
    canary_endpoint_spec = V1alpha2EndpointSpec(
        predictor=V1alpha2PredictorSpec(
            tensorflow=V1alpha2TensorflowSpec(
                storage_uri='gs://kfserving-samples/models/tensorflow/flowers-2',
                resources=V1ResourceRequirements(
                    requests={'cpu':'100m','memory':'256Mi'},
                    limits={'cpu':'100m', 'memory':'256Mi'}))))

    KFServing.rollout_canary(service_name, canary=canary_endpoint_spec, percent=10,
                             namespace=KFSERVING_TEST_NAMESPACE, watch=True, timeout_seconds=120)
    KFServing.wait_isvc_ready(service_name, namespace=KFSERVING_TEST_NAMESPACE)

    # Delete the InferenceService
    KFServing.delete(service_name, namespace=KFSERVING_TEST_NAMESPACE)
