import os

from kubernetes import client
from kubernetes.client import V1ResourceRequirements

from kserve import V1beta1PredictorSpec, V1beta1SKLearnSpec, V1beta1InferenceServiceSpec, V1beta1InferenceService, \
    constants, KServeClient, V1alpha1InferenceGraphSpec, V1alpha1InferenceRouter, V1alpha1InferenceGraph, \
    V1alpha1InferenceStep, V1beta1XGBoostSpec
import pytest
from ..common.utils import KSERVE_TEST_NAMESPACE, predict_ig


@pytest.mark.graph
def test_inference_graph():
    sklearn_name = "isvc-sklearn"
    xgb_name = "isvc-xgboost"
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
            name=sklearn_name,
            namespace=KSERVE_TEST_NAMESPACE
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
        metadata=client.V1ObjectMeta(
            name=xgb_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(predictor=xgb_predictor),
    )

    nodes = {"root": V1alpha1InferenceRouter(
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
    )}
    graph_spec = V1alpha1InferenceGraphSpec(
        nodes=nodes,
    )
    ig = V1alpha1InferenceGraph(
        api_version=constants.KSERVE_V1ALPHA1,
        kind=constants.KSERVE_KIND_INFERENCEGRAPH,
        metadata=client.V1ObjectMeta(
            name=graph_name,
            namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=graph_spec,
    )

    kserve_client = KServeClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))
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
