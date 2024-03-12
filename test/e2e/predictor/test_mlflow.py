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
from kserve import (
    constants,
    KServeClient,
    V1beta1InferenceService,
    V1beta1InferenceServiceSpec,
    V1beta1PredictorSpec,
    V1beta1ModelSpec,
    V1beta1ModelFormat,
)
from kubernetes.client import V1ResourceRequirements
import pytest

from ..common.utils import predict, get_cluster_ip
from ..common.utils import KSERVE_TEST_NAMESPACE


@pytest.mark.predictor
def test_mlflow_v2_runtime_kserve():
    service_name = "isvc-mlflow-v2-runtime"
    protocol_version = "v2"

    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="mlflow",
            ),
            storage_uri="gs://kfserving-examples/models/mlflow/wine",
            protocol_version=protocol_version,
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "1", "memory": "1Gi"},
            ),
        ),
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)
    kserve_client.wait_model_ready(service_name, model_name=service_name, isvc_namespace=KSERVE_TEST_NAMESPACE,
                                   cluster_ip=get_cluster_ip(), protocol_version=protocol_version)
    res = predict(service_name, "./data/mlflow_input_v2.json", protocol_version=protocol_version)
    assert res["outputs"][0]["data"] == [5.576883936610762]

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
