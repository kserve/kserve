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

"""E2E tests for LocalModelCache oci:// import (public + private pull secret)."""

import base64
import hashlib
import json
import os
import socket
import subprocess
import time

import pytest
from kubernetes import client
from kubernetes.client import (
    V1LocalVolumeSource,
    V1PersistentVolumeClaimSpec,
    V1PersistentVolumeSpec,
    V1ResourceRequirements,
    V1VolumeNodeAffinity,
)
from kubernetes.client.exceptions import ApiException

from kserve import constants
from kserve.api.kserve_client import KServeClient
from kserve.models.v1alpha1_local_model_cache import V1alpha1LocalModelCache
from kserve.models.v1alpha1_local_model_cache_spec import V1alpha1LocalModelCacheSpec
from kserve.models.v1alpha1_local_model_node_group import V1alpha1LocalModelNodeGroup
from kserve.models.v1alpha1_local_model_node_group_spec import (
    V1alpha1LocalModelNodeGroupSpec,
)

from ..common.utils import KSERVE_NAMESPACE

OCI_FETCH_TEST_IMAGE = "ghcr.io/kliukovkin/kserve-oci-test-fixture:v1"
EXPECTED_MODEL_FILE = "model.joblib"
JOB_NAMESPACE = "kserve-localmodel-jobs"
GROUP = "serving.kserve.io"
VERSION = "v1alpha1"
CACHE_DOWNLOAD_TIMEOUT = 600
REGISTRY_NAME = "oci-auth-registry"


def _storage_key(uri: str) -> str:
    return hashlib.sha256(uri.encode()).hexdigest()[:16]


def _worker_node_names(core: client.CoreV1Api) -> list[str]:
    labeled = core.list_node(label_selector="kserve/localmodel=worker")
    if labeled.items:
        return [n.metadata.name for n in labeled.items]
    names = []
    for node in core.list_node().items:
        labels = node.metadata.labels or {}
        if labels.get("node-role.kubernetes.io/control-plane") or labels.get(
            "node-role.kubernetes.io/master"
        ):
            continue
        names.append(node.metadata.name)
    if names:
        return names
    return [n.metadata.name for n in core.list_node().items]


def _node_exec(node: str, command: str) -> subprocess.CompletedProcess:
    kind = subprocess.run(
        ["docker", "exec", node, "sh", "-c", command],
        capture_output=True,
        text=True,
        check=False,
    )
    if kind.returncode == 0:
        return kind
    inspect = subprocess.run(
        ["docker", "inspect", node],
        capture_output=True,
        text=True,
        check=False,
    )
    if inspect.returncode == 0:
        return kind
    return subprocess.run(
        ["minikube", "ssh", "-n", node, "--", command],
        capture_output=True,
        text=True,
        check=False,
    )


def _wait_cache_downloaded(custom: client.CustomObjectsApi, name: str):
    deadline = time.monotonic() + CACHE_DOWNLOAD_TIMEOUT
    last = None
    while time.monotonic() < deadline:
        obj = custom.get_cluster_custom_object(GROUP, VERSION, "localmodelcaches", name)
        last = obj.get("status") or {}
        node_status = last.get("nodeStatus") or {}
        if node_status and all(v == "NodeDownloaded" for v in node_status.values()):
            return last
        time.sleep(5)
    pytest.fail(f"LocalModelCache {name} did not reach NodeDownloaded: {last}")


def _ensure_job_namespace(core: client.CoreV1Api):
    try:
        core.read_namespace(JOB_NAMESPACE)
    except ApiException as e:
        if e.status != 404:
            raise
        core.create_namespace(
            client.V1Namespace(metadata=client.V1ObjectMeta(name=JOB_NAMESPACE))
        )


def _patch_oci_insecure(core: client.CoreV1Api, enabled: bool) -> str:
    """Set ociInsecureRegistry. Returns the original storageInitializer JSON."""
    cm = core.read_namespaced_config_map("inferenceservice-config", KSERVE_NAMESPACE)
    original = cm.data.get("storageInitializer", "{}")
    cfg = json.loads(original)
    if bool(cfg.get("ociInsecureRegistry")) == enabled:
        return original
    cfg["ociInsecureRegistry"] = enabled
    cm.data["storageInitializer"] = json.dumps(cfg)
    core.patch_namespaced_config_map("inferenceservice-config", KSERVE_NAMESPACE, cm)
    return original


def _restore_storage_initializer(core: client.CoreV1Api, original: str):
    cm = core.read_namespaced_config_map("inferenceservice-config", KSERVE_NAMESPACE)
    if cm.data.get("storageInitializer") == original:
        return
    cm.data["storageInitializer"] = original
    core.patch_namespaced_config_map("inferenceservice-config", KSERVE_NAMESPACE, cm)


def _node_group(name: str, nodes: list[str]) -> V1alpha1LocalModelNodeGroup:
    pv_spec = V1PersistentVolumeSpec(
        access_modes=["ReadWriteOnce"],
        storage_class_name="standard",
        capacity={"storage": "1Gi"},
        local=V1LocalVolumeSource(path="/models"),
        persistent_volume_reclaim_policy="Delete",
        node_affinity=V1VolumeNodeAffinity(
            required=client.V1NodeSelector(
                node_selector_terms=[
                    client.V1NodeSelectorTerm(
                        match_expressions=[
                            client.V1NodeSelectorRequirement(
                                key="kubernetes.io/hostname",
                                operator="In",
                                values=nodes,
                            )
                        ]
                    )
                ]
            )
        ),
    )
    pvc_spec = V1PersistentVolumeClaimSpec(
        access_modes=["ReadWriteOnce"],
        resources=V1ResourceRequirements(requests={"storage": "1Gi"}),
        storage_class_name="standard",
    )
    return V1alpha1LocalModelNodeGroup(
        api_version=constants.KSERVE_V1ALPHA1,
        kind=constants.KSERVE_KIND_LOCALMODELNODEGROUP,
        metadata=client.V1ObjectMeta(name=name),
        spec=V1alpha1LocalModelNodeGroupSpec(
            storage_limit="1Gi",
            persistent_volume_spec=pv_spec,
            persistent_volume_claim_spec=pvc_spec,
        ),
    )


def _create_or_get_node_group(kserve_client: KServeClient, node_group):
    try:
        kserve_client.create_local_model_node_group(node_group)
    except RuntimeError as err:
        if "409" not in str(err) and "AlreadyExists" not in str(err):
            raise


def _create_or_get_cache(kserve_client: KServeClient, model_cache):
    try:
        kserve_client.create_local_model_cache(model_cache)
    except RuntimeError as err:
        if "409" not in str(err) and "AlreadyExists" not in str(err):
            raise


def _create_secret(core: client.CoreV1Api, secret: client.V1Secret):
    try:
        core.create_namespaced_secret(JOB_NAMESPACE, secret)
    except ApiException as e:
        if e.status != 409:
            raise


@pytest.mark.modelcache
def test_localmodelcache_public_oci_import_lands_on_pvc_not_image_cache():
    """Public oci:// import writes weights onto the PVC and does not leave the image in crictl."""
    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    core = kserve_client.core_api
    custom = client.CustomObjectsApi()
    nodes = _worker_node_names(core)
    assert nodes, "no worker nodes found for LocalModelCache"

    group_name = "oci-public-nodegroup"
    cache_name = "oci-public-fixture"
    storage_uri = f"oci://{OCI_FETCH_TEST_IMAGE}"
    node_group = _node_group(group_name, nodes)
    model_cache = V1alpha1LocalModelCache(
        api_version=constants.KSERVE_V1ALPHA1,
        kind=constants.KSERVE_KIND_LOCALMODELCACHE,
        metadata=client.V1ObjectMeta(name=cache_name),
        spec=V1alpha1LocalModelCacheSpec(
            model_size="50Mi",
            node_groups=[group_name],
            source_model_uri=storage_uri,
        ),
    )

    crictl_before = {n: _node_exec(n, "crictl images").stdout for n in nodes}

    _create_or_get_node_group(kserve_client, node_group)
    _create_or_get_cache(kserve_client, model_cache)
    try:
        _wait_cache_downloaded(custom, cache_name)
        storage_key = _storage_key(storage_uri)
        found = False
        for node in nodes:
            listing = _node_exec(node, f"ls /models/models/{storage_key} || true")
            if EXPECTED_MODEL_FILE in (listing.stdout or ""):
                found = True
                break
        assert found, (
            f"{EXPECTED_MODEL_FILE} not found under /models/models/{storage_key} "
            f"on {nodes}"
        )

        repo = OCI_FETCH_TEST_IMAGE.split(":")[0]
        for node in nodes:
            after = _node_exec(node, "crictl images").stdout
            if repo not in crictl_before[node]:
                assert repo not in after, (
                    f"modelcar image appeared in crictl on {node}: {after}"
                )
            after_df = _node_exec(node, "df -k /").stdout
            assert after_df, f"df failed on {node}"
    finally:
        kserve_client.delete_local_model_cache(cache_name)
        kserve_client.delete_local_model_node_group(group_name)


@pytest.mark.modelcache
def test_localmodelcache_private_oci_import_with_pull_secret():
    """Private HTTP registry import succeeds when imagePullSecrets is set."""
    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    core = kserve_client.core_api
    apps = client.AppsV1Api()
    custom = client.CustomObjectsApi()
    _ensure_job_namespace(core)
    original_storage_init = _patch_oci_insecure(core, True)
    try:
        nodes = _worker_node_names(core)
        assert nodes

        user, password = "ociuser", "ocipass"
        _create_secret(
            core,
            client.V1Secret(
                metadata=client.V1ObjectMeta(
                    name="oci-registry-htpasswd", namespace=JOB_NAMESPACE
                ),
                string_data={"htpasswd": _htpasswd_line(user, password)},
            ),
        )
        _ensure_auth_registry(core, apps)
        _wait_deployment_ready(apps, JOB_NAMESPACE, REGISTRY_NAME)
        _push_fixture_via_port_forward(user, password)

        dockerconfig = {
            "auths": {
                f"{REGISTRY_NAME}.{JOB_NAMESPACE}.svc.cluster.local:5000": {
                    "username": user,
                    "password": password,
                    "auth": base64.b64encode(f"{user}:{password}".encode()).decode(),
                }
            }
        }
        _create_secret(
            core,
            client.V1Secret(
                metadata=client.V1ObjectMeta(
                    name="oci-reg-cred", namespace=JOB_NAMESPACE
                ),
                type="kubernetes.io/dockerconfigjson",
                data={
                    ".dockerconfigjson": base64.b64encode(
                        json.dumps(dockerconfig).encode()
                    ).decode()
                },
            ),
        )

        group_name = "oci-private-nodegroup"
        cache_name = "oci-private-fixture"
        storage_uri = (
            f"oci://{REGISTRY_NAME}.{JOB_NAMESPACE}.svc.cluster.local:5000/"
            "oci-test-fixture:v1"
        )
        node_group = _node_group(group_name, nodes)
        model_cache = V1alpha1LocalModelCache(
            api_version=constants.KSERVE_V1ALPHA1,
            kind=constants.KSERVE_KIND_LOCALMODELCACHE,
            metadata=client.V1ObjectMeta(name=cache_name),
            spec=V1alpha1LocalModelCacheSpec(
                model_size="50Mi",
                node_groups=[group_name],
                source_model_uri=storage_uri,
                image_pull_secrets=[client.V1LocalObjectReference(name="oci-reg-cred")],
            ),
        )
        _create_or_get_node_group(kserve_client, node_group)
        _create_or_get_cache(kserve_client, model_cache)
        try:
            _wait_cache_downloaded(custom, cache_name)
        finally:
            kserve_client.delete_local_model_cache(cache_name)
            kserve_client.delete_local_model_node_group(group_name)
    finally:
        _restore_storage_initializer(core, original_storage_init)


def _htpasswd_line(user: str, password: str) -> str:
    try:
        out = subprocess.check_output(["htpasswd", "-Bbn", user, password], text=True)
        return out.strip()
    except (FileNotFoundError, subprocess.CalledProcessError):
        out = subprocess.check_output(
            [
                "docker",
                "run",
                "--rm",
                "httpd:2-alpine",
                "htpasswd",
                "-Bbn",
                user,
                password,
            ],
            text=True,
        )
        return out.strip()


def _ensure_auth_registry(core: client.CoreV1Api, apps: client.AppsV1Api):
    try:
        apps.read_namespaced_deployment(REGISTRY_NAME, JOB_NAMESPACE)
    except ApiException as e:
        if e.status != 404:
            raise
        apps.create_namespaced_deployment(
            JOB_NAMESPACE,
            client.V1Deployment(
                metadata=client.V1ObjectMeta(name=REGISTRY_NAME),
                spec=client.V1DeploymentSpec(
                    replicas=1,
                    selector=client.V1LabelSelector(
                        match_labels={"app": REGISTRY_NAME}
                    ),
                    template=client.V1PodTemplateSpec(
                        metadata=client.V1ObjectMeta(labels={"app": REGISTRY_NAME}),
                        spec=client.V1PodSpec(
                            containers=[
                                client.V1Container(
                                    name="registry",
                                    image="registry:2",
                                    env=[
                                        client.V1EnvVar(
                                            name="REGISTRY_AUTH", value="htpasswd"
                                        ),
                                        client.V1EnvVar(
                                            name="REGISTRY_AUTH_HTPASSWD_REALM",
                                            value="Registry Realm",
                                        ),
                                        client.V1EnvVar(
                                            name="REGISTRY_AUTH_HTPASSWD_PATH",
                                            value="/auth/htpasswd",
                                        ),
                                    ],
                                    ports=[client.V1ContainerPort(container_port=5000)],
                                    volume_mounts=[
                                        client.V1VolumeMount(
                                            name="auth",
                                            mount_path="/auth",
                                            read_only=True,
                                        )
                                    ],
                                )
                            ],
                            volumes=[
                                client.V1Volume(
                                    name="auth",
                                    secret=client.V1SecretVolumeSource(
                                        secret_name="oci-registry-htpasswd"
                                    ),
                                )
                            ],
                        ),
                    ),
                ),
            ),
        )
    try:
        core.create_namespaced_service(
            JOB_NAMESPACE,
            client.V1Service(
                metadata=client.V1ObjectMeta(
                    name=REGISTRY_NAME, namespace=JOB_NAMESPACE
                ),
                spec=client.V1ServiceSpec(
                    selector={"app": REGISTRY_NAME},
                    ports=[client.V1ServicePort(port=5000, target_port=5000)],
                ),
            ),
        )
    except ApiException as e:
        if e.status != 409:
            raise


def _wait_deployment_ready(
    apps: client.AppsV1Api, namespace: str, name: str, timeout=180
):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        dep = apps.read_namespaced_deployment(name, namespace)
        ready = dep.status.ready_replicas or 0
        if ready >= 1:
            return
        time.sleep(3)
    pytest.fail(f"deployment {name} not ready")


def _wait_tcp(host: str, port: int, timeout=30):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        try:
            with socket.create_connection((host, port), timeout=1):
                return
        except OSError:
            time.sleep(0.2)
    pytest.fail(f"{host}:{port} did not accept connections within {timeout}s")


def _push_fixture_via_port_forward(user: str, password: str):
    subprocess.check_call(["docker", "pull", OCI_FETCH_TEST_IMAGE])
    pf = subprocess.Popen(
        [
            "kubectl",
            "port-forward",
            "-n",
            JOB_NAMESPACE,
            f"svc/{REGISTRY_NAME}",
            "15000:5000",
        ],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )
    try:
        _wait_tcp("127.0.0.1", 15000)
        subprocess.run(
            ["docker", "login", "localhost:15000", "-u", user, "--password-stdin"],
            input=password,
            text=True,
            check=True,
        )
        subprocess.check_call(
            [
                "docker",
                "tag",
                OCI_FETCH_TEST_IMAGE,
                "localhost:15000/oci-test-fixture:v1",
            ]
        )
        subprocess.check_call(["docker", "push", "localhost:15000/oci-test-fixture:v1"])
    finally:
        pf.terminate()
        pf.wait(timeout=10)
