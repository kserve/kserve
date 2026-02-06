import pytest
from unittest.mock import MagicMock, patch
from kubernetes.client.rest import ApiException
from kubernetes import client

from kserve.api.kserve_client import KServeClient, isvc_watch
from kserve.constants import constants
from kserve.models import V1alpha1InferenceGraph

@patch("kserve.api.kserve_client.config.load_kube_config")
@patch("kserve.utils.utils.is_running_in_k8s", return_value=False)
@patch("kserve.api.kserve_client.client.CoreV1Api")
@patch("kserve.api.kserve_client.client.AppsV1Api")
@patch("kserve.api.kserve_client.client.CustomObjectsApi")
@patch("kserve.api.kserve_client.client.AutoscalingV2Api")
def test_init_loads_kube_config(
    mock_hpa,
    mock_custom,
    mock_apps,
    mock_core,
    mock_is_k8s,
    mock_load_kube,
):
    KServeClient()

    mock_load_kube.assert_called_once()
    mock_core.assert_called_once()
    mock_apps.assert_called_once()
    mock_custom.assert_called_once()
    mock_hpa.assert_called_once()

@patch("kserve.api.kserve_client.config.load_kube_config")
@patch("kserve.api.kserve_client.set_gcs_credentials")
@patch("kserve.utils.utils.get_default_target_namespace", return_value="default")
@patch("kserve.utils.utils.is_running_in_k8s", return_value=False)
def test_set_credentials_gcs(mock_is_k8s, mock_ns, mock_set_gcs, mock_load_kube):
    client = KServeClient()

    client.set_credentials("gcs")

    mock_set_gcs.assert_called_once_with(
        namespace="default",
        credentials_file=constants.GCS_DEFAULT_CREDS_FILE,
        service_account=constants.DEFAULT_SA_NAME,
    )

@patch("kserve.api.kserve_client.set_s3_credentials")
@patch("kserve.api.kserve_client.utils.get_default_target_namespace", return_value="default")
def test_set_credentials_s3(mock_ns, mock_set_s3):
    client = KServeClient()

    client.set_credentials(
        storage_type="s3",
        access_key_id="key",
        secret_access_key="secret",
    )

    mock_set_s3.assert_called_once_with(
        namespace="default",
        credentials_file=constants.S3_DEFAULT_CREDS_FILE,
        service_account=constants.DEFAULT_SA_NAME,
        access_key_id="key",
        secret_access_key="secret",
    )

@patch("kserve.api.kserve_client.set_azure_credentials")
@patch("kserve.api.kserve_client.utils.get_default_target_namespace", return_value="default")
def test_set_credentials_azure(mock_ns, mock_set_azure):
    client = KServeClient()

    client.set_credentials(storage_type="azure")

    mock_set_azure.assert_called_once_with(
        namespace="default",
        credentials_file=constants.AZ_DEFAULT_CREDS_FILE,
        service_account=constants.DEFAULT_SA_NAME,
    )

def test_set_credentials_invalid_storage_type():
    client = KServeClient()

    with pytest.raises(RuntimeError, match="Invalid storage_type"):
        client.set_credentials(storage_type="hdfs")


@patch("kserve.api.kserve_client.config.load_kube_config")
@patch("kserve.utils.utils.is_running_in_k8s", return_value=False)
@patch("kserve.api.kserve_client.client.CoreV1Api")
@patch("kserve.api.kserve_client.client.AppsV1Api")
@patch("kserve.api.kserve_client.client.CustomObjectsApi")
@patch("kserve.api.kserve_client.client.AutoscalingV2Api")
def test_create_inferenceservice_success(
    mock_hpa,
    mock_custom,
    mock_apps,
    mock_core,
    mock_is_k8s,
    mock_load_kube,
):
    client = KServeClient()

    client.api_instance.create_namespaced_custom_object.return_value = {
        "metadata": {"name": "test-isvc"}
    }

    isvc = MagicMock()
    isvc.api_version = "serving.kserve.io/v1beta1"

    result = client.create(isvc, namespace="default")

    assert result["metadata"]["name"] == "test-isvc"

@patch("kserve.api.kserve_client.config.load_kube_config")
@patch("kserve.api.kserve_client.utils.is_running_in_k8s", return_value=False)
@patch("kserve.api.kserve_client.isvc_watch")
def test_create_with_watch(
    mock_watch,
    mock_is_k8s,
    mock_load_kube,
):
    client = KServeClient()
    client.api_instance = MagicMock()

    isvc = MagicMock()
    isvc.api_version = "serving.kserve.io/v1beta1"

    client.api_instance.create_namespaced_custom_object.return_value = {
        "metadata": {"name": "test-isvc"}
    }

    client.create(isvc, namespace="default", watch=True)

    mock_watch.assert_called_once_with(
        name="test-isvc",
        namespace="default",
        timeout_seconds=600,
    )

@pytest.fixture(autouse=True)
def mock_kube_config():
    with patch("kserve.api.kserve_client.config.load_kube_config"), \
         patch("kserve.api.kserve_client.config.load_incluster_config"), \
         patch("kserve.api.kserve_client.utils.is_running_in_k8s", return_value=False):
        yield

def test_get_inferenceservice():
    client = KServeClient()
    client.api_instance = MagicMock()

    client.api_instance.get_namespaced_custom_object.return_value = {"kind": "InferenceService"}

    result = client.get(name="test", namespace="ns")

    assert result["kind"] == "InferenceService"

def test_is_isvc_ready_true():
    client = KServeClient()

    client.get = MagicMock(return_value={
        "status": {
            "conditions": [
                {"type": "Ready", "status": "True"}
            ]
        }
    })

    assert client.is_isvc_ready("test") is True

def test_wait_isvc_ready_timeout():
    client = KServeClient()

    client.is_isvc_ready = MagicMock(return_value=False)
    client.get = MagicMock(return_value={"metadata": {"name": "test"}})

    with pytest.raises(RuntimeError, match="Timeout to start the InferenceService"):
        client.wait_isvc_ready(
            name="test",
            timeout_seconds=1,
            polling_interval=1,
        )

@patch("kserve.api.kserve_client.requests.get")
def test_wait_model_ready_success(mock_get):
    client = KServeClient()

    client.get = MagicMock(return_value={
        "status": {"url": "http://example.com"}
    })

    mock_get.return_value.status_code = 200

    client.wait_model_ready(
        service_name="svc",
        model_name="model",
        cluster_ip="127.0.0.1",
        timeout_seconds=1,
        polling_interval=1,
    )

###############################################
# Tests for patch method
###############################################
@patch("kserve.api.kserve_client.utils.get_isvc_namespace", return_value="test-ns")
def test_patch_inferenceservice_no_watch(mock_get_ns):
    client = KServeClient()
    client.api_instance = MagicMock()

    isvc = MagicMock()
    isvc.api_version = "serving.kserve.io/v1beta1"

    client.api_instance.patch_namespaced_custom_object.return_value = {
        "metadata": {"name": "test-isvc"}
    }

    result = client.patch(
        name="test-isvc",
        inferenceservice=isvc,
        watch=False,
    )

    client.api_instance.patch_namespaced_custom_object.assert_called_once_with(
        constants.KSERVE_GROUP,
        "v1beta1",
        "test-ns",
        constants.KSERVE_PLURAL_INFERENCESERVICE,
        "test-isvc",
        isvc,
    )

    assert result["metadata"]["name"] == "test-isvc"

@patch("kserve.api.kserve_client.isvc_watch")
@patch("kserve.api.kserve_client.time.sleep")
@patch("kserve.api.kserve_client.utils.get_isvc_namespace", return_value="test-ns")
def test_patch_inferenceservice_with_watch(
    mock_get_ns,
    mock_sleep,
    mock_watch,
):
    client = KServeClient()
    client.api_instance = MagicMock()

    isvc = MagicMock()
    isvc.api_version = "serving.kserve.io/v1beta1"

    client.api_instance.patch_namespaced_custom_object.return_value = {
        "metadata": {"name": "test-isvc"}
    }

    result = client.patch(
        name="test-isvc",
        inferenceservice=isvc,
        watch=True,
        timeout_seconds=300,
    )

    # no return when watch=True
    assert result is None

    mock_sleep.assert_called_once_with(3)

    mock_watch.assert_called_once_with(
        name="test-isvc",
        namespace="test-ns",
        timeout_seconds=300,
    )

@patch("kserve.api.kserve_client.utils.get_isvc_namespace", return_value="test-ns")
def test_patch_inferenceservice_api_exception(mock_get_ns):
    client = KServeClient()
    client.api_instance = MagicMock()

    isvc = MagicMock()
    isvc.api_version = "serving.kserve.io/v1beta1"

    client.api_instance.patch_namespaced_custom_object.side_effect = ApiException(
        status=500, reason="Internal Error"
    )

    with pytest.raises(RuntimeError, match="patch_namespaced_custom_object"):
        client.patch(
            name="test-isvc",
            inferenceservice=isvc,
        )


###############################################
# Tests for replace method 
###############################################
@patch("kserve.api.kserve_client.utils.get_isvc_namespace", return_value="test-ns")
def test_replace_sets_resource_version_and_returns(mock_get_ns):
    client = KServeClient()
    client.api_instance = MagicMock()

    # mock existing ISVC returned by get()
    client.get = MagicMock(return_value={
        "metadata": {"resourceVersion": "123"}
    })

    isvc = MagicMock()
    isvc.api_version = "serving.kserve.io/v1beta1"
    isvc.metadata.resource_version = None

    client.api_instance.replace_namespaced_custom_object.return_value = {
        "metadata": {
            "name": "test-isvc",
            "generation": 2,
        }
    }

    result = client.replace(
        name="test-isvc",
        inferenceservice=isvc,
        watch=False,
    )

    # resourceVersion should be backfilled
    assert isvc.metadata.resource_version == "123"

    client.api_instance.replace_namespaced_custom_object.assert_called_once_with(
        constants.KSERVE_GROUP,
        "v1beta1",
        "test-ns",
        constants.KSERVE_PLURAL_INFERENCESERVICE,
        "test-isvc",
        isvc,
    )

    assert result["metadata"]["name"] == "test-isvc"

@patch("kserve.api.kserve_client.isvc_watch")
@patch("kserve.api.kserve_client.utils.get_isvc_namespace", return_value="test-ns")
def test_replace_with_watch(mock_get_ns, mock_watch):
    client = KServeClient()
    client.api_instance = MagicMock()

    isvc = MagicMock()
    isvc.api_version = "serving.kserve.io/v1beta1"
    isvc.metadata.resource_version = "456"

    client.api_instance.replace_namespaced_custom_object.return_value = {
        "metadata": {
            "name": "test-isvc",
            "generation": 3,
        }
    }

    result = client.replace(
        name="test-isvc",
        inferenceservice=isvc,
        watch=True,
        timeout_seconds=300,
    )

    # watch=True returns nothing
    assert result is None

    mock_watch.assert_called_once_with(
        name="test-isvc",
        namespace="test-ns",
        timeout_seconds=300,
        generation=3,
    )

@patch("kserve.api.kserve_client.utils.get_isvc_namespace", return_value="test-ns")
def test_replace_api_exception(mock_get_ns):
    client = KServeClient()
    client.api_instance = MagicMock()

    isvc = MagicMock()
    isvc.api_version = "serving.kserve.io/v1beta1"
    isvc.metadata.resource_version = "123"

    client.api_instance.replace_namespaced_custom_object.side_effect = ApiException(
        status=500,
        reason="Internal Error",
    )

    with pytest.raises(RuntimeError, match="replace_namespaced_custom_object"):
        client.replace(
            name="test-isvc",
            inferenceservice=isvc,
        )

#################################################
# Tests for delete method
#################################################
@patch("kserve.api.kserve_client.utils.get_default_target_namespace", return_value="test-ns")
def test_delete_inferenceservice_success(mock_ns):
    client = KServeClient()
    client.api_instance = MagicMock()

    client.api_instance.delete_namespaced_custom_object.return_value = {
        "status": "Success"
    }

    result = client.delete(name="test-isvc")

    client.api_instance.delete_namespaced_custom_object.assert_called_once_with(
        constants.KSERVE_GROUP,
        constants.KSERVE_V1BETA1_VERSION,
        "test-ns",
        constants.KSERVE_PLURAL_INFERENCESERVICE,
        "test-isvc",
    )

    assert result["status"] == "Success"

@patch("kserve.api.kserve_client.utils.get_default_target_namespace", return_value="test-ns")
def test_delete_inferenceservice_api_exception(mock_ns):
    client = KServeClient()
    client.api_instance = MagicMock()

    client.api_instance.delete_namespaced_custom_object.side_effect = ApiException(
        status=404,
        reason="Not Found",
    )

    with pytest.raises(RuntimeError, match="delete_namespaced_custom_object"):
        client.delete(name="test-isvc")

################################################
# Tests for Create method
################################################
def mock_isvc():
    isvc = MagicMock()
    isvc.api_version = "serving.kserve.io/v1beta1"
    isvc.metadata = MagicMock()
    return isvc

@patch("kserve.api.kserve_client.utils.get_isvc_namespace", return_value="test-ns")
def test_create_uses_isvc_namespace_when_namespace_none(mock_get_ns):
    client = KServeClient()
    client.api_instance = MagicMock()

    isvc = mock_isvc()

    client.api_instance.create_namespaced_custom_object.return_value = {
        "metadata": {"name": "test-isvc"}
    }

    result = client.create(inferenceservice=isvc)

    mock_get_ns.assert_called_once_with(isvc)

    client.api_instance.create_namespaced_custom_object.assert_called_once_with(
        constants.KSERVE_GROUP,
        "v1beta1",
        "test-ns",
        constants.KSERVE_PLURAL_INFERENCESERVICE,
        isvc,
    )

    assert result["metadata"]["name"] == "test-isvc"

@patch("kserve.api.kserve_client.utils.get_isvc_namespace", return_value="test-ns")
def test_create_raises_runtime_error_on_api_exception(mock_get_ns):
    client = KServeClient()
    client.api_instance = MagicMock()

    isvc = mock_isvc()

    client.api_instance.create_namespaced_custom_object.side_effect = ApiException(
        status=400,
        reason="Bad Request",
    )

    with pytest.raises(
        RuntimeError,
        match="create_namespaced_custom_object",
    ):
        client.create(inferenceservice=isvc)


#######################################################
# Tests for Get method
#######################################################
def mock_isvc():
    return MagicMock(api_version="serving.kserve.io/v1beta1")

@patch("kserve.api.kserve_client.utils.get_default_target_namespace", return_value="test-ns")
def test_get_named_isvc_success(mock_ns):
    client = KServeClient()
    client.api_instance = MagicMock()
    
    client.api_instance.get_namespaced_custom_object.return_value = {"metadata": {"name": "test-isvc"}}
    
    result = client.get(name="test-isvc")
    
    client.api_instance.get_namespaced_custom_object.assert_called_once_with(
        constants.KSERVE_GROUP,
        constants.KSERVE_V1BETA1_VERSION,
        "test-ns",
        constants.KSERVE_PLURAL_INFERENCESERVICE,
        "test-isvc"
    )
    
    assert result["metadata"]["name"] == "test-isvc"

@patch("kserve.api.kserve_client.isvc_watch")
@patch("kserve.api.kserve_client.utils.get_default_target_namespace", return_value="test-ns")
def test_get_named_isvc_watch(mock_ns, mock_watch):
    client = KServeClient()
    client.api_instance = MagicMock()
    
    client.get(name="test-isvc", watch=True, timeout_seconds=123)
    
    mock_watch.assert_called_once_with(
        name="test-isvc",
        namespace="test-ns",
        timeout_seconds=123
    )

@patch("kserve.api.kserve_client.utils.get_default_target_namespace", return_value="test-ns")
def test_get_list_isvc_success(mock_ns):
    client = KServeClient()
    client.api_instance = MagicMock()
    
    client.api_instance.list_namespaced_custom_object.return_value = {"items": [1, 2, 3]}
    
    result = client.get(name=None)
    
    client.api_instance.list_namespaced_custom_object.assert_called_once_with(
        constants.KSERVE_GROUP,
        constants.KSERVE_V1BETA1_VERSION,
        "test-ns",
        constants.KSERVE_PLURAL_INFERENCESERVICE
    )
    
    assert result["items"] == [1, 2, 3]

@patch("kserve.api.kserve_client.isvc_watch")
@patch("kserve.api.kserve_client.utils.get_default_target_namespace", return_value="test-ns")
def test_get_list_isvc_watch(mock_ns, mock_watch):
    client = KServeClient()
    client.api_instance = MagicMock()
    
    client.get(name=None, watch=True, timeout_seconds=321)
    
    mock_watch.assert_called_once_with(namespace="test-ns", timeout_seconds=321)

@patch("kserve.api.kserve_client.utils.get_default_target_namespace", return_value="test-ns")
def test_get_named_isvc_api_exception(mock_ns):
    client = KServeClient()
    client.api_instance = MagicMock()
    
    client.api_instance.get_namespaced_custom_object.side_effect = ApiException(status=404)
    
    with pytest.raises(RuntimeError, match="get_namespaced_custom_object"):
        client.get(name="test-isvc")

@patch("kserve.api.kserve_client.utils.get_default_target_namespace", return_value="test-ns")
def test_get_list_isvc_api_exception(mock_ns):
    client = KServeClient()
    client.api_instance = MagicMock()
    
    client.api_instance.list_namespaced_custom_object.side_effect = ApiException(status=500)
    
    with pytest.raises(RuntimeError, match="list_namespaced_custom_object"):
        client.get(name=None)

################################################
# Tests for isvc_watch function
################################################
@patch.object(KServeClient, "get", return_value={})
def test_is_isvc_ready_no_status(mock_get):
    client = KServeClient()
    
    result = client.is_isvc_ready(name="test-isvc")
    
    mock_get.assert_called_once_with("test-isvc", namespace=None, version="v1beta1")
    assert result is False  # <-- first arrow

@patch.object(KServeClient, "get", return_value={"status": {"conditions": [{"type": "Other", "status": "True"}]}})
def test_is_isvc_ready_no_ready_condition(mock_get):
    client = KServeClient()
    
    result = client.is_isvc_ready(name="test-isvc")
    
    mock_get.assert_called_once_with("test-isvc", namespace=None, version="v1beta1")
    assert result is False  # <-- second arrow

################################################
# Tests for wait_isvc_ready functions
################################################
@patch("kserve.api.kserve_client.isvc_watch")
def test_wait_isvc_ready_watch_calls_isvc_watch(mock_watch):
    client = KServeClient()
    
    # Call with watch=True
    client.wait_isvc_ready(name="test-isvc", watch=True, timeout_seconds=5)
    
    # Verify isvc_watch called with correct params
    mock_watch.assert_called_once_with(name="test-isvc", namespace=None, timeout_seconds=5)

@patch.object(KServeClient, "is_isvc_ready", return_value=True)
def test_wait_isvc_ready_ready_returns(mock_is_ready):
    client = KServeClient()
    
    # Should return immediately when is_isvc_ready returns True
    result = client.wait_isvc_ready(name="test-isvc", watch=False, timeout_seconds=5, polling_interval=1)
    
    mock_is_ready.assert_called()
    assert result is None  # <-- matches the return arrow

@patch.object(KServeClient, "is_isvc_ready", return_value=False)
@patch.object(KServeClient, "get", return_value={"metadata": {"name": "test-isvc"}})
def test_wait_isvc_ready_timeout_raises(mock_get, mock_is_ready):
    client = KServeClient()
    
    with pytest.raises(RuntimeError) as exc_info:
        client.wait_isvc_ready(name="test-isvc", watch=False, timeout_seconds=2, polling_interval=1)
    
    assert "Timeout to start the InferenceService" in str(exc_info.value)

#################################################
# Tests for create_trained_model method
#################################################
def test_create_trained_model_success():
    client_instance = KServeClient()
    
    # Mock trainedmodel object
    trained_model = MagicMock()
    trained_model.api_version = "kserve/v1alpha1"
    
    # Mock API instance
    client_instance.api_instance = MagicMock()
    
    # Call method
    client_instance.create_trained_model(trainedmodel=trained_model, namespace="default")
    
    # Ensure version extraction works
    assert trained_model.api_version.split("/")[1] == "v1alpha1"
    
    # Ensure API was called correctly
    client_instance.api_instance.create_namespaced_custom_object.assert_called_once_with(
        constants.KSERVE_GROUP,
        "v1alpha1",
        "default",
        constants.KSERVE_PLURAL_TRAINEDMODEL,
        trained_model,
    )

def test_create_trained_model_raises_runtime_error():
    client_instance = KServeClient()
    
    trained_model = MagicMock()
    trained_model.api_version = "kserve/v1alpha1"
    
    # Simulate ApiException
    api_exception = client.rest.ApiException("API failure")
    client_instance.api_instance = MagicMock()
    client_instance.api_instance.create_namespaced_custom_object.side_effect = api_exception
    
    with pytest.raises(RuntimeError) as exc_info:
        client_instance.create_trained_model(trainedmodel=trained_model, namespace="default")
    
    assert "Exception when calling CustomObjectsApi->create_namespaced_custom_object" in str(exc_info.value)

#################################################
# Tests for delete_trained_model method
#################################################
def test_delete_trained_model_with_namespace():
    client_instance = KServeClient()
    
    # Mock utils function to return a default namespace
    with patch("kserve.api.kserve_client.utils.get_default_target_namespace", return_value="default_ns"):
        client_instance.api_instance = MagicMock()
        
        # Call method with namespace None
        client_instance.delete_trained_model(name="model-1", namespace=None)
        
        # Check that default namespace was used
        client_instance.api_instance.delete_namespaced_custom_object.assert_called_once_with(
            constants.KSERVE_GROUP,
            constants.KSERVE_V1ALPHA1_VERSION,
            "default_ns",
            constants.KSERVE_PLURAL_TRAINEDMODEL,
            "model-1",
        )

def test_delete_trained_model_raises_runtime_error():
    client_instance = KServeClient()
    client_instance.api_instance = MagicMock()
    
    # Simulate ApiException
    api_exception = client.rest.ApiException("API failure")
    client_instance.api_instance.delete_namespaced_custom_object.side_effect = api_exception
    
    with patch("kserve.api.kserve_client.utils.get_default_target_namespace", return_value="default_ns"):
        with pytest.raises(RuntimeError) as exc_info:
            client_instance.delete_trained_model(name="model-1", namespace=None)
    
    assert "Exception when calling CustomObjectsApi->delete_namespaced_custom_object" in str(exc_info.value)

################################################
# Tests for create_inference_graph method
################################################
def test_create_inference_graph_success():
    client_instance = KServeClient()
    
    # Mock inference graph object
    mock_graph = MagicMock(spec=V1alpha1InferenceGraph)
    mock_graph.api_version = "kserve/v1alpha1"
    
    # Mock API instance and utils
    client_instance.api_instance = MagicMock()
    client_instance.api_instance.create_namespaced_custom_object.return_value = {"metadata": {"name": "graph1"}}
    
    with patch("kserve.api.kserve_client.utils.get_ig_namespace", return_value="default_ns"):
        result = client_instance.create_inference_graph(mock_graph, namespace=None)
        
    # Check version extraction
    assert mock_graph.api_version.split("/")[1] == "v1alpha1"
    
    # Check create_namespaced_custom_object called with correct arguments
    client_instance.api_instance.create_namespaced_custom_object.assert_called_once_with(
        constants.KSERVE_GROUP,
        "v1alpha1",
        "default_ns",
        constants.KSERVE_PLURAL_INFERENCEGRAPH,
        mock_graph,
    )
    
    # Check the method returns the API output
    assert result == {"metadata": {"name": "graph1"}}

def test_create_inference_graph_api_exception():
    client_instance = KServeClient()
    mock_graph = MagicMock(spec=V1alpha1InferenceGraph)
    mock_graph.api_version = "kserve/v1alpha1"
    
    client_instance.api_instance = MagicMock()
    client_instance.api_instance.create_namespaced_custom_object.side_effect = client.rest.ApiException("API failure")
    
    with patch("kserve.api.kserve_client.utils.get_ig_namespace", return_value="default_ns"):
        with pytest.raises(RuntimeError) as exc_info:
            client_instance.create_inference_graph(mock_graph, namespace=None)
    
    assert "Exception when calling CustomObjectsApi->create_namespaced_custom_object" in str(exc_info.value)


################################################
# Tests for delete_inference_graph method
################################################
@patch("kserve.api.kserve_client.utils.get_default_target_namespace", return_value="default-ns")
def test_delete_inference_graph_success(mock_ns):
    client_instance = KServeClient()
    client_instance.api_instance = MagicMock()

    client_instance.delete_inference_graph(name="graph-1", namespace=None)

    mock_ns.assert_called_once()

    client_instance.api_instance.delete_namespaced_custom_object.assert_called_once_with(
        constants.KSERVE_GROUP,
        constants.KSERVE_V1ALPHA1_VERSION,
        "default-ns",
        constants.KSERVE_PLURAL_INFERENCEGRAPH,
        "graph-1",
    )

@patch("kserve.api.kserve_client.utils.get_default_target_namespace", return_value="default-ns")
def test_delete_inference_graph_api_exception(mock_ns):
    client_instance = KServeClient()
    client_instance.api_instance = MagicMock()

    client_instance.api_instance.delete_namespaced_custom_object.side_effect = (
        client.rest.ApiException("delete failed")
    )

    with pytest.raises(
        RuntimeError,
        match="create_namespaced_custom_object",  
    ):
        client_instance.delete_inference_graph(name="graph-1", namespace=None)


################################################
# Tests for get_inference_graph method
################################################
@patch("kserve.api.kserve_client.utils.get_default_target_namespace", return_value="default-ns")
def test_get_inference_graph_success(mock_ns):
    client_instance = KServeClient()
    client_instance.api_instance = MagicMock()

    expected_response = {"metadata": {"name": "graph-1"}}
    client_instance.api_instance.get_namespaced_custom_object.return_value = expected_response

    result = client_instance.get_inference_graph(name="graph-1", namespace=None)

    client_instance.api_instance.get_namespaced_custom_object.assert_called_once_with(
        constants.KSERVE_GROUP,
        constants.KSERVE_V1ALPHA1_VERSION,
        "default-ns",
        constants.KSERVE_PLURAL_INFERENCEGRAPH,
        "graph-1",
    )

    assert result == expected_response

@patch("kserve.api.kserve_client.utils.get_default_target_namespace", return_value="default-ns")
def test_get_inference_graph_api_exception(mock_ns):
    client_instance = KServeClient()
    client_instance.api_instance = MagicMock()

    client_instance.api_instance.get_namespaced_custom_object.side_effect = (
        client.rest.ApiException("get failed")
    )

    with pytest.raises(
        RuntimeError,
        match="get_namespaced_custom_object",
    ):
        client_instance.get_inference_graph(name="graph-1", namespace=None)

################################################
# Test for is_ig_ready method
################################################
@patch("kserve.api.kserve_client.utils.get_default_target_namespace", return_value="default-ns")
def test_is_ig_ready_true(mock_ns):
    client_instance = KServeClient()

    client_instance.get_inference_graph = MagicMock(return_value={
        "status": {
            "conditions": [
                {"type": "Ready", "status": "True"}
            ]
        }
    })

    result = client_instance.is_ig_ready(name="graph-1", namespace=None)

    assert result is True
    client_instance.get_inference_graph.assert_called_once_with(
        "graph-1",
        namespace="default-ns",
        version="v1alpha1",
    )

@patch("kserve.api.kserve_client.utils.get_default_target_namespace", return_value="default-ns")
def test_is_ig_ready_false(mock_ns):
    client_instance = KServeClient()

    client_instance.get_inference_graph = MagicMock(return_value={
        "status": {
            "conditions": [
                {"type": "Ready", "status": "False"}
            ]
        }
    })

    result = client_instance.is_ig_ready(name="graph-1", namespace=None)

    assert result is False

@patch("kserve.api.kserve_client.utils.get_default_target_namespace", return_value="default-ns")
def test_is_ig_ready_no_ready_condition(mock_ns):
    client_instance = KServeClient()

    client_instance.get_inference_graph = MagicMock(return_value={
        "status": {
            "conditions": [
                {"type": "Other", "status": "True"}
            ]
        }
    })

    result = client_instance.is_ig_ready(name="graph-1", namespace=None)

    assert result is False

@patch("kserve.api.kserve_client.utils.get_default_target_namespace", return_value="default-ns")
def test_is_ig_ready_no_status(mock_ns):
    client_instance = KServeClient()

    client_instance.get_inference_graph = MagicMock(return_value={})

    result = client_instance.is_ig_ready(name="graph-1", namespace=None)

    assert result is False

################################################
# Tests for wait_ig_ready method
################################################
@patch("kserve.api.kserve_client.time.sleep", return_value=None)
def test_wait_ig_ready_success(mock_sleep):
    client_instance = KServeClient()

    # is_ig_ready returns False first, then True
    client_instance.is_ig_ready = MagicMock(side_effect=[False, True])

    client_instance.wait_ig_ready(
        name="graph-1",
        namespace="default-ns",
        timeout_seconds=20,
        polling_interval=10,
    )

    assert client_instance.is_ig_ready.call_count == 2
    mock_sleep.assert_called()

@patch("kserve.api.kserve_client.time.sleep", return_value=None)
def test_wait_ig_ready_timeout(mock_sleep):
    client_instance = KServeClient()

    client_instance.is_ig_ready = MagicMock(return_value=False)
    client_instance.get_inference_graph = MagicMock(return_value={"status": "NotReady"})

    with pytest.raises(
        RuntimeError,
        match="Timeout to start the InferenceGraph graph-1",
    ):
        client_instance.wait_ig_ready(
            name="graph-1",
            namespace="default-ns",
            timeout_seconds=20,
            polling_interval=10,
        )

    client_instance.get_inference_graph.assert_called_once_with(
        "graph-1",
        namespace="default-ns",
        version="v1alpha1",
    )

#################################################
# Tests for create_local_model_node_group method
#################################################
def test_create_local_model_node_group_success():
    client_instance = KServeClient()
    client_instance.api_instance = MagicMock()

    localmodelnodegroup = MagicMock()
    localmodelnodegroup.api_version = "serving.kserve.io/v1alpha1"

    expected_output = {"metadata": {"name": "lmng-1"}}
    client_instance.api_instance.create_cluster_custom_object.return_value = (
        expected_output
    )

    output = client_instance.create_local_model_node_group(localmodelnodegroup)

    assert output == expected_output
    client_instance.api_instance.create_cluster_custom_object.assert_called_once_with(
        constants.KSERVE_GROUP,
        "v1alpha1",
        constants.KSERVE_PLURAL_LOCALMODELNODEGROUP,
        localmodelnodegroup,
    )

def test_create_local_model_node_group_api_exception():
    client_instance = KServeClient()
    client_instance.api_instance = MagicMock()

    localmodelnodegroup = MagicMock()
    localmodelnodegroup.api_version = "serving.kserve.io/v1alpha1"

    client_instance.api_instance.create_cluster_custom_object.side_effect = (
        client.rest.ApiException("create failed")
    )

    with pytest.raises(
        RuntimeError,
        match="create_cluster_custom_object",
    ):
        client_instance.create_local_model_node_group(localmodelnodegroup)


#################################################
# Tests for get_local_model_node_group method
#################################################
def test_get_local_model_node_group_success():
    client_instance = KServeClient()
    client_instance.api_instance = MagicMock()

    expected_output = {"metadata": {"name": "lmng-1"}}
    client_instance.api_instance.get_cluster_custom_object.return_value = (
        expected_output
    )

    result = client_instance.get_local_model_node_group(name="lmng-1")

    assert result == expected_output
    client_instance.api_instance.get_cluster_custom_object.assert_called_once_with(
        constants.KSERVE_GROUP,
        constants.KSERVE_V1ALPHA1_VERSION,
        constants.KSERVE_PLURAL_LOCALMODELNODEGROUP,
        "lmng-1",
    )

def test_get_local_model_node_group_api_exception():
    client_instance = KServeClient()
    client_instance.api_instance = MagicMock()

    client_instance.api_instance.get_cluster_custom_object.side_effect = (
        client.rest.ApiException("get failed")
    )

    with pytest.raises(
        RuntimeError,
        match="get_cluster_custom_object",
    ):
        client_instance.get_local_model_node_group(name="lmng-1")

#################################################
# Tests for delete_local_model_node_group method
#################################################
def test_delete_local_model_node_group_success():
    client_instance = KServeClient()
    client_instance.api_instance = MagicMock()

    client_instance.delete_local_model_node_group(name="lmng-1")

    client_instance.api_instance.delete_cluster_custom_object.assert_called_once_with(
        constants.KSERVE_GROUP,
        constants.KSERVE_V1ALPHA1_VERSION,
        constants.KSERVE_PLURAL_LOCALMODELNODEGROUP,
        "lmng-1",
    )

def test_delete_local_model_node_group_api_exception():
    client_instance = KServeClient()
    client_instance.api_instance = MagicMock()

    client_instance.api_instance.delete_cluster_custom_object.side_effect = (
        client.rest.ApiException("delete failed")
    )

    with pytest.raises(
        RuntimeError,
        match="delete_cluster_custom_object",
    ):
        client_instance.delete_local_model_node_group(name="lmng-1")
    
##################################################
# Tests for create_local_model_cache method
##################################################
def test_create_local_model_cache_success():
    client_instance = KServeClient()
    client_instance.api_instance = MagicMock()

    localmodelcache = MagicMock()
    localmodelcache.api_version = "serving.kserve.io/v1alpha1"

    expected_output = {"metadata": {"name": "cache-1"}}
    client_instance.api_instance.create_cluster_custom_object.return_value = (
        expected_output
    )

    output = client_instance.create_local_model_cache(localmodelcache)

    assert output == expected_output
    client_instance.api_instance.create_cluster_custom_object.assert_called_once_with(
        constants.KSERVE_GROUP,
        "v1alpha1",
        constants.KSERVE_PLURAL_LOCALMODELCACHE,
        localmodelcache,
    )

def test_create_local_model_cache_api_exception():
    client_instance = KServeClient()
    client_instance.api_instance = MagicMock()

    localmodelcache = MagicMock()
    localmodelcache.api_version = "serving.kserve.io/v1alpha1"

    client_instance.api_instance.create_cluster_custom_object.side_effect = (
        client.rest.ApiException("create failed")
    )

    with pytest.raises(
        RuntimeError,
        match="create_cluster_custom_object",
    ):
        client_instance.create_local_model_cache(localmodelcache)

##################################################
# Tests for get_local_model_cache method
##################################################
def test_get_local_model_cache_success():
    client_instance = KServeClient()
    client_instance.api_instance = MagicMock()

    expected_output = {"metadata": {"name": "cache-1"}}
    client_instance.api_instance.get_cluster_custom_object.return_value = (
        expected_output
    )

    output = client_instance.get_local_model_cache(name="cache-1")

    assert output == expected_output
    client_instance.api_instance.get_cluster_custom_object.assert_called_once_with(
        constants.KSERVE_GROUP,
        constants.KSERVE_V1ALPHA1_VERSION,
        constants.KSERVE_PLURAL_LOCALMODELCACHE,
        "cache-1",
    )

def test_get_local_model_cache_api_exception():
    client_instance = KServeClient()
    client_instance.api_instance = MagicMock()

    client_instance.api_instance.get_cluster_custom_object.side_effect = (
        client.rest.ApiException("get failed")
    )

    with pytest.raises(
        RuntimeError,
        match="get_cluster_custom_object",
    ):
        client_instance.get_local_model_cache(name="cache-1")

##################################################
# Tests for delete_local_model_cache method
##################################################
def test_delete_local_model_cache_success():
    client_instance = KServeClient()
    client_instance.api_instance = MagicMock()

    client_instance.delete_local_model_cache(name="cache-1")

    client_instance.api_instance.delete_cluster_custom_object.assert_called_once_with(
        constants.KSERVE_GROUP,
        constants.KSERVE_V1ALPHA1_VERSION,
        constants.KSERVE_PLURAL_LOCALMODELCACHE,
        "cache-1",
    )

def test_delete_local_model_cache_api_exception():
    client_instance = KServeClient()
    client_instance.api_instance = MagicMock()

    client_instance.api_instance.delete_cluster_custom_object.side_effect = (
        client.rest.ApiException("delete failed")
    )

    with pytest.raises(
        RuntimeError,
        match="delete_cluster_custom_object",
    ):
        client_instance.delete_local_model_cache(name="cache-1")

#################################################
# Tests for is_local_model_cache_ready method
#################################################
def test_is_local_model_cache_ready_all_nodes_ready():
    client_instance = KServeClient()

    client_instance.get_local_model_cache = MagicMock(
        return_value={
            "status": {
                "nodeStatus": {
                    "node-1": "NodeDownloaded",
                    "node-2": "NodeDownloaded",
                }
            }
        }
    )

    result = client_instance.is_local_model_cache_ready(
        name="cache-1",
        nodes=["node-1", "node-2"],
    )

    assert result is True
    client_instance.get_local_model_cache.assert_called_once_with(
        "cache-1",
        version=client_instance.KSERVE_V1ALPHA1_VERSION
        if hasattr(client_instance, "KSERVE_V1ALPHA1_VERSION")
        else "v1alpha1",
    )

def test_is_local_model_cache_ready_node_not_ready():
    client_instance = KServeClient()

    client_instance.get_local_model_cache = MagicMock(
        return_value={
            "status": {
                "nodeStatus": {
                    "node-1": "NodeDownloaded",
                    "node-2": "Downloading",
                }
            }
        }
    )

    result = client_instance.is_local_model_cache_ready(
        name="cache-1",
        nodes=["node-1", "node-2"],
    )

    assert result is False

def test_is_local_model_cache_ready_missing_node_status():
    client_instance = KServeClient()

    client_instance.get_local_model_cache = MagicMock(return_value={})

    result = client_instance.is_local_model_cache_ready(
        name="cache-1",
        nodes=["node-1"],
    )

    assert result is False

##################################################
# Tests for wait_local_model_cache_ready method
##################################################
@patch("time.sleep", return_value=None)
def test_wait_local_model_cache_ready_success(mock_sleep):
    client_instance = KServeClient()

    # First call -> False, second call -> True
    client_instance.is_local_model_cache_ready = MagicMock(
        side_effect=[False, True]
    )

    client_instance.wait_local_model_cache_ready(
        name="cache-1",
        nodes=["node-1"],
        timeout_seconds=20,
        polling_interval=10,
    )

    assert client_instance.is_local_model_cache_ready.call_count == 2

@patch("time.sleep", return_value=None)
def test_wait_local_model_cache_ready_timeout(mock_sleep):
    client_instance = KServeClient()

    client_instance.is_local_model_cache_ready = MagicMock(return_value=False)
    client_instance.get_local_model_cache = MagicMock(
        return_value={"status": "not-ready"}
    )

    with pytest.raises(
        RuntimeError,
        match="Timeout while caching the model",
    ):
        client_instance.wait_local_model_cache_ready(
            name="cache-1",
            nodes=["node-1"],
            timeout_seconds=20,
            polling_interval=10,
        )

    client_instance.get_local_model_cache.assert_called_once_with(
        "cache-1",
        version="v1alpha1",
    )
