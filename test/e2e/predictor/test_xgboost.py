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

import json
import os
import time

import pytest
from kubernetes import client
from kubernetes.client import V1ContainerPort, V1EnvVar, V1ResourceRequirements

from kserve import (KServeClient, V1beta1InferenceService,
                    V1beta1InferenceServiceSpec, V1beta1ModelFormat,
                    V1beta1ModelSpec, V1beta1PredictorSpec, V1beta1XGBoostSpec,
                    constants)

from ..common.utils import KSERVE_TEST_NAMESPACE, predict, predict_grpc


@pytest.mark.predictor
@pytest.mark.path_based_routing
def test_xgboost_kserve():
    service_name = "isvc-xgboost"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        xgboost=V1beta1XGBoostSpec(
            storage_uri="gs://kfserving-examples/models/xgboost/1.5/model",
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "256Mi"},
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

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(
        service_name, namespace=KSERVE_TEST_NAMESPACE)
    res = predict(service_name, "./data/iris_input.json")
    assert res["predictions"] == [1, 1]
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.predictor
@pytest.mark.path_based_routing
def test_xgboost_v2_mlserver():
    service_name = "isvc-xgboost-v2-mlserver"
    protocol_version = "v2"

    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        xgboost=V1beta1XGBoostSpec(
            storage_uri="gs://kfserving-examples/models/xgboost/iris",
            env=[V1EnvVar(name="MLSERVER_MODEL_PARALLEL_WORKERS", value="0")],
            protocol_version=protocol_version,
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "1024Mi"},
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

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(
        service_name, namespace=KSERVE_TEST_NAMESPACE)
    # TODO: Remove sleep once wait_model_ready supports path based routing. Since path based routing genarates a url
    # different from the host based routing wait_model_ready will always fail.
    time.sleep(10)
    # kserve_client.wait_model_ready(
    #     service_name,
    #     model_name=service_name,
    #     isvc_namespace=KSERVE_TEST_NAMESPACE,
    #     isvc_version=constants.KSERVE_V1BETA1_VERSION,
    #     protocol_version=protocol_version,
    #     cluster_ip=get_cluster_ip(),
    # )

    res = predict(service_name, "./data/iris_input_v2.json",
                  protocol_version="v2")
    assert res["outputs"][0]["data"] == [1.0, 1.0]

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.predictor
@pytest.mark.path_based_routing
def test_xgboost_runtime_kserve():
    service_name = "isvc-xgboost-runtime"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="xgboost",
            ),
            storage_uri="gs://kfserving-examples/models/xgboost/1.5/model",
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "256Mi"},
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

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(
        service_name, namespace=KSERVE_TEST_NAMESPACE)
    res = predict(service_name, "./data/iris_input.json")
    assert res["predictions"] == [1, 1]
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.predictor
@pytest.mark.path_based_routing
def test_xgboost_v2_runtime_mlserver():
    service_name = "isvc-xgboost-v2-runtime"
    protocol_version = "v2"

    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="xgboost",
            ),
            runtime="kserve-mlserver",
            storage_uri="gs://kfserving-examples/models/xgboost/iris",
            protocol_version=protocol_version,
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "1024Mi"},
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

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(
        service_name, namespace=KSERVE_TEST_NAMESPACE)
    # TODO: Remove sleep once wait_model_ready supports path based routing. Since path based routing genarates a url
    # different from the host based routing wait_model_ready will always fail.
    time.sleep(10)
    # kserve_client.wait_model_ready(
    #     service_name,
    #     model_name=service_name,
    #     isvc_namespace=KSERVE_TEST_NAMESPACE,
    #     isvc_version=constants.KSERVE_V1BETA1_VERSION,
    #     protocol_version=protocol_version,
    #     cluster_ip=get_cluster_ip(),
    # )

    res = predict(service_name, "./data/iris_input_v2.json",
                  protocol_version="v2")
    assert res["outputs"][0]["data"] == [1.0, 1.0]

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.predictor
@pytest.mark.path_based_routing
def test_xgboost_v2():
    service_name = "isvc-xgboost-v2"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="xgboost",
            ),
            runtime="kserve-xgbserver",
            storage_uri="gs://kfserving-examples/models/xgboost/iris",
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "1024Mi"},
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

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(
        service_name, namespace=KSERVE_TEST_NAMESPACE)

    res = predict(service_name, "./data/iris_input_v2.json",
                  protocol_version="v2")
    assert res["outputs"][0]["data"] == [1.0, 1.0]

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.grpc
@pytest.mark.predictor
def test_xgboost_v2_grpc():
    service_name = "isvc-xgboost-v2-grpc"
    model_name = "xgboost"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="xgboost",
            ),
            runtime="kserve-xgbserver",
            storage_uri="gs://kfserving-examples/models/xgboost/iris",
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "1024Mi"},
            ),
            ports=[
                V1ContainerPort(
                    container_port=8081,
                    name="h2c",
                    protocol="TCP"
                )],
            args=["--model_name", model_name]
        )
    )

    isvc = V1beta1InferenceService(api_version=constants.KSERVE_V1BETA1,
                                   kind=constants.KSERVE_KIND,
                                   metadata=client.V1ObjectMeta(
                                       name=service_name, namespace=KSERVE_TEST_NAMESPACE),
                                   spec=V1beta1InferenceServiceSpec(predictor=predictor))

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(
        service_name, namespace=KSERVE_TEST_NAMESPACE)

    json_file = open("./data/iris_input_v2_grpc.json")
    payload = json.load(json_file)["inputs"]
    response = predict_grpc(service_name=service_name,
                            payload=payload, model_name=model_name)
    prediction = list(response.outputs[0].contents.fp32_contents)
    assert prediction == [1.0, 1.0]

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
