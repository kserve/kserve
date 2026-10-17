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

"""
E2E tests for the self-signed webhook certificate mode
(kserve.llmisvc.selfSignedCerts) of the kserve-llmisvc-crd and
kserve-llmisvc-resources Helm charts.

The behaviour under test depends on Helm's `lookup`, which only sees cluster
state during a real install/upgrade (or --dry-run=server), so these tests run
real `helm` commands against a live cluster. They validate:
1. Install-order and namespace guards (resources chart needs the CRD chart's CA)
2. One CA wired into the CRD conversion webhooks, the admission webhooks and
   the serving certificate, with admission and conversion actually served
3. Custom caSecretName / validityDays, and defaults when the keys are missing
4. Certificates reused across upgrade, uninstall and reinstall
5. CA rotation and incomplete Secrets
6. Migration of an existing cert-manager install (ownership guard)

The charts install cluster-scoped objects (CRDs, webhook configurations), so
the tests are destructive: they install and uninstall the two llmisvc
releases. Run them serially on a dedicated cluster with no existing llmisvc
install; they skip themselves otherwise. Prerequisites: Gateway API CRDs and
the LeaderWorkerSet operator (the llmisvc controller needs them to start);
cert-manager only for the migration test.

    pytest test/e2e/helm/test_llmisvc_selfsigned_certs.py -m llmisvc_selfsigned_certs

Environment: HELM_BIN (default "helm"), KUBE_CONTEXT (default: current
context), LLMISVC_SELFSIGNED_HELM_ARGS (extra args for every helm
install/upgrade, e.g. "--set kserve.llmisvc.controller.tag=latest").
"""

import base64
import logging
import os
import re
import shlex
import shutil
import subprocess
import time
from pathlib import Path
from types import SimpleNamespace

import pytest
from cryptography import x509
from kubernetes import client, config
from kubernetes.client.rest import ApiException

logger = logging.getLogger(__name__)

pytestmark = pytest.mark.llmisvc_selfsigned_certs

HELM_BIN = os.environ.get("HELM_BIN", "helm")
KUBE_CONTEXT = os.environ.get("KUBE_CONTEXT")
EXTRA_HELM_ARGS = shlex.split(os.environ.get("LLMISVC_SELFSIGNED_HELM_ARGS", ""))

REPO_ROOT = Path(__file__).resolve().parents[3]
CRD_CHART = REPO_ROOT / "charts" / "kserve-llmisvc-crd"
RES_CHART = REPO_ROOT / "charts" / "kserve-llmisvc-resources"
CRD_RELEASE = "kserve-llmisvc-crd"
RES_RELEASE = "kserve-llmisvc-resources"

# The CRD manifests point the conversion webhook at the kserve namespace.
NAMESPACE = "kserve"
OTHER_NAMESPACE = "llmisvc-selfsigned-other"
TEST_NAMESPACE = "llmisvc-selfsigned-e2e"

CA_SECRET = "llmisvc-selfsigned-ca"
CUSTOM_CA_SECRET = "llmisvc-e2e-custom-ca"
SERVING_SECRET = "llmisvc-webhook-server-cert"
CERT_MANAGER_CERTIFICATE = "llmisvc-serving-cert"
CONTROLLER = "llmisvc-controller-manager"
WEBHOOK_SERVICE = "llmisvc-webhook-server-service"
DEFAULT_VALIDITY_DAYS = 7300

SELF_SIGNED = {"kserve.llmisvc.selfSignedCerts.enabled": "true"}
INJECT_CA_ANNOTATION = "cert-manager.io/inject-ca-from"
LLMISVC_CRDS = [
    "llminferenceservices.serving.kserve.io",
    "llminferenceserviceconfigs.serving.kserve.io",
]
MUTATING_WEBHOOKS = ["llminferenceservice.serving.kserve.io"]
VALIDATING_WEBHOOKS = [
    "llminferenceservice.serving.kserve.io",
    "llminferenceserviceconfig.serving.kserve.io",
]
PREREQUISITE_CRDS = [
    "gateways.gateway.networking.k8s.io",
    "leaderworkersets.leaderworkerset.x-k8s.io",
]
CERT_MANAGER_CRD = "certificates.cert-manager.io"

KSERVE_GROUP = "serving.kserve.io"
LLMISVC_PLURAL = "llminferenceservices"
VALID_MODEL_URI = "hf://Qwen/Qwen2.5-0.5B-Instruct"
INVALID_MODEL_URI = "not-a-valid-uri ://"

CA_GUARD_MESSAGE = (
    "was not found or is incomplete. Install or upgrade the kserve-llmisvc-crd chart"
)
OWNERSHIP_GUARD_MESSAGE = "already exists and is not owned by release"


def wait_for(assertion_fn, timeout: float = 120.0, interval: float = 3.0):
    """Wait for the assertion to succeed within timeout."""
    deadline = time.time() + timeout
    while True:
        try:
            return assertion_fn()
        except (AssertionError, ApiException) as e:
            if time.time() >= deadline:
                raise AssertionError(
                    f"Timed out after {timeout}s waiting for assertion. Last error: {e}"
                ) from e
            time.sleep(interval)


# --- helm -------------------------------------------------------------------


def helm(*args, check: bool = True) -> subprocess.CompletedProcess:
    cmd = [HELM_BIN, *args]
    if KUBE_CONTEXT:
        cmd += ["--kube-context", KUBE_CONTEXT]
    logger.info("$ %s", shlex.join(cmd))
    result = subprocess.run(cmd, capture_output=True, text=True)
    if check and result.returncode != 0:
        raise AssertionError(f"{shlex.join(cmd)} failed:\n{result.stderr}")
    return result


def deploy(
    action: str,
    release: str,
    chart: Path,
    values: dict = None,
    namespace: str = NAMESPACE,
    extra=(),
    check: bool = True,
) -> subprocess.CompletedProcess:
    """helm install/upgrade a local chart with --set values."""
    args = [action, release, str(chart), "-n", namespace]
    for key, value in (values or {}).items():
        args += ["--set", f"{key}={value}"]
    if action == "install":
        args.append("--create-namespace")
    return helm(*args, *EXTRA_HELM_ARGS, *extra, check=check)


def assert_helm_failed(result: subprocess.CompletedProcess, *fragments: str):
    assert result.returncode != 0, f"helm unexpectedly succeeded:\n{result.stdout}"
    for fragment in fragments:
        assert fragment in result.stderr, (
            f"expected {fragment!r} in helm error:\n{result.stderr}"
        )


def release_exists(release: str, namespace: str = NAMESPACE) -> bool:
    return helm("status", release, "-n", namespace, check=False).returncode == 0


# --- kubernetes -------------------------------------------------------------


def get_secret(k8s, name: str, namespace: str = NAMESPACE):
    try:
        return k8s.core.read_namespaced_secret(name, namespace)
    except ApiException as e:
        if e.status == 404:
            return None
        raise


def delete_ignore_missing(fn, *args):
    try:
        fn(*args)
    except ApiException as e:
        if e.status != 404:
            raise


def crd_exists(k8s, name: str) -> bool:
    try:
        k8s.ext.read_custom_resource_definition(name)
        return True
    except ApiException as e:
        if e.status == 404:
            return False
        raise


def ensure_namespace(k8s, name: str):
    try:
        k8s.core.create_namespace(
            client.V1Namespace(metadata=client.V1ObjectMeta(name=name))
        )
    except ApiException as e:
        if e.status != 409:
            raise


def controller_pod_uids(k8s) -> set:
    deployment = k8s.apps.read_namespaced_deployment(CONTROLLER, NAMESPACE)
    selector = ",".join(
        f"{k}={v}" for k, v in deployment.spec.selector.match_labels.items()
    )
    pods = k8s.core.list_namespaced_pod(NAMESPACE, label_selector=selector).items
    return {pod.metadata.uid for pod in pods if pod.metadata.deletion_timestamp is None}


def wait_for_controller(k8s, timeout: float = 300.0):
    def _ready():
        status = k8s.apps.read_namespaced_deployment_status(
            CONTROLLER, NAMESPACE
        ).status
        assert status.updated_replicas and status.updated_replicas == status.replicas
        assert status.available_replicas == status.replicas
        assert not status.unavailable_replicas

    wait_for(_ready, timeout=timeout)


def restart_controller(k8s):
    patch = {
        "spec": {
            "template": {
                "metadata": {
                    "annotations": {
                        "kubectl.kubernetes.io/restartedAt": str(time.time())
                    }
                }
            }
        }
    }
    k8s.apps.patch_namespaced_deployment(CONTROLLER, NAMESPACE, patch)
    time.sleep(2)
    wait_for_controller(k8s)


def uninstall_all(k8s):
    """Remove both releases plus the Secrets/Certificates they leave behind."""
    for release, namespace in [
        (RES_RELEASE, NAMESPACE),
        (CRD_RELEASE, NAMESPACE),
        (CRD_RELEASE, OTHER_NAMESPACE),
    ]:
        if release_exists(release, namespace):
            helm("uninstall", release, "-n", namespace, "--wait", check=False)
    for namespace in (NAMESPACE, OTHER_NAMESPACE):
        for secret in (CA_SECRET, CUSTOM_CA_SECRET, SERVING_SECRET):
            delete_ignore_missing(k8s.core.delete_namespaced_secret, secret, namespace)
    if crd_exists(k8s, CERT_MANAGER_CRD):
        delete_ignore_missing(
            k8s.custom.delete_namespaced_custom_object,
            "cert-manager.io",
            "v1",
            NAMESPACE,
            "certificates",
            CERT_MANAGER_CERTIFICATE,
        )
    delete_ignore_missing(k8s.core.delete_namespace, OTHER_NAMESPACE)

    def _crds_gone():
        for name in LLMISVC_CRDS:
            assert not crd_exists(k8s, name), f"{name} still present"

    wait_for(_crds_gone, timeout=180)


# --- certificate assertions -------------------------------------------------


def load_cert(b64_pem: str) -> x509.Certificate:
    return x509.load_pem_x509_certificate(base64.b64decode(b64_pem))


def validity_days(cert: x509.Certificate) -> int:
    return (cert.not_valid_after_utc - cert.not_valid_before_utc).days


def assert_single_ca_wiring(k8s, ca_secret: str = CA_SECRET) -> SimpleNamespace:
    """Assert the CA Secret's CA is the only trust anchor anywhere, and return
    the base64 PEMs of the CA and serving certificates."""
    ca_b64 = get_secret(k8s, ca_secret).data["tls.crt"]

    for name in LLMISVC_CRDS:
        crd = k8s.ext.read_custom_resource_definition(name)
        ca_bundle = crd.spec.conversion.webhook.client_config.ca_bundle
        assert ca_bundle == ca_b64, f"{name} conversion caBundle is not the CA"
        assert INJECT_CA_ANNOTATION not in (crd.metadata.annotations or {})

    configs = [
        k8s.adm.read_mutating_webhook_configuration(name) for name in MUTATING_WEBHOOKS
    ] + [
        k8s.adm.read_validating_webhook_configuration(name)
        for name in VALIDATING_WEBHOOKS
    ]
    for cfg in configs:
        assert INJECT_CA_ANNOTATION not in (cfg.metadata.annotations or {})
        for webhook in cfg.webhooks:
            assert webhook.client_config.ca_bundle == ca_b64, (
                f"{webhook.name} caBundle is not the CA"
            )

    serving = get_secret(k8s, SERVING_SECRET)
    assert serving.data["ca.crt"] == ca_b64
    serving_cert = load_cert(serving.data["tls.crt"])
    serving_cert.verify_directly_issued_by(load_cert(ca_b64))
    sans = serving_cert.extensions.get_extension_for_class(
        x509.SubjectAlternativeName
    ).value.get_values_for_type(x509.DNSName)
    assert set(sans) == {
        WEBHOOK_SERVICE,
        f"{WEBHOOK_SERVICE}.{NAMESPACE}",
        f"{WEBHOOK_SERVICE}.{NAMESPACE}.svc",
        f"{WEBHOOK_SERVICE}.{NAMESPACE}.svc.cluster.local",
    }
    return SimpleNamespace(ca=ca_b64, serving=serving.data["tls.crt"])


def assert_no_cert_manager_objects(k8s):
    if not crd_exists(k8s, CERT_MANAGER_CRD):
        return
    for plural in ("certificates", "issuers"):
        items = k8s.custom.list_namespaced_custom_object(
            "cert-manager.io", "v1", NAMESPACE, plural
        )["items"]
        assert items == [], f"unexpected cert-manager {plural}: {items}"


# --- admission + conversion -------------------------------------------------


def llmisvc(version: str, name: str, uri: str = VALID_MODEL_URI) -> dict:
    return {
        "apiVersion": f"{KSERVE_GROUP}/{version}",
        "kind": "LLMInferenceService",
        "metadata": {"name": name, "namespace": TEST_NAMESPACE},
        "spec": {"model": {"uri": uri}},
    }


def create_llmisvc(k8s, version: str, body: dict, dry_run: bool = False):
    kwargs = {"dry_run": "All"} if dry_run else {}
    return k8s.custom.create_namespaced_custom_object(
        KSERVE_GROUP, version, TEST_NAMESPACE, LLMISVC_PLURAL, body, **kwargs
    )


def wait_for_webhooks(k8s, timeout: float = 240.0):
    """Wait until the API server can reach the webhook server over TLS for
    admission (v1alpha2 create) and conversion (v1alpha1 create)."""

    def _served():
        create_llmisvc(
            k8s, "v1alpha2", llmisvc("v1alpha2", "probe-v1alpha2"), dry_run=True
        )
        create_llmisvc(
            k8s, "v1alpha1", llmisvc("v1alpha1", "probe-v1alpha1"), dry_run=True
        )

    wait_for(_served, timeout=timeout)


def assert_webhooks_serve(k8s):
    wait_for_webhooks(k8s)

    # A webhook denial (not a TLS error) proves admission is actually served.
    with pytest.raises(ApiException) as exc:
        create_llmisvc(
            k8s,
            "v1alpha2",
            llmisvc("v1alpha2", "invalid-uri", INVALID_MODEL_URI),
            dry_run=True,
        )
    assert "denied the request" in exc.value.body
    assert INVALID_MODEL_URI in exc.value.body

    # Reading a stored v1alpha2 object as v1alpha1 goes through the conversion webhook.
    name = "conversion-proof"
    create_llmisvc(k8s, "v1alpha2", llmisvc("v1alpha2", name))
    try:
        as_v1alpha1 = k8s.custom.get_namespaced_custom_object(
            KSERVE_GROUP, "v1alpha1", TEST_NAMESPACE, LLMISVC_PLURAL, name
        )
        assert as_v1alpha1["apiVersion"] == f"{KSERVE_GROUP}/v1alpha1"
        assert as_v1alpha1["spec"]["model"]["uri"] == VALID_MODEL_URI
    finally:
        delete_ignore_missing(
            k8s.custom.delete_namespaced_custom_object,
            KSERVE_GROUP,
            "v1alpha2",
            TEST_NAMESPACE,
            LLMISVC_PLURAL,
            name,
        )

        # Wait for the controller to drop its finalizer so the CRD can be uninstalled.
        def _deleted():
            with pytest.raises(ApiException) as gone:
                k8s.custom.get_namespaced_custom_object(
                    KSERVE_GROUP, "v1alpha2", TEST_NAMESPACE, LLMISVC_PLURAL, name
                )
            assert gone.value.status == 404

        wait_for(_deleted)


def install_self_signed(k8s, values: dict = None):
    values = {**SELF_SIGNED, **(values or {})}
    deploy("install", CRD_RELEASE, CRD_CHART, values)
    deploy("install", RES_RELEASE, RES_CHART, values)
    wait_for_controller(k8s)


# --- fixtures ---------------------------------------------------------------


@pytest.fixture(scope="module")
def k8s():
    if shutil.which(HELM_BIN) is None:
        pytest.skip(f"helm binary {HELM_BIN!r} not found (set HELM_BIN)")
    config.load_kube_config(context=KUBE_CONTEXT)
    api = SimpleNamespace(
        core=client.CoreV1Api(),
        apps=client.AppsV1Api(),
        ext=client.ApiextensionsV1Api(),
        adm=client.AdmissionregistrationV1Api(),
        custom=client.CustomObjectsApi(),
    )
    missing = [name for name in PREREQUISITE_CRDS if not crd_exists(api, name)]
    if missing:
        pytest.skip(f"cluster is missing prerequisite CRDs: {missing}")
    # These tests uninstall the llmisvc releases; never run against a cluster
    # that already has an llmisvc install.
    if any(crd_exists(api, name) for name in LLMISVC_CRDS) or release_exists(
        RES_RELEASE
    ):
        pytest.skip(
            "cluster already has llmisvc installed; these destructive tests need a "
            "dedicated cluster (uninstall it first)"
        )
    return api


@pytest.fixture(autouse=True)
def clean_cluster(k8s):
    ensure_namespace(k8s, NAMESPACE)
    ensure_namespace(k8s, TEST_NAMESPACE)
    yield
    uninstall_all(k8s)


@pytest.fixture
def cert_manager(k8s):
    if not crd_exists(k8s, CERT_MANAGER_CRD):
        pytest.skip("cert-manager is not installed")


# --- tests ------------------------------------------------------------------


def test_resources_chart_requires_crd_chart_ca(k8s):
    result = deploy("install", RES_RELEASE, RES_CHART, SELF_SIGNED, check=False)

    assert_helm_failed(
        result, f"the CA Secret {NAMESPACE}/{CA_SECRET} {CA_GUARD_MESSAGE}"
    )
    assert not release_exists(RES_RELEASE)
    assert get_secret(k8s, SERVING_SECRET) is None


def test_resources_chart_rejects_ca_in_another_namespace(k8s):
    deploy("install", CRD_RELEASE, CRD_CHART, SELF_SIGNED, namespace=OTHER_NAMESPACE)

    result = deploy("install", RES_RELEASE, RES_CHART, SELF_SIGNED, check=False)

    assert_helm_failed(
        result, f"the CA Secret {NAMESPACE}/{CA_SECRET} {CA_GUARD_MESSAGE}"
    )
    assert not release_exists(RES_RELEASE)


def test_resources_chart_rejects_mismatched_ca_secret_name(k8s):
    deploy(
        "install",
        CRD_RELEASE,
        CRD_CHART,
        {
            **SELF_SIGNED,
            "kserve.llmisvc.selfSignedCerts.caSecretName": CUSTOM_CA_SECRET,
        },
    )

    result = deploy("install", RES_RELEASE, RES_CHART, SELF_SIGNED, check=False)

    assert_helm_failed(
        result, f"the CA Secret {NAMESPACE}/{CA_SECRET} {CA_GUARD_MESSAGE}"
    )


def test_guards_run_on_server_dry_run_but_not_client_render(k8s):
    # --dry-run=server performs lookups, so the guard reports the problem
    # before anything is applied.
    result = deploy(
        "install",
        RES_RELEASE,
        RES_CHART,
        SELF_SIGNED,
        extra=["--dry-run=server"],
        check=False,
    )
    assert_helm_failed(result, CA_GUARD_MESSAGE)

    # Client-side rendering cannot see the cluster and still renders. (A
    # client-side `install --dry-run` is not used: it still maps kinds through
    # the cluster and fails on the CRDs that the CRD chart has not installed.)
    template = ["template", RES_RELEASE, str(RES_CHART), "-n", NAMESPACE, "--set"]
    helm(*template, "kserve.llmisvc.selfSignedCerts.enabled=true")

    assert not release_exists(RES_RELEASE)
    assert get_secret(k8s, SERVING_SECRET) is None


def test_install_wires_one_ca_everywhere(k8s):
    install_self_signed(k8s)

    certs = assert_single_ca_wiring(k8s)
    assert validity_days(load_cert(certs.ca)) == DEFAULT_VALIDITY_DAYS
    assert validity_days(load_cert(certs.serving)) == DEFAULT_VALIDITY_DAYS
    ca_secret = get_secret(k8s, CA_SECRET)
    assert ca_secret.metadata.annotations.get("helm.sh/resource-policy") == "keep"
    assert_no_cert_manager_objects(k8s)
    assert_webhooks_serve(k8s)


def test_custom_ca_secret_name_and_validity(k8s):
    install_self_signed(
        k8s,
        {
            "kserve.llmisvc.selfSignedCerts.caSecretName": CUSTOM_CA_SECRET,
            "kserve.llmisvc.selfSignedCerts.validityDays": 400,
        },
    )

    certs = assert_single_ca_wiring(k8s, ca_secret=CUSTOM_CA_SECRET)
    assert validity_days(load_cert(certs.ca)) == 400
    assert validity_days(load_cert(certs.serving)) == 400
    assert get_secret(k8s, CA_SECRET) is None
    wait_for_webhooks(k8s)


def chart_without_new_keys(chart: Path, dest: Path) -> Path:
    """Copy a chart with caSecretName/validityDays removed from values.yaml,
    standing in for a chart version that predates them."""
    copy = shutil.copytree(chart, dest / chart.name)
    values_file = copy / "values.yaml"
    values, removed = re.subn(
        r"^\s+(caSecretName|validityDays):.*\n",
        "",
        values_file.read_text(),
        flags=re.MULTILINE,
    )
    assert removed == 2, f"expected 2 keys to remove from {values_file}"
    values_file.write_text(values)
    return copy


def test_missing_keys_fall_back_to_defaults_with_reuse_values(k8s, tmp_path):
    # Simulates a release installed from a chart that predates
    # caSecretName/validityDays: --reuse-values keeps that release's chart
    # defaults, so the keys stay missing after upgrading to the current chart.
    deploy(
        "install", CRD_RELEASE, chart_without_new_keys(CRD_CHART, tmp_path), SELF_SIGNED
    )
    deploy(
        "install", RES_RELEASE, chart_without_new_keys(RES_CHART, tmp_path), SELF_SIGNED
    )
    wait_for_controller(k8s)
    before = assert_single_ca_wiring(k8s)
    assert validity_days(load_cert(before.serving)) == DEFAULT_VALIDITY_DAYS

    deploy("upgrade", CRD_RELEASE, CRD_CHART, extra=["--reuse-values"])
    deploy("upgrade", RES_RELEASE, RES_CHART, extra=["--reuse-values"])

    after = assert_single_ca_wiring(k8s)
    assert (after.ca, after.serving) == (before.ca, before.serving)


def test_upgrade_and_reinstall_reuse_certificates(k8s):
    install_self_signed(k8s)
    original = assert_single_ca_wiring(k8s)
    pods = controller_pod_uids(k8s)

    # Upgrades re-render from the stored Secrets: nothing rotates or restarts.
    deploy("upgrade", CRD_RELEASE, CRD_CHART, SELF_SIGNED)
    deploy("upgrade", RES_RELEASE, RES_CHART, SELF_SIGNED)
    upgraded = assert_single_ca_wiring(k8s)
    assert (upgraded.ca, upgraded.serving) == (original.ca, original.serving)
    assert controller_pod_uids(k8s) == pods

    # Reinstalling the resources chart issues a new serving cert from the same CA.
    helm("uninstall", RES_RELEASE, "-n", NAMESPACE, "--wait")
    deploy("install", RES_RELEASE, RES_CHART, SELF_SIGNED)
    wait_for_controller(k8s)
    reinstalled = assert_single_ca_wiring(k8s)
    assert reinstalled.ca == original.ca
    assert reinstalled.serving != original.serving

    # The CA Secret survives uninstalling the CRD chart and is reused on reinstall.
    helm("uninstall", CRD_RELEASE, "-n", NAMESPACE, "--wait")
    assert get_secret(k8s, CA_SECRET).data["tls.crt"] == original.ca
    deploy("install", CRD_RELEASE, CRD_CHART, SELF_SIGNED)
    assert assert_single_ca_wiring(k8s).ca == original.ca
    assert_webhooks_serve(k8s)


def test_ca_rotation_reissues_serving_certificate(k8s):
    install_self_signed(k8s)
    original = assert_single_ca_wiring(k8s)

    # Rotate: delete the CA, upgrade the CRD chart (new CA), then the resources
    # chart, whose serving cert is now signed by a stale CA and gets re-issued.
    k8s.core.delete_namespaced_secret(CA_SECRET, NAMESPACE)
    deploy("upgrade", CRD_RELEASE, CRD_CHART, SELF_SIGNED)
    deploy("upgrade", RES_RELEASE, RES_CHART, SELF_SIGNED)

    rotated = assert_single_ca_wiring(k8s)
    assert rotated.ca != original.ca
    assert rotated.serving != original.serving
    assert_webhooks_serve(k8s)


def test_incomplete_secrets(k8s):
    install_self_signed(k8s)
    original = assert_single_ca_wiring(k8s)
    # kubernetes.io/tls Secrets must keep the tls.key entry, so empty it instead.
    empty_key = [{"op": "replace", "path": "/data/tls.key", "value": ""}]

    # A serving Secret with an empty key is re-issued from the same CA.
    k8s.core.patch_namespaced_secret(SERVING_SECRET, NAMESPACE, empty_key)
    deploy("upgrade", RES_RELEASE, RES_CHART, SELF_SIGNED)
    reissued = assert_single_ca_wiring(k8s)
    assert reissued.ca == original.ca
    assert get_secret(k8s, SERVING_SECRET).data.get("tls.key")

    # A CA Secret with an empty key is rejected rather than replaced by a new CA.
    k8s.core.patch_namespaced_secret(CA_SECRET, NAMESPACE, empty_key)
    result = deploy("upgrade", RES_RELEASE, RES_CHART, SELF_SIGNED, check=False)
    assert_helm_failed(
        result, f"the CA Secret {NAMESPACE}/{CA_SECRET} {CA_GUARD_MESSAGE}"
    )


def test_migration_from_cert_manager(k8s, cert_manager):
    deploy("install", CRD_RELEASE, CRD_CHART)
    deploy("install", RES_RELEASE, RES_CHART)

    def _certificate_ready():
        certificate = k8s.custom.get_namespaced_custom_object(
            "cert-manager.io", "v1", NAMESPACE, "certificates", CERT_MANAGER_CERTIFICATE
        )
        conditions = certificate.get("status", {}).get("conditions", [])
        assert any(c["type"] == "Ready" and c["status"] == "True" for c in conditions)

    wait_for(_certificate_ready)
    # The controller may have started before the cert was issued (same as the
    # Helm quick-install), so restart it once the Secret exists.
    restart_controller(k8s)
    wait_for_webhooks(k8s)

    # The resources chart cannot go first: the CRD chart's CA does not exist yet.
    result = deploy("upgrade", RES_RELEASE, RES_CHART, SELF_SIGNED, check=False)
    assert_helm_failed(result, CA_GUARD_MESSAGE)

    deploy("upgrade", CRD_RELEASE, CRD_CHART, SELF_SIGNED)

    # The serving Secret was issued by cert-manager, so the chart refuses to
    # adopt it and prints the migration commands.
    result = deploy("upgrade", RES_RELEASE, RES_CHART, SELF_SIGNED, check=False)
    assert_helm_failed(
        result,
        f"Secret {NAMESPACE}/{SERVING_SECRET} {OWNERSHIP_GUARD_MESSAGE} {RES_RELEASE!r}".replace(
            "'", '"'
        ),
        f"kubectl delete certificate {CERT_MANAGER_CERTIFICATE} -n {NAMESPACE} --ignore-not-found",
        f"kubectl delete secret {SERVING_SECRET} -n {NAMESPACE}",
    )

    # Follow the documented steps.
    k8s.custom.delete_namespaced_custom_object(
        "cert-manager.io", "v1", NAMESPACE, "certificates", CERT_MANAGER_CERTIFICATE
    )
    k8s.core.delete_namespaced_secret(SERVING_SECRET, NAMESPACE)
    deploy("upgrade", RES_RELEASE, RES_CHART, SELF_SIGNED)

    assert_single_ca_wiring(k8s)
    serving = get_secret(k8s, SERVING_SECRET)
    assert serving.metadata.labels.get("app.kubernetes.io/managed-by") == "Helm"
    assert serving.metadata.annotations.get("meta.helm.sh/release-name") == RES_RELEASE
    assert_no_cert_manager_objects(k8s)
    assert_webhooks_serve(k8s)
