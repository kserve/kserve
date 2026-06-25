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

"""Test that the x-served-by response header is emitted when opted in via annotation."""

from __future__ import annotations

import os
import pytest
import requests
from kserve import KServeClient
from kubernetes import client

from .fixtures import (
    generate_test_id,
    inject_k8s_proxy,
)
from .logging import log_execution, logger
from .diagnostic import collect_diagnostics
from .test_llm_inference_service import (
    TestCase,
    create_llmisvc,
    wait_for_llm_isvc_ready,
    wait_for_model_response,
    maybe_delete_llmisvc,
)


def assert_x_served_by(service_name: str):
    """Create a response assertion that checks for x-served-by header."""

    def assertion(response: requests.Response) -> None:
        served_by = response.headers.get("x-served-by")
        assert served_by is not None, (
            f"Expected x-served-by header, got headers: {dict(response.headers)}"
        )
        assert served_by == service_name, (
            f"Expected x-served-by: {service_name}, got: {served_by}"
        )

    return assertion


@pytest.mark.llminferenceservice
@pytest.mark.asyncio(loop_scope="session")
@pytest.mark.parametrize(
    "test_case",
    [
        pytest.param(
            TestCase(
                base_refs=[
                    "router-managed",
                    "workload-single-cpu",
                    "model-fb-opt-125m",
                ],
                prompt="KServe is a",
                service_name="x-served-by-test",
            ),
            marks=[pytest.mark.cluster_cpu, pytest.mark.cluster_single_node],
        ),
    ],
    indirect=["test_case"],
    ids=generate_test_id,
)
@log_execution
def test_x_served_by_header(test_case: TestCase):
    """Test that an LLMISVC with enable-served-by-header annotation includes x-served-by response header."""
    inject_k8s_proxy()

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config"),
        client_configuration=client.Configuration(),
    )

    llm_service = test_case.llm_service
    service_name = llm_service.metadata.name
    test_failed = False

    # Set the annotation before creating the service so the first deployment
    # already has the middleware injected - no rollout needed.
    if not llm_service.metadata.annotations:
        llm_service.metadata.annotations = {}
    llm_service.metadata.annotations["serving.kserve.io/enable-served-by-header"] = (
        "true"
    )

    try:
        logger.info(f"Creating LLMInferenceService {service_name}")
        create_llmisvc(kserve_client, llm_service)

        logger.info(f"Waiting for LLMInferenceService {service_name} to be ready")
        wait_for_llm_isvc_ready(kserve_client, llm_service, test_case.wait_timeout)

        # Override response assertion to check x-served-by header.
        # wait_for_model_response retries until 200, then runs the assertion once.
        test_case.response_assertion = assert_x_served_by(service_name)

        logger.info(
            f"Waiting for model response with x-served-by header from {service_name}"
        )
        wait_for_model_response(kserve_client, test_case)

    except Exception as e:
        test_failed = True
        logger.error(f"❌ ERROR: x-served-by test failed for {service_name}: {e}")
        collect_diagnostics(
            service_name,
            llm_service.metadata.namespace,
            kserve_client=kserve_client,
            log=logger.info,
        )
        raise
    finally:
        maybe_delete_llmisvc(kserve_client, llm_service, test_failed)
