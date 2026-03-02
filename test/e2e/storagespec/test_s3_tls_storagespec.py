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

from typing import Any, Optional
import json
import os
import time
from base64 import b64decode, b64encode
from contextlib import contextmanager
from kubernetes import client
from kserve import (
    constants,
    KServeClient,
    V1beta1InferenceService,
    V1beta1InferenceServiceSpec,
    V1beta1PredictorSpec,
    V1beta1SKLearnSpec,
    V1beta1StorageSpec,
)
from kubernetes.client import V1ResourceRequirements
import pytest

from ..common.utils import (
    KSERVE_NAMESPACE,
    KSERVE_TEST_NAMESPACE,
    wait_for_resource_deletion,
)


ssl_error = "[SSL: CERTIFICATE_VERIFY_FAILED] certificate verify failed"


def create_storage_config_json(
    service_name: str,
    cabundle_configmap: Optional[str],
) -> dict[str, Any]:
    config: dict[str, Any] = {
        "type": "s3",
        "access_key_id": "s3admin",
        "secret_access_key": "s3admin123",
        "endpoint_url": f"https://{service_name}.kserve.svc:8333",
        "bucket": "mlpipeline",
        "region": "us-south",
        "anonymous": "False",
    }
    if cabundle_configmap is not None:
        config["cabundle_configmap"] = cabundle_configmap
    return config


@pytest.fixture(scope="session")
def kserve_client():
    return KServeClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))


def create_isvc_resource(
    name: str,
    storage_key: str,
) -> V1beta1InferenceService:
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        sklearn=V1beta1SKLearnSpec(
            storage=V1beta1StorageSpec(
                key=storage_key,
                path="sklearn",
                parameters={"bucket": "example-models"},
            ),
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "256Mi"},
            ),
        ),
    )
    return V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=name,
            namespace=KSERVE_TEST_NAMESPACE,
            labels={
                constants.KSERVE_LABEL_NETWORKING_VISIBILITY: constants.KSERVE_LABEL_NETWORKING_VISIBILITY_EXPOSED,
            },
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )


@contextmanager
def managed_isvc(
    kserve_client: KServeClient,
    isvc: V1beta1InferenceService,
):
    service_name = isvc.metadata.name
    kserve_client.create(isvc)
    yield service_name
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
    wait_for_resource_deletion(
        read_func=lambda: kserve_client.api_instance.get_namespaced_custom_object(
            constants.KSERVE_GROUP,
            constants.KSERVE_V1BETA1_VERSION,
            KSERVE_TEST_NAMESPACE,
            constants.KSERVE_PLURAL_INFERENCESERVICE,
            service_name,
        ),
    )


@contextmanager
def managed_storage_config_key(
    kserve_client: KServeClient,
    storage_key: str,
    storage_config: dict[str, Any],
    namespace: str = KSERVE_TEST_NAMESPACE,
):
    secret_name = "storage-config"
    encoded_value = b64encode(json.dumps(storage_config).encode()).decode()
    # Patch to ADD the key (preserves other keys)
    kserve_client.core_api.patch_namespaced_secret(
        secret_name,
        namespace=namespace,
        body={"data": {storage_key: encoded_value}},
    )
    try:
        yield storage_key
    finally:
        # Patch to REMOVE only our key using JSON Patch
        kserve_client.core_api.patch_namespaced_secret(
            secret_name,
            namespace=namespace,
            body=[{"op": "remove", "path": f"/data/{storage_key}"}],
        )


ODH_TRUSTED_CA_BUNDLE_CONFIGMAP_NAME = "odh-trusted-ca-bundle"


@pytest.fixture(scope="module")
def odh_trusted_ca_bundle_configmap(kserve_client):
    """Create empty odh-trusted-ca-bundle configmap at module level."""
    odh_trusted_ca_configmap = client.V1ConfigMap(
        api_version="v1",
        kind="ConfigMap",
        metadata=client.V1ObjectMeta(name=ODH_TRUSTED_CA_BUNDLE_CONFIGMAP_NAME),
        data={},
    )
    try:
        kserve_client.core_api.create_namespaced_config_map(
            namespace=KSERVE_TEST_NAMESPACE, body=odh_trusted_ca_configmap
        )
    except client.ApiException as e:
        if e.status != 409:  # 409 = already exists (another worker created it)
            raise
    yield ODH_TRUSTED_CA_BUNDLE_CONFIGMAP_NAME
    try:
        kserve_client.core_api.delete_namespaced_config_map(
            name=ODH_TRUSTED_CA_BUNDLE_CONFIGMAP_NAME, namespace=KSERVE_TEST_NAMESPACE
        )
        wait_for_resource_deletion(
            read_func=lambda: kserve_client.core_api.read_namespaced_config_map(
                name=ODH_TRUSTED_CA_BUNDLE_CONFIGMAP_NAME, namespace=KSERVE_TEST_NAMESPACE
            ),
        )
    except client.ApiException as e:
        if e.status != 404:  # 404 = already deleted (another worker cleaned it up)
            raise


@contextmanager
def managed_ca_bundle_key(kserve_client: KServeClient, data_key: str):
    """Add a CA bundle key to the odh-trusted-ca-bundle configmap, remove on cleanup."""
    seaweedfs_tls_custom_certs = kserve_client.core_api.read_namespaced_secret(
        "seaweedfs-tls-custom", KSERVE_NAMESPACE
    ).data
    cert_data = b64decode(seaweedfs_tls_custom_certs["root.crt"]).decode()
    # Patch to ADD the key (preserves other keys)
    kserve_client.core_api.patch_namespaced_config_map(
        ODH_TRUSTED_CA_BUNDLE_CONFIGMAP_NAME,
        namespace=KSERVE_TEST_NAMESPACE,
        body={"data": {data_key: cert_data}},
    )
    try:
        yield data_key
    finally:
        # Patch to REMOVE only our key using JSON Patch
        kserve_client.core_api.patch_namespaced_config_map(
            ODH_TRUSTED_CA_BUNDLE_CONFIGMAP_NAME,
            namespace=KSERVE_TEST_NAMESPACE,
            body=[{"op": "remove", "path": f"/data/{data_key}"}],
        )


@pytest.mark.kserve_on_openshift
def test_s3_tls_global_custom_cert_storagespec_kserve(kserve_client, odh_trusted_ca_bundle_configmap):
    # Validate that the model is successfully loaded when the global custom cert is valid
    pass_storage_config = create_storage_config_json("seaweedfs-tls-custom-service", "odh-kserve-custom-ca-bundle")
    pass_service_name = "isvc-sklearn-s3-tls-global-pass"
    pass_isvc = create_isvc_resource(pass_service_name, "localTLSS3Global")
    with managed_ca_bundle_key(kserve_client, "ca-bundle.crt"):
        with managed_storage_config_key(kserve_client, "localTLSS3Global", pass_storage_config):
            with managed_isvc(kserve_client, pass_isvc):
                check_model_status(kserve_client, pass_service_name, KSERVE_TEST_NAMESPACE, "UpToDate")

    # Validate that the model fails to load when the cabundle_configmap is not referenced in the storage config
    fail_storage_config = create_storage_config_json("seaweedfs-tls-custom-service", None)
    fail_service_name = "isvc-sklearn-s3-tls-global-fail"
    fail_isvc = create_isvc_resource(fail_service_name, "localTLSS3Global")
    with managed_storage_config_key(kserve_client, "localTLSS3Global", fail_storage_config):
        with managed_isvc(kserve_client, fail_isvc):
            check_model_status(kserve_client, fail_service_name, KSERVE_TEST_NAMESPACE, "BlockedByFailedLoad", ssl_error)


@pytest.mark.kserve_on_openshift
def test_s3_tls_custom_cert_storagespec_kserve(kserve_client, odh_trusted_ca_bundle_configmap):
    # Validate that the model is successfully loaded when the custom cert is valid
    pass_storage_config = create_storage_config_json("seaweedfs-tls-custom-service", "odh-kserve-custom-ca-bundle")
    pass_service_name = "isvc-sklearn-s3-tls-custom-pass"
    pass_isvc = create_isvc_resource(pass_service_name, "localTLSS3Custom")
    with managed_ca_bundle_key(kserve_client, "odh-ca-bundle.crt"):
        with managed_storage_config_key(kserve_client, "localTLSS3Custom", pass_storage_config):
            with managed_isvc(kserve_client, pass_isvc):
                check_model_status(kserve_client, pass_service_name, KSERVE_TEST_NAMESPACE, "UpToDate")

    # Validate that the model fails to load when the cabundle_configmap is not referenced in the storage config
    fail_storage_config = create_storage_config_json("seaweedfs-tls-custom-service", None)
    fail_service_name = "isvc-sklearn-s3-tls-custom-fail"
    fail_isvc = create_isvc_resource(fail_service_name, "localTLSS3Custom")
    with managed_storage_config_key(kserve_client, "localTLSS3Custom", fail_storage_config):
        with managed_isvc(kserve_client, fail_isvc):
            check_model_status(kserve_client, fail_service_name, KSERVE_TEST_NAMESPACE, "BlockedByFailedLoad", ssl_error)


@pytest.mark.kserve_on_openshift
def test_s3_tls_serving_cert_storagespec_kserve(kserve_client):
    # Validate that the model is successfully loaded when the serving cert is valid
    pass_storage_config = create_storage_config_json("seaweedfs-tls-serving-service", "odh-kserve-custom-ca-bundle")
    pass_service_name = "isvc-sklearn-s3-tls-serving-pass"
    pass_isvc = create_isvc_resource(pass_service_name, storage_key="localTLSS3Serving")
    with managed_storage_config_key(kserve_client, "localTLSS3Serving", pass_storage_config):
        with managed_isvc(kserve_client, pass_isvc):
            check_model_status(kserve_client, pass_service_name, KSERVE_TEST_NAMESPACE, "UpToDate")

    # Validate that the model fails to load when the serving cert is not referenced in the storage config
    fail_storage_config = create_storage_config_json("seaweedfs-tls-serving-service", None)
    fail_service_name = "isvc-sklearn-s3-tls-serving-fail"
    fail_isvc = create_isvc_resource(fail_service_name, storage_key="localTLSS3Serving")
    with managed_storage_config_key(kserve_client, "localTLSS3Serving", fail_storage_config):
        with managed_isvc(kserve_client, fail_isvc):
            check_model_status(kserve_client, fail_service_name, KSERVE_TEST_NAMESPACE, "BlockedByFailedLoad", ssl_error)


def check_model_status(
    kserve_client: KServeClient,
    isvc_name: str,
    isvc_namespace: str,
    expected_status: str,
    expected_failure_message: Optional[str] = None,
    timeout_seconds: int = 660,  # Default progressDeadlineSeconds + 60 seconds
    polling_interval: int = 10,
):
    model_status = {}
    for _ in range(round(timeout_seconds / polling_interval)):
        time.sleep(polling_interval)
        isvc = kserve_client.get(
            name=isvc_name,
            namespace=isvc_namespace,
            version=constants.KSERVE_V1BETA1_VERSION,
        )
        model_status = isvc.get("status", {}).get("modelStatus", {})

        failure_message_match = True
        if expected_failure_message is not None:
            failure_message_match = expected_failure_message in model_status.get("lastFailureInfo", {}).get("message", "")

        if (
            model_status.get("transitionStatus") == expected_status
            and failure_message_match
        ):
            return

    actual_status = model_status.get("transitionStatus", "")
    if expected_failure_message is not None:
        actual_failure_message = (
            model_status.get("lastFailureInfo", {}).get("message", "")
        )
        raise RuntimeError(
            f"Expected inferenceservice {isvc_name} to have model transition status '{expected_status}' "
            f"and last failure info '{expected_failure_message}' after timeout, "
            f"but got model transition status '{actual_status}' "
            f"and last failure info '{actual_failure_message}'"
        )
    raise RuntimeError(
        f"Expected inferenceservice {isvc_name} to have model transition status '{expected_status}' "
        f"after timeout, but got '{actual_status}'"
    )
