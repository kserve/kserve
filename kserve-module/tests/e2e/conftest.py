"""Shared fixtures for kserve-module E2E tests."""

import shutil
import subprocess
import time
from dataclasses import dataclass

import pytest
import yaml


# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------
KSERVE_CR_NAME = "default-kserve"
NAMESPACE = "opendatahub"
OPERATOR_DEPLOYMENT = "kserve-module-controller-manager"
TIMEOUT_120S = 120
TIMEOUT_60S = 60

OPERAND_DEPLOYMENTS_XKS = [
    "llmisvc-controller-manager",
]
OPERAND_DEPLOYMENTS_OCP = [
    "kserve-controller-manager",
    "llmisvc-controller-manager",
    "odh-model-controller",
    "model-serving-api",
]

WVA_DEPLOYMENT = "workload-variant-autoscaler-controller-manager"
WVA_CONFIGMAP = "workload-variant-autoscaler-saturation-scaling-config"
MODEL_CONTROLLER_DEPLOYMENT = "odh-model-controller"

KSERVE_CR_TEMPLATE = {
    "apiVersion": "components.platform.opendatahub.io/v1alpha1",
    "kind": "Kserve",
    "metadata": {"name": KSERVE_CR_NAME},
    "spec": {"managementState": "Managed"},
}

PV_NAME = "kserve-localmodelnode-pv"
PVC_NAME = "kserve-localmodelnode-pvc"
LMNG_NAME = "workers"
LMNG_RESOURCE = "localmodelnodegroups.serving.kserve.io"


@dataclass
class ClusterInfo:
    is_openshift: bool
    kubectl: str  # "oc" or "kubectl"


# ---------------------------------------------------------------------------
# Cluster detection (shared by hook and fixture)
# ---------------------------------------------------------------------------
_cluster_detection_cache = None


def _detect_openshift():
    """Detect cluster type once; cached for the process lifetime."""
    global _cluster_detection_cache
    if _cluster_detection_cache is not None:
        return _cluster_detection_cache

    cli = "oc" if shutil.which("oc") else "kubectl"
    if not shutil.which(cli):
        _cluster_detection_cache = (False, cli)
        return _cluster_detection_cache

    result = subprocess.run(
        [cli, "api-resources", "--api-group=config.openshift.io"],
        capture_output=True,
        text=True,
        timeout=10,
    )
    is_ocp = result.returncode == 0 and "clusterversions" in result.stdout.lower()
    _cluster_detection_cache = (is_ocp, cli)
    return _cluster_detection_cache


# ---------------------------------------------------------------------------
# Pytest hooks
# ---------------------------------------------------------------------------
def pytest_collection_modifyitems(config, items):
    """Skip @pytest.mark.ocp_only tests on non-OpenShift clusters.

    Runs at collection time — before any fixture setup — so expensive
    fixtures like apply_kserve_cr never execute on vanilla-k8s clusters.
    """
    is_ocp, _ = _detect_openshift()
    if is_ocp:
        return

    skip_marker = pytest.mark.skip(reason="OCP-only test (not running on OpenShift)")
    for item in items:
        if "ocp_only" in item.keywords:
            item.add_marker(skip_marker)


# ---------------------------------------------------------------------------
# Helper functions - pure
# ---------------------------------------------------------------------------
def operand_deployments(is_openshift):
    """Return the expected operand deployments for the detected platform."""
    return OPERAND_DEPLOYMENTS_OCP if is_openshift else OPERAND_DEPLOYMENTS_XKS


def is_cr_ready(cr):
    """Check if a Kserve CR dict has Ready=True."""
    conditions = cr.get("status", {}).get("conditions", [])
    return any(
        c.get("type") == "Ready" and c.get("status") == "True" for c in conditions
    )


def get_conditions(kubectl_bin, name=KSERVE_CR_NAME):
    """Fetch conditions as a dict keyed by condition type."""
    cr = get_cr(kubectl_bin, name)
    return {c["type"]: c for c in cr.get("status", {}).get("conditions", [])}


# ---------------------------------------------------------------------------
# Helper functions - shell / kubectl
# ---------------------------------------------------------------------------
def run(cmd, check=True, timeout=60, input_text=None):
    """Run a command and return the result."""
    result = subprocess.run(
        cmd, capture_output=True, text=True, timeout=timeout, input=input_text
    )
    if check and result.returncode != 0:
        raise RuntimeError(
            f"Command failed: {cmd}\nstdout: {result.stdout}\nstderr: {result.stderr}"
        )
    return result


def get_cr(kubectl_bin, name=KSERVE_CR_NAME, check=True):
    """Fetch the Kserve CR and return parsed YAML. Returns None on failure when check=False."""
    result = run([kubectl_bin, "get", "kserve", name, "-o", "yaml"], check=False)
    if result.returncode != 0:
        if check:
            raise RuntimeError(
                f"Failed to get kserve {name}\nstdout: {result.stdout}\nstderr: {result.stderr}"
            )
        return None
    return yaml.safe_load(result.stdout)


def cr_exists(kubectl_bin, name=KSERVE_CR_NAME):
    """Check if the Kserve CR already exists."""
    return get_cr(kubectl_bin, name, check=False) is not None


def trigger_reconcile(kubectl_bin, name=KSERVE_CR_NAME, trigger_id=None):
    """Trigger reconcile by patching an annotation."""
    trigger_id = trigger_id or f"e2e-{int(time.time())}"
    run(
        [
            kubectl_bin,
            "annotate",
            "kserve",
            name,
            f"test-trigger={trigger_id}",
            "--overwrite",
        ]
    )


def create_kserve_cr(kubectl_bin, cr_dict=None):
    """Create a Kserve CR if it doesn't already exist."""
    if cr_exists(kubectl_bin):
        return _poll_cr(
            kubectl_bin,
            KSERVE_CR_NAME,
            is_cr_ready,
            TIMEOUT_120S,
            f"Kserve CR {KSERVE_CR_NAME} not ready within {TIMEOUT_120S}s",
        )
    cr = yaml.safe_dump(cr_dict or KSERVE_CR_TEMPLATE)
    run([kubectl_bin, "create", "-f", "-"], input_text=cr)
    return _poll_cr(
        kubectl_bin,
        KSERVE_CR_NAME,
        is_cr_ready,
        TIMEOUT_120S,
        f"Kserve CR {KSERVE_CR_NAME} not ready within {TIMEOUT_120S}s",
    )


# ---------------------------------------------------------------------------
# Helper functions - polling / wait
# ---------------------------------------------------------------------------
def wait_for(assertion_fn, timeout=60.0, interval=5.0):
    """Poll until assertion_fn() succeeds or timeout expires."""
    deadline = time.time() + timeout
    last_error = None
    while True:
        try:
            return assertion_fn()
        except (AssertionError, Exception) as e:
            last_error = e
            if time.time() >= deadline:
                raise AssertionError(
                    f"Timed out after {timeout}s waiting for assertion. "
                    f"Last error: {last_error}"
                ) from e
            time.sleep(interval)


def wait_consistently(assertion_fn, duration=30.0, interval=5.0):
    """Poll and assert condition stays true for the entire duration."""
    deadline = time.time() + duration
    while time.time() < deadline:
        assertion_fn()
        time.sleep(interval)


def _poll_cr(kubectl_bin, name, predicate, timeout, msg):
    """Poll the Kserve CR until predicate(cr) returns True."""
    deadline = time.time() + timeout
    while time.time() < deadline:
        cr = get_cr(kubectl_bin, name, check=False)
        if cr is None:
            time.sleep(5)
            continue
        if predicate(cr):
            return cr
        time.sleep(5)
    raise TimeoutError(msg)


def get_worker_node(kubectl_bin, is_openshift=True):
    """Return the name of a worker node."""
    if is_openshift:
        result = run(
            [
                kubectl_bin,
                "get",
                "nodes",
                "-l",
                "node-role.kubernetes.io/worker",
                "-o",
                "jsonpath={.items[0].metadata.name}",
            ]
        )
    else:
        result = run(
            [
                kubectl_bin,
                "get",
                "nodes",
                "-o",
                "jsonpath={.items[0].metadata.name}",
            ]
        )
    name = result.stdout.strip()
    if not name:
        raise RuntimeError("No worker node found")
    return name


def resource_exists(kubectl_bin, resource_type, name, namespace=None):
    """Check if a kubernetes resource exists."""
    cmd = [kubectl_bin, "get", resource_type, name, "--ignore-not-found"]
    if namespace:
        cmd.extend(["-n", namespace])
    result = run(cmd, check=False)
    return result.returncode == 0 and bool(result.stdout.strip())


def get_jsonpath(kubectl_bin, resource_type, name, jsonpath, namespace=None):
    """Get a jsonpath value from a resource. Returns empty string if not found."""
    cmd = [kubectl_bin, "get", resource_type, name, "-o", f"jsonpath={jsonpath}"]
    if namespace:
        cmd.extend(["-n", namespace])
    result = run(cmd, check=False)
    return result.stdout.strip()


def enable_model_cache(kubectl_bin, worker_node, cache_size="5Gi"):
    """Patch the Kserve CR to enable ModelCache with nodeNames."""
    import json

    patch = json.dumps(
        {
            "spec": {
                "modelCache": {
                    "managementState": "Managed",
                    "cacheSize": cache_size,
                    "nodeNames": [worker_node],
                }
            }
        }
    )
    run(
        [kubectl_bin, "patch", "kserve", KSERVE_CR_NAME, "--type", "merge", "-p", patch]
    )


def disable_model_cache(kubectl_bin):
    """Remove the modelCache spec entirely from the Kserve CR."""
    import json

    patch = json.dumps([{"op": "remove", "path": "/spec/modelCache"}])
    run([kubectl_bin, "patch", "kserve", KSERVE_CR_NAME, "--type", "json", "-p", patch])


def generation_matches(cr):
    """Check if observedGeneration matches generation."""
    gen = cr.get("metadata", {}).get("generation", -1)
    observed = cr.get("status", {}).get("observedGeneration", -2)
    return gen == observed


def wait_for_kserve_cleanup(
    kubectl_bin, name=KSERVE_CR_NAME, is_openshift=False, timeout=TIMEOUT_120S
):
    """Wait until the Kserve CR is fully deleted."""
    result = run([kubectl_bin, "get", "kserve", name, "--ignore-not-found"])
    if result.stdout.strip():
        run(
            [
                kubectl_bin,
                "wait",
                "--for=delete",
                f"kserve/{name}",
                f"--timeout={timeout}s",
            ]
        )
    _wait_for_managed_deployments_gc(kubectl_bin, is_openshift, timeout=TIMEOUT_60S)


def wait_for_deployment(kubectl_bin, name, namespace=NAMESPACE, timeout=TIMEOUT_120S):
    """Wait until a deployment exists and has Available=True."""
    deadline = time.time() + timeout
    while time.time() < deadline:
        result = run(
            [kubectl_bin, "get", "deployment", name, "-n", namespace, "-o", "yaml"],
            check=False,
        )
        if result.returncode == 0:
            dep = yaml.safe_load(result.stdout)
            conditions = {
                c["type"]: c for c in dep.get("status", {}).get("conditions", [])
            }
            avail = conditions.get("Available", {})
            if avail.get("status") == "True":
                return dep
        time.sleep(5)
    raise TimeoutError(f"deployment {name} not Available within {timeout}s")


def wait_for_deployment_gone(
    kubectl_bin, name, namespace=NAMESPACE, timeout=TIMEOUT_60S
):
    """Wait until a deployment no longer exists."""
    result = run(
        [
            kubectl_bin,
            "wait",
            "--for=delete",
            f"deployment/{name}",
            "-n",
            namespace,
            f"--timeout={timeout}s",
        ],
        check=False,
    )
    if result.returncode != 0 and "not found" not in result.stderr.lower():
        raise RuntimeError(f"wait_for_deployment_gone failed: {result.stderr}")


def _wait_for_managed_deployments_gc(kubectl_bin, is_openshift, timeout=TIMEOUT_60S):
    """Wait until managed deployments are cleaned up by garbage collection."""
    for dep in operand_deployments(is_openshift):
        wait_for_deployment_gone(kubectl_bin, dep, timeout=timeout)


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------
@pytest.fixture(scope="session")
def cluster_info():
    """Detect cluster type and pick the right CLI binary (oc or kubectl)."""
    is_ocp, cli = _detect_openshift()
    if not shutil.which(cli):
        pytest.fail("Neither 'oc' nor 'kubectl' found in PATH")
    return ClusterInfo(is_openshift=is_ocp, kubectl=cli)


@pytest.fixture(scope="session")
def kubectl(cluster_info):
    """Return the kubectl binary name for the cluster."""
    return cluster_info.kubectl


@pytest.fixture
def apply_kserve_cr(kubectl, cluster_info):
    """Create a Kserve CR and delete after test."""
    created = not cr_exists(kubectl)
    cr = create_kserve_cr(kubectl)
    yield cr
    if created:
        run(
            [kubectl, "delete", "kserve", KSERVE_CR_NAME, "--ignore-not-found"],
            check=False,
        )
        wait_for_kserve_cleanup(kubectl, is_openshift=cluster_info.is_openshift)


@pytest.fixture
def model_cache_enabled(kubectl, cluster_info, apply_kserve_cr):
    """Enable ModelCache before the test and disable it after.

    Requires @pytest.mark.ocp_only on the test; the collection hook
    skips non-OpenShift clusters before this fixture runs.
    """
    worker = get_worker_node(kubectl, is_openshift=cluster_info.is_openshift)
    enable_model_cache(kubectl, worker)
    _poll_cr(
        kubectl,
        KSERVE_CR_NAME,
        generation_matches,
        TIMEOUT_120S,
        f"ModelCache enable not reconciled within {TIMEOUT_120S}s",
    )
    _poll_cr(
        kubectl,
        KSERVE_CR_NAME,
        lambda cr: any(
            c.get("type") == "ModelCacheReady" and c.get("status") == "True"
            for c in cr.get("status", {}).get("conditions", [])
        ),
        TIMEOUT_120S,
        f"ModelCacheReady not True within {TIMEOUT_120S}s",
    )
    yield worker
    disable_model_cache(kubectl)
    _poll_cr(
        kubectl,
        KSERVE_CR_NAME,
        generation_matches,
        TIMEOUT_120S,
        f"ModelCache disable not reconciled within {TIMEOUT_120S}s",
    )


PLATFORM_VERSION_CM = "odh-kserve-config"
TEST_PLATFORM_VERSION = "99.0.0"


def platform_configmap_exists(kubectl_bin):
    """Check if the platform version ConfigMap already exists."""
    return resource_exists(kubectl_bin, "configmap", PLATFORM_VERSION_CM, namespace=NAMESPACE)


@pytest.fixture
def ensure_platform_configmap(kubectl, apply_kserve_cr):
    """Ensure odh-kserve-config ConfigMap exists with a platformVersion.

    If it already exists (e.g. platform operator created it), leave it alone.
    Otherwise create a test one and clean up after.
    """
    already_existed = platform_configmap_exists(kubectl)

    if not already_existed:
        cm_yaml = yaml.safe_dump({
            "apiVersion": "v1",
            "kind": "ConfigMap",
            "metadata": {"name": PLATFORM_VERSION_CM, "namespace": NAMESPACE},
            "data": {"platformVersion": TEST_PLATFORM_VERSION},
        })
        run([kubectl, "apply", "-f", "-"], input_text=cm_yaml)
        _poll_cr(
            kubectl,
            KSERVE_CR_NAME,
            lambda cr: any(
                r.get("name") == "platform"
                for r in cr.get("status", {}).get("releases", [])
            ),
            TIMEOUT_60S,
            "platform release not reported after ConfigMap creation",
        )

    yield

    if not already_existed:
        run(
            [kubectl, "delete", "configmap", PLATFORM_VERSION_CM, "-n", NAMESPACE, "--ignore-not-found"],
            check=False,
        )
