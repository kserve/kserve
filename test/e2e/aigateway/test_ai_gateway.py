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

import json
import os

import pytest
from kubernetes import client
from kubernetes.client import V1ResourceRequirements

from kserve import (
    V1beta1PredictorSpec,
    V1beta1ModelSpec,
    V1beta1ModelFormat,
    V1beta1InferenceService,
    V1beta1InferenceServiceSpec,
    V1beta1TrafficPolicy,
    V1beta1RateLimit,
    KServeClient,
)
from kserve.constants import constants
from kserve.logging import trace_logger as logger
from ..common.utils import KSERVE_TEST_NAMESPACE, chat_completion_stream, generate


@pytest.mark.aigateway
def test_aigateway_raw_deployment_qwen_vllm(network_layer):
    """
    Test AI Gateway integration with KServe InferenceService using RawDeployment mode.
    This test creates an InferenceService with AI Gateway enabled using Qwen model.
    """
    service_name = "aigateway-qwen-vllm"
    model_name = "hf-qwen-chat"

    # AI Gateway annotations for enabling AI Gateway integration
    annotations = {
        "serving.kserve.io/deploymentMode": "RawDeployment",
        "serving.kserve.io/enable-aigateway": "true",
    }

    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="huggingface",
            ),
            args=[
                "--model_id",
                "Qwen/Qwen2-0.5B-Instruct",
                "--model_name",
                model_name,
                "--backend",
                "vllm",
                "--max_model_len",
                "512",
                "--dtype",
                "bfloat16",
            ],
            env=[
                # Disable OpenAI route prefix to work with AI Gateway
                client.V1EnvVar(name="KSERVE_OPENAI_ROUTE_PREFIX", value="")
            ],
            resources=V1ResourceRequirements(
                requests={"cpu": "2", "memory": "4Gi"},
                limits={"cpu": "2", "memory": "6Gi"},
            ),
        ),
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name,
            namespace=KSERVE_TEST_NAMESPACE,
            annotations=annotations,
        ),
        spec=V1beta1InferenceServiceSpec(
            predictor=predictor,
            # Rate limit rule for hf-qwen-chat: 100 total tokens per hour per user
            traffic_policy=V1beta1TrafficPolicy(
                rate_limit=V1beta1RateLimit(
                    _global={
                        "rules": [
                            {
                                "clientSelectors": [
                                    {
                                        "headers": [
                                            {"name": "x-user-id", "type": "Distinct"},
                                            {
                                                "name": "x-ai-eg-model",
                                                "type": "Exact",
                                                "value": model_name,
                                            },
                                        ]
                                    }
                                ],
                                "limit": {"requests": 100, "unit": "Hour"},
                                "cost": {
                                    "request": {
                                        "from": "Number",
                                        "number": 0,  # Set to 0 so only token usage counts
                                    },
                                    "response": {
                                        "from": "Metadata",
                                        "metadata": {
                                            "namespace": "io.envoy.ai_gateway",
                                            "key": "llm_total_token",  # Uses total tokens from the responses
                                        },
                                    },
                                },
                            }
                        ],
                    }
                ),
            ),
        ),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )

    # Create ReferenceGrant first
    create_reference_grant(service_name)

    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

    # Test chat completion
    res = generate(
        service_name,
        "./data/qwen_input_chat.json",
        raw_response=True,
        disable_openai_prefix=True,
        network_layer=network_layer,
    )
    result = json.loads(res.content.decode("utf-8"))
    assert result["choices"][0]["message"]["content"] == "The result of 2 + 2 is 4."

    logger.info("Response headers: %s", res.headers)
    # verify rate limit headers
    assert "x-ratelimit-remaining" in res.headers
    assert "x-ratelimit-reset" in res.headers
    assert "x-ratelimit-limit" in res.headers

    # Test chat completion streaming
    full_response, _, res_headers = chat_completion_stream(
        service_name,
        "./data/qwen_input_chat_stream.json",
        disable_openai_prefix=True,
        network_layer=network_layer,
    )
    assert full_response.strip() == "The result of 2 + 2 is 4."

    logger.info("Streaming response headers: %s", res_headers)
    # verify rate limit headers
    assert "x-ratelimit-remaining" in res_headers
    assert "x-ratelimit-reset" in res_headers
    assert "x-ratelimit-limit" in res_headers

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
    cleanup_reference_grant(service_name)


def create_reference_grant(service_name):
    """Create a ReferenceGrant for cross-namespace access."""
    kube_client = client.ApiClient()
    custom_objects_api = client.CustomObjectsApi(kube_client)

    reference_grant = {
        "apiVersion": "gateway.networking.k8s.io/v1beta1",
        "kind": "ReferenceGrant",
        "metadata": {
            "name": f"{service_name}-ref-grant",
            "namespace": KSERVE_TEST_NAMESPACE,
        },
        "spec": {
            "from": [
                {
                    "group": "gateway.networking.k8s.io",
                    "kind": "HTTPRoute",
                    "namespace": "kserve",
                }
            ],
            "to": [{"group": "", "kind": "Service"}],
        },
    }

    try:
        custom_objects_api.create_namespaced_custom_object(
            group="gateway.networking.k8s.io",
            version="v1beta1",
            namespace=KSERVE_TEST_NAMESPACE,
            plural="referencegrants",
            body=reference_grant,
        )
        print(f"Created ReferenceGrant {service_name}-ref-grant")
    except client.ApiException as e:
        if e.status != 409:  # Ignore if already exists
            raise e


def cleanup_reference_grant(service_name):
    """Clean up ReferenceGrant."""
    kube_client = client.ApiClient()
    custom_objects_api = client.CustomObjectsApi(kube_client)

    try:
        custom_objects_api.delete_namespaced_custom_object(
            group="gateway.networking.k8s.io",
            version="v1beta1",
            namespace=KSERVE_TEST_NAMESPACE,
            plural="referencegrants",
            name=f"{service_name}-ref-grant",
        )
        print(f"Deleted ReferenceGrant {service_name}-ref-grant")
    except client.ApiException as e:
        if e.status != 404:
            print(f"Failed to delete ReferenceGrant: {e}")
