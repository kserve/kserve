import logging
import os
import yaml


import pytest
from requests.exceptions import HTTPError
from kserve import (
    V1beta1PredictorSpec,
    V1beta1SKLearnSpec,
    V1beta1InferenceServiceSpec,
    V1beta1InferenceService,
    constants,
    KServeClient,
    V1alpha1InferenceGraphSpec,
    V1alpha1InferenceRouter,
    V1alpha1InferenceGraph,
    V1alpha1InferenceStep,
    V1beta1XGBoostSpec,
)
from kubernetes import client, config
from kubernetes.client import V1Container
from kubernetes.client import V1ResourceRequirements

from ..common.utils import KSERVE_TEST_NAMESPACE, predict_ig

logging.basicConfig(level=logging.INFO)

SUCCESS_ISVC_IMAGE = "kserve/success-200-isvc:" + os.environ.get("GITHUB_SHA")
ERROR_ISVC_IMAGE = "kserve/error-404-isvc:" + os.environ.get("GITHUB_SHA")


@pytest.mark.graph
@pytest.mark.kourier
def test_inference_graph():
    sklearn_name = "isvc-sklearn-graph"
    xgb_name = "isvc-xgboost-graph"
    graph_name = "model-chainer"

    sklearn_predictor = V1beta1PredictorSpec(
        min_replicas=1,
        sklearn=V1beta1SKLearnSpec(
            storage_uri="gs://kfserving-examples/models/sklearn/1.0/model",
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "256Mi"},
            ),
        ),
    )
    sklearn_isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND,
        metadata=client.V1ObjectMeta(
            name=sklearn_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(predictor=sklearn_predictor),
    )

    xgb_predictor = V1beta1PredictorSpec(
        min_replicas=1,
        xgboost=V1beta1XGBoostSpec(
            storage_uri="gs://kfserving-examples/models/xgboost/1.5/model",
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "256Mi"},
            ),
        ),
    )
    xgb_isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND,
        metadata=client.V1ObjectMeta(name=xgb_name, namespace=KSERVE_TEST_NAMESPACE),
        spec=V1beta1InferenceServiceSpec(predictor=xgb_predictor),
    )

    nodes = {
        "root": V1alpha1InferenceRouter(
            router_type="Sequence",
            steps=[
                V1alpha1InferenceStep(
                    service_name=sklearn_name,
                ),
                V1alpha1InferenceStep(
                    service_name=xgb_name,
                    data="$request",
                ),
            ],
        )
    }
    graph_spec = V1alpha1InferenceGraphSpec(
        nodes=nodes,
    )
    ig = V1alpha1InferenceGraph(
        api_version=constants.KSERVE_V1ALPHA1,
        kind=constants.KSERVE_KIND_INFERENCEGRAPH,
        metadata=client.V1ObjectMeta(name=graph_name, namespace=KSERVE_TEST_NAMESPACE),
        spec=graph_spec,
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(sklearn_isvc)
    kserve_client.create(xgb_isvc)
    kserve_client.create_inference_graph(ig)

    kserve_client.wait_isvc_ready(sklearn_name, namespace=KSERVE_TEST_NAMESPACE)
    kserve_client.wait_isvc_ready(xgb_name, namespace=KSERVE_TEST_NAMESPACE)
    kserve_client.wait_ig_ready(graph_name, namespace=KSERVE_TEST_NAMESPACE)

    res = predict_ig(graph_name, "./data/iris_input.json")
    assert res["predictions"] == [1, 1]

    kserve_client.delete_inference_graph(graph_name, KSERVE_TEST_NAMESPACE)
    kserve_client.delete(sklearn_name, KSERVE_TEST_NAMESPACE)
    kserve_client.delete(xgb_name, KSERVE_TEST_NAMESPACE)


def construct_isvc_to_submit(service_name, image, model_name):
    predictor = V1beta1PredictorSpec(
        containers=[
            V1Container(
                name="kserve-container",
                image=image,
                resources=V1ResourceRequirements(
                    requests={"cpu": "50m", "memory": "128Mi"},
                    limits={"cpu": "100m", "memory": "1Gi"},
                ),
                args=["--model_name", model_name],
            )
        ]
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    return isvc


# @pytest.mark.rc4test
@pytest.mark.graph
@pytest.mark.kourier
def test_ig_scenario1():
    """
    Scenario: Sequence graph with 2 steps that rae both soft dependencies.
     success_isvc(soft) -> error_isvc (soft)

    We are not marking steps as soft or hard explicitly so this will test that default behavior of steps being soft
    is as expected.
    Expectation: IG will return response of error_isvc and predict_ig will raise exception
    :return:
    """

    logging.info("starting test")
    logging.info(f"SUCCESS_ISVC_IMAGE is {SUCCESS_ISVC_IMAGE}")
    logging.info(f"ERROR_ISVC_IMAGE is {ERROR_ISVC_IMAGE}")

    # Create success isvc
    model_name = success_isvc_name = "success-200-isvc"
    success_isvc = construct_isvc_to_submit(
        success_isvc_name, image=SUCCESS_ISVC_IMAGE, model_name=model_name
    )

    # Create error isvc
    model_name = error_isvc_name = "error-404-isvc"
    error_isvc = construct_isvc_to_submit(
        error_isvc_name, image=ERROR_ISVC_IMAGE, model_name=model_name
    )

    # Create graph
    graph_name = "sequence-graph"

    nodes = {
        "root": V1alpha1InferenceRouter(
            router_type="Sequence",
            steps=[
                V1alpha1InferenceStep(
                    service_name=success_isvc_name,
                ),
                V1alpha1InferenceStep(
                    service_name=error_isvc_name,
                    data="$request",
                ),
            ],
        )
    }
    graph_spec = V1alpha1InferenceGraphSpec(
        nodes=nodes,
    )
    ig = V1alpha1InferenceGraph(
        api_version=constants.KSERVE_V1ALPHA1,
        kind=constants.KSERVE_KIND_INFERENCEGRAPH,
        metadata=client.V1ObjectMeta(name=graph_name, namespace=KSERVE_TEST_NAMESPACE),
        spec=graph_spec,
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(success_isvc)
    kserve_client.create(error_isvc)
    kserve_client.create_inference_graph(ig)

    kserve_client.wait_isvc_ready(success_isvc_name, namespace=KSERVE_TEST_NAMESPACE)
    kserve_client.wait_isvc_ready(error_isvc_name, namespace=KSERVE_TEST_NAMESPACE)
    kserve_client.wait_ig_ready(graph_name, namespace=KSERVE_TEST_NAMESPACE)

    with pytest.raises(HTTPError) as exc_info:
        predict_ig(graph_name, "./data/iris_input.json")

    assert exc_info.value.response.json() == {"detail": "Intentional 404 code"}
    assert exc_info.value.response.status_code == 404

    kserve_client.delete_inference_graph(graph_name, KSERVE_TEST_NAMESPACE)
    kserve_client.delete(success_isvc_name, KSERVE_TEST_NAMESPACE)
    kserve_client.delete(error_isvc_name, KSERVE_TEST_NAMESPACE)


# @pytest.mark.rc4test
@pytest.mark.graph
@pytest.mark.kourier
def test_ig_scenario2():
    """
    Scenario: Sequence graph with 2 steps that rae both soft dependencies.
       error_isvc (soft) -> success_isvc(soft)

    Expectation: IG will return response of success_isvc and predict_ig will not raise any exception
    :return:
    """

    logging.info("starting test")
    logging.info(f"SUCCESS_ISVC_IMAGE is {SUCCESS_ISVC_IMAGE}")
    logging.info(f"ERROR_ISVC_IMAGE is {ERROR_ISVC_IMAGE}")

    # Create success isvc
    model_name = success_isvc_name = "success-200-isvc"
    success_isvc = construct_isvc_to_submit(
        success_isvc_name, image=SUCCESS_ISVC_IMAGE, model_name=model_name
    )

    # Create error isvc
    model_name = error_isvc_name = "error-404-isvc"
    error_isvc = construct_isvc_to_submit(
        error_isvc_name, image=ERROR_ISVC_IMAGE, model_name=model_name
    )

    # Create graph
    graph_name = "sequence-graph"

    nodes = {
        "root": V1alpha1InferenceRouter(
            router_type="Sequence",
            steps=[
                V1alpha1InferenceStep(
                    service_name=error_isvc_name,
                ),
                V1alpha1InferenceStep(
                    service_name=success_isvc_name,
                    data="$request",
                ),
            ],
        )
    }
    graph_spec = V1alpha1InferenceGraphSpec(
        nodes=nodes,
    )
    ig = V1alpha1InferenceGraph(
        api_version=constants.KSERVE_V1ALPHA1,
        kind=constants.KSERVE_KIND_INFERENCEGRAPH,
        metadata=client.V1ObjectMeta(name=graph_name, namespace=KSERVE_TEST_NAMESPACE),
        spec=graph_spec,
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(success_isvc)
    kserve_client.create(error_isvc)
    kserve_client.create_inference_graph(ig)

    kserve_client.wait_isvc_ready(success_isvc_name, namespace=KSERVE_TEST_NAMESPACE)
    kserve_client.wait_isvc_ready(error_isvc_name, namespace=KSERVE_TEST_NAMESPACE)
    kserve_client.wait_ig_ready(graph_name, namespace=KSERVE_TEST_NAMESPACE)


    response = predict_ig(graph_name, "/Users/rchauhan4/Desktop/git-personal/rachitchauhan43-forks/kserve/test/e2e/data/iris_input.json")

    assert response == {"message": "SUCCESS"}


    kserve_client.delete_inference_graph(graph_name, KSERVE_TEST_NAMESPACE)
    kserve_client.delete(success_isvc_name, KSERVE_TEST_NAMESPACE)
    kserve_client.delete(error_isvc_name, KSERVE_TEST_NAMESPACE)


def create_ig_using_custom_object_api(resource_body):
    config.load_kube_config()
    k8s_client_custom_object = client.CustomObjectsApi()
    try:
        resource = k8s_client_custom_object.create_namespaced_custom_object(
            group="serving.kserve.io",
            version="v1alpha1",
            plural="inferencegraphs",
            namespace="kserve-ci-e2e-test",
            body=resource_body,
        )
    except Exception as e:
        raise e

@pytest.mark.rc4test
@pytest.mark.graph
@pytest.mark.kourier
def test_ig_scenario3():
    """
     Scenario: Sequence graph with 2 steps - first is hard (and returns non-200) and second is soft dependency.
     error_isvc(hard) -> success_isvc (soft)

    Expectation: IG will return response of error_isvc and predict_ig will raise exception
    """
    logging.info("starting test")
    logging.info(f"SUCCESS_ISVC_IMAGE is {SUCCESS_ISVC_IMAGE}")
    logging.info(f"ERROR_ISVC_IMAGE is {ERROR_ISVC_IMAGE}")

    # Create success isvc
    model_name = success_isvc_name = "success-200-isvc"
    success_isvc = construct_isvc_to_submit(
        success_isvc_name, image=SUCCESS_ISVC_IMAGE, model_name=model_name
    )

    # Create error isvc
    model_name = error_isvc_name = "error-404-isvc"
    error_isvc = construct_isvc_to_submit(
        error_isvc_name, image=ERROR_ISVC_IMAGE, model_name=model_name
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(success_isvc)
    kserve_client.create(error_isvc)

    # Create graph
    graph_name = "sequence-graph"
    deployment_yaml_path = "graph/test-resources/ig_test_scenario_3.yaml"
    # Read YAML file
    with open(deployment_yaml_path, 'r') as stream:
        resource_body = yaml.safe_load(stream)
        create_ig_using_custom_object_api(resource_body)

    kserve_client.wait_isvc_ready(success_isvc_name, namespace=KSERVE_TEST_NAMESPACE)
    kserve_client.wait_isvc_ready(error_isvc_name, namespace=KSERVE_TEST_NAMESPACE)
    kserve_client.wait_ig_ready(graph_name, namespace=KSERVE_TEST_NAMESPACE)

    with pytest.raises(HTTPError) as exc_info:
        predict_ig(graph_name, "./data/iris_input.json")

    assert exc_info.value.response.json() == {"detail": "Intentional 404 code"}
    assert exc_info.value.response.status_code == 404

    kserve_client.delete_inference_graph(graph_name, KSERVE_TEST_NAMESPACE)
    kserve_client.delete(success_isvc_name, KSERVE_TEST_NAMESPACE)
    kserve_client.delete(error_isvc_name, KSERVE_TEST_NAMESPACE)

