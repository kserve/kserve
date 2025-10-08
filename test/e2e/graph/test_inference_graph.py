import os
import uuid

import pytest
import yaml
from jinja2 import Template
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
from kserve.logging import trace_logger as logger
from kubernetes import client, config
from kubernetes.client import V1Container
from kubernetes.client import V1ResourceRequirements
from httpx import HTTPStatusError

from ..common.utils import KSERVE_TEST_NAMESPACE, predict_ig

if os.environ.get("SUCCESS_200_ISVC_IMAGE") is not None:
    SUCCESS_ISVC_IMAGE = os.environ.get("SUCCESS_200_ISVC_IMAGE")
else:
    SUCCESS_ISVC_IMAGE = "kserve/success-200-isvc:" + os.environ.get("GITHUB_SHA")
if os.environ.get("ERROR_404_ISVC_IMAGE") is not None:
    ERROR_ISVC_IMAGE = os.environ.get("ERROR_404_ISVC_IMAGE")
else:
    ERROR_ISVC_IMAGE = "kserve/error-404-isvc:" + os.environ.get("GITHUB_SHA")
IG_TEST_RESOURCES_BASE_LOCATION = "graph/test-resources"


@pytest.mark.graph
@pytest.mark.kourier
@pytest.mark.asyncio(scope="session")
async def test_inference_graph(rest_v1_client):
    logger.info("Starting test test_inference_graph")
    sklearn_name_1 = "isvc-sklearn-graph-1"
    sklearn_name_2 = "isvc-sklearn-graph-2"
    xgb_name = "isvc-xgboost-graph"
    graph_name = "model-chainer"

    sklearn_predictor_1 = V1beta1PredictorSpec(
        min_replicas=1,
        sklearn=V1beta1SKLearnSpec(
            storage_uri="gs://kfserving-examples/models/sklearn/1.0/model",
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "256Mi"},
            ),
            args=["--model_name=sklearn"],
        ),
    )
    sklearn_predictor_2 = V1beta1PredictorSpec(
        min_replicas=1,
        sklearn=V1beta1SKLearnSpec(
            storage_uri="gs://kfserving-examples/models/sklearn/1.0/model",
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "256Mi"},
            ),
            args=["--model_name", "iris"],
        ),
    )
    sklearn_isvc_1 = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=sklearn_name_1, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(predictor=sklearn_predictor_1),
    )
    sklearn_isvc_2 = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=sklearn_name_2, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(predictor=sklearn_predictor_2),
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
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(name=xgb_name, namespace=KSERVE_TEST_NAMESPACE),
        spec=V1beta1InferenceServiceSpec(predictor=xgb_predictor),
    )

    nodes = {
        "root": V1alpha1InferenceRouter(
            router_type="Sequence",
            steps=[
                V1alpha1InferenceStep(
                    service_name=sklearn_name_1,
                    dependency="Hard",
                ),
                V1alpha1InferenceStep(
                    service_name=xgb_name,
                    data="$request",
                    dependency="Hard",
                ),
                V1alpha1InferenceStep(
                    service_name=sklearn_name_2,
                    data="$request",
                    dependency="Hard",
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
    kserve_client.create(sklearn_isvc_1)
    kserve_client.create(xgb_isvc)
    kserve_client.create(sklearn_isvc_2)
    kserve_client.wait_isvc_ready(sklearn_name_1, namespace=KSERVE_TEST_NAMESPACE)
    kserve_client.wait_isvc_ready(xgb_name, namespace=KSERVE_TEST_NAMESPACE)
    kserve_client.wait_isvc_ready(sklearn_name_2, namespace=KSERVE_TEST_NAMESPACE)

    kserve_client.create_inference_graph(ig)
    kserve_client.wait_ig_ready(graph_name, namespace=KSERVE_TEST_NAMESPACE)

    res = await predict_ig(
        rest_v1_client,
        graph_name,
        os.path.join(IG_TEST_RESOURCES_BASE_LOCATION, "iris_input.json"),
    )
    assert res["predictions"] == [1, 1]

    kserve_client.delete_inference_graph(graph_name, KSERVE_TEST_NAMESPACE)
    kserve_client.delete(sklearn_name_1, KSERVE_TEST_NAMESPACE)
    kserve_client.delete(xgb_name, KSERVE_TEST_NAMESPACE)
    kserve_client.delete(sklearn_name_2, KSERVE_TEST_NAMESPACE)


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

    return resource


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
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    return isvc


def setup_isvcs_for_test(suffix):
    logger.info(f"SUCCESS_ISVC_IMAGE is {SUCCESS_ISVC_IMAGE}")
    logger.info(f"ERROR_ISVC_IMAGE is {ERROR_ISVC_IMAGE}")

    # construct_isvc_to_submit
    model_name = success_isvc_name = ("-").join(["success-200-isvc", suffix])
    success_isvc = construct_isvc_to_submit(
        success_isvc_name, image=SUCCESS_ISVC_IMAGE, model_name=model_name
    )

    # construct_isvc_to_submit
    model_name = error_isvc_name = ("-").join(["error-404-isvc", suffix])
    error_isvc = construct_isvc_to_submit(
        error_isvc_name, image=ERROR_ISVC_IMAGE, model_name=model_name
    )

    return success_isvc_name, error_isvc_name, success_isvc, error_isvc


@pytest.mark.graph
@pytest.mark.kourier
@pytest.mark.asyncio(scope="session")
async def test_ig_scenario1(rest_v1_client):
    """
    Scenario: Sequence graph with 2 steps that are both soft dependencies.
     success_isvc(soft) -> error_isvc (soft)

    We are not marking steps as soft or hard explicitly so this will test that default behavior of steps being soft
    is as expected.
    Expectation: IG will return response of error_isvc and predict_ig will raise exception
    :return:
    """

    logger.info("Starting test test_ig_scenario1")
    suffix = str(uuid.uuid4())[1:6]
    success_isvc_name, error_isvc_name, success_isvc, error_isvc = setup_isvcs_for_test(
        suffix
    )
    logger.info(f"success_isvc_name is {success_isvc_name}")
    logger.info(f"error_isvc_name is {error_isvc_name}")

    # Create graph
    graph_name = "-".join(["sequence-graph", suffix])

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
    kserve_client.wait_isvc_ready(success_isvc_name, namespace=KSERVE_TEST_NAMESPACE)
    kserve_client.wait_isvc_ready(error_isvc_name, namespace=KSERVE_TEST_NAMESPACE)

    kserve_client.create_inference_graph(ig)
    kserve_client.wait_ig_ready(graph_name, namespace=KSERVE_TEST_NAMESPACE)

    with pytest.raises(HTTPStatusError) as exc_info:
        await predict_ig(
            rest_v1_client,
            graph_name,
            os.path.join(
                IG_TEST_RESOURCES_BASE_LOCATION, "custom_predictor_input.json"
            ),
        )

    assert exc_info.value.response.json() == {"detail": "Intentional 404 code"}
    assert exc_info.value.response.status_code == 404

    kserve_client.delete_inference_graph(graph_name, KSERVE_TEST_NAMESPACE)
    kserve_client.delete(success_isvc_name, KSERVE_TEST_NAMESPACE)
    kserve_client.delete(error_isvc_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.graph
@pytest.mark.kourier
@pytest.mark.asyncio(scope="session")
async def test_ig_scenario2(rest_v1_client):
    """
    Scenario: Sequence graph with 2 steps that are both soft dependencies.
       error_isvc (soft) -> success_isvc(soft)

    Expectation: IG will return response of success_isvc and predict_ig will not raise any exception
    :return:
    """

    logger.info("Starting test test_ig_scenario2")
    suffix = str(uuid.uuid4())[1:6]
    success_isvc_name, error_isvc_name, success_isvc, error_isvc = setup_isvcs_for_test(
        suffix
    )
    logger.info(f"success_isvc_name is {success_isvc_name}")
    logger.info(f"error_isvc_name is {error_isvc_name}")

    # Create graph
    graph_name = "-".join(["sequence-graph", suffix])

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
    kserve_client.wait_isvc_ready(success_isvc_name, namespace=KSERVE_TEST_NAMESPACE)
    kserve_client.wait_isvc_ready(error_isvc_name, namespace=KSERVE_TEST_NAMESPACE)

    kserve_client.create_inference_graph(ig)
    kserve_client.wait_ig_ready(graph_name, namespace=KSERVE_TEST_NAMESPACE)

    response = await predict_ig(
        rest_v1_client,
        graph_name,
        os.path.join(IG_TEST_RESOURCES_BASE_LOCATION, "custom_predictor_input.json"),
    )
    assert response == {"predictions": [{"message": "SUCCESS"}]}

    kserve_client.delete_inference_graph(graph_name, KSERVE_TEST_NAMESPACE)
    kserve_client.delete(success_isvc_name, KSERVE_TEST_NAMESPACE)
    kserve_client.delete(error_isvc_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.graph
@pytest.mark.kourier
@pytest.mark.asyncio(scope="session")
async def test_ig_scenario3(rest_v1_client):
    """
     Scenario: Sequence graph with 2 steps - first is hard (and returns non-200) and second is soft dependency.
     error_isvc(hard) -> success_isvc (soft)

    Expectation: IG will return response of error_isvc and predict_ig will raise exception
    """
    logger.info("Starting test test_ig_scenario3")
    suffix = str(uuid.uuid4())[1:6]
    success_isvc_name, error_isvc_name, success_isvc, error_isvc = setup_isvcs_for_test(
        suffix
    )
    logger.info(f"success_isvc_name is {success_isvc_name}")
    logger.info(f"error_isvc_name is {error_isvc_name}")

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(success_isvc)
    kserve_client.create(error_isvc)

    # Create graph
    graph_name = "-".join(["sequence-graph", suffix])

    # Because we run from test/e2e location in run-e2e-tests.sh
    deployment_yaml_path = os.path.join(
        IG_TEST_RESOURCES_BASE_LOCATION, "ig_test_seq_scenario_3.yaml"
    )

    # Read YAML file
    with open(deployment_yaml_path, "r") as stream:
        file_content = stream.read()
        resource_template = Template(file_content)
        substitutions = {
            "graph_name": graph_name,
            "error_404_isvc_id": error_isvc_name,
            "success_200_isvc_id": success_isvc_name,
        }
        resource_body_after_rendering = yaml.safe_load(
            resource_template.render(substitutions)
        )

    kserve_client.wait_isvc_ready(success_isvc_name, namespace=KSERVE_TEST_NAMESPACE)
    kserve_client.wait_isvc_ready(error_isvc_name, namespace=KSERVE_TEST_NAMESPACE)

    create_ig_using_custom_object_api(resource_body_after_rendering)
    kserve_client.wait_ig_ready(graph_name, namespace=KSERVE_TEST_NAMESPACE)

    with pytest.raises(HTTPStatusError) as exc_info:
        await predict_ig(
            rest_v1_client,
            graph_name,
            os.path.join(
                IG_TEST_RESOURCES_BASE_LOCATION, "custom_predictor_input.json"
            ),
        )

    assert exc_info.value.response.json() == {"detail": "Intentional 404 code"}
    assert exc_info.value.response.status_code == 404

    kserve_client.delete_inference_graph(graph_name, KSERVE_TEST_NAMESPACE)
    kserve_client.delete(success_isvc_name, KSERVE_TEST_NAMESPACE)
    kserve_client.delete(error_isvc_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.graph
@pytest.mark.kourier
@pytest.mark.asyncio(scope="session")
async def test_ig_scenario4(rest_v1_client):
    """
    Scenario: Switch graph with 1 step as hard dependency and other one as soft dependency.
    Will be testing 3 cases in this test case:
    Expectation:
    Case 1. IG will return response of error_isvc when condition for that step matches
    Case 2. IG will return response of success_isvc when condition for that step matches
    Case 3. IG will return 404 with error message when no condition matches
       {
               "error": "Failed to process request",
               "cause": "None of the routes matched with the switch condition",
       }
    """
    logger.info("Starting test test_ig_scenario4")
    suffix = str(uuid.uuid4())[1:6]
    success_isvc_name, error_isvc_name, success_isvc, error_isvc = setup_isvcs_for_test(
        suffix
    )
    logger.info(f"success_isvc_name is {success_isvc_name}")
    logger.info(f"error_isvc_name is {error_isvc_name}")

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(success_isvc)
    kserve_client.create(error_isvc)

    # Create graph
    graph_name = "-".join(["switch-graph", suffix])

    # Because we run from test/e2e location in run-e2e-tests.sh
    deployment_yaml_path = os.path.join(
        IG_TEST_RESOURCES_BASE_LOCATION, "ig_test_switch_scenario_4.yaml"
    )

    # Read YAML file
    with open(deployment_yaml_path, "r") as stream:
        file_content = stream.read()
        resource_template = Template(file_content)
        substitutions = {
            "graph_name": graph_name,
            "error_404_isvc_id": error_isvc_name,
            "success_200_isvc_id": success_isvc_name,
        }
        resource_body_after_rendering = yaml.safe_load(
            resource_template.render(substitutions)
        )
    kserve_client.wait_isvc_ready(success_isvc_name, namespace=KSERVE_TEST_NAMESPACE)
    kserve_client.wait_isvc_ready(error_isvc_name, namespace=KSERVE_TEST_NAMESPACE)

    create_ig_using_custom_object_api(resource_body_after_rendering)
    kserve_client.wait_ig_ready(graph_name, namespace=KSERVE_TEST_NAMESPACE)

    # Case 1
    with pytest.raises(HTTPStatusError) as exc_info:
        await predict_ig(
            rest_v1_client,
            graph_name,
            os.path.join(
                IG_TEST_RESOURCES_BASE_LOCATION, "switch_call_error_picker_input.json"
            ),
        )

    assert exc_info.value.response.json() == {"detail": "Intentional 404 code"}
    assert exc_info.value.response.status_code == 404

    # Case 2
    response = await predict_ig(
        rest_v1_client,
        graph_name,
        os.path.join(
            IG_TEST_RESOURCES_BASE_LOCATION, "switch_call_success_picker_input.json"
        ),
    )
    assert response == {"predictions": [{"message": "SUCCESS"}]}

    # Case 3
    with pytest.raises(HTTPStatusError) as exc_info:
        await predict_ig(
            rest_v1_client,
            graph_name,
            os.path.join(
                IG_TEST_RESOURCES_BASE_LOCATION, "switch_call_no_match_input.json"
            ),
        )

    assert exc_info.value.response.json() == {
        "error": "Failed to process request",
        "cause": "None of the routes matched with the switch condition",
    }
    assert exc_info.value.response.status_code == 404

    kserve_client.delete_inference_graph(graph_name, KSERVE_TEST_NAMESPACE)
    kserve_client.delete(success_isvc_name, KSERVE_TEST_NAMESPACE)
    kserve_client.delete(error_isvc_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.graph
@pytest.mark.kourier
@pytest.mark.asyncio(scope="session")
async def test_ig_scenario5(rest_v1_client):
    """
    Scenario: Switch graph where a match would happen for error node and then error would return but IG will continue
    execution and call the next step in the flow as error step will be a soft dependency.
    Expectation: IG will return response of success_isvc.
    """
    logger.info("Starting test test_ig_scenario5")
    suffix = str(uuid.uuid4())[1:6]
    success_isvc_name, error_isvc_name, success_isvc, error_isvc = setup_isvcs_for_test(
        suffix
    )
    logger.info(f"success_isvc_name is {success_isvc_name}")
    logger.info(f"error_isvc_name is {error_isvc_name}")

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(success_isvc)
    kserve_client.create(error_isvc)

    # Create graph
    graph_name = "-".join(["switch-graph", suffix])

    # Because we run from test/e2e location in run-e2e-tests.sh
    deployment_yaml_path = os.path.join(
        IG_TEST_RESOURCES_BASE_LOCATION, "ig_test_switch_scenario_5.yaml"
    )

    # Read YAML file
    with open(deployment_yaml_path, "r") as stream:
        file_content = stream.read()
        resource_template = Template(file_content)
        substitutions = {
            "graph_name": graph_name,
            "error_404_isvc_id": error_isvc_name,
            "success_200_isvc_id": success_isvc_name,
        }
        resource_body_after_rendering = yaml.safe_load(
            resource_template.render(substitutions)
        )

    kserve_client.wait_isvc_ready(success_isvc_name, namespace=KSERVE_TEST_NAMESPACE)
    kserve_client.wait_isvc_ready(error_isvc_name, namespace=KSERVE_TEST_NAMESPACE)

    create_ig_using_custom_object_api(resource_body_after_rendering)
    kserve_client.wait_ig_ready(graph_name, namespace=KSERVE_TEST_NAMESPACE)

    response = await predict_ig(
        rest_v1_client,
        graph_name,
        os.path.join(
            IG_TEST_RESOURCES_BASE_LOCATION, "switch_call_error_picker_input.json"
        ),
    )
    assert response == {"predictions": [{"message": "SUCCESS"}]}

    kserve_client.delete_inference_graph(graph_name, KSERVE_TEST_NAMESPACE)
    kserve_client.delete(success_isvc_name, KSERVE_TEST_NAMESPACE)
    kserve_client.delete(error_isvc_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.graph
@pytest.mark.kourier
@pytest.mark.asyncio(scope="session")
async def test_ig_scenario6(rest_v1_client):
    """
    Scenario: Switch graph where a match would happen for error node and then error would return and IG will NOT
    continue execution and call the next step in the flow as error step will be a HARD dependency.
    Expectation: IG will return response of success_isvc.
    """
    logger.info("Starting test test_ig_scenario6")
    suffix = str(uuid.uuid4())[1:6]
    success_isvc_name, error_isvc_name, success_isvc, error_isvc = setup_isvcs_for_test(
        suffix
    )
    logger.info(f"success_isvc_name is {success_isvc_name}")
    logger.info(f"error_isvc_name is {error_isvc_name}")

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(success_isvc)
    kserve_client.create(error_isvc)

    # Create graph
    graph_name = "-".join(["switch-graph", suffix])

    # Because we run from test/e2e location in run-e2e-tests.sh
    deployment_yaml_path = os.path.join(
        IG_TEST_RESOURCES_BASE_LOCATION, "ig_test_switch_scenario_6.yaml"
    )

    # Read YAML file
    with open(deployment_yaml_path, "r") as stream:
        file_content = stream.read()
        resource_template = Template(file_content)
        substitutions = {
            "graph_name": graph_name,
            "error_404_isvc_id": error_isvc_name,
            "success_200_isvc_id": success_isvc_name,
        }
        resource_body_after_rendering = yaml.safe_load(
            resource_template.render(substitutions)
        )

    kserve_client.wait_isvc_ready(success_isvc_name, namespace=KSERVE_TEST_NAMESPACE)
    kserve_client.wait_isvc_ready(error_isvc_name, namespace=KSERVE_TEST_NAMESPACE)

    create_ig_using_custom_object_api(resource_body_after_rendering)
    kserve_client.wait_ig_ready(graph_name, namespace=KSERVE_TEST_NAMESPACE)

    with pytest.raises(HTTPStatusError) as exc_info:
        await predict_ig(
            rest_v1_client,
            graph_name,
            os.path.join(
                IG_TEST_RESOURCES_BASE_LOCATION, "switch_call_error_picker_input.json"
            ),
        )

    assert exc_info.value.response.json() == {"detail": "Intentional 404 code"}
    assert exc_info.value.response.status_code == 404

    kserve_client.delete_inference_graph(graph_name, KSERVE_TEST_NAMESPACE)
    kserve_client.delete(success_isvc_name, KSERVE_TEST_NAMESPACE)
    kserve_client.delete(error_isvc_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.graph
@pytest.mark.kourier
@pytest.mark.asyncio(scope="session")
async def test_ig_scenario7(rest_v1_client):
    """
    Scenario: Ensemble graph with 2 steps, where both the steps are soft deps.

    Expectation: IG will return combined response of both the steps.
    """
    logger.info("Starting test test_ig_scenario7")
    suffix = str(uuid.uuid4())[1:6]
    success_isvc_name, error_isvc_name, success_isvc, error_isvc = setup_isvcs_for_test(
        suffix
    )
    logger.info(f"success_isvc_name is {success_isvc_name}")
    logger.info(f"error_isvc_name is {error_isvc_name}")

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(success_isvc)
    kserve_client.create(error_isvc)

    # Create graph
    graph_name = "-".join(["ensemble-graph", suffix])

    # Because we run from test/e2e location in run-e2e-tests.sh
    deployment_yaml_path = os.path.join(
        IG_TEST_RESOURCES_BASE_LOCATION, "ig_test_ensemble_scenario_7.yaml"
    )

    # Read YAML file
    with open(deployment_yaml_path, "r") as stream:
        file_content = stream.read()
        resource_template = Template(file_content)
        substitutions = {
            "graph_name": graph_name,
            "error_404_isvc_id": error_isvc_name,
            "success_200_isvc_id": success_isvc_name,
        }
        resource_body_after_rendering = yaml.safe_load(
            resource_template.render(substitutions)
        )

    kserve_client.wait_isvc_ready(success_isvc_name, namespace=KSERVE_TEST_NAMESPACE)
    kserve_client.wait_isvc_ready(error_isvc_name, namespace=KSERVE_TEST_NAMESPACE)

    create_ig_using_custom_object_api(resource_body_after_rendering)
    kserve_client.wait_ig_ready(graph_name, namespace=KSERVE_TEST_NAMESPACE)

    response = await predict_ig(
        rest_v1_client,
        graph_name,
        os.path.join(
            IG_TEST_RESOURCES_BASE_LOCATION, "switch_call_success_picker_input.json"
        ),
    )
    assert response == {
        "rootStep1": {"predictions": [{"message": "SUCCESS"}]},
        "rootStep2": {"detail": "Intentional 404 code"},
    }

    kserve_client.delete_inference_graph(graph_name, KSERVE_TEST_NAMESPACE)
    kserve_client.delete(success_isvc_name, KSERVE_TEST_NAMESPACE)
    kserve_client.delete(error_isvc_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.graph
@pytest.mark.kourier
@pytest.mark.asyncio(scope="session")
async def test_ig_scenario8(rest_v1_client):
    """
    Scenario: Ensemble graph with 3 steps, where 2 steps are soft and 1 step is hard and returns non-200

    Expectation: Since HARD step will return non-200, so IG will return that step's output as IG's output
    """
    logger.info("Starting test test_ig_scenario8")
    suffix = str(uuid.uuid4())[1:6]
    success_isvc_name, error_isvc_name, success_isvc, error_isvc = setup_isvcs_for_test(
        suffix
    )
    logger.info(f"success_isvc_name is {success_isvc_name}")
    logger.info(f"error_isvc_name is {error_isvc_name}")

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(success_isvc)
    kserve_client.create(error_isvc)

    # Create graph
    graph_name = "-".join(["ensemble-graph", suffix])

    # Because we run from test/e2e location in run-e2e-tests.sh
    deployment_yaml_path = os.path.join(
        IG_TEST_RESOURCES_BASE_LOCATION, "ig_test_ensemble_scenario_8.yaml"
    )

    # Read YAML file
    with open(deployment_yaml_path, "r") as stream:
        file_content = stream.read()
        resource_template = Template(file_content)
        substitutions = {
            "graph_name": graph_name,
            "error_404_isvc_id": error_isvc_name,
            "success_200_isvc_id": success_isvc_name,
        }
        resource_body_after_rendering = yaml.safe_load(
            resource_template.render(substitutions)
        )

    kserve_client.wait_isvc_ready(success_isvc_name, namespace=KSERVE_TEST_NAMESPACE)
    kserve_client.wait_isvc_ready(error_isvc_name, namespace=KSERVE_TEST_NAMESPACE)

    create_ig_using_custom_object_api(resource_body_after_rendering)
    kserve_client.wait_ig_ready(graph_name, namespace=KSERVE_TEST_NAMESPACE)

    with pytest.raises(HTTPStatusError) as exc_info:
        await predict_ig(
            rest_v1_client,
            graph_name,
            os.path.join(
                IG_TEST_RESOURCES_BASE_LOCATION, "switch_call_success_picker_input.json"
            ),
        )

    assert exc_info.value.response.json() == {"detail": "Intentional 404 code"}

    kserve_client.delete_inference_graph(graph_name, KSERVE_TEST_NAMESPACE)
    kserve_client.delete(success_isvc_name, KSERVE_TEST_NAMESPACE)
    kserve_client.delete(error_isvc_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.graph
@pytest.mark.kourier
@pytest.mark.asyncio(scope="session")
async def test_ig_scenario9(rest_v1_client):
    """
    Scenario: Splitter graph where a match would happen for error node and then error would return but IG will continue
    execution and call the next step in the flow as error step will be a soft dependency.
    Expectation: IG will return response of success_isvc.
    """
    logger.info("Starting test test_ig_scenario9")
    suffix = str(uuid.uuid4())[1:6]
    success_isvc_name, error_isvc_name, success_isvc, error_isvc = setup_isvcs_for_test(
        suffix
    )
    logger.info(f"success_isvc_name is {success_isvc_name}")
    logger.info(f"error_isvc_name is {error_isvc_name}")

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(success_isvc)
    kserve_client.create(error_isvc)

    # Create graph
    graph_name = "-".join(["splitter-graph", suffix])

    # Because we run from test/e2e location in run-e2e-tests.sh
    deployment_yaml_path = os.path.join(
        IG_TEST_RESOURCES_BASE_LOCATION, "ig_test_switch_scenario_9.yaml"
    )

    # Read YAML file
    with open(deployment_yaml_path, "r") as stream:
        file_content = stream.read()
        resource_template = Template(file_content)
        substitutions = {
            "graph_name": graph_name,
            "error_404_isvc_id": error_isvc_name,
            "success_200_isvc_id": success_isvc_name,
        }
        resource_body_after_rendering = yaml.safe_load(
            resource_template.render(substitutions)
        )

    kserve_client.wait_isvc_ready(success_isvc_name, namespace=KSERVE_TEST_NAMESPACE)
    kserve_client.wait_isvc_ready(error_isvc_name, namespace=KSERVE_TEST_NAMESPACE)

    create_ig_using_custom_object_api(resource_body_after_rendering)
    kserve_client.wait_ig_ready(graph_name, namespace=KSERVE_TEST_NAMESPACE)

    response = await predict_ig(
        rest_v1_client,
        graph_name,
        os.path.join(IG_TEST_RESOURCES_BASE_LOCATION, "iris_input.json"),
    )
    assert response == {"predictions": [{"message": "SUCCESS"}]}

    kserve_client.delete_inference_graph(graph_name, KSERVE_TEST_NAMESPACE)
    kserve_client.delete(success_isvc_name, KSERVE_TEST_NAMESPACE)
    kserve_client.delete(error_isvc_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.graph
@pytest.mark.kourier
@pytest.mark.asyncio(scope="session")
async def test_ig_scenario10(rest_v1_client):
    """
    Scenario: Splitter graph where a match would happen for error node and then error would return and IG will NOT
    continue execution and call the next step in the flow as error step will be a HARD dependency.
    Expectation: IG will return response of success_isvc.
    """
    logger.info("Starting test test_ig_scenario10")
    suffix = str(uuid.uuid4())[1:6]
    success_isvc_name, error_isvc_name, success_isvc, error_isvc = setup_isvcs_for_test(
        suffix
    )
    logger.info(f"success_isvc_name is {success_isvc_name}")
    logger.info(f"error_isvc_name is {error_isvc_name}")

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(success_isvc)
    kserve_client.create(error_isvc)

    # Create graph
    graph_name = "-".join(["splitter-graph", suffix])

    # Because we run from test/e2e location in run-e2e-tests.sh
    deployment_yaml_path = os.path.join(
        IG_TEST_RESOURCES_BASE_LOCATION, "ig_test_switch_scenario_10.yaml"
    )

    # Read YAML file
    with open(deployment_yaml_path, "r") as stream:
        file_content = stream.read()
        resource_template = Template(file_content)
        substitutions = {
            "graph_name": graph_name,
            "error_404_isvc_id": error_isvc_name,
            "success_200_isvc_id": success_isvc_name,
        }
        resource_body_after_rendering = yaml.safe_load(
            resource_template.render(substitutions)
        )

    kserve_client.wait_isvc_ready(success_isvc_name, namespace=KSERVE_TEST_NAMESPACE)
    kserve_client.wait_isvc_ready(error_isvc_name, namespace=KSERVE_TEST_NAMESPACE)

    create_ig_using_custom_object_api(resource_body_after_rendering)
    kserve_client.wait_ig_ready(graph_name, namespace=KSERVE_TEST_NAMESPACE)

    with pytest.raises(HTTPStatusError) as exc_info:
        await predict_ig(
            rest_v1_client,
            graph_name,
            os.path.join(IG_TEST_RESOURCES_BASE_LOCATION, "iris_input.json"),
        )

    assert exc_info.value.response.json() == {"detail": "Intentional 404 code"}
    assert exc_info.value.response.status_code == 404

    kserve_client.delete_inference_graph(graph_name, KSERVE_TEST_NAMESPACE)
    kserve_client.delete(success_isvc_name, KSERVE_TEST_NAMESPACE)
    kserve_client.delete(error_isvc_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.raw
@pytest.mark.asyncio(scope="session")
async def test_inference_graph_raw_mode(rest_v1_client, network_layer):
    logger.info("Starting test test_inference_graph_raw_mode")
    suffix = str(uuid.uuid4())[1:6]
    sklearn_name = "isvc-sklearn-graph-raw-" + suffix
    xgb_name = "isvc-xgboost-graph-raw-" + suffix
    graph_name = "model-chainer-raw-" + suffix

    annotations = dict()
    annotations["serving.kserve.io/deploymentMode"] = "RawDeployment"
    labels = dict()
    labels["networking.kserve.io/visibility"] = "exposed"

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
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=sklearn_name,
            namespace=KSERVE_TEST_NAMESPACE,
            annotations=annotations,
            labels=labels,
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
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=xgb_name,
            namespace=KSERVE_TEST_NAMESPACE,
            annotations=annotations,
            labels=labels,
        ),
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
        metadata=client.V1ObjectMeta(
            name=graph_name, namespace=KSERVE_TEST_NAMESPACE, annotations=annotations
        ),
        spec=graph_spec,
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(sklearn_isvc)
    kserve_client.create(xgb_isvc)
    kserve_client.wait_isvc_ready(sklearn_name, namespace=KSERVE_TEST_NAMESPACE)
    kserve_client.wait_isvc_ready(xgb_name, namespace=KSERVE_TEST_NAMESPACE)

    kserve_client.create_inference_graph(ig)
    kserve_client.wait_ig_ready(graph_name, namespace=KSERVE_TEST_NAMESPACE)

    # Below checks are raw deployment specific.  They ensure raw k8s resources created instead of knative resources
    dep = kserve_client.app_api.read_namespaced_deployment(
        graph_name, namespace=KSERVE_TEST_NAMESPACE
    )
    if not dep:
        raise RuntimeError(
            "Deployment doesn't exist for InferenceGraph {} in raw deployment mode".format(
                graph_name
            )
        )

    svc = kserve_client.core_api.read_namespaced_service(
        graph_name, namespace=KSERVE_TEST_NAMESPACE
    )
    if not svc:
        raise RuntimeError(
            "Service doesn't exist for InferenceGraph {} in raw deployment mode".format(
                graph_name
            )
        )

    try:
        knativeroute = kserve_client.api_instance.get_namespaced_custom_object(
            "serving.knative.dev", "v1", KSERVE_TEST_NAMESPACE, "routes", graph_name
        )
        if knativeroute:
            raise RuntimeError(
                "Knative route resource shouldn't exist for InferenceGraph {}".format(
                    graph_name
                )
                + "in raw deployment mode"
            )
    except client.rest.ApiException:
        logger.info("Expected error in finding knative route in raw deployment mode")

    try:
        knativesvc = kserve_client.api_instance.get_namespaced_custom_object(
            "serving.knative.dev", "v1", KSERVE_TEST_NAMESPACE, "services", graph_name
        )
        if knativesvc:
            raise RuntimeError(
                "Knative resources shouldn't exist for InferenceGraph {} ".format(
                    graph_name
                )
                + "in raw deployment mode"
            )
    except client.rest.ApiException:
        logger.info("Expected error in finding knative service in raw deployment mode")

    # TODO Fix this when we enable ALB creation for IG raw deployment mode. This is required for traffic ingress
    # for this predict api call to work
    #
    # res = await predict_ig(
    #    rest_v1_client,
    #     graph_name,
    #     os.path.join(IG_TEST_RESOURCES_BASE_LOCATION, "iris_input.json"),
    #     network_layer=network_layer,
    # )
    # assert res["predictions"] == [1, 1]

    kserve_client.delete_inference_graph(graph_name, KSERVE_TEST_NAMESPACE)
    kserve_client.delete(sklearn_name, KSERVE_TEST_NAMESPACE)
    kserve_client.delete(xgb_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.raw
@pytest.mark.asyncio(scope="session")
async def test_inference_graph_raw_mode_with_hpa(rest_v1_client, network_layer):
    logger.info("Starting test test_inference_graph_raw_mode_with_hpa")
    suffix = str(uuid.uuid4())[1:6]
    sklearn_name = "isvc-sklearn-graph-raw-hpa-" + suffix
    xgb_name = "isvc-xgboost-graph-raw-hpa-" + suffix
    graph_name = "model-chainer-raw-hpa-" + suffix

    annotations = dict()
    annotations["serving.kserve.io/deploymentMode"] = "RawDeployment"
    # annotations["serving.kserve.io/max-scale"] = '5'
    # annotations["serving.kserve.io/metric"] = 'rps'
    # annotations["serving.kserve.io/min-scale"] = '2'
    # annotations["serving.kserve.io/target"] = '30'
    labels = dict()
    labels["networking.kserve.io/visibility"] = "exposed"

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
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=sklearn_name,
            namespace=KSERVE_TEST_NAMESPACE,
            annotations=annotations,
            labels=labels,
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
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=xgb_name,
            namespace=KSERVE_TEST_NAMESPACE,
            annotations=annotations,
            labels=labels,
        ),
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
        metadata=client.V1ObjectMeta(
            name=graph_name, namespace=KSERVE_TEST_NAMESPACE, annotations=annotations
        ),
        spec=graph_spec,
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(sklearn_isvc)
    kserve_client.create(xgb_isvc)
    kserve_client.wait_isvc_ready(sklearn_name, namespace=KSERVE_TEST_NAMESPACE)
    kserve_client.wait_isvc_ready(xgb_name, namespace=KSERVE_TEST_NAMESPACE)

    kserve_client.create_inference_graph(ig)
    kserve_client.wait_ig_ready(graph_name, namespace=KSERVE_TEST_NAMESPACE)

    # Below checks are raw deployment specific.  They ensure raw k8s resources created instead of knative resources
    dep = kserve_client.app_api.read_namespaced_deployment(
        graph_name, namespace=KSERVE_TEST_NAMESPACE
    )
    if not dep:
        raise RuntimeError(
            "Deployment doesn't exist for InferenceGraph {} in raw deployment mode".format(
                graph_name
            )
        )

    svc = kserve_client.core_api.read_namespaced_service(
        graph_name, namespace=KSERVE_TEST_NAMESPACE
    )
    if not svc:
        raise RuntimeError(
            "Service doesn't exist for InferenceGraph {} in raw deployment mode".format(
                graph_name
            )
        )

    # hpa = kserve_client.hpa_v2_api.read_namespaced_horizontal_pod_autoscaler(graph_name,
    #                                                                          namespace=KSERVE_TEST_NAMESPACE)
    # if not hpa:
    #     raise RuntimeError("HPA doesn't exist for InferenceGraph {} in raw deployment mode".format(graph_name))

    try:
        knativeroute = kserve_client.api_instance.get_namespaced_custom_object(
            "serving.knative.dev", "v1", KSERVE_TEST_NAMESPACE, "routes", graph_name
        )
        if knativeroute:
            raise RuntimeError(
                "Knative route resource shouldn't exist for InferenceGraph {} ".format(
                    graph_name
                )
                + "in raw deployment mode"
            )
    except client.rest.ApiException:
        logger.info("Expected error in finding knative route in raw deployment mode")

    try:
        knativesvc = kserve_client.api_instance.get_namespaced_custom_object(
            "serving.knative.dev", "v1", KSERVE_TEST_NAMESPACE, "services", graph_name
        )
        if knativesvc:
            raise RuntimeError(
                "Knative resources shouldn't exist for InferenceGraph {} ".format(
                    graph_name
                )
                + "in raw deployment mode"
            )
    except client.rest.ApiException:
        logger.info("Expected error in finding knative route in raw deployment mode")

    # TODO Fix this when we enable ALB creation for IG raw deployment mode. This is required for traffic ingress
    # for this predict api call to work
    #
    # res = await predict_ig(
    #     rest_v1_client,
    #     graph_name,
    #     os.path.join(IG_TEST_RESOURCES_BASE_LOCATION, "iris_input.json"),
    #     network_layer=network_layer,
    # )
    # assert res["predictions"] == [1, 1]

    kserve_client.delete_inference_graph(graph_name, KSERVE_TEST_NAMESPACE)
    kserve_client.delete(sklearn_name, KSERVE_TEST_NAMESPACE)
    kserve_client.delete(xgb_name, KSERVE_TEST_NAMESPACE)
