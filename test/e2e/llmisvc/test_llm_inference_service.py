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

import time
from urllib.parse import urlparse

import os
import pytest
import requests
import yaml
from dataclasses import dataclass, field
from kserve import KServeClient, V1alpha1LLMInferenceService, constants
from kubernetes import client
from typing import Any, Callable, Dict, List, Optional

from .diagnostic import (
    collect_pod_logs,
    kinds_matching_by_labels,
    print_all_events_table,
    strip_managed_fields,
)
from .fixtures import (
    KSERVE_TEST_NAMESPACE,
    create_router_resources,
    create_scheduler_configmap,
    delete_scheduler_configmap,
    ensure_pvc_with_model,
    generate_test_id,
    inject_k8s_proxy,
)
from .test_resources import (
    ROUTER_GATEWAYS,
    ROUTER_ROUTES,
)
from .logging import log_execution, logger
from ..common.http_retry import get_with_retry, post_with_retry

KSERVE_PLURAL_LLMINFERENCESERVICE = "llminferenceservices"


def assert_200(response: requests.Response) -> None:
    """Default response assertion that checks for 200 status code."""
    assert (
        response.status_code == 200
    ), f"Service returned {response.status_code}: {response.text}"


def assert_200_with_choices(response: requests.Response) -> None:
    """Assert 200 status code with choices in response."""
    assert (
        response.status_code == 200
        and response.json().get("choices") is not None
        and len(response.json().get("choices", [])) > 0
    ), f"Expected 200 with choices, got {response.status_code}: {response.text}"


def create_response_assertion(
    status_code: int = 200, with_field: str = ""
) -> Callable[[requests.Response], None]:
    """Factory for creating flexible response assertions with arbitrary status codes and field checks."""

    def response_assertion(response: requests.Response) -> None:
        assert (
            response.status_code == status_code
        ), f"Expected status code {status_code}, but service returned {response.status_code}: {response.text}"
        if with_field:
            body = response.json()
            field_value = body.get(with_field)
            assert (
                field_value is not None and len(field_value) > 0
            ), f"Expected response body to contain non empty field '{with_field}': {response.text}"

    return response_assertion


def assert_model_field_matches(
    expected_model: str,
) -> Callable[[requests.Response], None]:
    """Assert 200 with choices and response model field matching expected_model."""

    def response_assertion(response: requests.Response) -> None:
        assert (
            response.status_code == 200
        ), f"Expected 200, got {response.status_code}: {response.text}"
        body = response.json()
        choices = body.get("choices")
        assert (
            choices and len(choices) > 0
        ), f"Expected non-empty choices: {response.text}"
        got_model = body.get("model", "")
        assert (
            got_model == expected_model
        ), f"Expected model {expected_model!r}, got {got_model!r}"

    return response_assertion


def assert_models_contains(*model_ids: str) -> Callable[[requests.Response], None]:
    """Assert 200 with data[] containing entries whose ids match all given model_ids."""

    def response_assertion(response: requests.Response) -> None:
        assert (
            response.status_code == 200
        ), f"Expected 200, got {response.status_code}: {response.text}"
        body = response.json()
        data = body.get("data", [])
        assert data, f"Expected non-empty data[], got: {response.text}"
        ids = [m.get("id") for m in data]
        for model_id in model_ids:
            assert (
                model_id in ids
            ), f"Expected model {model_id!r} in data[].id, found: {ids}"

    return response_assertion


MODEL_ROUTING_ADDRESS_SUFFIX = "-model-routing"
MODEL_ROUTING_HEADER = "X-Gateway-Model-Name"


@log_execution
def get_model_routing_url(
    kserve_client: KServeClient, llm_isvc: V1alpha1LLMInferenceService
):
    """Get the model-routing base URL from status.addresses.

    Model-routing addresses are identified by their name ending with
    "-model-routing" (set by AddressTypeName in the controller). As a
    secondary check, the URL path must not start with /{ns}/{name}
    (the path-based prefix).
    """
    service_name = llm_isvc.metadata.name

    try:
        llm_isvc_status = get_llmisvc(
            kserve_client,
            llm_isvc.metadata.name,
            llm_isvc.metadata.namespace,
            llm_isvc.api_version.split("/")[1],
        )

        status = llm_isvc_status.get("status", {})
        addresses = status.get("addresses", [])

        if not addresses:
            raise ValueError(
                f"❌ No addresses found in LLM inference service {service_name} status"
            )

        namespace = llm_isvc.metadata.namespace
        path_based_prefix = f"/{namespace}/{service_name}"
        other_entries = []
        for addr in addresses:
            url = addr.get("url", "")
            name = addr.get("name", "")
            if not url:
                continue

            if not name.endswith(MODEL_ROUTING_ADDRESS_SUFFIX):
                other_entries.append(f"{name}={url}")
                continue

            parsed = urlparse(url)
            path = parsed.path.rstrip("/")
            if path.startswith(path_based_prefix):
                raise ValueError(
                    f"❌ Address {name!r} has model-routing suffix but its path "
                    f"{parsed.path!r} starts with path-based prefix {path_based_prefix!r}"
                )

            logger.info(
                f"Found model-routing URL for {service_name}: {url} "
                f"(name={name!r}, path={parsed.path!r})"
            )
            return url.rstrip("/")

        raise ValueError(
            f"❌ No model-routing URL found for {service_name}. "
            f"Addresses without '{MODEL_ROUTING_ADDRESS_SUFFIX}' suffix: {other_entries}"
        )

    except Exception as e:
        raise ValueError(
            f"❌ Failed to get model-routing URL for LLM inference service {service_name}: {e}"
        ) from e


@dataclass
class TestCase:
    """Test case configuration for LLM inference service tests."""

    __test__ = False  # So pytest will not try to execute it.
    base_refs: List[str]
    prompt: Optional[str] = None
    service_name: Optional[str] = None
    endpoint: str = "/v1/completions"
    max_tokens: int = 20
    payload_formatter: Optional[Callable[["TestCase"], Dict[str, Any]]] = None
    response_assertion: Callable[[requests.Response], None] = assert_200
    wait_timeout: int = 900
    response_timeout: int = 60
    extra_headers: Optional[Dict[str, str]] = None
    url_getter: Optional[Callable] = None
    expected_gateway: Optional[Dict[str, Any]] = None
    before_test: List[Callable[[], Any]] = field(default_factory=list)
    after_test: List[Callable[[], Any]] = field(default_factory=list)
    peers: List["TestCase"] = field(default_factory=list)
    response_assertion_factory: Optional[
        Callable[[str], Callable[[requests.Response], None]]
    ] = None
    # Factory provided
    llm_service: V1alpha1LLMInferenceService = None  # Generated by llm_service_factory
    model_name: str = "default/model"  # This will be generated by the factory

    @property
    def log_prefix(self) -> str:
        return f"[{'-'.join(self.base_refs)}]"


def completions_payload(test_case: TestCase) -> Dict[str, Any]:
    """Payload formatter for the /v1/completions endpoint."""
    return {
        "model": test_case.model_name,
        "prompt": test_case.prompt,
        "max_tokens": test_case.max_tokens,
    }


def chat_completions_payload(test_case: TestCase) -> Dict[str, Any]:
    """Payload formatter for the /v1/chat/completions endpoint."""
    return {
        "model": test_case.model_name,
        "messages": [{"role": "user", "content": test_case.prompt}],
        "max_tokens": test_case.max_tokens,
    }


@pytest.mark.llminferenceservice
@pytest.mark.asyncio(loop_scope="session")
@pytest.mark.parametrize(
    "test_case",
    [
        pytest.param(
            TestCase(
                base_refs=[
                    "router-with-gateway-ref",
                    "router-with-managed-route",
                    "model-fb-opt-125m",
                    "workload-llmd-simulator",
                ],
                endpoint="/v1/completions",
                prompt="KServe is a",
                payload_formatter=completions_payload,
                response_assertion=create_response_assertion(with_field="choices"),
                expected_gateway=ROUTER_GATEWAYS[0],
                before_test=[
                    lambda: create_router_resources(
                        gateways=[ROUTER_GATEWAYS[0]],
                    )
                ],
            ),
            marks=[
                pytest.mark.cluster_cpu,
                pytest.mark.cluster_single_node,
                pytest.mark.llmd_simulator,
                pytest.mark.custom_gateway,
            ],
        ),
        pytest.param(
            TestCase(
                base_refs=[
                    "router-managed",
                    "workload-single-cpu",
                    "model-fb-opt-125m",
                ],
                prompt="KServe is a",
                payload_formatter=completions_payload,
                response_assertion=assert_200_with_choices,
            ),
            marks=[pytest.mark.cluster_cpu, pytest.mark.cluster_single_node],
        ),
        pytest.param(
            TestCase(
                base_refs=[
                    "router-custom-route-timeout",
                    "scheduler-managed",
                    "workload-single-cpu",
                    "model-fb-opt-125m",
                ],
                prompt="KServe is a",
                service_name="custom-route-timeout-test",
            ),
            marks=[pytest.mark.cluster_cpu, pytest.mark.cluster_single_node],
        ),
        pytest.param(
            TestCase(
                base_refs=[
                    "router-with-refs",
                    "scheduler-managed",
                    "workload-single-cpu",
                    "model-fb-opt-125m",
                ],
                prompt="KServe is a",
                service_name="router-with-refs-test",
                expected_gateway=ROUTER_GATEWAYS[0],
                before_test=[
                    lambda: create_router_resources(
                        gateways=[ROUTER_GATEWAYS[0]],
                        routes=[ROUTER_ROUTES[0], ROUTER_ROUTES[1]],
                    )
                ],
            ),
            marks=[
                pytest.mark.cluster_cpu,
                pytest.mark.cluster_single_node,
                pytest.mark.custom_gateway,
            ],
        ),
        pytest.param(
            TestCase(
                base_refs=["router-managed", "workload-pd-cpu", "model-fb-opt-125m"],
                prompt="You are an expert in Kubernetes-native machine learning serving platforms, with deep knowledge of the KServe project. "
                "Explain the challenges of serving large-scale models, GPU scheduling, and how KServe integrates with capabilities like multi-model serving. "
                "Provide a detailed comparison with open source alternatives, focusing on operational trade-offs.",
                response_assertion=assert_200_with_choices,
            ),
            marks=[pytest.mark.cluster_cpu, pytest.mark.cluster_single_node],
        ),
        pytest.param(
            TestCase(
                base_refs=[
                    "router-custom-route-timeout-pd",
                    "scheduler-managed",
                    "workload-pd-cpu",
                    "model-fb-opt-125m",
                ],
                prompt="You are an expert in Kubernetes-native machine learning serving platforms, with deep knowledge of the KServe project. "
                "Explain the challenges of serving large-scale models, GPU scheduling, and how KServe integrates with capabilities like multi-model serving. "
                "Provide a detailed comparison with open source alternatives, focusing on operational trade-offs.",
                service_name="custom-route-timeout-pd-test",
                response_assertion=assert_200_with_choices,
            ),
            marks=[pytest.mark.cluster_cpu, pytest.mark.cluster_single_node],
        ),
        pytest.param(
            TestCase(
                base_refs=[
                    "router-with-refs-pd",
                    "scheduler-managed",
                    "workload-pd-cpu",
                    "model-fb-opt-125m",
                ],
                prompt="You are an expert in Kubernetes-native machine learning serving platforms, with deep knowledge of the KServe project. "
                "Explain the challenges of serving large-scale models, GPU scheduling, and how KServe integrates with capabilities like multi-model serving. "
                "Provide a detailed comparison with open source alternatives, focusing on operational trade-offs.",
                service_name="router-with-refs-pd-test",
                response_assertion=assert_200_with_choices,
                expected_gateway=ROUTER_GATEWAYS[1],
                before_test=[
                    lambda: create_router_resources(
                        gateways=[ROUTER_GATEWAYS[1]],
                        routes=[ROUTER_ROUTES[2], ROUTER_ROUTES[3]],
                    )
                ],
            ),
            marks=[
                pytest.mark.cluster_cpu,
                pytest.mark.cluster_single_node,
                pytest.mark.custom_gateway,
            ],
        ),
        pytest.param(
            TestCase(
                base_refs=[
                    "router-managed",
                    "workload-dp-ep-gpu",
                    "workload-dp-ep-prefill-gpu",
                    "model-deepseek-v2-lite",
                ],
                prompt="Delve into the multifaceted implications of a fully disaggregated cloud architecture, specifically "
                "where the compute plane (P) and the data plane (D) are independently deployed and managed for a "
                "geographically distributed, high-throughput, low-latency microservices ecosystem. Beyond the "
                "fundamental challenges of network latency and data consistency, elaborate on the advanced "
                "considerations and trade-offs inherent in such a setup: 1. Network Architecture and Protocols: "
                "How would the network fabric and underlying protocols (e.g., RDMA, custom transport layers) need to "
                "evolve to support optimal performance and minimize inter-plane communication overhead, especially for "
                "synchronous operations? Discuss the role of network programmability (e.g., SDN, P4) in dynamically "
                "optimizing routing and traffic flow between P and D. 2. Advanced Data Consistency and Durability: "
                "Explore sophisticated data consistency models (e.g., causal consistency, strong eventual consistency) "
                "and their applicability in balancing performance and data integrity across a globally distributed data plane. "
                "Detail strategies for ensuring data durability and fault tolerance, including multi-region replication, "
                "intelligent partitioning, and recovery mechanisms in the event of partial or full plane failures. "
                "3. Dynamic Resource Orchestration and Cost Optimization: Analyze how an orchestration layer would intelligently "
                "manage the independent scaling of compute (P) and data (D) resources, considering fluctuating workloads, "
                "cost efficiency, and performance targets (e.g., using predictive analytics for resource provisioning). "
                "Discuss mechanisms for dynamically reallocating compute nodes to different data partitions based on "
                "workload patterns and data locality, potentially involving live migration strategies. "
                "4. Security and Compliance in a Distributed Landscape: Address the enhanced security perimeter "
                "challenges, including securing communication channels between P and D (encryption in transit, mutual TLS), "
                "fine-grained access control to data at rest and in motion, and identity management across disaggregated "
                "components. Discuss how such an architecture impacts compliance with regulatory frameworks (e.g., GDPR, HIPAA) "
                "concerning data sovereignty, privacy, and auditability. 5. Operational Complexity and Observability: "
                "Examine the increased complexity in monitoring, logging, and tracing across highly decoupled compute and "
                "data planes. What specialized tooling and practices (e.g., distributed tracing with OpenTelemetry, advanced AIOps) "
                "would be essential? How would incident response and troubleshooting differ in this disaggregated environment "
                "compared to traditional integrated systems? Consider the challenges of pinpointing root causes across "
                "independent failures. 6. Real-world Applicability and Future Trends: Identify specific industries "
                "or use cases (e.g., high-frequency trading, IoT edge processing, large language model inference) "
                "where the benefits of P/D disaggregation would strongly outweigh its complexities. "
                "Conclude by speculating on emerging technologies or paradigms (e.g., serverless compute functions "
                "directly interacting with object storage, in-memory disaggregation) that could further drive or "
                "transform P/D disaggregation in cloud computing.",
                max_tokens=2000,
            ),
            marks=[
                pytest.mark.cluster_gpu,
                pytest.mark.cluster_nvidia,
                pytest.mark.cluster_nvidia_roce,
            ],
        ),
        pytest.param(
            TestCase(
                base_refs=[
                    "router-no-scheduler",
                    "workload-single-cpu",
                    "model-fb-opt-125m",
                ],
                prompt="What is KServe?",
            ),
            marks=[
                pytest.mark.cluster_cpu,
                pytest.mark.cluster_single_node,
                pytest.mark.no_scheduler,
            ],
        ),
        pytest.param(
            TestCase(
                base_refs=[
                    "router-managed",
                    "workload-simulated-dp-ep-cpu",
                    "model-fb-opt-125m",
                ],
                prompt="This test simulates DP+EP that can run on CPU, the idea is to test the LWS-based deployment, "
                "but without the resources requirements for DP+EP (GPUs and ROCe/IB).",
            ),
            marks=[pytest.mark.cluster_cpu, pytest.mark.cluster_multi_node],
        ),
        # Scheduler config tests
        pytest.param(
            TestCase(
                base_refs=[
                    "router-managed",
                    "scheduler-with-inline-config",
                    "workload-llmd-simulator",
                ],
                prompt="KServe is a",
                service_name="scheduler-inline-config-test",
            ),
            marks=[pytest.mark.cluster_cpu, pytest.mark.cluster_single_node],
        ),
        # Chat completions endpoint coverage
        pytest.param(
            TestCase(
                base_refs=[
                    "router-managed",
                    "workload-llmd-simulator",
                    "model-qwen2.5-0.5b",
                ],
                model_name="Qwen/Qwen2.5-0.5B-Instruct",
                endpoint="/v1/chat/completions",
                prompt="What is KServe?",
                payload_formatter=chat_completions_payload,
                response_assertion=create_response_assertion(with_field="choices"),
            ),
            marks=[
                pytest.mark.cluster_cpu,
                pytest.mark.cluster_single_node,
                pytest.mark.llmd_simulator,
            ],
        ),
        pytest.param(
            TestCase(
                base_refs=[
                    "router-managed",
                    "scheduler-with-configmap-ref",
                    "workload-llmd-simulator",
                ],
                prompt="KServe is a",
                service_name="scheduler-configmap-ref-test",
                before_test=[create_scheduler_configmap],
                after_test=[delete_scheduler_configmap],
            ),
            marks=[pytest.mark.cluster_cpu, pytest.mark.cluster_single_node],
        ),
        pytest.param(
            TestCase(
                base_refs=[
                    "router-managed",
                    "scheduler-with-replicas",
                    "workload-llmd-simulator",
                ],
                prompt="KServe is a",
                service_name="scheduler-ha-replicas-test",
            ),
            marks=[pytest.mark.cluster_cpu, pytest.mark.cluster_single_node],
        ),
        pytest.param(
            TestCase(
                base_refs=[
                    "router-managed",
                    "scheduler-with-custom-template",
                    "workload-llmd-simulator",
                ],
                prompt="KServe is a",
                service_name="scheduler-custom-template-test",
            ),
            marks=[pytest.mark.cluster_cpu, pytest.mark.cluster_single_node],
        ),
        # Scheduler v0.6 → v0.7 migration tests.
        # Deploy v0.6-style configs and verify the controller migrates them
        # so the v0.7 scheduler boots successfully.
        pytest.param(
            TestCase(
                base_refs=[
                    "router-managed",
                    "scheduler-v06-pd-config-migration",
                    "workload-llmd-simulator-pd",
                ],
                prompt="KServe is a",
                service_name="scheduler-v06-pd-migration-test",
                response_assertion=assert_200_with_choices,
            ),
            marks=[pytest.mark.cluster_cpu, pytest.mark.cluster_single_node],
        ),
        pytest.param(
            TestCase(
                base_refs=[
                    "router-managed",
                    "scheduler-v06-nonzero-threshold-migration",
                    "workload-llmd-simulator-pd",
                ],
                prompt="KServe is a",
                service_name="scheduler-v06-threshold-migration-test",
                response_assertion=assert_200_with_choices,
            ),
            marks=[pytest.mark.cluster_cpu, pytest.mark.cluster_single_node],
        ),
        # Precise prefix KV cache routing test
        pytest.param(
            TestCase(
                base_refs=[
                    "router-managed",
                    "scheduler-with-precise-prefix-cache-inline-config",
                    "workload-llmd-simulator-kvcache",
                ],
                prompt="KServe is a",
                service_name="precise-prefix-cache-test",
            ),
            marks=[
                pytest.mark.cluster_cpu,
                pytest.mark.cluster_single_node,
                pytest.mark.llmd_simulator,
            ],
        ),
        # Models endpoint coverage
        pytest.param(
            TestCase(
                base_refs=[
                    "router-managed",
                    "workload-llmd-simulator",
                ],
                endpoint="/v1/models",
                response_assertion=create_response_assertion(with_field="data"),
            ),
            marks=[
                pytest.mark.cluster_cpu,
                pytest.mark.cluster_single_node,
                pytest.mark.llmd_simulator,
            ],
        ),
        # Model-based routing via X-Gateway-Model-Name header — /v1/completions
        pytest.param(
            TestCase(
                base_refs=[
                    "router-managed",
                    "workload-llmd-simulator",
                ],
                endpoint="/v1/completions",
                prompt="KServe is a",
                payload_formatter=completions_payload,
                response_assertion_factory=lambda m: assert_model_field_matches(m),
                url_getter=get_model_routing_url,
                peers=[
                    TestCase(
                        base_refs=[
                            "router-managed",
                            "workload-llmd-simulator",
                            "model-qwen2.5-0.5b",
                        ],
                        endpoint="/v1/completions",
                        prompt="KServe is a",
                        payload_formatter=completions_payload,
                        response_assertion_factory=lambda m: assert_model_field_matches(
                            m
                        ),
                        url_getter=get_model_routing_url,
                    ),
                ],
            ),
            marks=[
                pytest.mark.cluster_cpu,
                pytest.mark.cluster_single_node,
                pytest.mark.llmd_simulator,
                pytest.mark.model_routing,
            ],
        ),
        # Model-based routing via X-Gateway-Model-Name header — /v1/chat/completions
        pytest.param(
            TestCase(
                base_refs=[
                    "router-managed",
                    "workload-llmd-simulator",
                ],
                endpoint="/v1/chat/completions",
                prompt="What is KServe?",
                payload_formatter=chat_completions_payload,
                response_assertion_factory=lambda m: assert_model_field_matches(m),
                url_getter=get_model_routing_url,
                peers=[
                    TestCase(
                        base_refs=[
                            "router-managed",
                            "workload-llmd-simulator",
                            "model-qwen2.5-0.5b",
                        ],
                        endpoint="/v1/chat/completions",
                        prompt="What is KServe?",
                        payload_formatter=chat_completions_payload,
                        response_assertion_factory=lambda m: assert_model_field_matches(
                            m
                        ),
                        url_getter=get_model_routing_url,
                    ),
                ],
            ),
            marks=[
                pytest.mark.cluster_cpu,
                pytest.mark.cluster_single_node,
                pytest.mark.llmd_simulator,
                pytest.mark.model_routing,
            ],
        ),
        # Model-based routing via X-Gateway-Model-Name header — LoRA adapter
        pytest.param(
            TestCase(
                base_refs=[
                    "router-managed",
                    "workload-single-cpu",
                    "model-fb-opt-125m-with-lora-hf",
                ],
                endpoint="/v1/completions",
                prompt="KServe is a",
                model_name=f"publishers/{KSERVE_TEST_NAMESPACE}/models/lora-adapter-1",
                payload_formatter=completions_payload,
                response_assertion=assert_model_field_matches(
                    f"publishers/{KSERVE_TEST_NAMESPACE}/models/lora-adapter-1"
                ),
                url_getter=get_model_routing_url,
                extra_headers={
                    MODEL_ROUTING_HEADER: f"publishers/{KSERVE_TEST_NAMESPACE}/models/lora-adapter-1",
                },
            ),
            marks=[
                pytest.mark.cluster_cpu,
                pytest.mark.cluster_single_node,
                pytest.mark.model_routing,
                pytest.mark.lora,
            ],
        ),
        # Model-based routing via X-Gateway-Model-Name header — /v1/models (base + LoRA)
        pytest.param(
            TestCase(
                base_refs=[
                    "router-managed",
                    "workload-single-cpu",
                    "model-fb-opt-125m-with-lora-hf",
                ],
                endpoint="/v1/models",
                response_assertion_factory=lambda m: assert_models_contains(
                    m,
                    f"publishers/{KSERVE_TEST_NAMESPACE}/models/{m}",
                    "lora-adapter-1",
                    f"publishers/{KSERVE_TEST_NAMESPACE}/models/lora-adapter-1",
                ),
                url_getter=get_model_routing_url,
            ),
            marks=[
                pytest.mark.cluster_cpu,
                pytest.mark.cluster_single_node,
                pytest.mark.model_routing,
                pytest.mark.lora,
            ],
        ),
        # PVC storage tests -- validate direct PVC volume mount with real vLLM serving
        pytest.param(
            TestCase(
                base_refs=[
                    "router-managed",
                    "workload-single-cpu",
                    "model-pvc",
                ],
                prompt="KServe is a",
                response_assertion=assert_200_with_choices,
                before_test=[ensure_pvc_with_model],
            ),
            marks=[
                pytest.mark.cluster_cpu,
                pytest.mark.cluster_single_node,
                pytest.mark.pvc_storage,
            ],
        ),
        pytest.param(
            TestCase(
                base_refs=[
                    "router-managed",
                    "workload-pd-cpu",
                    "model-pvc",
                ],
                prompt="KServe is a",
                response_assertion=assert_200_with_choices,
                before_test=[ensure_pvc_with_model],
            ),
            marks=[
                pytest.mark.cluster_cpu,
                pytest.mark.cluster_single_node,
                pytest.mark.pvc_storage,
            ],
        ),
        pytest.param(
            TestCase(
                base_refs=[
                    "router-managed",
                    "workload-simulated-dp-ep-cpu",
                    "model-pvc",
                ],
                prompt="KServe is a",
                before_test=[ensure_pvc_with_model],
            ),
            marks=[
                pytest.mark.cluster_cpu,
                pytest.mark.cluster_multi_node,
                pytest.mark.pvc_storage,
            ],
        ),
    ],
    indirect=["test_case"],
    ids=generate_test_id,
)
@log_execution
def test_llm_inference_service(test_case: TestCase):  # noqa: F811
    inject_k8s_proxy()

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config"),
        client_configuration=client.Configuration(),
    )

    service_name = test_case.llm_service.metadata.name
    prefix = test_case.log_prefix

    test_failed = False
    try:
        print(f"{prefix} Creating LLMInferenceService {service_name}")
        create_llmisvc(kserve_client, test_case.llm_service)
        print(f"{prefix} Waiting for LLMInferenceService {service_name} to be ready")
        wait_for_llm_isvc_ready(
            kserve_client, test_case.llm_service, test_case.wait_timeout
        )
        print(f"{prefix} Waiting for model response from {service_name}")
        wait_for_model_response(
            kserve_client,
            test_case,
            test_case.wait_timeout,
            extra_headers=test_case.extra_headers,
        )

        for peer in test_case.peers:
            test_llm_inference_service(peer)
        assert_address_origins(
            kserve_client, test_case.llm_service, test_case.expected_gateway
        )
        assert_address_models(kserve_client, test_case.llm_service)
    except Exception as e:
        test_failed = True
        logger.error(
            f"{prefix} ❌ ERROR: Failed to call llm inference service %s: %s",
            service_name,
            e,
        )
        _collect_diagnostics(kserve_client, test_case.llm_service)
        raise
    finally:
        maybe_delete_llmisvc(kserve_client, test_case.llm_service, test_failed)


@log_execution
def create_llmisvc(kserve_client: KServeClient, llm_isvc: V1alpha1LLMInferenceService):
    try:
        outputs = kserve_client.api_instance.create_namespaced_custom_object(
            constants.KSERVE_GROUP,
            llm_isvc.api_version.split("/")[1],
            llm_isvc.metadata.namespace,
            KSERVE_PLURAL_LLMINFERENCESERVICE,
            llm_isvc,
        )
        print(f"✅ LLM inference service {llm_isvc.metadata.name} created successfully")
        return outputs
    except client.rest.ApiException as e:
        raise RuntimeError(
            f"❌ Exception when calling CustomObjectsApi->"
            f"create_namespaced_custom_object for LLMInferenceService: {e}"
        ) from e


def assert_address_origins(
    kserve_client: KServeClient,
    llm_isvc: V1alpha1LLMInferenceService,
    expected_gateway: Optional[Dict[str, Any]] = None,
):
    """Verify that every address in status carries a valid origin reference.

    When expected_gateway is a Gateway resource dict, also asserts the
    origin matches its metadata.name and metadata.namespace.

    Reads via v1alpha2 (hub) because v1alpha1 conversion drops origin.
    """
    svc = get_llmisvc(
        kserve_client,
        llm_isvc.metadata.name,
        llm_isvc.metadata.namespace,
        "v1alpha2",
    )

    addresses = svc.get("status", {}).get("addresses", [])
    assert (
        len(addresses) > 0
    ), f"Expected at least one address in status, got: {svc.get('status')}"

    gw_meta = expected_gateway.get("metadata", {}) if expected_gateway else {}

    for addr in addresses:
        origin = addr.get("origin")
        assert origin is not None, f"Address {addr.get('url')} is missing origin"
        assert (
            origin.get("kind") == "Gateway"
        ), f"Expected origin kind 'Gateway', got '{origin.get('kind')}' for {addr.get('url')}"
        assert (
            origin.get("group") == "gateway.networking.k8s.io"
        ), f"Expected origin group 'gateway.networking.k8s.io', got '{origin.get('group')}'"

        if gw_meta:
            assert (
                origin.get("name") == gw_meta["name"]
            ), f"Expected origin gateway '{gw_meta['name']}', got '{origin.get('name')}'"
            assert (
                origin.get("namespace") == gw_meta["namespace"]
            ), f"Expected origin namespace '{gw_meta['namespace']}', got '{origin.get('namespace')}'"

    logger.info(f"All {len(addresses)} addresses have valid origin references")


def assert_address_models(
    kserve_client: KServeClient,
    llm_isvc: V1alpha1LLMInferenceService,
):
    """Verify that every address in status carries a non-empty models list.

    For model-routing addresses (name ends with '-model-routing'), model names
    must use the 'publishers/{namespace}/models/{name}' format. Path-based
    addresses may use either plain names or the publishers format.

    Reads via v1alpha2 (hub) because v1alpha1 conversion may drop models.
    """
    svc = get_llmisvc(
        kserve_client,
        llm_isvc.metadata.name,
        llm_isvc.metadata.namespace,
        "v1alpha2",
    )

    addresses = svc.get("status", {}).get("addresses", [])
    assert (
        len(addresses) > 0
    ), f"Expected at least one address in status, got: {svc.get('status')}"

    namespace = llm_isvc.metadata.namespace

    for addr in addresses:
        name = addr.get("name", "")
        models = addr.get("models", [])
        assert len(models) > 0, f"Address {name!r} ({addr.get('url')}) has no models"

        model_names = [m.get("name") for m in models]

        if name.endswith(MODEL_ROUTING_ADDRESS_SUFFIX):
            for model_name in model_names:
                assert model_name.startswith(f"publishers/{namespace}/models/"), (
                    f"Model-routing address model {model_name!r} does not use "
                    f"publishers/{namespace}/models/... format"
                )

    logger.info(f"All {len(addresses)} addresses have valid models")


@log_execution
def delete_llmisvc(kserve_client: KServeClient, llm_isvc: V1alpha1LLMInferenceService):
    name = llm_isvc.metadata.name
    namespace = llm_isvc.metadata.namespace
    try:
        result = kserve_client.api_instance.delete_namespaced_custom_object(
            constants.KSERVE_GROUP,
            llm_isvc.api_version.split("/")[1],
            namespace,
            KSERVE_PLURAL_LLMINFERENCESERVICE,
            name,
        )
        print(f"✅ LLM inference service {name} deleted successfully")
        _wait_for_llmisvc_pods_deleted(name, namespace)
        return result
    except client.rest.ApiException as e:
        raise RuntimeError(
            f"❌ Exception when calling CustomObjectsApi->"
            f"delete_namespaced_custom_object for LLMInferenceService: {e}"
        ) from e


def _wait_for_llmisvc_pods_deleted(
    service_name: str, namespace: str, timeout: int = 120
):
    """Block until all workload pods for the service are fully gone from the node.

    Without this, the next test can start before Terminating pods release their
    CPU/memory, causing scheduling failures on resource-constrained CI nodes.
    """
    core_v1 = client.CoreV1Api()
    label_selector = f"app.kubernetes.io/name={service_name}"

    def assert_no_pods():
        pods = core_v1.list_namespaced_pod(namespace, label_selector=label_selector)
        pod_names = [p.metadata.name for p in pods.items]
        assert not pod_names, (
            f"{len(pod_names)} pod(s) for {service_name} still terminating: {pod_names}"
        )

    try:
        wait_for(assert_no_pods, timeout=timeout, interval=5.0)
        print(f"✅ All pods for {service_name} terminated")
    except AssertionError:
        print(f"⚠️ Timed out waiting for pods of {service_name} to terminate")


def maybe_delete_llmisvc(
    kserve_client: KServeClient,
    llm_isvc: V1alpha1LLMInferenceService,
    test_failed: bool = False,
):
    """Delete LLMInferenceService unless env vars instruct otherwise.

    Respects SKIP_RESOURCE_DELETION (skip always) and
    SKIP_DELETION_ON_FAILURE (skip only when test_failed is True).
    """
    service_name = llm_isvc.metadata.name
    try:
        skip_all = os.getenv("SKIP_RESOURCE_DELETION", "False").lower() in (
            "true",
            "1",
            "t",
        )
        skip_on_failure = os.getenv("SKIP_DELETION_ON_FAILURE", "False").lower() in (
            "true",
            "1",
            "t",
        )

        should_skip = skip_all or (skip_on_failure and test_failed)

        if not should_skip:
            delete_llmisvc(kserve_client, llm_isvc)
        elif skip_all:
            print(
                f"⏭️  Skipping deletion of {service_name} (SKIP_RESOURCE_DELETION=True)"
            )
        elif test_failed and skip_on_failure:
            print(
                f"⏭️  Skipping deletion of {service_name} due to test failure (SKIP_DELETION_ON_FAILURE=True)"
            )
    except Exception as e:
        print(f"⚠️ Warning: Failed to cleanup service {service_name}: {e}")


def get_llmisvc(
    kserve_client: KServeClient,
    name,
    namespace,
    version=constants.KSERVE_V1ALPHA1_VERSION,
):
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
            f"❌ Exception when calling CustomObjectsApi->"
            f"get_namespaced_custom_object for LLMInferenceService: {e}"
        ) from e


@log_execution
def wait_for_model_response(
    kserve_client: KServeClient,
    test_case: TestCase,  # noqa: F811
    timeout_seconds: int = 900,
    extra_headers: Optional[Dict[str, str]] = None,
) -> str:
    def get_successful_response():
        try:
            if test_case.url_getter:
                service_url = test_case.url_getter(kserve_client, test_case.llm_service)
            else:
                service_url = get_llm_service_url(kserve_client, test_case.llm_service)
        except Exception as e:
            raise AssertionError(f"❌ Failed to get service URL: {e}") from e

        model_url = service_url + test_case.endpoint

        headers = {"Content-Type": "application/json"}
        if extra_headers:
            headers.update(extra_headers)

        if test_case.payload_formatter is not None:
            test_payload = test_case.payload_formatter(test_case)
        elif test_case.prompt is not None:
            test_payload = {
                "model": (
                    test_case.model_name
                    if not extra_headers or MODEL_ROUTING_HEADER not in extra_headers
                    else extra_headers[MODEL_ROUTING_HEADER]
                ),
                "prompt": test_case.prompt,
                "max_tokens": test_case.max_tokens,
            }
        else:
            test_payload = None

        logger.info(f"Calling LLM service at {model_url} with payload {test_payload}")
        try:
            if test_payload is not None:
                response = post_with_retry(
                    model_url,
                    headers=headers,
                    json_data=test_payload,
                    timeout=test_case.response_timeout,
                )
            else:
                response = get_with_retry(
                    model_url,
                    headers=headers,
                    timeout=test_case.response_timeout,
                )
        except Exception as e:
            logger.error(f"❌ Failed to call model: {e}")
            raise AssertionError(f"❌ Failed to call model: {e}") from e

        logger.info(f"Model response is {response.status_code}: {response.text[:500]}")

        if 200 <= response.status_code < 300:
            return response
        raise AssertionError(
            f"Service returned {response.status_code}: {response.text}"
        )

    response = wait_for(get_successful_response, timeout=timeout_seconds, interval=5.0)
    test_case.response_assertion(response)
    return response.text[: test_case.max_tokens]


@log_execution
def get_llm_service_url(
    kserve_client: KServeClient, llm_isvc: V1alpha1LLMInferenceService
):
    service_name = llm_isvc.metadata.name

    try:
        llm_isvc = get_llmisvc(
            kserve_client,
            llm_isvc.metadata.name,
            llm_isvc.metadata.namespace,
            llm_isvc.api_version.split("/")[1],
        )

        if "status" not in llm_isvc:
            raise ValueError(
                f"❌ No status found in LLM inference service {service_name} status: {llm_isvc}"
            )

        status = llm_isvc["status"]

        if "url" in status and status["url"]:
            return status["url"]

        if (
            "addresses" in status
            and status["addresses"]
            and len(status["addresses"]) > 0
        ):
            first_address = status["addresses"][0]
            if "url" in first_address:
                return first_address["url"]

        raise ValueError(
            f"❌ No URL found in LLM inference service {service_name} status"
        )

    except Exception as e:
        raise ValueError(
            f"❌ Failed to get URL for LLM inference service {service_name}: {e}"
        ) from e


@log_execution
def wait_for_llm_isvc_ready(
    kserve_client: KServeClient,
    given: V1alpha1LLMInferenceService,
    timeout_seconds: int = 900,
) -> str:
    def assert_llm_isvc_ready():
        out = get_llmisvc(
            kserve_client,
            given.metadata.name,
            given.metadata.namespace,
            given.api_version.split("/")[1],
        )

        if "status" not in out:
            raise AssertionError("No status found in LLM inference service")

        status = out["status"]
        if "conditions" not in status:
            raise AssertionError("No conditions found in status")

        expected_true_conditions = {"Ready", "WorkloadsReady", "RouterReady"}
        got_true_conditions = set()

        conditions = status["conditions"]

        for condition in conditions:
            if condition.get("status") == "True":
                got_true_conditions.add(condition.get("type"))

        missing_conditions = expected_true_conditions - got_true_conditions
        if missing_conditions:
            raise AssertionError(
                f"Missing true conditions: {missing_conditions}, expected {expected_true_conditions}, got {conditions}"
            )
        return True

    return wait_for(assert_llm_isvc_ready, timeout=timeout_seconds, interval=1.0)


def wait_for(
    assertion_fn: Callable[[], Any], timeout: float = 5.0, interval: float = 0.1
) -> Any:
    """Wait for the assertion to succeed within timeout."""
    deadline = time.time() + timeout
    last_msg = None
    while True:
        try:
            return assertion_fn()
        except AssertionError as e:
            msg = str(e)
            if time.time() >= deadline:
                logger.error("Timed out waiting: %s", e)
                raise
            if msg != last_msg:
                logger.info("Waiting: %s", e)
                last_msg = msg
            time.sleep(interval)


def _collect_diagnostics(
    kserve_client: KServeClient,
    llm_isvc: V1alpha1LLMInferenceService,
    log_prefix: str = "",
):
    name = llm_isvc.metadata.name
    ns = llm_isvc.metadata.namespace

    labels = {
        "app.kubernetes.io/part-of": "llminferenceservice",
        "app.kubernetes.io/name": name,
    }

    logger.info(f"{log_prefix}🔍 # Diagnostics for %r in %r", name, ns)
    logger.info(f"{log_prefix} ---")
    logger.info(f"{log_prefix} # LLMInferenceService %s", name)
    try:
        svc = get_llmisvc(kserve_client, name, ns)
        logger.info(yaml.safe_dump(svc, sort_keys=False))
    except Exception as e:
        logger.info(f"{log_prefix} # ❌ failed to dump LLMInferenceService: %s", e)

    print_all_events_table(ns, log=logger.info)
    collect_pod_logs(ns, labels, log=logger.info)

    all_resources = kinds_matching_by_labels(ns, labels)
    for obj in all_resources:
        logger.info(f"{log_prefix} ---")
        logger.info(
            yaml.safe_dump(strip_managed_fields(obj.to_dict()), sort_keys=False)
        )
