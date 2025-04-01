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
import numpy as np
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

from ..common.utils import predict_isvc
from ..common.utils import KSERVE_TEST_NAMESPACE


@pytest.mark.predictor
@pytest.mark.asyncio(scope="session")
async def test_tensorflow_kserve(rest_v1_client):
    service_name = "isvc-tensorflow"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        tensorflow=V1beta1TFServingSpec(
            storage_uri="gs://kfserving-examples/models/tensorflow/flowers",
            resources=V1ResourceRequirements(
                requests={"cpu": "10m", "memory": "256Mi"},
                limits={"cpu": "100m", "memory": "512Mi"},
            ),
        ),
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)
    res = await predict_isvc(rest_v1_client, service_name, "./data/flower_input.json")
    assert np.argmax(res["predictions"][0].get("scores")) == 0

    # Delete the InferenceService
    kserve_client.delete(service_name, namespace=KSERVE_TEST_NAMESPACE)


# In ODH, this test generates the following response:
#  502 Server Error: Bad Gateway for url
@pytest.mark.predictor
@pytest.mark.asyncio(scope="session")
@pytest.mark.skip("Not testable in ODH at the moment")
async def test_tensorflow_runtime_kserve(rest_v1_client):
    service_name = "isvc-tensorflow-runtime"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="tensorflow",
            ),
            storage_uri="gs://kfserving-examples/models/tensorflow/flowers",
            resources=V1ResourceRequirements(
                requests={"cpu": "10m", "memory": "256Mi"},
                limits={"cpu": "100m", "memory": "512Mi"},
            ),
        ),
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)
    res = await predict_isvc(rest_v1_client, service_name, "./data/flower_input.json")
    assert np.argmax(res["predictions"][0].get("scores")) == 0

    # Delete the InferenceService
    kserve_client.delete(service_name, namespace=KSERVE_TEST_NAMESPACE)
