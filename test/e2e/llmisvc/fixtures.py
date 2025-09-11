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
from kserve import KServeClient, constants, V1alpha1LLMInferenceService
from kubernetes import client, config
from kubernetes.client.rest import ApiException
from typing import List

from .logging import logger

KSERVE_PLURAL_LLMINFERENCESERVICECONFIG = "llminferenceserviceconfigs"
KSERVE_TEST_NAMESPACE = "kserve-ci-e2e-test"

LLMINFERENCESERVICE_CONFIGS = {
    "workload-single-cpu": {
        "template": {
            "containers": [
                {
                    "name": "main",
                    "image": "quay.io/pierdipi/vllm-cpu:latest",
                    "env": [{"name": "VLLM_LOGGING_LEVEL", "value": "DEBUG"}],
                    "resources": {
                        "limits": {"cpu": "2", "memory": "10Gi"},
                        "requests": {"cpu": "1", "memory": "8Gi"},
                    },
                    "livenessProbe": {
                        "initialDelaySeconds": 30,
                        "periodSeconds": 30,
                        "timeoutSeconds": 30,
                        "failureThreshold": 5,
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
                        "limits": {"cpu": "2", "memory": "10Gi"},
                        "requests": {"cpu": "1", "memory": "8Gi"},
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
                            "limits": {"cpu": "2", "memory": "10Gi"},
                            "requests": {"cpu": "1", "memory": "8Gi"},
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
        "replicas": 2,
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
                        "--disable-log-requests",
                        "--enable-ssl-refresh",
                        "--ssl-certfile",
                        "/etc/ssl/certs/tls.crt",
                        "--ssl-keyfile",
                        "/etc/ssl/certs/tls.key",
                    ],
                    "resources": {
                        "limits": {"cpu": "2", "memory": "16Gi"},
                        "requests": {"cpu": "1", "memory": "8Gi"},
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
                        "--disable-log-requests",
                        "--enable-ssl-refresh",
                        "--ssl-certfile",
                        "/etc/ssl/certs/tls.crt",
                        "--ssl-keyfile",
                        "/etc/ssl/certs/tls.key",
                    ],
                    "resources": {
                        "limits": {"cpu": "2", "memory": "16Gi"},
                        "requests": {"cpu": "1", "memory": "8Gi"},
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

    try:
        # Validate base_refs defined in the test fixture exist in LLMINFERENCESERVICE_CONFIGS
        missing_refs = [
            ref for ref in tc.base_refs if ref not in LLMINFERENCESERVICE_CONFIGS
        ]
        if missing_refs:
            raise ValueError(
                f"Missing base_refs in LLMINFERENCESERVICE_CONFIGS: {missing_refs}"
            )

        service_name = generate_service_name(request.node.name, tc.base_refs)
        tc.model_name = _get_model_name_from_configs(tc.base_refs)

        # Create unique configs for this test
        unique_base_refs = []
        for base_ref in tc.base_refs:
            unique_config_name = generate_k8s_safe_suffix(base_ref, [service_name])
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
                name=service_name, namespace=KSERVE_TEST_NAMESPACE
            ),
            spec={
                "baseRefs": [{"name": base_ref} for base_ref in unique_base_refs],
            },
        )

        yield tc

    finally:
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
        if config_name.startswith("model-"):
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
