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
    V1alpha2XGBoostSpec,
    V1alpha2InferenceServiceSpec,
    V1alpha2InferenceService,
    V1beta1InferenceService,
    V1beta1InferenceServiceSpec,
    V1beta1PredictorSpec,
    V1beta1XGBoostSpec,
)
from kubernetes.client import V1ResourceRequirements

from ..common.utils import predict, KFSERVING_TEST_NAMESPACE

api_version = f"{constants.KFSERVING_GROUP}/{constants.KFSERVING_VERSION}"
api_v1beta1_version = (
    f"{constants.KFSERVING_GROUP}/{constants.KFSERVING_V1BETA1_VERSION}"
)
KFServing = KFServingClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))


def test_xgboost_kfserving():
    service_name = "isvc-xgboost"
    default_endpoint_spec = V1alpha2EndpointSpec(
        predictor=V1alpha2PredictorSpec(
            min_replicas=1,
            xgboost=V1alpha2XGBoostSpec(
                storage_uri="gs://kfserving-samples/models/xgboost/iris",
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
    KFServing.wait_isvc_ready(service_name, namespace=KFSERVING_TEST_NAMESPACE)
    res = predict(service_name, "./data/iris_input.json")
    assert res["predictions"] == [1, 1]
    KFServing.delete(service_name, KFSERVING_TEST_NAMESPACE)


def test_xgboost_v2_kfserving():
    service_name = "isvc-xgboost-v2"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        xgboost=V1beta1XGBoostSpec(
            storage_uri="gs://kfserving-samples/models/xgboost/iris",
            protocol_version="v2",
            resources=V1ResourceRequirements(
                requests={"cpu": "100m", "memory": "256Mi"},
                limits={"cpu": "100m", "memory": "256Mi"},
            ),
        ),
    )

    isvc = V1beta1InferenceService(
        api_version=api_v1beta1_version,
        kind=constants.KFSERVING_KIND,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KFSERVING_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    KFServing.create(isvc, version=constants.KFSERVING_V1BETA1_VERSION)
    KFServing.wait_isvc_ready(service_name, namespace=KFSERVING_TEST_NAMESPACE)

    res = predict(service_name, "./data/iris_input_v2.json", protocol_version="v2")
    assert res["outputs"][0]["data"] == [1.0, 1.0]

    KFServing.delete(service_name, KFSERVING_TEST_NAMESPACE)
