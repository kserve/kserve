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

"""E2E tests for LLMInferenceService with LoRA adapters."""

import logging
import os
import pytest
from dataclasses import dataclass
from kserve import KServeClient, V1alpha1LLMInferenceService, constants
from kubernetes import client
from typing import List, Optional

from .diagnostic import print_all_events_table
from .fixtures import (
    KSERVE_TEST_NAMESPACE,
    LLMINFERENCESERVICE_CONFIGS,
    _create_or_update_llmisvc_config,
    _get_model_name_from_configs,
    generate_k8s_safe_suffix,
    generate_test_id,
    inject_k8s_proxy,
)
from .logging import log_execution
from .test_llm_inference_service import (
    assert_200_with_choices,
    create_llmisvc,
    delete_llmisvc,
    get_llm_service_url,
    wait_for_llm_isvc_ready,
)
from ..common.http_retry import post_with_retry

KSERVE_PLURAL_LLMINFERENCESERVICE = "llminferenceservices"
KSERVE_PLURAL_LLMINFERENCESERVICECONFIG = "llminferenceserviceconfigs"

logger = logging.getLogger(__name__)


@dataclass
class LoRATestCase:
    """Test case configuration for LoRA adapter tests."""

    __test__ = False  # So pytest will not try to execute it
    base_refs: List[str]
    prompt: str
    expected_adapter_names: List[str]
    service_name: Optional[str] = None
    endpoint: str = "/v1/completions"
    max_tokens: int = 100
    wait_timeout: int = 900
    response_timeout: int = 60


def build_llm_service_from_refs(
    service_name: str, base_refs: List[str]
) -> V1alpha1LLMInferenceService:
    """Build an LLMInferenceService from base reference configs."""
    return V1alpha1LLMInferenceService(
        api_version="serving.kserve.io/v1alpha1",
        kind="LLMInferenceService",
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec={"baseRefs": [{"name": ref} for ref in base_refs]},
    )


def run_lora_test(test_case: LoRATestCase):
    """Execute a LoRA adapter test case."""
    inject_k8s_proxy()
    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config"),
        client_configuration=client.Configuration(),
    )

    test_id = generate_test_id(test_case)
    service_name = test_case.service_name or f"lora-test-{test_id}"
    model_name = _get_model_name_from_configs(test_case.base_refs)
    created_configs = []
    llm_service = None

    try:
        # Create unique LLMInferenceServiceConfig resources for each base ref
        unique_base_refs = []
        for base_ref in test_case.base_refs:
            if base_ref not in LLMINFERENCESERVICE_CONFIGS:
                raise ValueError(f"Unknown base reference: {base_ref}")

            # Generate unique config name to avoid conflicts in parallel test runs
            unique_config_name = generate_k8s_safe_suffix(base_ref, [service_name])
            unique_base_refs.append(unique_config_name)

            config = LLMINFERENCESERVICE_CONFIGS[base_ref]
            config_body = {
                "apiVersion": "serving.kserve.io/v1alpha1",
                "kind": "LLMInferenceServiceConfig",
                "metadata": {
                    "name": unique_config_name,
                    "namespace": KSERVE_TEST_NAMESPACE,
                },
                "spec": config,
            }

            logger.info("Creating LLMInferenceServiceConfig: %s", unique_config_name)
            _create_or_update_llmisvc_config(
                kserve_client, config_body, KSERVE_TEST_NAMESPACE
            )
            created_configs.append(unique_config_name)

        # Create the service with unique base refs
        llm_service = build_llm_service_from_refs(service_name, unique_base_refs)
        logger.info("Creating LLMInferenceService: %s", service_name)
        create_llmisvc(kserve_client, llm_service)

        # Wait for service to be ready
        logger.info("Waiting for service %s to be ready...", service_name)
        wait_for_llm_isvc_ready(
            kserve_client, llm_service, timeout_seconds=test_case.wait_timeout
        )

        # Get inference URL
        base_url = get_llm_service_url(kserve_client, llm_service)
        inference_url = base_url + test_case.endpoint

        # Test base model inference
        logger.info("Testing base model inference...")
        base_payload = {
            "model": model_name,
            "prompt": test_case.prompt,
            "max_tokens": test_case.max_tokens,
        }

        base_response = post_with_retry(
            inference_url,
            json_data=base_payload,
            timeout=test_case.response_timeout,
        )

        assert_200_with_choices(base_response)
        logger.info("✓ Base model inference successful")

        # Test LoRA adapter inference
        for adapter_name in test_case.expected_adapter_names:
            logger.info("Testing LoRA adapter inference: %s", adapter_name)
            lora_payload = {
                "model": adapter_name,
                "prompt": test_case.prompt,
                "max_tokens": test_case.max_tokens,
            }

            lora_response = post_with_retry(
                inference_url,
                json_data=lora_payload,
                timeout=test_case.response_timeout,
            )

            assert_200_with_choices(lora_response)
            logger.info("✓ LoRA adapter %s inference successful", adapter_name)

    finally:
        # Cleanup service
        if llm_service is not None:
            try:
                logger.info("Cleaning up service %s", service_name)
                delete_llmisvc(kserve_client, llm_service)
            except Exception as e:
                logger.warning(f"Service cleanup failed: {e}")

        # Cleanup configs
        for config_name in created_configs:
            try:
                logger.info("Cleaning up config %s", config_name)
                kserve_client.api_instance.delete_namespaced_custom_object(
                    constants.KSERVE_GROUP,
                    constants.KSERVE_V1ALPHA1_VERSION,
                    KSERVE_TEST_NAMESPACE,
                    KSERVE_PLURAL_LLMINFERENCESERVICECONFIG,
                    config_name,
                )
            except Exception as e:
                logger.warning(f"Config cleanup failed for {config_name}: {e}")

        # Print diagnostics
        print_all_events_table(KSERVE_TEST_NAMESPACE)


@pytest.mark.parametrize(
    "test_case",
    [
        pytest.param(
            LoRATestCase(
                base_refs=[
                    "router-no-scheduler",
                    "workload-single-cpu",
                    "model-fb-opt-125m-with-lora-hf",
                ],
                prompt="What is Kubernetes?",
                expected_adapter_names=["lora-adapter-1"],
                service_name="lora-single-adapter-test",
            ),
            marks=[
                pytest.mark.llminferenceservice,
                pytest.mark.cluster_cpu,
                pytest.mark.lora,
            ],
            id="single-lora-adapter-hf",
        ),
        pytest.param(
            LoRATestCase(
                base_refs=[
                    "router-no-scheduler",
                    "workload-single-cpu",
                    "model-fb-opt-125m-with-multiple-lora",
                ],
                prompt="Explain machine learning in simple terms.",
                expected_adapter_names=["lora-adapter-1", "lora-adapter-2"],
                service_name="lora-multiple-adapters-test",
            ),
            marks=[
                pytest.mark.llminferenceservice,
                pytest.mark.cluster_cpu,
                pytest.mark.lora,
            ],
            id="multiple-lora-adapters",
        ),
    ],
)
@log_execution
def test_llm_with_lora_adapters(test_case: LoRATestCase):
    """Test LLMInferenceService with LoRA adapters."""
    run_lora_test(test_case)
