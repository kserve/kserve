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
from kserve import (
    constants,
    KServeClient,
    V1beta1PredictorSpec,
    V1alpha1TrainedModel,
    V1beta1InferenceService,
    V1beta1InferenceServiceSpec,
    V1alpha1ModelSpec,
    V1alpha1TrainedModelSpec,
    V1beta1SKLearnSpec,
    V1beta1XGBoostSpec,
)

from ..common.utils import predict_isvc, get_cluster_ip
from ..common.utils import KSERVE_TEST_NAMESPACE


@pytest.mark.parametrize(
    "protocol_version,storage_uri",
    [
        (
            "v1",
            "gs://kfserving-examples/models/sklearn/1.0/model",
        ),
        (
            "v2",
            "gs://seldon-models/sklearn/mms/lr_model",
        ),
    ],
)
@pytest.mark.mms
@pytest.mark.asyncio(scope="session")
async def test_mms_sklearn_kserve(
    protocol_version: str, storage_uri: str, rest_v1_client, rest_v2_client
):
    service_name = f"isvc-sklearn-mms-{protocol_version}"
    # Define an inference service
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        sklearn=V1beta1SKLearnSpec(
            protocol_version=protocol_version,
            resources=client.V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "1024Mi"},
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

    # Create an instance of inference service with isvc
    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

    cluster_ip = get_cluster_ip()

    model_names = [
        f"model1-sklearn-{protocol_version}",
        f"model2-sklearn-{protocol_version}",
    ]

    for model_name in model_names:
        model_spec = V1alpha1ModelSpec(
            storage_uri=storage_uri,
            memory="128Mi",
            framework="sklearn",
        )

        model = V1alpha1TrainedModel(
            api_version=constants.KSERVE_V1ALPHA1,
            kind=constants.KSERVE_KIND_TRAINEDMODEL,
            metadata=client.V1ObjectMeta(
                name=model_name, namespace=KSERVE_TEST_NAMESPACE
            ),
            spec=V1alpha1TrainedModelSpec(
                inference_service=service_name, model=model_spec
            ),
        )

        # Create instances of trained models using model1 and model2
        kserve_client.create_trained_model(model, KSERVE_TEST_NAMESPACE)

        kserve_client.wait_model_ready(
            service_name,
            model_name,
            isvc_namespace=KSERVE_TEST_NAMESPACE,
            isvc_version=constants.KSERVE_V1BETA1_VERSION,
            protocol_version=protocol_version,
            cluster_ip=cluster_ip,
        )

    if protocol_version == "v1":
        responses = [
            await predict_isvc(
                rest_v1_client,
                service_name,
                "./data/iris_input.json",
                model_name=model_name,
            )
            for model_name in model_names
        ]
        assert responses[0]["predictions"] == [1, 1]
        assert responses[1]["predictions"] == [1, 1]
    elif protocol_version == "v2":
        responses = [
            await predict_isvc(
                rest_v2_client,
                service_name,
                "./data/iris_input_v2.json",
                model_name=model_name,
            )
            for model_name in model_names
        ]
        assert responses[0].outputs[0].data == [1, 1]
        assert responses[1].outputs[0].data == [1, 1]

    # Clean up inference service and trained models
    for model_name in model_names:
        kserve_client.delete_trained_model(model_name, KSERVE_TEST_NAMESPACE)
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.parametrize(
    "protocol_version,storage_uri",
    [
        (
            "v1",
            "gs://kfserving-examples/models/xgboost/1.5/model",
        ),
        (
            "v2",
            "gs://seldon-models/xgboost/mms/iris",
        ),
    ],
)
@pytest.mark.mms
@pytest.mark.asyncio(scope="session")
async def test_mms_xgboost_kserve(
    protocol_version: str, storage_uri: str, rest_v1_client, rest_v2_client
):
    service_name = f"isvc-xgboost-mms-{protocol_version}"
    # Define an inference service
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        xgboost=V1beta1XGBoostSpec(
            env=[client.V1EnvVar(name="MLSERVER_MODEL_PARALLEL_WORKERS", value="0")],
            protocol_version=protocol_version,
            resources=client.V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "1024Mi"},
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

    # Create an instance of inference service with isvc
    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

    cluster_ip = get_cluster_ip()
    model_names = [
        f"model1-xgboost-{protocol_version}",
        f"model2-xgboost-{protocol_version}",
    ]

    for model_name in model_names:
        # Define trained models
        model_spec = V1alpha1ModelSpec(
            storage_uri=storage_uri,
            memory="128Mi",
            framework="xgboost",
        )

        model = V1alpha1TrainedModel(
            api_version=constants.KSERVE_V1ALPHA1,
            kind=constants.KSERVE_KIND_TRAINEDMODEL,
            metadata=client.V1ObjectMeta(
                name=model_name, namespace=KSERVE_TEST_NAMESPACE
            ),
            spec=V1alpha1TrainedModelSpec(
                inference_service=service_name, model=model_spec
            ),
        )

        # Create instances of trained models using model1 and model2
        kserve_client.create_trained_model(model, KSERVE_TEST_NAMESPACE)

        kserve_client.wait_model_ready(
            service_name,
            model_name,
            isvc_namespace=KSERVE_TEST_NAMESPACE,
            isvc_version=constants.KSERVE_V1BETA1_VERSION,
            protocol_version=protocol_version,
            cluster_ip=cluster_ip,
        )

    if protocol_version == "v1":
        responses = [
            await predict_isvc(
                rest_v1_client,
                service_name,
                "./data/iris_input.json",
                model_name=model_name,
            )
            for model_name in model_names
        ]
        assert responses[0]["predictions"] == [1, 1]
        assert responses[1]["predictions"] == [1, 1]
    elif protocol_version == "v2":
        responses = [
            await predict_isvc(
                rest_v2_client,
                service_name,
                "./data/iris_input_v2.json",
                model_name=model_name,
            )
            for model_name in model_names
        ]
        assert responses[0].outputs[0].data == [1.0, 1.0]
        assert responses[1].outputs[0].data == [1.0, 1.0]

    # Clean up inference service and trained models
    for model_name in model_names:
        kserve_client.delete_trained_model(model_name, KSERVE_TEST_NAMESPACE)
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
