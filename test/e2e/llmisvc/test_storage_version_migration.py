# Copyright 2025 The KServe Authors.
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

"""
E2E test for storage version migration.

Verifies that the LLMInferenceService controller correctly runs storage version
migration after the webhook server is ready. The migration patches all resources
to re-encode them in the current storage version (v1alpha2) and then updates the
CRD status.storedVersions to drop stale versions.

To trigger an actual migration, we simulate an upgrade scenario by patching the
CRD status to include a stale stored version, then restarting the controller.
"""

import os
import time
import subprocess
import pytest
from kserve import KServeClient, constants
from kubernetes import client

from .fixtures import (
    inject_k8s_proxy,
    KSERVE_TEST_NAMESPACE,
    KSERVE_PLURAL_LLMINFERENCESERVICECONFIG,
)
from .logging import logger

LLMISVC_CRD_NAME = "llminferenceservices.serving.kserve.io"
LLMISVC_CONFIG_CRD_NAME = "llminferenceserviceconfigs.serving.kserve.io"
CONTROLLER_NAMESPACE = os.environ.get("KSERVE_NAMESPACE", "opendatahub")
CONTROLLER_DEPLOYMENT = "llmisvc-controller-manager"


def wait_for(assertion_fn, timeout: float = 60.0, interval: float = 1.0):
    """Wait for the assertion to succeed within timeout."""
    deadline = time.time() + timeout
    last_error = None
    while True:
        try:
            return assertion_fn()
        except (AssertionError, Exception) as e:
            last_error = e
            if time.time() >= deadline:
                raise AssertionError(
                    f"Timed out after {timeout}s waiting for assertion. Last error: {last_error}"
                ) from e
            time.sleep(interval)


@pytest.mark.llminferenceservice
@pytest.mark.conversion
class TestStorageVersionMigration:
    """Test storage version migration runs correctly during controller startup."""

    @pytest.fixture(autouse=True)
    def setup(self):
        """Setup test fixtures."""
        inject_k8s_proxy()
        self.kserve_client = KServeClient(
            config_file=os.environ.get("KUBECONFIG", "~/.kube/config"),
            client_configuration=client.Configuration(),
        )
        self.apix_client = client.ApiextensionsV1Api()
        self.namespace = KSERVE_TEST_NAMESPACE
        self.created_resources = []
        yield
        self._cleanup_resources()

    def _cleanup_resources(self):
        """Clean up created resources and restore CRD status."""
        # Always restore CRD storedVersions to prevent dirty state
        for crd_name in [LLMISVC_CONFIG_CRD_NAME, LLMISVC_CRD_NAME]:
            try:
                self.apix_client.patch_custom_resource_definition_status(
                    crd_name,
                    body={"status": {"storedVersions": ["v1alpha2"]}},
                )
            except Exception as e:
                logger.warning(f"Failed to restore storedVersions for {crd_name}: {e}")

        if os.getenv("SKIP_RESOURCE_DELETION", "False").lower() in ("true", "1", "t"):
            logger.info("Skipping resource deletion after test execution.")
            return

        for resource_type, name, version in self.created_resources:
            try:
                if resource_type == "config":
                    self.kserve_client.api_instance.delete_namespaced_custom_object(
                        constants.KSERVE_GROUP,
                        version,
                        self.namespace,
                        KSERVE_PLURAL_LLMINFERENCESERVICECONFIG,
                        name,
                    )
            except Exception as e:
                logger.warning(f"Failed to cleanup {resource_type} {name}: {e}")

    @pytest.mark.cluster_cpu
    @pytest.mark.cluster_single_node
    def test_storage_version_migration_after_simulated_upgrade(self):
        """Test that storage version migration runs successfully after controller restart.

        Simulates an upgrade by:
        1. Creating a resource via v1alpha1 API
        2. Patching CRD storedVersions to include the stale v1alpha1 version
        3. Restarting the controller (which triggers migration on startup)
        4. Verifying storedVersions is cleaned up to only contain v1alpha2
        """
        # 1. Create a config resource via v1alpha1 so we have something to migrate
        config_name = "migration-test-config"
        config = {
            "apiVersion": f"{constants.KSERVE_GROUP}/{constants.KSERVE_V1ALPHA1_VERSION}",
            "kind": "LLMInferenceServiceConfig",
            "metadata": {
                "name": config_name,
                "namespace": self.namespace,
            },
            "spec": {
                "model": {"uri": "hf://facebook/opt-125m", "name": "facebook/opt-125m"},
                "router": {"route": {}},
                "template": {
                    "containers": [
                        {
                            "name": "main",
                            "image": "public.ecr.aws/q9t5s3a7/vllm-cpu-release-repo:v0.17.1",
                            "resources": {
                                "limits": {"cpu": "1", "memory": "2Gi"},
                                "requests": {"cpu": "100m", "memory": "512Mi"},
                            },
                        }
                    ]
                },
            },
        }
        self.kserve_client.api_instance.create_namespaced_custom_object(
            constants.KSERVE_GROUP,
            constants.KSERVE_V1ALPHA1_VERSION,
            self.namespace,
            KSERVE_PLURAL_LLMINFERENCESERVICECONFIG,
            config,
        )
        self.created_resources.append(
            ("config", config_name, constants.KSERVE_V1ALPHA2_VERSION)
        )
        logger.info(f"Created LLMInferenceServiceConfig {config_name} via v1alpha1")

        # 2. Patch CRD storedVersions to simulate upgrade state.
        # After a real upgrade from v1alpha1-only to v1alpha1+v1alpha2,
        # storedVersions would be ["v1alpha1", "v1alpha2"]. This triggers
        # the migrator to re-encode all resources in the current storage version.
        for crd_name in [LLMISVC_CONFIG_CRD_NAME, LLMISVC_CRD_NAME]:
            self.apix_client.patch_custom_resource_definition_status(
                crd_name,
                body={"status": {"storedVersions": ["v1alpha1", "v1alpha2"]}},
            )
            logger.info(f"Patched {crd_name} storedVersions to [v1alpha1, v1alpha2]")

        # Verify the patch took effect
        for crd_name in [LLMISVC_CONFIG_CRD_NAME, LLMISVC_CRD_NAME]:
            crd = self.apix_client.read_custom_resource_definition(crd_name)
            assert set(crd.status.stored_versions) == {"v1alpha1", "v1alpha2"}, (
                f"Expected storedVersions to contain both versions, got {crd.status.stored_versions}"
            )

        # 3. Restart the controller to trigger migration on startup.
        # The controller runs migration as a manager Runnable that executes
        # after the webhook server is ready.
        logger.info(f"Restarting {CONTROLLER_DEPLOYMENT} in {CONTROLLER_NAMESPACE}")
        subprocess.run(
            [
                "kubectl",
                "rollout",
                "restart",
                f"deployment/{CONTROLLER_DEPLOYMENT}",
                "-n",
                CONTROLLER_NAMESPACE,
            ],
            check=True,
        )
        subprocess.run(
            [
                "kubectl",
                "rollout",
                "status",
                f"deployment/{CONTROLLER_DEPLOYMENT}",
                "-n",
                CONTROLLER_NAMESPACE,
                "--timeout=120s",
            ],
            check=True,
        )
        logger.info("Controller restarted successfully")

        # 4. Verify storedVersions has been cleaned up by the migrator.
        # The migrator patches all resources with an empty merge patch to
        # re-encode them in v1alpha2, then drops v1alpha1 from storedVersions.
        def assert_stored_versions_migrated():
            for crd_name in [LLMISVC_CONFIG_CRD_NAME, LLMISVC_CRD_NAME]:
                crd = self.apix_client.read_custom_resource_definition(crd_name)
                assert crd.status.stored_versions == ["v1alpha2"], (
                    f"Expected storedVersions=['v1alpha2'] after migration, "
                    f"got {crd.status.stored_versions} for {crd_name}"
                )

        wait_for(assert_stored_versions_migrated, timeout=180.0, interval=5.0)
        logger.info("Storage version migration completed - storedVersions cleaned up")

        # 5. Verify the resource is still accessible via both API versions
        v1 = self.kserve_client.api_instance.get_namespaced_custom_object(
            constants.KSERVE_GROUP,
            constants.KSERVE_V1ALPHA1_VERSION,
            self.namespace,
            KSERVE_PLURAL_LLMINFERENCESERVICECONFIG,
            config_name,
        )
        assert v1 is not None
        assert v1["metadata"]["name"] == config_name

        v2 = self.kserve_client.api_instance.get_namespaced_custom_object(
            constants.KSERVE_GROUP,
            constants.KSERVE_V1ALPHA2_VERSION,
            self.namespace,
            KSERVE_PLURAL_LLMINFERENCESERVICECONFIG,
            config_name,
        )
        assert v2 is not None
        assert v2["metadata"]["name"] == config_name

        logger.info(
            "Resource accessible via both v1alpha1 and v1alpha2 after migration"
        )
