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
import time

import pytest
from kubernetes import client
from kubernetes.client import V1ResourceRequirements

from kserve import (
    KServeClient,
    V1beta1InferenceService,
    V1beta1InferenceServiceSpec,
    V1beta1ModelFormat,
    V1beta1ModelSpec,
    V1beta1PredictorSpec,
    V1beta1SKLearnSpec,
    constants,
)

from ..common.utils import KSERVE_TEST_NAMESPACE


kserve_client = KServeClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))


def _get_storage_uri(name: str) -> str | None:
    """Read back the storageUri from the stored InferenceService spec."""
    isvc = kserve_client.get(name, namespace=KSERVE_TEST_NAMESPACE)
    model = isvc.get("spec", {}).get("predictor", {}).get("model")
    if model is None:
        return None
    return model.get("storageUri")


@pytest.mark.predictor
def test_storage_uri_preserved_with_legacy_spec_on_create():
    """The mutating webhook must preserve model.storageUri when a legacy spec
    (sklearn) triggers assignSKLearnRuntime() during CREATE defaulting.

    This is the exact code path fixed by the storageUri preservation guard in
    setPredictorModelDefaults().
    """
    service_name = "isvc-webhook-legacy-uri"

    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        sklearn=V1beta1SKLearnSpec(
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "256Mi"},
            ),
        ),
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(name="sklearn"),
            storage_uri="oci://quay.io/example/model:latest",
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "256Mi"},
            ),
        ),
        image_pull_secrets=[
            client.V1LocalObjectReference(name="nonexistent-secret"),
        ],
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name,
            namespace=KSERVE_TEST_NAMESPACE,
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    try:
        kserve_client.create(isvc)
        assert _get_storage_uri(service_name) == "oci://quay.io/example/model:latest"
    finally:
        kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.predictor
def test_storage_uri_preserved_on_replace():
    """storageUri must survive a PUT/replace that changes imagePullSecrets."""
    service_name = "isvc-webhook-replace-uri"

    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(name="sklearn"),
            storage_uri="oci://quay.io/example/model:latest",
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "256Mi"},
            ),
        ),
        image_pull_secrets=[
            client.V1LocalObjectReference(name="test-secret-1"),
        ],
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name,
            namespace=KSERVE_TEST_NAMESPACE,
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    try:
        kserve_client.create(isvc)
        assert _get_storage_uri(service_name) == "oci://quay.io/example/model:latest"

        # GET → modify imagePullSecrets → PUT (mirrors dashboard replace)
        # Retry on 409 Conflict since the controller may update status concurrently.
        for _ in range(5):
            isvc_dict = kserve_client.get(service_name, namespace=KSERVE_TEST_NAMESPACE)
            isvc_dict["spec"]["predictor"]["imagePullSecrets"] = [
                {"name": "test-secret-2"}
            ]
            try:
                kserve_client.api_instance.replace_namespaced_custom_object(
                    constants.KSERVE_GROUP,
                    constants.KSERVE_V1BETA1_VERSION,
                    KSERVE_TEST_NAMESPACE,
                    constants.KSERVE_PLURAL_INFERENCESERVICE,
                    service_name,
                    isvc_dict,
                )
                break
            except client.ApiException as e:
                if e.status == 409:
                    time.sleep(1)
                    continue
                raise
        else:
            pytest.fail("replace failed after 5 retries due to 409 Conflict")

        assert _get_storage_uri(service_name) == "oci://quay.io/example/model:latest"
    finally:
        kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.predictor
def test_no_spurious_storage_uri_injected():
    """When no storageUri is set, the defaulting fix must not inject one."""
    service_name = "isvc-webhook-no-uri"

    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        sklearn=V1beta1SKLearnSpec(
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "256Mi"},
            ),
        ),
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name,
            namespace=KSERVE_TEST_NAMESPACE,
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    try:
        kserve_client.create(isvc)
        assert _get_storage_uri(service_name) is None
    finally:
        kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.predictor
def test_legacy_storage_uri_takes_precedence():
    """When the legacy spec has its own storageUri, it must win over the
    model spec's storageUri after assignSKLearnRuntime() conversion."""
    service_name = "isvc-webhook-legacy-wins"

    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        sklearn=V1beta1SKLearnSpec(
            storage_uri="s3://bucket/legacy-model",
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "256Mi"},
            ),
        ),
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(name="sklearn"),
            storage_uri="oci://quay.io/other/model:latest",
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "256Mi"},
            ),
        ),
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name,
            namespace=KSERVE_TEST_NAMESPACE,
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    try:
        kserve_client.create(isvc)
        assert _get_storage_uri(service_name) == "s3://bucket/legacy-model"
    finally:
        kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
