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

import hashlib
import os
import pytest
from ..common.gw_api import (
    create_or_update_gateway,
    create_or_update_route,
    delete_gateway,
    delete_route,
)
from kserve import KServeClient, constants, V1alpha1LLMInferenceService
from kubernetes import client, config
from typing import List

from .logging import logger

KSERVE_PLURAL_LLMINFERENCESERVICECONFIG = "llminferenceserviceconfigs"
KSERVE_TEST_NAMESPACE = "kserve-ci-e2e-test"

# Scheduler config constants
SCHEDULER_CONFIGMAP_NAME = "scheduler-config-e2e"
SCHEDULER_CONFIGMAP_KEY = "epp"

LLMINFERENCESERVICE_CONFIGS = {
    "workload-single-cpu": {
        "template": {
            "containers": [
                {
                    "name": "main",
                    "image": "quay.io/pierdipi/vllm-cpu:latest",
                    "env": [{"name": "VLLM_LOGGING_LEVEL", "value": "DEBUG"}],
                    "resources": {
                        "limits": {"cpu": "2", "memory": "7Gi"},
                        "requests": {"cpu": "200m", "memory": "2Gi"},
                    },
                    "livenessProbe": {
                        "initialDelaySeconds": 180,
                        "periodSeconds": 30,
                        "timeoutSeconds": 30,
                        "failureThreshold": 8,
                    },
                    "readinessProbe": {
                        "httpGet": {"path": "/health", "port": 8000},
                        "initialDelaySeconds": 30,
                        "periodSeconds": 10,
                        "timeoutSeconds": 5,
                        "failureThreshold": 3,
                    },
                    "securityContext": {
                        "runAsNonRoot": False,
                        "runAsUser": 0,
                    },
                }
            ]
        },
    },
    "workload-pd-cpu": {
        "template": {
            "containers": [
                {
                    "name": "main",
                    "image": "quay.io/pierdipi/vllm-cpu:latest",
                    "env": [{"name": "VLLM_LOGGING_LEVEL", "value": "DEBUG"}],
                    "resources": {
                        "limits": {"cpu": "2", "memory": "7Gi"},
                        "requests": {"cpu": "200m", "memory": "2Gi"},
                    },
                    "livenessProbe": {
                        "httpGet": {"path": "/health", "port": 8000},
                        "initialDelaySeconds": 180,
                        "periodSeconds": 30,
                        "timeoutSeconds": 30,
                        "failureThreshold": 8,
                    },
                    "readinessProbe": {
                        "httpGet": {"path": "/health", "port": 8000},
                        "initialDelaySeconds": 30,
                        "periodSeconds": 10,
                        "timeoutSeconds": 5,
                        "failureThreshold": 3,
                    },
                    "securityContext": {
                        "runAsNonRoot": False,
                        "runAsUser": 0,
                    },
                }
            ]
        },
        "prefill": {
            "template": {
                "containers": [
                    {
                        "name": "main",
                        "image": "quay.io/pierdipi/vllm-cpu:latest",
                        "env": [{"name": "VLLM_LOGGING_LEVEL", "value": "DEBUG"}],
                        "resources": {
                            "limits": {"cpu": "2", "memory": "7Gi"},
                            "requests": {"cpu": "200m", "memory": "2Gi"},
                        },
                        "livenessProbe": {
                            "httpGet": {"path": "/health", "port": 8000},
                            "initialDelaySeconds": 180,
                            "periodSeconds": 30,
                            "timeoutSeconds": 30,
                            "failureThreshold": 8,
                        },
                        "readinessProbe": {
                            "httpGet": {"path": "/health", "port": 8000},
                            "initialDelaySeconds": 30,
                            "periodSeconds": 10,
                            "timeoutSeconds": 5,
                            "failureThreshold": 3,
                        },
                        "securityContext": {
                            "runAsNonRoot": False,
                            "runAsUser": 0,
                        },
                    }
                ]
            }
        },
    },
    "model-fb-opt-125m": {
        "model": {"uri": "hf://facebook/opt-125m", "name": "facebook/opt-125m"},
    },
    "model-deepseek-v2-lite": {
        "model": {
            "uri": "hf://deepseek-ai/DeepSeek-V2-Lite-Chat",
            "name": "deepseek-ai/DeepSeek-V2-Lite-Chat",
        },
    },
    "workload-dp-ep-gpu": {
        "replicas": 2,
        "parallelism": {
            "data": 1,
            "dataLocal": 8,
            "expert": True,
            "tensor": 1,
        },
        "template": {
            "containers": [
                {
                    "name": "main",
                    "env": [
                        {"name": "VLLM_LOGGING_LEVEL", "value": "DEBUG"},
                        {"name": "TRITON_LIBCUDA_PATH", "value": "/usr/lib64"},
                        {"name": "HF_HUB_DISABLE_XET", "value": "1"},
                        {"name": "VLLM_SKIP_P2P_CHECK", "value": "1"},
                        {"name": "VLLM_RANDOMIZE_DP_DUMMY_INPUTS", "value": "1"},
                        {"name": "VLLM_USE_DEEP_GEMM", "value": "0"},
                        {
                            "name": "VLLM_ALL2ALL_BACKEND",
                            "value": "deepep_high_throughput",
                        },
                        {"name": "NVIDIA_GDRCOPY", "value": "enabled"},
                        {"name": "HF_HUB_CACHE", "value": "/huggingface-cache"},
                    ],
                    "resources": {
                        "limits": {
                            "cpu": "16",
                            "memory": "512Gi",
                            "nvidia.com/gpu": "8",
                        },
                        "requests": {
                            "cpu": "8",
                            "memory": "256Gi",
                            "nvidia.com/gpu": "8",
                        },
                    },
                    "livenessProbe": {
                        "httpGet": {"path": "/health", "port": 8001, "scheme": "HTTPS"},
                        "initialDelaySeconds": 400,
                        "periodSeconds": 10,
                        "timeoutSeconds": 10,
                        "failureThreshold": 3,
                    },
                }
            ]
        },
        "worker": {
            "containers": [
                {
                    "name": "main",
                    "env": [
                        {"name": "VLLM_LOGGING_LEVEL", "value": "DEBUG"},
                        {"name": "TRITON_LIBCUDA_PATH", "value": "/usr/lib64"},
                        {"name": "HF_HUB_DISABLE_XET", "value": "1"},
                        {"name": "VLLM_SKIP_P2P_CHECK", "value": "1"},
                        {"name": "VLLM_RANDOMIZE_DP_DUMMY_INPUTS", "value": "1"},
                        {"name": "VLLM_USE_DEEP_GEMM", "value": "0"},
                        {
                            "name": "VLLM_ALL2ALL_BACKEND",
                            "value": "deepep_high_throughput",
                        },
                        {"name": "NVIDIA_GDRCOPY", "value": "enabled"},
                        {"name": "HF_HUB_CACHE", "value": "/huggingface-cache"},
                    ],
                    "resources": {
                        "limits": {
                            "cpu": "16",
                            "memory": "512Gi",
                            "nvidia.com/gpu": "8",
                        },
                        "requests": {
                            "cpu": "8",
                            "memory": "256Gi",
                            "nvidia.com/gpu": "8",
                        },
                    },
                }
            ]
        },
    },
    "workload-dp-ep-prefill-gpu": {
        "prefill": {
            "parallelism": {
                "data": 1,
                "dataLocal": 8,
                "expert": True,
                "tensor": 1,
            },
            "template": {
                "containers": [
                    {
                        "name": "main",
                        "env": [
                            {"name": "VLLM_LOGGING_LEVEL", "value": "DEBUG"},
                            {"name": "TRITON_LIBCUDA_PATH", "value": "/usr/lib64"},
                            {"name": "HF_HUB_DISABLE_XET", "value": "1"},
                            {"name": "VLLM_SKIP_P2P_CHECK", "value": "1"},
                            {"name": "VLLM_RANDOMIZE_DP_DUMMY_INPUTS", "value": "1"},
                            {"name": "VLLM_USE_DEEP_GEMM", "value": "0"},
                            {
                                "name": "VLLM_ALL2ALL_BACKEND",
                                "value": "deepep_high_throughput",
                            },
                            {"name": "NVIDIA_GDRCOPY", "value": "enabled"},
                            {"name": "HF_HUB_CACHE", "value": "/huggingface-cache"},
                        ],
                        "resources": {
                            "limits": {
                                "cpu": "16",
                                "memory": "512Gi",
                                "nvidia.com/gpu": "8",
                            },
                            "requests": {
                                "cpu": "8",
                                "memory": "256Gi",
                                "nvidia.com/gpu": "8",
                            },
                        },
                        "livenessProbe": {
                            "httpGet": {
                                "path": "/health",
                                "port": 8000,
                                "scheme": "HTTPS",
                            },
                            "initialDelaySeconds": 400,
                            "periodSeconds": 10,
                            "timeoutSeconds": 10,
                            "failureThreshold": 3,
                        },
                    }
                ]
            },
            "worker": {
                "containers": [
                    {
                        "name": "main",
                        "env": [
                            {"name": "VLLM_LOGGING_LEVEL", "value": "DEBUG"},
                            {"name": "TRITON_LIBCUDA_PATH", "value": "/usr/lib64"},
                            {"name": "HF_HUB_DISABLE_XET", "value": "1"},
                            {"name": "VLLM_SKIP_P2P_CHECK", "value": "1"},
                            {"name": "VLLM_RANDOMIZE_DP_DUMMY_INPUTS", "value": "1"},
                            {"name": "VLLM_USE_DEEP_GEMM", "value": "0"},
                            {
                                "name": "VLLM_ALL2ALL_BACKEND",
                                "value": "deepep_high_throughput",
                            },
                            {"name": "NVIDIA_GDRCOPY", "value": "enabled"},
                            {"name": "HF_HUB_CACHE", "value": "/huggingface-cache"},
                        ],
                        "resources": {
                            "limits": {
                                "cpu": "16",
                                "memory": "512Gi",
                                "nvidia.com/gpu": "8",
                            },
                            "requests": {
                                "cpu": "8",
                                "memory": "256Gi",
                                "nvidia.com/gpu": "8",
                            },
                        },
                    }
                ]
            },
        },
    },
    "router-managed": {
        "router": {"scheduler": {}, "route": {}, "gateway": {}},
    },
    "router-no-scheduler": {
        "router": {"route": {}},
    },
    # This preset simulates DP+EP that can run on CPU, the idea is to test the LWS-based deployment
    # but without the resources requirements for DP+EP (GPUs and ROCe/IB)
    "workload-simulated-dp-ep-cpu": {
        "replicas": 1,
        "parallelism": {
            "data": 2,
            "dataLocal": 1,
            "expert": True,
            "tensor": 1,
        },
        "template": {
            "containers": [
                {
                    "name": "main",
                    "image": "quay.io/pierdipi/vllm-cpu:latest",
                    "command": ["vllm", "serve", "/mnt/models"],
                    "args": [
                        "--served-model-name",
                        "{{ .Spec.Model.Name }}",
                        "--port",
                        "8000",
                        # SSL disabled to match HTTP-only Gateway setup
                        # "--enable-ssl-refresh",
                        # "--ssl-certfile",
                        # "/etc/ssl/certs/tls.crt",
                        # "--ssl-keyfile",
                        # "/etc/ssl/certs/tls.key",
                    ],
                    "resources": {
                        "limits": {"cpu": "2", "memory": "7Gi"},
                        "requests": {"cpu": "200m", "memory": "2Gi"},
                    },
                    "livenessProbe": {
                        "httpGet": {"path": "/health", "port": 8000, "scheme": "HTTP"},
                        "initialDelaySeconds": 180,
                        "periodSeconds": 30,
                        "timeoutSeconds": 30,
                        "failureThreshold": 8,
                    },
                    "readinessProbe": {
                        "httpGet": {"path": "/health", "port": 8000, "scheme": "HTTP"},
                        "initialDelaySeconds": 30,
                        "periodSeconds": 10,
                        "timeoutSeconds": 5,
                        "failureThreshold": 3,
                    },
                    "securityContext": {
                        "runAsNonRoot": False,
                        "runAsUser": 0,
                    },
                }
            ]
        },
        "worker": {
            "containers": [
                {
                    "name": "main",
                    "image": "quay.io/pierdipi/vllm-cpu:latest",
                    "command": ["vllm", "serve", "/mnt/models"],
                    "args": [
                        "--served-model-name",
                        "{{ .Spec.Model.Name }}",
                        "--port",
                        "8000",
                        "--enable-ssl-refresh",
                        "--ssl-certfile",
                        "/etc/ssl/certs/tls.crt",
                        "--ssl-keyfile",
                        "/etc/ssl/certs/tls.key",
                    ],
                    "resources": {
                        "limits": {"cpu": "2", "memory": "7Gi"},
                        "requests": {"cpu": "200m", "memory": "2Gi"},
                    },
                    "livenessProbe": {
                        "httpGet": {"path": "/health", "port": 8000, "scheme": "HTTP"},
                        "initialDelaySeconds": 180,
                        "periodSeconds": 30,
                        "timeoutSeconds": 30,
                        "failureThreshold": 8,
                    },
                    "readinessProbe": {
                        "httpGet": {"path": "/health", "port": 8000, "scheme": "HTTP"},
                        "initialDelaySeconds": 30,
                        "periodSeconds": 10,
                        "timeoutSeconds": 5,
                        "failureThreshold": 3,
                    },
                    "securityContext": {
                        "runAsNonRoot": False,
                        "runAsUser": 0,
                    },
                }
            ]
        },
    },
    "router-custom-route-timeout": {
        "router": {
            "route": {
                "http": {
                    "spec": {
                        "rules": [
                            {
                                "timeouts": {
                                    "request": "30s",
                                    "backendRequest": "30s",
                                },
                                "matches": [
                                    {
                                        "path": {
                                            "type": "PathPrefix",
                                            "value": "/kserve-ci-e2e-test/custom-route-timeout-test",
                                        },
                                    },
                                ],
                                "filters": [
                                    {
                                        "type": "URLRewrite",
                                        "urlRewrite": {
                                            "path": {
                                                "replacePrefixMatch": "/",
                                                "type": "ReplacePrefixMatch",
                                            },
                                        },
                                    },
                                ],
                                "backendRefs": [
                                    {
                                        "group": "inference.networking.k8s.io",
                                        "kind": "InferencePool",
                                        "name": "custom-route-timeout-test-inference-pool",
                                        "namespace": KSERVE_TEST_NAMESPACE,
                                        "port": 8000,
                                    }
                                ],
                            },
                        ],
                    },
                },
            },
            "gateway": {},
        },
    },
    "router-custom-route-timeout-pd": {
        "router": {
            "route": {
                "http": {
                    "spec": {
                        "rules": [
                            {
                                "timeouts": {
                                    "request": "30s",
                                    "backendRequest": "30s",
                                },
                                "matches": [
                                    {
                                        "path": {
                                            "type": "PathPrefix",
                                            "value": "/kserve-ci-e2e-test/custom-route-timeout-pd-test",
                                        },
                                    },
                                ],
                                "filters": [
                                    {
                                        "type": "URLRewrite",
                                        "urlRewrite": {
                                            "path": {
                                                "replacePrefixMatch": "/",
                                                "type": "ReplacePrefixMatch",
                                            },
                                        },
                                    },
                                ],
                                "backendRefs": [
                                    {
                                        "group": "inference.networking.k8s.io",
                                        "kind": "InferencePool",
                                        "name": "custom-route-timeout-pd-test-inference-pool",
                                        "namespace": KSERVE_TEST_NAMESPACE,
                                        "port": 8000,
                                    }
                                ],
                            },
                        ],
                    },
                },
            },
            "gateway": {},
        },
    },
    "router-with-refs": {
        "router": {
            "route": {
                "http": {
                    "refs": [
                        {"name": "router-route-1"},
                        {"name": "router-route-2"},
                    ],
                },
            },
            "gateway": {
                "refs": [
                    {"name": "router-gateway-1", "namespace": KSERVE_TEST_NAMESPACE},
                ],
            },
        },
    },
    "router-with-refs-pd": {
        "router": {
            "route": {
                "http": {
                    "refs": [
                        {"name": "router-route-3"},
                        {"name": "router-route-4"},
                    ],
                },
            },
            "gateway": {
                "refs": [
                    {"name": "router-gateway-2", "namespace": KSERVE_TEST_NAMESPACE},
                ],
            },
        },
    },
    "scheduler-managed": {
        "router": {
            "scheduler": {},
        },
    },
    "scheduler-with-inline-config": {
        "router": {
            "scheduler": {
                "config": {
                    "inline": {
                        "apiVersion": "inference.networking.x-k8s.io/v1alpha1",
                        "kind": "EndpointPickerConfig",
                        "plugins": [
                            {"type": "single-profile-handler"},
                            {"type": "queue-scorer"},
                            {"type": "prefix-cache-scorer"},
                            {"type": "max-score-picker"},
                        ],
                        "schedulingProfiles": [
                            {
                                "name": "default",
                                "plugins": [
                                    {"pluginRef": "queue-scorer", "weight": 2},
                                    {"pluginRef": "prefix-cache-scorer", "weight": 3},
                                    {"pluginRef": "max-score-picker"},
                                ],
                            },
                        ],
                    },
                },
            },
        },
    },
    "scheduler-with-configmap-ref": {
        "router": {
            "scheduler": {
                "config": {
                    "ref": {
                        "name": SCHEDULER_CONFIGMAP_NAME,
                        "key": SCHEDULER_CONFIGMAP_KEY,
                    },
                },
            },
        },
    },
    "router-with-gateway-ref": {
        "router": {
            "gateway": {
                "refs": [
                    {"name": "router-gateway-1", "namespace": KSERVE_TEST_NAMESPACE},
                ],
            },
        },
    },
    "router-with-managed-route": {
        "router": {"route": {}},
    },
    "workload-llmd-simulator": {
        "replicas": 1,
        "model": {"uri": "hf://facebook/opt-125m", "name": "facebook/opt-125m"},
        "template": {
            "containers": [
                {
                    "name": "main",
                    "image": "ghcr.io/llm-d/llm-d-inference-sim:v0.5.1",
                    "command": ["/app/llm-d-inference-sim"],
                    "args": [
                        "--port",
                        "8000",
                        "--model",
                        "{{ .Spec.Model.Name }}",
                        "--mode",
                        "random",
                        # "--ssl-certfile",
                        # "/etc/ssl/certs/tls.crt",
                        # "--ssl-keyfile",
                        # "/etc/ssl/certs/tls.key",
                    ],
                    "resources": {
                        "limits": {"cpu": "1", "memory": "2Gi"},
                        "requests": {"cpu": "200m", "memory": "2Gi"},
                    },
                }
            ]
        },
    },
}


@pytest.fixture(scope="function")
def test_case(request):
    tc = request.param
    created_configs = []

    inject_k8s_proxy()

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config"),
        client_configuration=client.Configuration(),
    )

    # Execute before test hooks
    try:
        for func in tc.before_test:
            func()
    except Exception as before_test_error:
        raise RuntimeError(
            f"Failed to execute before test hook: {before_test_error}"
        ) from before_test_error

    try:
        # Validate base_refs defined in the test fixture exist in LLMINFERENCESERVICE_CONFIGS
        missing_refs = [
            ref for ref in tc.base_refs if ref not in LLMINFERENCESERVICE_CONFIGS
        ]
        if missing_refs:
            raise ValueError(
                f"Missing base_refs in LLMINFERENCESERVICE_CONFIGS: {missing_refs}"
            )
        if not tc.service_name:
            tc.service_name = generate_service_name(request.node.name, tc.base_refs)
        tc.model_name = _get_model_name_from_configs(tc.base_refs)

        # Create unique configs for this test
        unique_base_refs = []
        for base_ref in tc.base_refs:
            unique_config_name = generate_k8s_safe_suffix(base_ref, [tc.service_name])
            unique_base_refs.append(unique_config_name)

            original_spec = LLMINFERENCESERVICE_CONFIGS[base_ref]

            unique_config_body = {
                "apiVersion": "serving.kserve.io/v1alpha1",
                "kind": "LLMInferenceServiceConfig",
                "metadata": {
                    "name": unique_config_name,
                    "namespace": KSERVE_TEST_NAMESPACE,
                },
                "spec": original_spec,
            }

            _create_or_update_llmisvc_config(
                kserve_client, unique_config_body, KSERVE_TEST_NAMESPACE
            )
            created_configs.append(unique_config_name)

        tc.llm_service = V1alpha1LLMInferenceService(
            api_version="serving.kserve.io/v1alpha1",
            kind="LLMInferenceService",
            metadata=client.V1ObjectMeta(
                name=tc.service_name, namespace=KSERVE_TEST_NAMESPACE
            ),
            spec={
                "baseRefs": [{"name": base_ref} for base_ref in unique_base_refs],
            },
        )

        yield tc

    finally:
        if os.getenv("SKIP_RESOURCE_DELETION", "False").lower() in ("true", "1", "t"):
            logger.info("Skipping resource deletion after test execution.")
            return  # noqa: B012

        # Execute after test hooks
        for func in tc.after_test:
            try:
                func()
            except Exception as after_test_error:
                logger.warning(f"Failed to execute after test hook: {after_test_error}")

        # Cleanup created configs
        for config_name in created_configs:
            try:
                logger.info(
                    f"Cleaning up unique LLMInferenceServiceConfig {config_name}"
                )

                if os.getenv("SKIP_RESOURCE_DELETION", "False").lower() in (
                    "false",
                    "0",
                    "f",
                ):
                    _delete_llmisvc_config(
                        kserve_client, config_name, KSERVE_TEST_NAMESPACE
                    )
                logger.info(f"✓ Deleted unique LLMInferenceServiceConfig {config_name}")
            except Exception as e:
                logger.warning(
                    f"Failed to cleanup LLMInferenceServiceConfig {config_name}: {e}"
                )


def _get_model_name_from_configs(config_names):
    """Extract the model name from model config."""
    for config_name in config_names:
        config = LLMINFERENCESERVICE_CONFIGS[config_name]
        if "model" in config and "name" in config["model"]:
            return config["model"]["name"]
    return "default-model"


def generate_k8s_safe_suffix(base_name: str, extra_parts: List[str] = None) -> str:
    """Generate a Kubernetes-safe name suffix with hash."""
    if extra_parts:
        full_name = f"{base_name}-{'-'.join(sorted(extra_parts))}"
    else:
        full_name = base_name

    full_name = full_name.lower().replace("_", "-")

    name_hash = hashlib.md5(full_name.encode()).hexdigest()[:8]

    # TODO: we can't use the real maximum (63), LWS and STS add additional suffixes (ie `-0`) and don't handle that case.
    max_total = 40
    sep = "-"
    max_base = max_total - len(sep) - len(name_hash)
    safe_base = full_name[:max_base].rstrip(sep)

    return f"{safe_base}{sep}{name_hash}"


def generate_service_name(test_name: str, base_refs: List[str]) -> str:
    base_name = test_name.split("[", 1)[0]
    base_name = base_name.replace("test_llm_inference_service", "llmisvc")
    return generate_k8s_safe_suffix(base_name, base_refs)


def generate_test_id(test_case) -> str:
    """Generate a test ID from base refs."""
    return "-".join(test_case.base_refs)


def create_router_resources(gateways, routes, kserve_client=None):
    if not kserve_client:
        kserve_client = KServeClient(
            config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
        )

    gateways_created = []
    routes_created = []

    try:
        for gateway in gateways:
            create_or_update_gateway(kserve_client, gateway)
            gateways_created.append(gateway)
        for route in routes:
            create_or_update_route(kserve_client, route)
            routes_created.append(route)
    except Exception as e:
        logger.warning(f"Failed to create LLMInferenceService router resources: {e}")
        delete_router_resources(gateways_created, routes_created, kserve_client)
        raise


def delete_router_resources(gateways, routes, kserve_client=None):
    if not kserve_client:
        kserve_client = KServeClient(
            config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
        )

    for route in routes:
        try:
            logger.info(
                f"Cleaning up HttpRoute {route.get('metadata', {}).get('name')}"
            )
            delete_route(
                kserve_client,
                route.get("metadata", {}).get("name"),
                route.get("metadata", {}).get("namespace", "default"),
            )
            logger.info(f"✓ Deleted HttpRoute {route.get('metadata', {}).get('name')}")
        except Exception as e:
            logger.warning(
                f"Failed to cleanup HttpRoute {route.get('metadata', {}).get('name')}: {e}"
            )

    for gateway in gateways:
        try:
            logger.info(
                f"Cleaning up Gateway {gateway.get('metadata', {}).get('name')}"
            )
            delete_gateway(
                kserve_client,
                gateway.get("metadata", {}).get("name"),
                gateway.get("metadata", {}).get("namespace", "default"),
            )
            logger.info(f"✓ Deleted Gateway {gateway.get('metadata', {}).get('name')}")
        except Exception as e:
            logger.warning(
                f"Failed to cleanup Gateway {gateway.get('metadata', {}).get('name')}: {e}"
            )


def _create_or_update_llmisvc_config(kserve_client, llm_config, namespace=None):
    """Create or update an LLMInferenceServiceConfig resource."""
    version = llm_config["apiVersion"].split("/")[1]

    if namespace is None:
        namespace = llm_config.get("metadata", {}).get("namespace", "default")

    name = llm_config.get("metadata", {}).get("name")
    if not name:
        raise ValueError("LLMInferenceServiceConfig must have a name in metadata")

    logger.info(f"Checking LLMInferenceServiceConfig {name} in namespace {namespace}")

    try:
        existing_config = kserve_client.api_instance.get_namespaced_custom_object(
            constants.KSERVE_GROUP,
            version,
            namespace,
            KSERVE_PLURAL_LLMINFERENCESERVICECONFIG,
            name,
        )

        llm_config["metadata"] = existing_config["metadata"]

        outputs = kserve_client.api_instance.replace_namespaced_custom_object(
            constants.KSERVE_GROUP,
            version,
            namespace,
            KSERVE_PLURAL_LLMINFERENCESERVICECONFIG,
            name,
            llm_config,
        )
        logger.info(f"✓ Successfully updated LLMInferenceServiceConfig {name}")
        return outputs

    except client.rest.ApiException as e:
        if e.status == 404:  # Not found - create it
            logger.info(
                f"Resource not found, creating LLMInferenceServiceConfig {name}"
            )
            outputs = kserve_client.api_instance.create_namespaced_custom_object(
                constants.KSERVE_GROUP,
                version,
                namespace,
                KSERVE_PLURAL_LLMINFERENCESERVICECONFIG,
                llm_config,
            )
            logger.info(f"✓ Successfully created LLMInferenceServiceConfig {name}")
            return outputs
        else:
            raise RuntimeError(
                f"Failed to get/create LLMInferenceServiceConfig {name}: {e}"
            ) from e


def _delete_llmisvc_config(
    kserve_client, name, namespace, version=constants.KSERVE_V1ALPHA1_VERSION
):
    try:
        print(f"Deleting LLMInferenceServiceConfig {name} in namespace {namespace}")
        return kserve_client.api_instance.delete_namespaced_custom_object(
            constants.KSERVE_GROUP,
            version,
            namespace,
            KSERVE_PLURAL_LLMINFERENCESERVICECONFIG,
            name,
        )
    except client.rest.ApiException as e:
        raise RuntimeError(
            f"Exception when calling CustomObjectsApi->"
            f"delete_namespaced_custom_object for LLMInferenceServiceConfig: {e}"
        ) from e


def _get_llmisvc_config(
    kserve_client, name, namespace, version=constants.KSERVE_V1ALPHA1_VERSION
):
    try:
        return kserve_client.api_instance.get_namespaced_custom_object(
            constants.KSERVE_GROUP,
            version,
            namespace,
            KSERVE_PLURAL_LLMINFERENCESERVICECONFIG,
            name,
        )
    except client.rest.ApiException as e:
        raise RuntimeError(
            f"Exception when calling CustomObjectsApi->"
            f"get_namespaced_custom_object for LLMInferenceServiceConfig: {e}"
        ) from e


def inject_k8s_proxy():
    config.load_kube_config()
    proxy_url = os.getenv("HTTPS_PROXY", os.getenv("HTTP_PROXY", None))
    if proxy_url:
        logger.info(f"✅ Using Proxy URL: {proxy_url} for k8s client")
        client.Configuration._default.proxy = proxy_url
    else:
        logger.info("No HTTP proxy configured for k8s client")


# Scheduler config YAML used for ConfigMap ref tests
SCHEDULER_CONFIG_YAML = """apiVersion: inference.networking.x-k8s.io/v1alpha1
kind: EndpointPickerConfig
plugins:
- type: single-profile-handler
- type: queue-scorer
- type: prefix-cache-scorer
- type: max-score-picker
schedulingProfiles:
- name: default
  plugins:
  - pluginRef: queue-scorer
    weight: 2
  - pluginRef: prefix-cache-scorer
    weight: 3
  - pluginRef: max-score-picker
"""


def create_scheduler_configmap():
    """Create ConfigMap with scheduler configuration."""
    inject_k8s_proxy()
    core_v1 = client.CoreV1Api()

    configmap = client.V1ConfigMap(
        api_version="v1",
        kind="ConfigMap",
        metadata=client.V1ObjectMeta(
            name=SCHEDULER_CONFIGMAP_NAME,
            namespace=KSERVE_TEST_NAMESPACE,
        ),
        data={
            SCHEDULER_CONFIGMAP_KEY: SCHEDULER_CONFIG_YAML,
        },
    )

    try:
        core_v1.create_namespaced_config_map(
            namespace=KSERVE_TEST_NAMESPACE,
            body=configmap,
        )
        logger.info(
            f"Created ConfigMap {SCHEDULER_CONFIGMAP_NAME} in namespace {KSERVE_TEST_NAMESPACE}"
        )
    except client.rest.ApiException as e:
        if e.status == 409:  # Already exists
            core_v1.replace_namespaced_config_map(
                name=SCHEDULER_CONFIGMAP_NAME,
                namespace=KSERVE_TEST_NAMESPACE,
                body=configmap,
            )
            logger.info(
                f"Updated ConfigMap {SCHEDULER_CONFIGMAP_NAME} in namespace {KSERVE_TEST_NAMESPACE}"
            )
        else:
            raise


def delete_scheduler_configmap():
    """Delete ConfigMap with scheduler configuration."""
    inject_k8s_proxy()
    core_v1 = client.CoreV1Api()

    try:
        core_v1.delete_namespaced_config_map(
            name=SCHEDULER_CONFIGMAP_NAME,
            namespace=KSERVE_TEST_NAMESPACE,
        )
        logger.info(
            f"Deleted ConfigMap {SCHEDULER_CONFIGMAP_NAME} from namespace {KSERVE_TEST_NAMESPACE}"
        )
    except client.rest.ApiException as e:
        if e.status != 404:  # Ignore not found
            raise
