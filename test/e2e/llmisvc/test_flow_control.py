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

import os
import time

import pytest
import requests
from kserve import KServeClient
from kubernetes import client

from .fixtures import (
    create_inference_objectives,
    delete_inference_objectives,
    inject_k8s_proxy,
)
from .logging import logger
from .test_llm_inference_service import (
    TestCase,
    completions_payload,
    create_response_assertion,
    create_llmisvc,
    get_llm_service_url,
    maybe_delete_llmisvc,
    wait_for_llm_isvc_ready,
    wait_for_model_response,
)

FC_OBJECTIVES = [
    {"name": "fc-high-priority", "priority": 100},
    {"name": "fc-low-priority", "priority": -1},
]
FC_OBJECTIVE_NAMES = [o["name"] for o in FC_OBJECTIVES]


@pytest.mark.llminferenceservice
@pytest.mark.asyncio(loop_scope="session")
@pytest.mark.parametrize(
    "test_case",
    [
        pytest.param(
            TestCase(
                base_refs=[
                    "router-managed",
                    "scheduler-flow-control-round-robin",
                    "workload-llmd-simulator",
                ],
                prompt="KServe is a",
                service_name="fc-smoke-test",
                payload_formatter=completions_payload,
                response_assertion=create_response_assertion(with_field="choices"),
            ),
            marks=[
                pytest.mark.cluster_cpu,
                pytest.mark.cluster_single_node,
                pytest.mark.llmd_simulator,
                pytest.mark.flow_control,
            ],
            id="flow-control-utilization-detector",
        ),
        pytest.param(
            TestCase(
                base_refs=[
                    "router-managed",
                    "scheduler-flow-control-concurrency-detector",
                    "workload-llmd-simulator",
                ],
                prompt="KServe is a",
                service_name="fc-concurrency-test",
                payload_formatter=completions_payload,
                response_assertion=create_response_assertion(with_field="choices"),
            ),
            marks=[
                pytest.mark.cluster_cpu,
                pytest.mark.cluster_single_node,
                pytest.mark.llmd_simulator,
                pytest.mark.flow_control,
            ],
            id="flow-control-concurrency-detector",
        ),
    ],
    indirect=True,
)
def test_flow_control_smoke(test_case: TestCase, flow_control_auth):
    """Verify that the EPP boots and serves traffic with flow control enabled.

    Sends requests with different fairness IDs, InferenceObjective headers, and
    default headers. When a downstream auth provider is available (via the
    flow_control_auth fixture), also verifies the auth -> flow control pipeline.
    """
    inject_k8s_proxy()
    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config"),
        client_configuration=client.Configuration(),
    )

    service_name = test_case.llm_service.metadata.name
    prefix = test_case.log_prefix
    test_failed = False

    if flow_control_auth:
        if not test_case.llm_service.metadata.annotations:
            test_case.llm_service.metadata.annotations = {}
        test_case.llm_service.metadata.annotations.update(
            flow_control_auth.get("annotations", {})
        )

    try:
        print(f"{prefix} Creating LLMInferenceService {service_name}")
        create_llmisvc(kserve_client, test_case.llm_service)
        print(f"{prefix} Waiting for ready")
        wait_for_llm_isvc_ready(
            kserve_client, test_case.llm_service, test_case.wait_timeout
        )
        print(f"{prefix} Waiting for model response")
        wait_for_model_response(kserve_client, test_case, test_case.wait_timeout)

        url = get_llm_service_url(kserve_client, test_case.llm_service)
        payload = test_case.payload_formatter(test_case)

        for tenant in ["tenant-a", "tenant-b"]:
            print(f"{prefix} Sending request with fairness_id={tenant}")
            resp = requests.post(
                f"{url}/v1/completions",
                json=payload,
                headers={"x-gateway-inference-fairness-id": tenant},
                timeout=test_case.response_timeout,
            )
            assert resp.status_code == 200, (
                f"fairness_id={tenant} failed: {resp.status_code} {resp.text}"
            )

        pool_name = f"{service_name}-inference-pool"
        create_inference_objectives(pool_name, FC_OBJECTIVES)
        time.sleep(5)

        for obj in FC_OBJECTIVES:
            print(
                f"{prefix} Sending request with objective={obj['name']} "
                f"(priority={obj['priority']})"
            )
            resp = requests.post(
                f"{url}/v1/completions",
                json=payload,
                headers={"x-gateway-inference-objective": obj["name"]},
                timeout=test_case.response_timeout,
            )
            assert resp.status_code == 200, (
                f"objective={obj['name']} failed: {resp.status_code} {resp.text}"
            )

        if flow_control_auth:
            _verify_auth_pipeline(
                flow_control_auth, kserve_client, test_case, url, payload, prefix
            )

    except Exception as e:
        test_failed = True
        logger.error(f"{prefix} Failed: {e}")
        raise
    finally:
        if flow_control_auth and "cleanup" in flow_control_auth:
            flow_control_auth["cleanup"](kserve_client, service_name)
        delete_inference_objectives(FC_OBJECTIVE_NAMES)
        maybe_delete_llmisvc(kserve_client, test_case.llm_service, test_failed)


def _verify_auth_pipeline(auth, kserve_client, test_case, url, payload, prefix):
    service_name = test_case.llm_service.metadata.name
    token = auth["setup"](kserve_client, service_name)
    endpoint = f"{url}/v1/completions"

    print(f"{prefix} Verifying unauthenticated request is rejected")
    resp = requests.post(
        endpoint,
        json=payload,
        headers={"Content-Type": "application/json"},
        timeout=test_case.response_timeout,
    )
    assert resp.status_code in [401, 403], (
        f"Expected 401/403 without token, got {resp.status_code}"
    )
    print(f"{prefix} Unauthenticated rejected: {resp.status_code}")

    print(f"{prefix} Verifying authenticated request succeeds")
    resp = None
    for attempt in range(24):
        resp = requests.post(
            endpoint,
            json=payload,
            headers={
                "Content-Type": "application/json",
                "Authorization": f"Bearer {token}",
            },
            timeout=test_case.response_timeout,
        )
        if resp.status_code == 200:
            break
        if resp.status_code in [401, 403, 502, 503, 504]:
            logger.info(
                f"Attempt {attempt + 1}: got {resp.status_code}, "
                "waiting for auth/routing propagation..."
            )
            time.sleep(5)
        else:
            break
    assert resp.status_code == 200, (
        f"Expected 200 with token, got {resp.status_code}: {resp.text}"
    )
    print(f"{prefix} Authenticated request succeeded through flow control")
