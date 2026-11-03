# Copyright 2026 The KServe Authors.
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

"""ModelExpress e2e tests.

cluster_cpu tests cover admission and the rendered engine pods.
cluster_nvidia tests load models through the server installed by
test/scripts/gh-actions/setup-modelexpress.sh and need MODELEXPRESS_VLLM_CUDA_IMAGE.
"""

from __future__ import annotations

import os
import time
from typing import Dict, List, Optional

import pytest
from kserve import KServeClient, constants
from kubernetes import client
from kubernetes.stream import stream as k8s_stream

from .diagnostic import collect_diagnostics
from .fixtures import (
    MODELEXPRESS_ADDRESS,
    MODELEXPRESS_MODEL_URI,
    MODELEXPRESS_VLLM_CUDA_IMAGE,
    OPT_125M_MODEL_URI,
    generate_test_id,
    inject_k8s_proxy,
)
from .logging import log_execution, logger
from .test_llm_inference_service import (
    TestCase,
    create_llmisvc,
    get_llmisvc,
    maybe_delete_llmisvc,
    wait_for,
    wait_for_llm_isvc_ready,
    wait_for_model_response,
)

KSERVE_PLURAL_LLMINFERENCESERVICE = "llminferenceservices"

MODE_ANNOTATION = "serving.kserve.io/exp-modelexpress-mode"
ADDRESS_ANNOTATION = "serving.kserve.io/exp-modelexpress-address"
AUDIENCE_ANNOTATION = "serving.kserve.io/exp-modelexpress-token-audience"

WEIGHT_SUFFIXES = (".safetensors", ".bin", ".pt", ".pth", ".gguf", ".h5", ".msgpack")

requires_cuda_image = pytest.mark.skipif(
    not MODELEXPRESS_VLLM_CUDA_IMAGE,
    reason="MODELEXPRESS_VLLM_CUDA_IMAGE is not set",
)


def _kserve_client() -> KServeClient:
    inject_k8s_proxy()
    return KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config"),
        client_configuration=client.Configuration(),
    )


def _namespace(test_case: TestCase) -> str:
    assert test_case.namespace, "test case has no namespace"
    return test_case.namespace


def _annotate(test_case: TestCase, annotations: Dict[str, str]) -> None:
    metadata = test_case.llm_service.metadata
    metadata.annotations = {**(metadata.annotations or {}), **annotations}


def _condition(kserve_client: KServeClient, test_case: TestCase, cond_type: str):
    svc = get_llmisvc(
        kserve_client,
        test_case.llm_service.metadata.name,
        _namespace(test_case),
        test_case.llm_service.api_version.split("/")[1],
    )
    for cond in svc.get("status", {}).get("conditions", []):
        if cond.get("type") == cond_type:
            return cond
    return None


def _wait_for_modelexpress_condition(
    kserve_client: KServeClient,
    test_case: TestCase,
    status: str,
    reason: Optional[str] = None,
    timeout: float = 120,
):
    def assertion():
        cond = _condition(kserve_client, test_case, "ModelExpressReady")
        assert cond is not None, "ModelExpressReady condition is not set"
        assert cond.get("status") == status, f"ModelExpressReady: {cond}"
        if reason is not None:
            assert cond.get("reason") == reason, f"ModelExpressReady: {cond}"
        return cond

    return wait_for(assertion, timeout=timeout, interval=2)


def _deployment_name(test_case: TestCase) -> str:
    return f"{test_case.llm_service.metadata.name}-kserve"


def _wait_for_deployment(namespace: str, name: str, timeout: float = 180):
    apps = client.AppsV1Api()

    def assertion():
        try:
            return apps.read_namespaced_deployment(name, namespace)
        except client.rest.ApiException as e:
            assert e.status != 404, f"deployment {namespace}/{name} not found"
            raise

    return wait_for(assertion, timeout=timeout, interval=2)


def _container(pod_spec, name: str):
    for c in pod_spec.containers or []:
        if c.name == name:
            return c
    raise AssertionError(f"container {name} not found")


def _env(container) -> Dict[str, client.V1EnvVar]:
    return {e.name: e for e in container.env or []}


def _engine_pods(namespace: str, service_name: str) -> List[client.V1Pod]:
    pods = client.CoreV1Api().list_namespaced_pod(
        namespace,
        label_selector=(
            f"app.kubernetes.io/name={service_name},"
            "app.kubernetes.io/component=llminferenceservice-workload"
        ),
    )
    return [p for p in pods.items if p.metadata.deletion_timestamp is None]


def _exec(namespace: str, pod: str, command: List[str]) -> str:
    return k8s_stream(
        client.CoreV1Api().connect_get_namespaced_pod_exec,
        pod,
        namespace,
        container="main",
        command=command,
        stderr=True,
        stdout=True,
        stdin=False,
        tty=False,
    )


def _strategies(namespace: str, pod: str) -> Dict[str, List[str]]:
    """Return the ModelExpress strategies a pod tried and the ones that failed."""
    response = client.CoreV1Api().read_namespaced_pod_log(
        pod, namespace, container="main", _preload_content=False
    )
    log = response.data.decode("utf-8", "replace")
    tried, failed = [], []
    for line in log.splitlines():
        if "Trying strategy: " in line:
            tried.append(line.rsplit("Trying strategy: ", 1)[1].strip())
        elif "] Strategy " in line and (
            " failed, trying next" in line or " raised unexpected error" in line
        ):
            failed.append(line.split("] Strategy ", 1)[1].split(" ", 1)[0])
    return {"tried": tried, "failed": failed}


def _loaded_by(namespace: str, pod: str) -> Optional[str]:
    result = _strategies(namespace, pod)
    for name in result["tried"]:
        if name not in result["failed"]:
            return name
    return None


@pytest.mark.llmisvc_modelexpress
@pytest.mark.cluster_cpu
@pytest.mark.parametrize(
    "annotations,model_uri,lora_uri,expected",
    [
        pytest.param(
            {MODE_ANNOTATION: "native"},
            "pvc://models/opt-125m",
            None,
            "spec.model.uri",
            id="native-rejects-pvc",
        ),
        pytest.param(
            {MODE_ANNOTATION: "native"},
            "s3://example-models/facebook/opt-125m",
            "hf://org/adapter",
            "only pvc:// LoRA adapters",
            id="native-s3-rejects-remote-lora",
        ),
        pytest.param(
            {MODE_ANNOTATION: "p2p"},
            "s3://example-models/facebook/opt-125m",
            None,
            MODE_ANNOTATION,
            id="rejects-unknown-mode",
        ),
        pytest.param(
            {ADDRESS_ANNOTATION: "modelexpress:8001"},
            "s3://example-models/facebook/opt-125m",
            None,
            MODE_ANNOTATION,
            id="rejects-options-without-mode",
        ),
    ],
)
@log_execution
def test_modelexpress_admission(
    test_namespace, annotations, model_uri, lora_uri, expected
):
    kserve_client = _kserve_client()
    model = {"uri": model_uri, "name": "facebook/opt-125m"}
    if lora_uri:
        model["lora"] = {"adapters": [{"name": "adapter", "uri": lora_uri}]}
    body = {
        "apiVersion": "serving.kserve.io/v1alpha1",
        "kind": "LLMInferenceService",
        "metadata": {
            "name": "mx-admission",
            "namespace": test_namespace,
            "annotations": annotations,
        },
        "spec": {"model": model},
    }

    with pytest.raises(client.rest.ApiException) as exc_info:
        kserve_client.api_instance.create_namespaced_custom_object(
            constants.KSERVE_GROUP,
            "v1alpha1",
            test_namespace,
            KSERVE_PLURAL_LLMINFERENCESERVICE,
            body,
        )

    assert exc_info.value.status == 422, exc_info.value
    assert expected in exc_info.value.body, exc_info.value.body


@pytest.mark.llmisvc_modelexpress
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
                service_name="mx-native-render",
            ),
            marks=[pytest.mark.cluster_cpu, pytest.mark.cluster_single_node],
        ),
    ],
    indirect=["test_case"],
    ids=generate_test_id,
)
@log_execution
def test_modelexpress_native_renders_engine_pods(test_case: TestCase):
    """Native mode renders ModelExpress into the engine pod without KServe's weight download."""
    kserve_client = _kserve_client()
    namespace = _namespace(test_case)
    _annotate(
        test_case,
        {
            MODE_ANNOTATION: "native",
            ADDRESS_ANNOTATION: MODELEXPRESS_ADDRESS,
            AUDIENCE_ANNOTATION: "modelexpress",
        },
    )
    service_name = test_case.llm_service.metadata.name
    test_failed = False
    try:
        create_llmisvc(kserve_client, test_case.llm_service)
        _wait_for_modelexpress_condition(kserve_client, test_case, "True")

        deployment = _wait_for_deployment(namespace, _deployment_name(test_case))
        pod_spec = deployment.spec.template.spec
        main = _container(pod_spec, "main")
        env = _env(main)

        assert pod_spec.service_account_name == f"{service_name}-kserve"
        client.CoreV1Api().read_namespaced_service_account(
            pod_spec.service_account_name, namespace
        )

        args = main.args or []
        assert "--load-format" in args, args
        assert args[args.index("--load-format") + 1] == "modelexpress", args
        assert env["MX_SERVER_ADDRESS"].value == MODELEXPRESS_ADDRESS
        assert env["MODEL_EXPRESS_URL"].value == MODELEXPRESS_ADDRESS
        assert env["MX_MODEL_REVISION"].value
        assert env["MX_WORKER_HOST"].value_from.field_ref.field_path == "status.podIP"
        assert env["MX_AUTH_TOKEN_PATH"].value == "/var/run/secrets/modelexpress/token"
        token = [v for v in pod_spec.volumes if v.name == "modelexpress-token"]
        assert token, [v.name for v in pod_spec.volumes]
        projection = token[0].projected.sources[0].service_account_token
        assert projection.audience == "modelexpress"

        init = {c.name: c for c in pod_spec.init_containers or []}
        if OPT_125M_MODEL_URI.startswith("s3://"):
            assert env["MX_MODEL_URI"].value == OPT_125M_MODEL_URI
            assert "KSERVE_MODEL_ARGS" not in env
            ignore = _env(init["storage-initializer"])["STORAGE_IGNORE_PATTERNS"].value
            assert '"*.safetensors"' in ignore, ignore
            assert "AWS_ACCESS_KEY_ID" in env, sorted(env)
        elif OPT_125M_MODEL_URI.startswith("hf://"):
            assert env["KSERVE_MODEL_ARGS"].value.startswith("facebook/opt-125m")
            assert env["MODEL_EXPRESS_NO_SHARED_STORAGE"].value == "1"
            assert (
                env["MODEL_EXPRESS_CACHE_DIRECTORY"].value == env["HF_HUB_CACHE"].value
            )
            assert "storage-initializer" not in init, sorted(init)
        else:
            pytest.fail(f"unexpected OPT_125M_MODEL_URI {OPT_125M_MODEL_URI}")
    except Exception:
        test_failed = True
        collect_diagnostics(
            service_name, namespace, kserve_client=kserve_client, log=logger.info
        )
        raise
    finally:
        maybe_delete_llmisvc(kserve_client, test_case.llm_service, test_failed)


@pytest.mark.llmisvc_modelexpress
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
                service_name="mx-native-unresolved",
            ),
            marks=[pytest.mark.cluster_cpu, pytest.mark.cluster_single_node],
        ),
    ],
    indirect=["test_case"],
    ids=generate_test_id,
)
@log_execution
def test_modelexpress_native_without_server_renders_nothing(test_case: TestCase):
    kserve_client = _kserve_client()
    namespace = _namespace(test_case)
    _annotate(test_case, {MODE_ANNOTATION: "native"})
    test_failed = False
    try:
        create_llmisvc(kserve_client, test_case.llm_service)
        _wait_for_modelexpress_condition(
            kserve_client, test_case, "False", reason="ServerNotResolved"
        )

        deadline = time.time() + 20
        while time.time() < deadline:
            try:
                client.AppsV1Api().read_namespaced_deployment(
                    _deployment_name(test_case), namespace
                )
                pytest.fail(
                    "native mode rendered a workload without a ModelExpress server"
                )
            except client.rest.ApiException as e:
                if e.status != 404:
                    raise
            time.sleep(2)
    except Exception:
        test_failed = True
        raise
    finally:
        maybe_delete_llmisvc(kserve_client, test_case.llm_service, test_failed)


@pytest.mark.llmisvc_modelexpress
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
                service_name="mx-layered-unresolved",
            ),
            marks=[pytest.mark.cluster_cpu, pytest.mark.cluster_single_node],
        ),
    ],
    indirect=["test_case"],
    ids=generate_test_id,
)
@log_execution
def test_modelexpress_layered_without_server_serves(test_case: TestCase):
    """Layered mode degrades to KServe's own model delivery when no server is configured."""
    kserve_client = _kserve_client()
    namespace = _namespace(test_case)
    _annotate(test_case, {MODE_ANNOTATION: "layered"})
    service_name = test_case.llm_service.metadata.name
    test_failed = False
    try:
        create_llmisvc(kserve_client, test_case.llm_service)
        _wait_for_modelexpress_condition(
            kserve_client, test_case, "False", reason="ServerNotResolved"
        )
        wait_for_llm_isvc_ready(
            kserve_client, test_case.llm_service, test_case.wait_timeout
        )
        wait_for_model_response(kserve_client, test_case, test_case.wait_timeout)

        deployment = _wait_for_deployment(namespace, _deployment_name(test_case))
        main = _container(deployment.spec.template.spec, "main")
        assert "--load-format" not in (main.args or [])
        assert "MX_SERVER_ADDRESS" not in _env(main)
    except Exception:
        test_failed = True
        collect_diagnostics(
            service_name, namespace, kserve_client=kserve_client, log=logger.info
        )
        raise
    finally:
        maybe_delete_llmisvc(kserve_client, test_case.llm_service, test_failed)


@pytest.mark.llmisvc_modelexpress
@requires_cuda_image
@pytest.mark.parametrize(
    "test_case",
    [
        pytest.param(
            TestCase(
                base_refs=[
                    "router-managed",
                    "workload-single-gpu-modelexpress",
                    "model-modelexpress",
                ],
                prompt="KServe is a",
                service_name="mx-native-serve",
            ),
            marks=[pytest.mark.cluster_nvidia, pytest.mark.cluster_single_node],
        ),
    ],
    indirect=["test_case"],
    ids=generate_test_id,
)
@log_execution
def test_modelexpress_native_serves(test_case: TestCase):
    """Native mode serves a model whose weights never land on the pod's disk."""
    kserve_client = _kserve_client()
    namespace = _namespace(test_case)
    _annotate(
        test_case,
        {MODE_ANNOTATION: "native", ADDRESS_ANNOTATION: MODELEXPRESS_ADDRESS},
    )
    service_name = test_case.llm_service.metadata.name
    test_failed = False
    try:
        create_llmisvc(kserve_client, test_case.llm_service)
        _wait_for_modelexpress_condition(kserve_client, test_case, "True")
        wait_for_llm_isvc_ready(
            kserve_client, test_case.llm_service, test_case.wait_timeout
        )
        wait_for_model_response(kserve_client, test_case, test_case.wait_timeout)

        pods = _engine_pods(namespace, service_name)
        assert len(pods) == 1, [p.metadata.name for p in pods]
        pod = pods[0].metadata.name
        if MODELEXPRESS_MODEL_URI.startswith("s3://"):
            listing = _exec(namespace, pod, ["ls", "-1", "/mnt/models"]).split()
            assert "config.json" in listing, listing
            assert not [f for f in listing if f.endswith(WEIGHT_SUFFIXES)], listing
            assert _loaded_by(namespace, pod) == "model_streamer", _strategies(
                namespace, pod
            )
        else:
            assert _loaded_by(namespace, pod) == "server-cache", _strategies(
                namespace, pod
            )
    except Exception:
        test_failed = True
        collect_diagnostics(
            service_name, namespace, kserve_client=kserve_client, log=logger.info
        )
        raise
    finally:
        maybe_delete_llmisvc(kserve_client, test_case.llm_service, test_failed)


@pytest.mark.llmisvc_modelexpress
@requires_cuda_image
@pytest.mark.parametrize(
    "test_case",
    [
        pytest.param(
            TestCase(
                base_refs=[
                    "router-managed",
                    "workload-single-gpu-modelexpress",
                    "model-modelexpress",
                ],
                prompt="KServe is a",
                service_name="mx-p2p",
            ),
            marks=[pytest.mark.cluster_nvidia, pytest.mark.cluster_multi_node],
        ),
    ],
    indirect=["test_case"],
    ids=generate_test_id,
)
@log_execution
def test_modelexpress_second_replica_loads_from_peer(test_case: TestCase):
    """A replica added after the first is up loads its weights from that replica over RDMA."""
    kserve_client = _kserve_client()
    namespace = _namespace(test_case)
    _annotate(
        test_case,
        {MODE_ANNOTATION: "native", ADDRESS_ANNOTATION: MODELEXPRESS_ADDRESS},
    )
    service_name = test_case.llm_service.metadata.name
    version = test_case.llm_service.api_version.split("/")[1]
    test_failed = False
    try:
        create_llmisvc(kserve_client, test_case.llm_service)
        wait_for_llm_isvc_ready(
            kserve_client, test_case.llm_service, test_case.wait_timeout
        )
        wait_for_model_response(kserve_client, test_case, test_case.wait_timeout)
        first = {p.metadata.name for p in _engine_pods(namespace, service_name)}
        assert len(first) == 1, first

        kserve_client.api_instance.patch_namespaced_custom_object(
            constants.KSERVE_GROUP,
            version,
            namespace,
            KSERVE_PLURAL_LLMINFERENCESERVICE,
            service_name,
            {"spec": {"replicas": 2}},
        )

        def second_ready():
            pods = _engine_pods(namespace, service_name)
            new = [p for p in pods if p.metadata.name not in first]
            assert new, "second replica not created"
            statuses = new[0].status.container_statuses or []
            assert any(s.name == "main" and s.ready for s in statuses), (
                "second replica not ready"
            )
            return new[0].metadata.name

        second = wait_for(second_ready, timeout=test_case.wait_timeout, interval=5)
        assert _loaded_by(namespace, second) == "rdma", _strategies(namespace, second)
        wait_for_model_response(kserve_client, test_case, test_case.wait_timeout)
    except Exception:
        test_failed = True
        collect_diagnostics(
            service_name, namespace, kserve_client=kserve_client, log=logger.info
        )
        raise
    finally:
        maybe_delete_llmisvc(kserve_client, test_case.llm_service, test_failed)
