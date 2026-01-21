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
E2E tests for LLMInferenceService v1alpha1 <-> v1alpha2 API conversion.

These tests validate:
1. v1alpha1 -> v1alpha2 conversion (read as different version)
2. v1alpha2 -> v1alpha1 conversion (read as different version)
3. Criticality field preservation via annotations during conversion
4. Round-trip conversion preserves all fields
"""

import os
import time
import pytest
from kserve import KServeClient, constants
from kubernetes import client

from .fixtures import (
    inject_k8s_proxy,
    KSERVE_TEST_NAMESPACE,
    KSERVE_PLURAL_LLMINFERENCESERVICECONFIG,
)
from .logging import log_execution, logger

KSERVE_PLURAL_LLMINFERENCESERVICE = "llminferenceservices"

# Annotation keys used for criticality preservation during conversion
MODEL_CRITICALITY_ANNOTATION_KEY = "internal.serving.kserve.io/model-criticality"
LORA_CRITICALITIES_ANNOTATION_KEY = "internal.serving.kserve.io/lora-criticalities"


def wait_for(
    assertion_fn, timeout: float = 60.0, interval: float = 1.0
):
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


@log_execution
def create_llmisvc_raw(kserve_client: KServeClient, llm_isvc: dict, version: str):
    """Create an LLMInferenceService using raw dict."""
    try:
        namespace = llm_isvc.get("metadata", {}).get("namespace", KSERVE_TEST_NAMESPACE)
        outputs = kserve_client.api_instance.create_namespaced_custom_object(
            constants.KSERVE_GROUP,
            version,
            namespace,
            KSERVE_PLURAL_LLMINFERENCESERVICE,
            llm_isvc,
        )
        print(f"✅ LLM inference service {llm_isvc['metadata']['name']} created with {version}")
        return outputs
    except client.rest.ApiException as e:
        raise RuntimeError(
            f"❌ Exception creating LLMInferenceService: {e}"
        ) from e


@log_execution
def get_llmisvc_raw(kserve_client: KServeClient, name: str, namespace: str, version: str):
    """Get an LLMInferenceService as a specific API version."""
    try:
        return kserve_client.api_instance.get_namespaced_custom_object(
            constants.KSERVE_GROUP,
            version,
            namespace,
            KSERVE_PLURAL_LLMINFERENCESERVICE,
            name,
        )
    except client.rest.ApiException as e:
        raise RuntimeError(
            f"❌ Exception getting LLMInferenceService {name} as {version}: {e}"
        ) from e


@log_execution
def delete_llmisvc_raw(kserve_client: KServeClient, name: str, namespace: str, version: str):
    """Delete an LLMInferenceService."""
    try:
        result = kserve_client.api_instance.delete_namespaced_custom_object(
            constants.KSERVE_GROUP,
            version,
            namespace,
            KSERVE_PLURAL_LLMINFERENCESERVICE,
            name,
        )
        print(f"✅ LLM inference service {name} deleted")
        return result
    except client.rest.ApiException as e:
        raise RuntimeError(
            f"❌ Exception deleting LLMInferenceService {name}: {e}"
        ) from e


@log_execution
def create_llmisvc_config_raw(kserve_client: KServeClient, config: dict, version: str):
    """Create an LLMInferenceServiceConfig using raw dict."""
    try:
        namespace = config.get("metadata", {}).get("namespace", KSERVE_TEST_NAMESPACE)
        outputs = kserve_client.api_instance.create_namespaced_custom_object(
            constants.KSERVE_GROUP,
            version,
            namespace,
            KSERVE_PLURAL_LLMINFERENCESERVICECONFIG,
            config,
        )
        print(f"✅ LLMInferenceServiceConfig {config['metadata']['name']} created with {version}")
        return outputs
    except client.rest.ApiException as e:
        if e.status == 409:  # Already exists
            print(f"⚠️ LLMInferenceServiceConfig {config['metadata']['name']} already exists")
            return None
        raise RuntimeError(
            f"❌ Exception creating LLMInferenceServiceConfig: {e}"
        ) from e


@log_execution
def delete_llmisvc_config_raw(kserve_client: KServeClient, name: str, namespace: str, version: str):
    """Delete an LLMInferenceServiceConfig."""
    try:
        result = kserve_client.api_instance.delete_namespaced_custom_object(
            constants.KSERVE_GROUP,
            version,
            namespace,
            KSERVE_PLURAL_LLMINFERENCESERVICECONFIG,
            name,
        )
        print(f"✅ LLMInferenceServiceConfig {name} deleted")
        return result
    except client.rest.ApiException as e:
        if e.status == 404:
            print(f"⚠️ LLMInferenceServiceConfig {name} not found")
            return None
        raise RuntimeError(
            f"❌ Exception deleting LLMInferenceServiceConfig {name}: {e}"
        ) from e


@pytest.mark.llminferenceservice
@pytest.mark.conversion
class TestLLMInferenceServiceConversion:
    """Test suite for LLMInferenceService API version conversion."""

    @pytest.fixture(autouse=True)
    def setup(self):
        """Setup test fixtures."""
        inject_k8s_proxy()
        self.kserve_client = KServeClient(
            config_file=os.environ.get("KUBECONFIG", "~/.kube/config"),
            client_configuration=client.Configuration(),
        )
        self.namespace = KSERVE_TEST_NAMESPACE
        self.created_resources = []
        yield
        # Cleanup
        self._cleanup_resources()

    def _cleanup_resources(self):
        """Clean up created resources."""
        if os.getenv("SKIP_RESOURCE_DELETION", "False").lower() in ("true", "1", "t"):
            logger.info("Skipping resource deletion after test execution.")
            return

        for resource_type, name, version in self.created_resources:
            try:
                if resource_type == "llmisvc":
                    delete_llmisvc_raw(self.kserve_client, name, self.namespace, version)
                elif resource_type == "config":
                    delete_llmisvc_config_raw(self.kserve_client, name, self.namespace, version)
            except Exception as e:
                logger.warning(f"Failed to cleanup {resource_type} {name}: {e}")

    @pytest.mark.cluster_cpu
    @pytest.mark.cluster_single_node
    def test_v1alpha1_to_v1alpha2_conversion(self):
        """Test that a v1alpha1 LLMInferenceService can be read as v1alpha2."""
        service_name = "conv-test-v1a1-to-v1a2"

        # Create base config first
        config_name = f"{service_name}-config"
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
                    "containers": [{
                        "name": "main",
                        "image": "quay.io/pierdipi/vllm-cpu:latest",
                        "resources": {
                            "limits": {"cpu": "2", "memory": "7Gi"},
                            "requests": {"cpu": ".5", "memory": "4Gi"},
                        },
                    }]
                },
            },
        }
        create_llmisvc_config_raw(self.kserve_client, config, constants.KSERVE_V1ALPHA1_VERSION)
        self.created_resources.append(("config", config_name, constants.KSERVE_V1ALPHA1_VERSION))

        # Create v1alpha1 LLMInferenceService
        v1alpha1_isvc = {
            "apiVersion": f"{constants.KSERVE_GROUP}/{constants.KSERVE_V1ALPHA1_VERSION}",
            "kind": "LLMInferenceService",
            "metadata": {
                "name": service_name,
                "namespace": self.namespace,
            },
            "spec": {
                "baseRefs": [{"name": config_name}],
            },
        }

        create_llmisvc_raw(self.kserve_client, v1alpha1_isvc, constants.KSERVE_V1ALPHA1_VERSION)
        self.created_resources.append(("llmisvc", service_name, constants.KSERVE_V1ALPHA1_VERSION))

        # Read as v1alpha2 - this tests the conversion webhook
        def assert_readable_as_v1alpha2():
            v1alpha2_isvc = get_llmisvc_raw(
                self.kserve_client, service_name, self.namespace, constants.KSERVE_V1ALPHA2_VERSION
            )

            # Verify basic fields are present
            assert v1alpha2_isvc is not None, "Should be able to read v1alpha1 resource as v1alpha2"
            assert v1alpha2_isvc["apiVersion"] == f"{constants.KSERVE_GROUP}/{constants.KSERVE_V1ALPHA2_VERSION}"
            assert v1alpha2_isvc["metadata"]["name"] == service_name

            # Verify spec fields are converted
            spec = v1alpha2_isvc.get("spec", {})
            assert "baseRefs" in spec, "baseRefs should be present in converted spec"

            return v1alpha2_isvc

        v1alpha2_result = wait_for(assert_readable_as_v1alpha2, timeout=30.0)
        print(f"✅ Successfully read v1alpha1 resource as v1alpha2: {v1alpha2_result['metadata']['name']}")

    @pytest.mark.cluster_cpu
    @pytest.mark.cluster_single_node
    def test_v1alpha2_to_v1alpha1_conversion(self):
        """Test that a v1alpha2 LLMInferenceService can be read as v1alpha1."""
        service_name = "conv-test-v1a2-to-v1a1"

        # Create base config first (using v1alpha2)
        config_name = f"{service_name}-config"
        config = {
            "apiVersion": f"{constants.KSERVE_GROUP}/{constants.KSERVE_V1ALPHA2_VERSION}",
            "kind": "LLMInferenceServiceConfig",
            "metadata": {
                "name": config_name,
                "namespace": self.namespace,
            },
            "spec": {
                "model": {"uri": "hf://facebook/opt-125m", "name": "facebook/opt-125m"},
                "router": {"route": {}},
                "template": {
                    "containers": [{
                        "name": "main",
                        "image": "quay.io/pierdipi/vllm-cpu:latest",
                        "resources": {
                            "limits": {"cpu": "2", "memory": "7Gi"},
                            "requests": {"cpu": ".5", "memory": "4Gi"},
                        },
                    }]
                },
            },
        }
        create_llmisvc_config_raw(self.kserve_client, config, constants.KSERVE_V1ALPHA2_VERSION)
        self.created_resources.append(("config", config_name, constants.KSERVE_V1ALPHA2_VERSION))

        # Create v1alpha2 LLMInferenceService
        v1alpha2_isvc = {
            "apiVersion": f"{constants.KSERVE_GROUP}/{constants.KSERVE_V1ALPHA2_VERSION}",
            "kind": "LLMInferenceService",
            "metadata": {
                "name": service_name,
                "namespace": self.namespace,
            },
            "spec": {
                "baseRefs": [{"name": config_name}],
            },
        }

        create_llmisvc_raw(self.kserve_client, v1alpha2_isvc, constants.KSERVE_V1ALPHA2_VERSION)
        self.created_resources.append(("llmisvc", service_name, constants.KSERVE_V1ALPHA2_VERSION))

        # Read as v1alpha1 - this tests the conversion webhook
        def assert_readable_as_v1alpha1():
            v1alpha1_isvc = get_llmisvc_raw(
                self.kserve_client, service_name, self.namespace, constants.KSERVE_V1ALPHA1_VERSION
            )

            # Verify basic fields are present
            assert v1alpha1_isvc is not None, "Should be able to read v1alpha2 resource as v1alpha1"
            assert v1alpha1_isvc["apiVersion"] == f"{constants.KSERVE_GROUP}/{constants.KSERVE_V1ALPHA1_VERSION}"
            assert v1alpha1_isvc["metadata"]["name"] == service_name

            # Verify spec fields are converted
            spec = v1alpha1_isvc.get("spec", {})
            assert "baseRefs" in spec, "baseRefs should be present in converted spec"

            return v1alpha1_isvc

        v1alpha1_result = wait_for(assert_readable_as_v1alpha1, timeout=30.0)
        print(f"✅ Successfully read v1alpha2 resource as v1alpha1: {v1alpha1_result['metadata']['name']}")

    @pytest.mark.cluster_cpu
    @pytest.mark.cluster_single_node
    def test_criticality_preservation_via_annotations(self):
        """Test that criticality field is preserved via annotations during conversion.

        v1alpha1 has a 'criticality' field that doesn't exist in v1alpha2.
        The conversion webhook should store this value in an annotation when
        converting to v1alpha2, and restore it when converting back to v1alpha1.
        """
        service_name = "conv-test-criticality"

        # Create base config with model (v1alpha1 with criticality)
        config_name = f"{service_name}-config"
        config = {
            "apiVersion": f"{constants.KSERVE_GROUP}/{constants.KSERVE_V1ALPHA1_VERSION}",
            "kind": "LLMInferenceServiceConfig",
            "metadata": {
                "name": config_name,
                "namespace": self.namespace,
            },
            "spec": {
                "model": {
                    "uri": "hf://facebook/opt-125m",
                    "name": "facebook/opt-125m",
                    "criticality": "Critical",  # v1alpha1-specific field
                },
                "router": {"route": {}},
                "template": {
                    "containers": [{
                        "name": "main",
                        "image": "quay.io/pierdipi/vllm-cpu:latest",
                        "resources": {
                            "limits": {"cpu": "2", "memory": "7Gi"},
                            "requests": {"cpu": ".5", "memory": "4Gi"},
                        },
                    }]
                },
            },
        }
        create_llmisvc_config_raw(self.kserve_client, config, constants.KSERVE_V1ALPHA1_VERSION)
        self.created_resources.append(("config", config_name, constants.KSERVE_V1ALPHA1_VERSION))

        # Create v1alpha1 LLMInferenceService with criticality in model spec
        v1alpha1_isvc = {
            "apiVersion": f"{constants.KSERVE_GROUP}/{constants.KSERVE_V1ALPHA1_VERSION}",
            "kind": "LLMInferenceService",
            "metadata": {
                "name": service_name,
                "namespace": self.namespace,
            },
            "spec": {
                "model": {
                    "uri": "hf://facebook/opt-125m",
                    "name": "facebook/opt-125m",
                    "criticality": "Critical",
                },
                "baseRefs": [{"name": config_name}],
            },
        }

        create_llmisvc_raw(self.kserve_client, v1alpha1_isvc, constants.KSERVE_V1ALPHA1_VERSION)
        self.created_resources.append(("llmisvc", service_name, constants.KSERVE_V1ALPHA1_VERSION))

        # Step 1: Read as v1alpha2 and verify criticality is stored in annotation
        def assert_criticality_in_annotation():
            v1alpha2_isvc = get_llmisvc_raw(
                self.kserve_client, service_name, self.namespace, constants.KSERVE_V1ALPHA2_VERSION
            )

            annotations = v1alpha2_isvc.get("metadata", {}).get("annotations", {})

            # The criticality should be stored in an annotation
            assert MODEL_CRITICALITY_ANNOTATION_KEY in annotations, (
                f"Criticality should be preserved in annotation {MODEL_CRITICALITY_ANNOTATION_KEY}. "
                f"Annotations: {annotations}"
            )
            assert annotations[MODEL_CRITICALITY_ANNOTATION_KEY] == "Critical", (
                f"Criticality annotation value should be 'Critical', got: "
                f"{annotations.get(MODEL_CRITICALITY_ANNOTATION_KEY)}"
            )

            # v1alpha2 model spec should NOT have criticality field
            model_spec = v1alpha2_isvc.get("spec", {}).get("model", {})
            assert "criticality" not in model_spec, (
                "v1alpha2 model spec should not have criticality field"
            )

            return v1alpha2_isvc

        wait_for(assert_criticality_in_annotation, timeout=30.0)
        print("✅ Criticality preserved in annotation when reading as v1alpha2")

        # Step 2: Read back as v1alpha1 and verify criticality is restored
        def assert_criticality_restored():
            v1alpha1_isvc = get_llmisvc_raw(
                self.kserve_client, service_name, self.namespace, constants.KSERVE_V1ALPHA1_VERSION
            )

            model_spec = v1alpha1_isvc.get("spec", {}).get("model", {})

            # Criticality should be restored in model spec
            assert "criticality" in model_spec, (
                "Criticality should be restored in v1alpha1 model spec"
            )
            assert model_spec["criticality"] == "Critical", (
                f"Criticality should be 'Critical', got: {model_spec.get('criticality')}"
            )

            # The annotation should be cleaned up after conversion back
            annotations = v1alpha1_isvc.get("metadata", {}).get("annotations", {})
            assert MODEL_CRITICALITY_ANNOTATION_KEY not in annotations, (
                "Criticality annotation should be cleaned up after converting back to v1alpha1"
            )

            return v1alpha1_isvc

        wait_for(assert_criticality_restored, timeout=30.0)
        print("✅ Criticality restored when reading back as v1alpha1")

    @pytest.mark.cluster_cpu
    @pytest.mark.cluster_single_node
    def test_lora_criticality_preservation(self):
        """Test that LoRA adapter criticality fields are preserved via annotations.

        When a v1alpha1 LLMInferenceService has LoRA adapters with criticality,
        the conversion should preserve those values in a JSON annotation.
        """
        service_name = "conv-test-lora-crit"

        # Create base config
        config_name = f"{service_name}-config"
        config = {
            "apiVersion": f"{constants.KSERVE_GROUP}/{constants.KSERVE_V1ALPHA1_VERSION}",
            "kind": "LLMInferenceServiceConfig",
            "metadata": {
                "name": config_name,
                "namespace": self.namespace,
            },
            "spec": {
                "router": {"route": {}},
                "template": {
                    "containers": [{
                        "name": "main",
                        "image": "quay.io/pierdipi/vllm-cpu:latest",
                        "resources": {
                            "limits": {"cpu": "2", "memory": "7Gi"},
                            "requests": {"cpu": ".5", "memory": "4Gi"},
                        },
                    }]
                },
            },
        }
        create_llmisvc_config_raw(self.kserve_client, config, constants.KSERVE_V1ALPHA1_VERSION)
        self.created_resources.append(("config", config_name, constants.KSERVE_V1ALPHA1_VERSION))

        # Create v1alpha1 LLMInferenceService with LoRA adapters having criticality
        v1alpha1_isvc = {
            "apiVersion": f"{constants.KSERVE_GROUP}/{constants.KSERVE_V1ALPHA1_VERSION}",
            "kind": "LLMInferenceService",
            "metadata": {
                "name": service_name,
                "namespace": self.namespace,
            },
            "spec": {
                "model": {
                    "uri": "hf://meta-llama/Llama-2-7b",
                    "name": "llama-2-7b",
                    "criticality": "Critical",
                    "lora": {
                        "adapters": [
                            {
                                "uri": "hf://adapter-1",
                                "name": "adapter-1",
                                "criticality": "Standard",
                            },
                            {
                                "uri": "hf://adapter-2",
                                "name": "adapter-2",
                                "criticality": "Sheddable",
                            },
                        ],
                    },
                },
                "baseRefs": [{"name": config_name}],
            },
        }

        create_llmisvc_raw(self.kserve_client, v1alpha1_isvc, constants.KSERVE_V1ALPHA1_VERSION)
        self.created_resources.append(("llmisvc", service_name, constants.KSERVE_V1ALPHA1_VERSION))

        # Read as v1alpha2 and verify LoRA criticalities are in annotation
        def assert_lora_criticalities_in_annotation():
            v1alpha2_isvc = get_llmisvc_raw(
                self.kserve_client, service_name, self.namespace, constants.KSERVE_V1ALPHA2_VERSION
            )

            annotations = v1alpha2_isvc.get("metadata", {}).get("annotations", {})

            # Model criticality should be in annotation
            assert MODEL_CRITICALITY_ANNOTATION_KEY in annotations, (
                "Model criticality should be preserved in annotation"
            )

            # LoRA criticalities should be in annotation as JSON
            assert LORA_CRITICALITIES_ANNOTATION_KEY in annotations, (
                f"LoRA criticalities should be preserved in annotation {LORA_CRITICALITIES_ANNOTATION_KEY}"
            )

            import json
            lora_crit_data = json.loads(annotations[LORA_CRITICALITIES_ANNOTATION_KEY])

            # Verify both adapter criticalities are stored (keys are string indices)
            assert "0" in lora_crit_data or 0 in lora_crit_data, "Adapter 0 criticality should be stored"
            assert "1" in lora_crit_data or 1 in lora_crit_data, "Adapter 1 criticality should be stored"

            return v1alpha2_isvc

        wait_for(assert_lora_criticalities_in_annotation, timeout=30.0)
        print("✅ LoRA adapter criticalities preserved in annotation")

        # Read back as v1alpha1 and verify LoRA criticalities are restored
        def assert_lora_criticalities_restored():
            v1alpha1_isvc = get_llmisvc_raw(
                self.kserve_client, service_name, self.namespace, constants.KSERVE_V1ALPHA1_VERSION
            )

            model_spec = v1alpha1_isvc.get("spec", {}).get("model", {})
            lora_spec = model_spec.get("lora", {})
            adapters = lora_spec.get("adapters", [])

            assert len(adapters) >= 2, "Should have at least 2 LoRA adapters"

            # Check adapter criticalities are restored
            assert adapters[0].get("criticality") == "Standard", (
                f"Adapter 0 criticality should be 'Standard', got: {adapters[0].get('criticality')}"
            )
            assert adapters[1].get("criticality") == "Sheddable", (
                f"Adapter 1 criticality should be 'Sheddable', got: {adapters[1].get('criticality')}"
            )

            return v1alpha1_isvc

        wait_for(assert_lora_criticalities_restored, timeout=30.0)
        print("✅ LoRA adapter criticalities restored when reading as v1alpha1")

    @pytest.mark.cluster_cpu
    @pytest.mark.cluster_single_node
    def test_round_trip_conversion_preserves_fields(self):
        """Test that all fields are preserved through a v1alpha1 -> v1alpha2 -> v1alpha1 round-trip."""
        service_name = "conv-test-round-trip"

        # Create a comprehensive config
        config_name = f"{service_name}-config"
        config = {
            "apiVersion": f"{constants.KSERVE_GROUP}/{constants.KSERVE_V1ALPHA1_VERSION}",
            "kind": "LLMInferenceServiceConfig",
            "metadata": {
                "name": config_name,
                "namespace": self.namespace,
            },
            "spec": {
                "router": {"route": {}, "gateway": {}},
                "template": {
                    "containers": [{
                        "name": "main",
                        "image": "quay.io/pierdipi/vllm-cpu:latest",
                        "resources": {
                            "limits": {"cpu": "2", "memory": "7Gi"},
                            "requests": {"cpu": ".5", "memory": "4Gi"},
                        },
                    }]
                },
            },
        }
        create_llmisvc_config_raw(self.kserve_client, config, constants.KSERVE_V1ALPHA1_VERSION)
        self.created_resources.append(("config", config_name, constants.KSERVE_V1ALPHA1_VERSION))

        # Create v1alpha1 LLMInferenceService with various fields
        original_annotations = {
            "user-annotation": "test-value",
        }
        original_labels = {
            "user-label": "test-label",
        }

        v1alpha1_isvc = {
            "apiVersion": f"{constants.KSERVE_GROUP}/{constants.KSERVE_V1ALPHA1_VERSION}",
            "kind": "LLMInferenceService",
            "metadata": {
                "name": service_name,
                "namespace": self.namespace,
                "annotations": original_annotations,
                "labels": original_labels,
            },
            "spec": {
                "model": {
                    "uri": "hf://facebook/opt-125m",
                    "name": "test-model",
                },
                "replicas": 1,
                "baseRefs": [{"name": config_name}],
            },
        }

        create_llmisvc_raw(self.kserve_client, v1alpha1_isvc, constants.KSERVE_V1ALPHA1_VERSION)
        self.created_resources.append(("llmisvc", service_name, constants.KSERVE_V1ALPHA1_VERSION))

        # Read as v1alpha2
        def get_as_v1alpha2():
            return get_llmisvc_raw(
                self.kserve_client, service_name, self.namespace, constants.KSERVE_V1ALPHA2_VERSION
            )

        v1alpha2_isvc = wait_for(get_as_v1alpha2, timeout=30.0)

        # Verify key fields in v1alpha2
        assert v1alpha2_isvc["spec"].get("replicas") == 1, "Replicas should be preserved"
        model_spec = v1alpha2_isvc["spec"].get("model", {})
        assert model_spec.get("name") == "test-model", "Model name should be preserved"

        # User annotations/labels should be preserved
        annotations = v1alpha2_isvc.get("metadata", {}).get("annotations", {})
        assert annotations.get("user-annotation") == "test-value", "User annotations should be preserved"
        labels = v1alpha2_isvc.get("metadata", {}).get("labels", {})
        assert labels.get("user-label") == "test-label", "User labels should be preserved"

        print("✅ Fields preserved when converting to v1alpha2")

        # Read back as v1alpha1
        def get_as_v1alpha1():
            return get_llmisvc_raw(
                self.kserve_client, service_name, self.namespace, constants.KSERVE_V1ALPHA1_VERSION
            )

        v1alpha1_result = wait_for(get_as_v1alpha1, timeout=30.0)

        # Verify all original fields are preserved
        assert v1alpha1_result["spec"].get("replicas") == 1, "Replicas should be preserved in round-trip"
        model_spec = v1alpha1_result["spec"].get("model", {})
        assert model_spec.get("name") == "test-model", "Model name should be preserved in round-trip"

        # User annotations/labels should still be there
        annotations = v1alpha1_result.get("metadata", {}).get("annotations", {})
        assert annotations.get("user-annotation") == "test-value", "User annotations should survive round-trip"
        labels = v1alpha1_result.get("metadata", {}).get("labels", {})
        assert labels.get("user-label") == "test-label", "User labels should survive round-trip"

        print("✅ All fields preserved through v1alpha1 -> v1alpha2 -> v1alpha1 round-trip")
