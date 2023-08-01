import logging
import os

import pytest
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
from kubernetes import client
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


def construct_isvc_to_submit(service_name, image):
    predictor = V1beta1PredictorSpec(
        containers=[
            V1Container(
                name="kserve-container",
                image=image,
                resources=V1ResourceRequirements(
                    requests={"cpu": "50m", "memory": "128Mi"},
                    limits={"cpu": "100m", "memory": "1Gi"},
                ),
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
    Scenario: Sequence graph with 2 nodes: error_isvc (soft) -> success_isvc(soft)
    Expectation: IG will return response of success_isvc
    :return:
    """

    logging.info("starting test")
    logging.info(f"SUCCESS_ISVC_IMAGE is {SUCCESS_ISVC_IMAGE}")
    logging.info(f"ERROR_ISVC_IMAGE is {ERROR_ISVC_IMAGE}")
    model_name = "model"

    # Create success isvc
    success_isvc_name = "success-200-isvc"
    success_isvc = construct_isvc_to_submit(success_isvc_name, image=SUCCESS_ISVC_IMAGE)

    # Create error isvc
    error_isvc_name = "error-404-isvc"
    error_isvc = construct_isvc_to_submit(error_isvc_name, image=ERROR_ISVC_IMAGE)

    # Create graph
    graph_name = "sequence-graph"

    nodes = {
        "root": V1alpha1InferenceRouter(
            router_type="Sequence",
            steps=[
                V1alpha1InferenceStep(
                    service_url=f"http://{success_isvc_name}.{KSERVE_TEST_NAMESPACE}.svc.cluster.local"
                                f"/v1/models/{model_name}:predict",
                ),
                V1alpha1InferenceStep(
                    service_url=f"http://{error_isvc_name}.{KSERVE_TEST_NAMESPACE}.svc.cluster.local"
                                f"/v1/models/{model_name}:predict",
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

    # res = predict(success_isvc_name,
    #               input_json="./data/custom_model_input_v2.json",
    #               protocol_version="v1", model_name=model_name)
    try:
        res = predict_ig(graph_name, "./data/iris_input.json")
        logging.info(f"result returned is = {res}")
    except Exception as e:
        assert e.response.json() == {"detail": "Intentional 404 code"}
        assert e.response.status_code == 404

    kserve_client.delete_inference_graph(graph_name, KSERVE_TEST_NAMESPACE)
    kserve_client.delete(success_isvc_name, KSERVE_TEST_NAMESPACE)
    kserve_client.delete(error_isvc_name, KSERVE_TEST_NAMESPACE)
