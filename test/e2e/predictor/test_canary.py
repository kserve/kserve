# Copyright 2022 The KServe Authors.
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

from kserve import KServeClient
from kserve import constants
from kserve import V1beta1PredictorSpec
from kserve import V1beta1TFServingSpec
from kserve import V1beta1InferenceServiceSpec
from kserve import V1beta1InferenceService
from kserve import V1beta1ModelSpec, V1beta1ModelFormat
from kubernetes.client import V1ResourceRequirements
import pytest

from ..common.utils import KSERVE_TEST_NAMESPACE


kserve_client = KServeClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))


@pytest.mark.predictor
@pytest.mark.path_based_routing
def test_canary_rollout():
    service_name = "isvc-canary"
    default_endpoint_spec = V1beta1InferenceServiceSpec(
        predictor=V1beta1PredictorSpec(
            min_replicas=1,
            tensorflow=V1beta1TFServingSpec(
                storage_uri="gs://kfserving-examples/models/tensorflow/flowers",
                resources=V1ResourceRequirements(
                    requests={"cpu": "10m", "memory": "128Mi"},
                    limits={"cpu": "100m", "memory": "256Mi"},
                ),
            ),
        )
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=default_endpoint_spec,
    )

    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

    # define canary endpoint spec, and then rollout 10% traffic to the canary version
    canary_endpoint_spec = V1beta1InferenceServiceSpec(
        predictor=V1beta1PredictorSpec(
            canary_traffic_percent=10,
            tensorflow=V1beta1TFServingSpec(
                storage_uri="gs://kfserving-examples/models/tensorflow/flowers-2",
                resources=V1ResourceRequirements(
                    requests={"cpu": "10m", "memory": "128Mi"},
                    limits={"cpu": "100m", "memory": "256Mi"},
                ),
            ),
        )
    )
    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=canary_endpoint_spec,
    )

    kserve_client.patch(service_name, isvc, namespace=KSERVE_TEST_NAMESPACE)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

    canary_isvc = kserve_client.get(service_name, namespace=KSERVE_TEST_NAMESPACE)
    for traffic in canary_isvc["status"]["components"]["predictor"]["traffic"]:
        if traffic["latestRevision"]:
            assert traffic["percent"] == 10

    # Delete the InferenceService
    kserve_client.delete(service_name, namespace=KSERVE_TEST_NAMESPACE)


@pytest.mark.predictor
@pytest.mark.path_based_routing
def test_canary_rollout_runtime():
    service_name = "isvc-canary-runtime"
    default_endpoint_spec = V1beta1InferenceServiceSpec(
        predictor=V1beta1PredictorSpec(
            min_replicas=1,
            model=V1beta1ModelSpec(
                model_format=V1beta1ModelFormat(
                    name="tensorflow",
                ),
                storage_uri="gs://kfserving-examples/models/tensorflow/flowers",
                resources=V1ResourceRequirements(
                    requests={"cpu": "10m", "memory": "128Mi"},
                    limits={"cpu": "100m", "memory": "256Mi"},
                ),
            ),
        )
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=default_endpoint_spec,
    )

    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

    # define canary endpoint spec, and then rollout 10% traffic to the canary version
    canary_endpoint_spec = V1beta1InferenceServiceSpec(
        predictor=V1beta1PredictorSpec(
            canary_traffic_percent=10,
            model=V1beta1ModelSpec(
                model_format=V1beta1ModelFormat(
                    name="tensorflow",
                ),
                storage_uri="gs://kfserving-examples/models/tensorflow/flowers-2",
                resources=V1ResourceRequirements(
                    requests={"cpu": "10m", "memory": "128Mi"},
                    limits={"cpu": "100m", "memory": "256Mi"},
                ),
            ),
        )
    )
    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=canary_endpoint_spec,
    )

    kserve_client.patch(service_name, isvc, namespace=KSERVE_TEST_NAMESPACE)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

    canary_isvc = kserve_client.get(service_name, namespace=KSERVE_TEST_NAMESPACE)
    for traffic in canary_isvc["status"]["components"]["predictor"]["traffic"]:
        if traffic["latestRevision"]:
            assert traffic["percent"] == 10

    # Delete the InferenceService
    kserve_client.delete(service_name, namespace=KSERVE_TEST_NAMESPACE)
