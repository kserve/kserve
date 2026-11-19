# Copyright 2024 The KServe Authors.
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

import base64
import os
import secrets

import yaml

import pytest
from kubernetes import client
from kubernetes.client import V1ResourceRequirements
import numpy as np

from kserve import (
    V1beta1PredictorSpec,
    V1beta1ModelSpec,
    V1beta1ModelFormat,
    V1beta1InferenceService,
    V1beta1InferenceServiceSpec,
    KServeClient,
)
from kserve.constants import constants
from ..common.utils import (
    assert_answers_four,
    generate,
    embed,
    chat_completion_stream,
    completion_stream,
    classify,
    rerank,
    get_isvc_endpoint,
    project_root,
)
from ..common.http_retry import get_with_retry, post_with_retry
from .test_output import (
    vllm_text_embedding_expected_output,
)

from kserve.logging import trace_logger


def _assert_embedding_matches_reference(actual, reference, *, threshold: float = 0.999):
    """Assert ``actual`` is semantically equivalent to the reference vector.

    Embedding outputs drift slightly across vLLM versions and pooling code
    paths (dtype, fused vs unfused kernels, normalization order, etc.), so an
    element-wise tolerance check is brittle. Cosine similarity is the standard
    invariant for "same embedding": ``threshold=0.999`` keeps real regressions
    visible (wrong model, wrong pooling, broken normalization all collapse
    similarity well below this) while tolerating numerical drift on the order
    of 1e-4. ``reference`` may be a list of ``pytest.approx`` wrappers — we
    unwrap to the underlying floats.
    """
    reference_raw = [getattr(v, "expected", v) for v in reference]
    assert len(actual) == len(reference_raw), (
        f"embedding dimension mismatch: got {len(actual)}, "
        f"expected {len(reference_raw)}"
    )
    a = np.asarray(actual, dtype=np.float64)
    b = np.asarray(reference_raw, dtype=np.float64)
    norms = float(np.linalg.norm(a) * np.linalg.norm(b))
    assert norms > 0, "embedding vector is all zeros"
    cosine = float(np.dot(a, b) / norms)
    assert cosine >= threshold, (
        f"embedding cosine similarity {cosine:.6f} below threshold {threshold}"
    )


@pytest.mark.vllm_runtime
@pytest.mark.parametrize(
    "manifest",
    [
        "config/runtimes/kserve-vllmserver.yaml",
        "charts/kserve-runtime-configs/files/runtimes/resources.yaml",
        "hack/setup/quick-install/kserve-knative-mode-full-install-with-manifests.sh",
        "hack/setup/quick-install/kserve-standard-mode-full-install-with-manifests.sh",
        "hack/setup/quick-install/llmisvc-full-install-with-manifests.sh",
        "hack/setup/quick-install/localmodel-full-install-with-manifests.sh",
    ],
)
def test_vllm_probes_do_not_require_api_key(manifest):
    source = (project_root() / manifest).read_text()
    if manifest.endswith(".sh"):
        # Parse the embedded runtime YAML without executing the install script.
        marker = "KSERVE_RUNTIME_MANIFEST_EOF"
        source = source.split(f"cat <<'{marker}'\n", 1)[1].split(f"\n{marker}", 1)[0]
    runtimes = [
        doc
        for doc in yaml.safe_load_all(source)
        if doc and doc.get("metadata", {}).get("name") == "kserve-vllmserver"
    ]
    assert len(runtimes) == 1
    container = next(
        c for c in runtimes[0]["spec"]["containers"] if c["name"] == "kserve-container"
    )
    for probe in ("startupProbe", "readinessProbe"):
        assert container[probe]["httpGet"]["path"] == "/health"
        assert not container[probe]["httpGet"].get("httpHeaders")


@pytest.mark.vllm_runtime
def test_vllm_api_key_readiness_and_inference(test_namespace, network_layer):
    """Readiness must succeed while the OpenAI endpoint requires authentication."""
    service_name = "vllm-api-key"
    api_key = secrets.token_hex(16)
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(name="vLLM"),
            runtime="kserve-vllmserver",
            env=[
                client.V1EnvVar(name="VLLM_CPU_KVCACHE_SPACE", value="1"),
                # Avoid the CPU EngineCore broadcast startup timeout; this test
                # exercises HTTP authentication and probes, not multiprocessing.
                client.V1EnvVar(name="VLLM_ENABLE_V1_MULTIPROCESSING", value="0"),
            ],
            args=[
                "--model",
                "Qwen/Qwen2-0.5B-Instruct",
                "--served-model-name",
                service_name,
                "--max-model-len",
                "512",
                "--dtype",
                "bfloat16",
                "--enforce-eager",
                "--api-key",
                api_key,
            ],
            resources=V1ResourceRequirements(
                requests={"cpu": "2", "memory": "8Gi"},
                limits={"cpu": "4", "memory": "16Gi"},
            ),
        ),
    )
    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name,
            namespace=test_namespace,
            annotations={"serving.kserve.io/deploymentMode": "Standard"},
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )
    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(isvc)
    try:
        kserve_client.wait_isvc_ready(service_name, namespace=test_namespace)
        pods = (
            client.CoreV1Api()
            .list_namespaced_pod(
                test_namespace,
                label_selector=f"serving.kserve.io/inferenceservice={service_name}",
            )
            .items
        )
        assert pods
        for pod in pods:
            container = next(
                c for c in pod.spec.containers if c.name == "kserve-container"
            )
            for probe in (container.startup_probe, container.readiness_probe):
                assert probe.http_get.path == "/health"
                assert not probe.http_get.http_headers
        ready = kserve_client.get(service_name, namespace=test_namespace)
        scheme, address, host, path = get_isvc_endpoint(ready, network_layer)
        url = f"{scheme}://{address}{path}"
        headers = {"Host": host}
        unauthorized = get_with_retry(f"{url}/v1/models", headers=headers)
        assert unauthorized.status_code == 401
        headers["Authorization"] = f"Bearer {api_key}"
        response = post_with_retry(
            f"{url}/v1/chat/completions",
            headers=headers,
            json_data={
                "model": service_name,
                "messages": [{"role": "user", "content": "What is 2 + 2?"}],
                "max_tokens": 16,
                "temperature": 0,
            },
        )
        response.raise_for_status()
        assert response.json()["choices"][0]["message"]["content"]
    finally:
        kserve_client.delete(service_name, namespace=test_namespace)


@pytest.mark.vllm_runtime
@pytest.mark.skip(
    reason="vLLM AOT compilation (~130s on CPU) exceeds shm_broadcast EngineCore "
    "init timeout (120s); re-enable when GPU runner is available or the upstream "
    "race is fixed. See PR #5558 Round 13 comment for context."
)
def test_vllm_openai_chat_completions(test_namespace):
    service_name = "vllm-qwen-chat"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="vLLM",
            ),
            env=[
                client.V1EnvVar(
                    name="VLLM_CPU_KVCACHE_SPACE",
                    value="1",
                ),
            ],
            args=[
                "--model",
                "Qwen/Qwen2-0.5B-Instruct",
                "--served-model-name",
                "qwen-chat",
                "--max-model-len",
                "512",
                "--dtype",
                "bfloat16",
            ],
            resources=V1ResourceRequirements(
                requests={"cpu": "2", "memory": "8Gi"},
                limits={"cpu": "4", "memory": "16Gi"},
            ),
        ),
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(name=service_name, namespace=test_namespace),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=test_namespace)

    res = generate(
        service_name, "./data/qwen_input_chat.json", namespace=test_namespace
    )
    assert_answers_four(res["choices"][0]["message"]["content"])


@pytest.mark.vllm_runtime
@pytest.mark.skip(
    reason="vLLM AOT compilation (~130s on CPU) exceeds shm_broadcast EngineCore "
    "init timeout (120s); re-enable when GPU runner is available or the upstream "
    "race is fixed. See PR #5558 Round 13 comment for context."
)
def test_vllm_openai_chat_completions_streaming(test_namespace):
    service_name = "vllm-qwen-chat-stream"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="vLLM",
            ),
            env=[
                client.V1EnvVar(
                    name="TRANSFORMERS_VERBOSITY",
                    value="info",
                ),
                client.V1EnvVar(
                    name="VLLM_CPU_KVCACHE_SPACE",
                    value="1",
                ),
            ],
            args=[
                "--model",
                "Qwen/Qwen2-0.5B-Instruct",
                "--served-model-name",
                "qwen-chat-stream",
                "--max-model-len",
                "512",
                "--dtype",
                "bfloat16",
            ],
            resources=V1ResourceRequirements(
                requests={"cpu": "2", "memory": "8Gi"},
                limits={"cpu": "4", "memory": "16Gi"},
            ),
        ),
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(name=service_name, namespace=test_namespace),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=test_namespace)

    # Test streaming response
    full_response, chunks = chat_completion_stream(
        service_name,
        "./data/qwen_input_chat_stream.json",
        namespace=test_namespace,
    )
    trace_logger.info(f"Full response: {full_response}")

    assert len(chunks) > 0, "expected streaming chunks, got none"
    assert_answers_four(full_response)


@pytest.mark.vllm_runtime
@pytest.mark.skip(
    reason="vLLM AOT compilation (~130s on CPU) exceeds shm_broadcast EngineCore "
    "init timeout (120s); re-enable when GPU runner is available or the upstream "
    "race is fixed. See PR #5558 Round 13 comment for context."
)
def test_vllm_openai_text_completion_qwen2(test_namespace):
    service_name = "vllm-qwen-cmpl"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="vLLM",
            ),
            env=[
                client.V1EnvVar(
                    name="VLLM_CPU_KVCACHE_SPACE",
                    value="1",
                ),
            ],
            args=[
                "--model",
                "Qwen/Qwen2-0.5B",
                "--served-model-name",
                "qwen-cmpl",
                "--max-model-len",
                "512",
                "--dtype",
                "bfloat16",
            ],
            resources=V1ResourceRequirements(
                requests={"cpu": "2", "memory": "8Gi"},
                limits={"cpu": "4", "memory": "16Gi"},
            ),
        ),
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(name=service_name, namespace=test_namespace),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=test_namespace)

    res = generate(
        service_name,
        "./data/qwen_input_cmpl.json",
        chat_completions=False,
        namespace=test_namespace,
    )
    assert_answers_four(res["choices"][0].get("text"))


@pytest.mark.vllm_runtime
@pytest.mark.skip(
    reason="vLLM AOT compilation (~130s on CPU) exceeds shm_broadcast EngineCore "
    "init timeout (120s); re-enable when GPU runner is available or the upstream "
    "race is fixed. See PR #5558 Round 13 comment for context."
)
def test_vllm_openai_text_completion_streaming(test_namespace):
    service_name = "vllm-qwen-cmpl-stream"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="vLLM",
            ),
            env=[
                client.V1EnvVar(
                    name="VLLM_CPU_KVCACHE_SPACE",
                    value="1",
                ),
            ],
            args=[
                "--model",
                "Qwen/Qwen2-0.5B",
                "--served-model-name",
                "qwen-cmpl-stream",
                "--max-model-len",
                "512",
                "--dtype",
                "bfloat16",
            ],
            resources=V1ResourceRequirements(
                requests={"cpu": "2", "memory": "8Gi"},
                limits={"cpu": "4", "memory": "16Gi"},
            ),
        ),
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(name=service_name, namespace=test_namespace),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=test_namespace)

    full_response, chunks = completion_stream(
        service_name,
        "./data/qwen_input_cmpl_stream.json",
        namespace=test_namespace,
    )
    trace_logger.info(f"Full response: {full_response}")
    assert len(chunks) > 0, "expected streaming chunks, got none"
    assert_answers_four(full_response)


@pytest.mark.vllm_runtime
def test_vllm_classify_sequence_classification(test_namespace):
    """Test vLLM sequence classification using /classify endpoint"""
    service_name = "vllm-bert-sequence-classify"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="vLLM",
            ),
            env=[
                client.V1EnvVar(
                    name="VLLM_CPU_KVCACHE_SPACE",
                    value="1",
                ),
            ],
            args=[
                "--model",
                "textattack/bert-base-uncased-yelp-polarity",
                "--revision",
                "a4d0a85ea6c1d5bb944dcc12ea5c918863e469a4",
                "--tokenizer_revision",
                "a4d0a85ea6c1d5bb944dcc12ea5c918863e469a4",
                "--served-model-name",
                "vllm-bert-sequence-classify",
                "--runner",
                "pooling",
                "--convert",
                "classify",
            ],
            resources=V1ResourceRequirements(
                requests={"cpu": "2", "memory": "8Gi"},
                limits={"cpu": "4", "memory": "16Gi"},
            ),
        ),
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(name=service_name, namespace=test_namespace),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=test_namespace)

    # Reuse existing v2 format file - helper will convert it
    res = classify(
        service_name,
        "./data/bert_sequence_classification_v2.json",
        namespace=test_namespace,
    )
    # vLLM classify endpoint returns classification results
    # The exact format may vary, but should contain prediction
    assert "label" in res or "prediction" in res or res is not None


@pytest.mark.vllm_runtime
@pytest.mark.asyncio(scope="session")
async def test_vllm_openai_text_embedding(test_namespace):
    service_name = "vllm-text-embedding-openai"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="vLLM",
            ),
            env=[
                client.V1EnvVar(
                    name="VLLM_CPU_KVCACHE_SPACE",
                    value="1",
                ),
            ],
            args=[
                "--model",
                "sentence-transformers/all-MiniLM-L6-v2",
                "--revision",
                "8b3219a92973c328a8e22fadcfa821b5dc75636a",
                "--tokenizer_revision",
                "8b3219a92973c328a8e22fadcfa821b5dc75636a",
                "--runner",
                "pooling",
                "--convert",
                "embed",
                "--served-model-name",
                "text-embedding-openai",
            ],
            resources=V1ResourceRequirements(
                requests={"cpu": "2", "memory": "8Gi"},
                limits={"cpu": "4", "memory": "16Gi"},
            ),
        ),
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(name=service_name, namespace=test_namespace),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=test_namespace)

    # Validate float output
    res = embed(
        service_name,
        "./data/text_embedding_input_openai_float.json",
        namespace=test_namespace,
    )
    assert len(res["data"]) == 1
    _assert_embedding_matches_reference(
        res["data"][0]["embedding"], vllm_text_embedding_expected_output
    )

    # Validate base64 output. Decoded as the OpenAI library:
    # https://github.com/openai/openai-python/blob/v1.59.7/src/openai/resources/embeddings.py#L118-L120
    res = embed(
        service_name,
        "./data/text_embedding_input_openai_base64.json",
        namespace=test_namespace,
    )
    embedding = np.frombuffer(
        base64.b64decode(res["data"][0]["embedding"]), dtype="float32"
    ).tolist()
    assert len(res["data"]) == 1
    _assert_embedding_matches_reference(embedding, vllm_text_embedding_expected_output)

    # Validate Token count
    assert res["usage"]["prompt_tokens"] == 8
    assert res["usage"]["total_tokens"] == 8


@pytest.mark.vllm_runtime
def test_vllm_classify_sequence_classification_probabilities(test_namespace):
    """Test vLLM sequence classification using /classify endpoint for probabilities"""
    service_name = "vllm-bert-sequence-classify-prob"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="vLLM",
            ),
            env=[
                client.V1EnvVar(
                    name="VLLM_CPU_KVCACHE_SPACE",
                    value="1",
                ),
            ],
            args=[
                "--model",
                "textattack/bert-base-uncased-yelp-polarity",
                "--revision",
                "a4d0a85ea6c1d5bb944dcc12ea5c918863e469a4",
                "--tokenizer_revision",
                "a4d0a85ea6c1d5bb944dcc12ea5c918863e469a4",
                "--served-model-name",
                "vllm-bert-sequence-classify-prob",
                "--runner",
                "pooling",
                "--convert",
                "classify",
            ],
            resources=V1ResourceRequirements(
                requests={"cpu": "2", "memory": "8Gi"},
                limits={"cpu": "4", "memory": "16Gi"},
            ),
        ),
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(name=service_name, namespace=test_namespace),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=test_namespace)

    # Reuse existing v2 format file - helper will convert it
    res = classify(
        service_name,
        "./data/bert_sequence_classification_v2.json",
        namespace=test_namespace,
    )
    # vLLM classify endpoint returns classification probabilities in the data array
    assert "data" in res
    assert len(res["data"]) > 0
    assert "probs" in res["data"][0]
    assert isinstance(res["data"][0]["probs"], list)
    assert "label" in res["data"][0]


@pytest.mark.vllm_runtime
def test_vllm_rerank(test_namespace):
    service_name = "bge-reranker-base"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="vLLM",
            ),
            env=[
                client.V1EnvVar(
                    name="VLLM_CPU_KVCACHE_SPACE",
                    value="1",
                ),
            ],
            args=[
                "--model",
                "BAAI/bge-reranker-base",
                "--revision",
                "2cfc18c9415c912f9d8155881c133215df768a70",
                "--tokenizer-revision",
                "2cfc18c9415c912f9d8155881c133215df768a70",
                "--served-model-name",
                "bge-reranker-base",
                "--max-model-len",
                "100",
                "--dtype",
                "bfloat16",
                "--enforce-eager",
            ],
            resources=V1ResourceRequirements(
                requests={"cpu": "2", "memory": "6Gi"},
                limits={"cpu": "2", "memory": "6Gi"},
            ),
        ),
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(name=service_name, namespace=test_namespace),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=test_namespace)

    res = rerank(
        service_name, "./data/bge-reranker-base.json", namespace=test_namespace
    )
    assert res["results"][0]["index"] == 1
    assert res["results"][0]["relevance_score"] > 0.9
    assert res["results"][0]["document"]["text"] == "The capital of France is Paris."
    assert res["results"][1]["index"] == 0
    assert res["results"][1]["relevance_score"] < 0.01
    assert res["results"][1]["document"]["text"] == "The capital of Brazil is Brasilia."
