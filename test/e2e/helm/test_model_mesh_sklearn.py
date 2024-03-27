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

import pytest
from kubernetes import client
from kubernetes.client import V1ResourceRequirements

from kserve import (
    KServeClient,
    V1beta1InferenceService,
    V1beta1InferenceServiceSpec,
    V1beta1ModelFormat,
    V1beta1ModelSpec,
    V1beta1PredictorSpec,
    V1beta1StorageSpec,
    constants,
)
from ..common.utils import predict_modelmesh


# TODO: Enable e2e test post ingress implementation in model mesh serving
# https://github.com/kserve/modelmesh-serving/issues/295
# @pytest.mark.helm
@pytest.mark.skip
@pytest.mark.asyncio(scope="session")
async def test_sklearn_modelmesh():
    service_name = "isvc-sklearn-modelmesh"
    annotations = dict()
    annotations["serving.kserve.io/deploymentMode"] = "ModelMesh"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="sklearn",
            ),
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "512Mi"},
            ),
            storage=V1beta1StorageSpec(
                key="localMinIO", path="sklearn/mnist-svm.joblib"
            ),
        ),
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND,
        metadata=client.V1ObjectMeta(name=service_name, annotations=annotations),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name)
    pods = kserve_client.core_api.list_namespaced_pod(
        "default", label_selector="name=modelmesh-serving-mlserver-1.x"
    )

    pod_name = pods.items[0].metadata.name
    res = await predict_modelmesh(
        service_name, "./data/mm_sklearn_input.json", pod_name
    )
    assert res.outputs[0].data == [8]

    kserve_client.delete(service_name)
