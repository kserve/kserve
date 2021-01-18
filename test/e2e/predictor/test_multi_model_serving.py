# Copyright 2021 kubeflow.org.
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
    constants,
    KFServingClient,
    V1beta1PredictorSpec,
    V1alpha1TrainedModel,
    V1alpha2EndpointSpec,
    V1beta1InferenceService,
    V1beta1InferenceServiceSpec,
    V1alpha1ModelSpec,
    V1alpha1TrainedModelSpec,
    V1beta1SKLearnSpec,
    V1beta1XGBoostSpec,
    V1alpha2TritonSpec,
)

from ..common.utils import predict, get_cluster_ip
from ..common.utils import KFSERVING_TEST_NAMESPACE

api_v1beta1_version = constants.KFSERVING_GROUP + "/" + constants.KFSERVING_V1BETA1_VERSION

KFServing = KFServingClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))

def test_mms_sklearn_kfserving():
    # Define an inference service
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        sklearn=V1beta1SKLearnSpec(
            protocol_version="v1",
            resources=client.V1ResourceRequirements(
                requests={"cpu": "100m", "memory": "256Mi"},
                limits={"cpu": "100m", "memory": "256Mi"},
            )
        )
    )

    service_name = "isvc-sklearn-mms"
    isvc = V1beta1InferenceService(
        api_version=api_v1beta1_version,
        kind=constants.KFSERVING_KIND,
        metadata=client.V1ObjectMeta(
            name=service_name,
            namespace=KFSERVING_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor)
    )

    # Define trained models
    model1_spec = V1alpha1ModelSpec(
        storage_uri=f"gs://kfserving-samples/models/sklearn/iris",
        memory='256Mi',
        framework="sklearn"
    )

    model2_spec = V1alpha1ModelSpec(
        storage_uri=f"gs://kfserving-samples/models/sklearn/iris",
        memory='256Mi',
        framework="sklearn"
    )

    model1_name = "model1-sklearn"
    model2_name = "model2-sklearn"
    model1 = V1alpha1TrainedModel(
        api_version=constants.KFSERVING_V1ALPHA1,
        kind=constants.KFSERVING_KIND_TRAINEDMODEL,
        metadata=client.V1ObjectMeta(
            name=model1_name,
            namespace=KFSERVING_TEST_NAMESPACE
        ),
        spec=V1alpha1TrainedModelSpec(
            inference_service=service_name,
            model=model1_spec
        )
    )

    model2 = V1alpha1TrainedModel(
        api_version=constants.KFSERVING_V1ALPHA1,
        kind=constants.KFSERVING_KIND_TRAINEDMODEL,
        metadata=client.V1ObjectMeta(
            name=model2_name,
            namespace=KFSERVING_TEST_NAMESPACE
        ),
        spec=V1alpha1TrainedModelSpec(
            inference_service=service_name,
            model=model2_spec
        )
    )

    # Create an instance of inference service with isvc
    KFServing.create(isvc)
    KFServing.wait_isvc_ready(service_name, namespace=KFSERVING_TEST_NAMESPACE)

    # Create instances of trained models using model1 and model2
    KFServing.create_trained_model(model1, KFSERVING_TEST_NAMESPACE)
    KFServing.create_trained_model(model2, KFSERVING_TEST_NAMESPACE)

    cluster_ip = get_cluster_ip()

    KFServing.wait_model_ready(service_name, model1_name, isvc_namespace=KFSERVING_TEST_NAMESPACE,
                                isvc_version=constants.KFSERVING_V1BETA1_VERSION, cluster_ip=cluster_ip)
    KFServing.wait_model_ready(service_name, model2_name, isvc_namespace=KFSERVING_TEST_NAMESPACE,
                                isvc_version=constants.KFSERVING_V1BETA1_VERSION, cluster_ip=cluster_ip)

    # Call predict on the two models
    res_model1 = predict(service_name, "./data/iris_input.json", model_name=model1_name)
    res_model2 = predict(service_name, "./data/iris_input.json", model_name=model2_name)

    assert res_model1["predictions"] == [1,1]
    assert res_model2["predictions"] == [1,1]

    # Clean up inference service
    KFServing.delete(service_name, KFSERVING_TEST_NAMESPACE)

def test_mms_xgboost_kfserving():
    # Define an inference service
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        xgboost=V1beta1XGBoostSpec(
            protocol_version="v1",
            resources=client.V1ResourceRequirements(
                requests={"cpu": "100m", "memory": "256Mi"},
                limits={"cpu": "100m", "memory": "256Mi"},
            )
        )
    )

    service_name = "isvc-xgboost-mms"
    isvc = V1beta1InferenceService(
        api_version=api_v1beta1_version,
        kind=constants.KFSERVING_KIND,
        metadata=client.V1ObjectMeta(
            name=service_name,
            namespace=KFSERVING_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor)
    )

    # Define trained models
    model1_spec = V1alpha1ModelSpec(
        storage_uri="gs://kfserving-samples/models/xgboost/iris",
        memory='256Mi',
        framework="xgboost"
    )

    model2_spec = V1alpha1ModelSpec(
        storage_uri="gs://kfserving-samples/models/xgboost/iris",
        memory='256Mi',
        framework="xgboost"
    )

    model1_name = "model1-xgboost"
    model2_name = "model2-xgboost"
    model1 = V1alpha1TrainedModel(
        api_version=constants.KFSERVING_V1ALPHA1,
        kind=constants.KFSERVING_KIND_TRAINEDMODEL,
        metadata=client.V1ObjectMeta(
            name=model1_name,
            namespace=KFSERVING_TEST_NAMESPACE
        ),
        spec=V1alpha1TrainedModelSpec(
            inference_service=service_name,
            model=model1_spec
        )
    )

    model2 = V1alpha1TrainedModel(
        api_version=constants.KFSERVING_V1ALPHA1,
        kind=constants.KFSERVING_KIND_TRAINEDMODEL,
        metadata=client.V1ObjectMeta(
            name=model2_name,
            namespace=KFSERVING_TEST_NAMESPACE
        ),
        spec=V1alpha1TrainedModelSpec(
            inference_service=service_name,
            model=model2_spec
        )
    )

    # Create an instance of inference service with isvc
    KFServing.create(isvc)
    KFServing.wait_isvc_ready(service_name, namespace=KFSERVING_TEST_NAMESPACE)

    # Create instances of trained models using model1 and model2
    KFServing.create_trained_model(model1, KFSERVING_TEST_NAMESPACE)
    KFServing.create_trained_model(model2, KFSERVING_TEST_NAMESPACE)

    cluster_ip = get_cluster_ip()

    KFServing.wait_model_ready(service_name, model1_name, isvc_namespace=KFSERVING_TEST_NAMESPACE,
                                isvc_version=constants.KFSERVING_V1BETA1_VERSION, cluster_ip=cluster_ip)
    KFServing.wait_model_ready(service_name, model2_name, isvc_namespace=KFSERVING_TEST_NAMESPACE,
                                isvc_version=constants.KFSERVING_V1BETA1_VERSION, cluster_ip=cluster_ip)

    # Call predict on the two models
    res_model1 = predict(service_name, "./data/iris_input.json", model_name=model1_name)
    res_model2 = predict(service_name, "./data/iris_input.json", model_name=model2_name)

    assert res_model1["predictions"] == [1,1]
    assert res_model2["predictions"] == [1,1]

    # Clean up inference service
    KFServing.delete(service_name, KFSERVING_TEST_NAMESPACE)
