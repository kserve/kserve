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
from kfserving import (
    KFServingClient,
    constants,
    V1alpha2EndpointSpec,
    V1alpha2PredictorSpec,
    V1alpha2LightGBMSpec,
    V1alpha2InferenceServiceSpec,
    V1alpha2InferenceService
)
from kubernetes.client import V1ResourceRequirements

from ..common.utils import predict, KFSERVING_TEST_NAMESPACE

api_version = f"{constants.KFSERVING_GROUP}/{constants.KFSERVING_VERSION}"
api_v1beta1_version = (
    f"{constants.KFSERVING_GROUP}/{constants.KFSERVING_V1BETA1_VERSION}"
)
KFServing = KFServingClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))

def test_lightgbm_kfserving():
    service_name = "isvc-lightgbm"
    default_endpoint_spec = V1alpha2EndpointSpec(
        predictor=V1alpha2PredictorSpec(
            min_replicas=1,
            lightgbm=V1alpha2LightGBMSpec(
                storage_uri="gs://photon-models/kfserving-samples/models/lightgbm",
                resources=V1ResourceRequirements(
                    requests={"cpu": "100m", "memory": "256Mi"},
                    limits={"cpu": "100m", "memory": "256Mi"},
                ),
            ),
        )
    )

    isvc = V1alpha2InferenceService(
        api_version=api_version,
        kind=constants.KFSERVING_KIND,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KFSERVING_TEST_NAMESPACE
        ),
        spec=V1alpha2InferenceServiceSpec(default=default_endpoint_spec),
    )

    KFServing.create(isvc)
    try:
        KFServing.wait_isvc_ready(service_name, namespace=KFSERVING_TEST_NAMESPACE, 
                                version=constants.KFSERVING_VERSION)
    except RuntimeError as e:
        KFServing.wait_isvc_ready(service_name, namespace=KFSERVING_TEST_NAMESPACE, 
                                version=constants.KFSERVING_V1BETA1_VERSION)
    try:                                
        res = predict(service_name, "./data/iris_input_v3.json", 
                                version=constants.KFSERVING_VERSION)
    except KeyError:
        res = predict(service_name, "./data/iris_input_v3.json", 
                                version=constants.KFSERVING_V1BETA1_VERSION) 
    assert res["predictions"][0][0] > 0.5
    KFServing.delete(service_name, KFSERVING_TEST_NAMESPACE)
