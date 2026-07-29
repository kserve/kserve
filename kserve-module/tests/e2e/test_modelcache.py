"""E2E tests for ModelCache reconciliation: enable, disable, CEL validation, recovery."""

import json

import pytest

from conftest import (
    run,
    _poll_cr,
    get_cr,
    resource_exists,
    get_jsonpath,
    get_worker_node,
    enable_model_cache,
    disable_model_cache,
    generation_matches,
    trigger_reconcile,
    is_cr_ready,
    wait_for,
    KSERVE_CR_NAME,
    NAMESPACE,
    PV_NAME,
    PVC_NAME,
    LMNG_NAME,
    LMNG_RESOURCE,
    TIMEOUT_120S,
)


@pytest.mark.modelcache
@pytest.mark.ocp_only
class TestModelCacheEnable:
    """Verify enabling ModelCache creates all expected resources."""

    def test_enable_creates_resources(self, kubectl, cluster_info, model_cache_enabled):
        """Patching modelCache.managementState=Managed creates PV, PVC,
        LocalModelNodeGroup, labels the worker node, elevates namespace PSA,
        and updates the inferenceservice-config ConfigMap.
        """
        worker = model_cache_enabled

        assert resource_exists(kubectl, "pv", PV_NAME), f"PV {PV_NAME} should exist"
        assert resource_exists(kubectl, "pvc", PVC_NAME, namespace=NAMESPACE), (
            f"PVC {PVC_NAME} should exist in {NAMESPACE}"
        )
        assert resource_exists(kubectl, LMNG_RESOURCE, LMNG_NAME), (
            f"LocalModelNodeGroup {LMNG_NAME} should exist"
        )

        label = get_jsonpath(
            kubectl, "node", worker, "{.metadata.labels.kserve/localmodel}"
        )
        assert label == "worker", (
            f"Node {worker} should have label kserve/localmodel=worker, got '{label}'"
        )

        psa = get_jsonpath(
            kubectl,
            "namespace",
            NAMESPACE,
            "{.metadata.labels.pod-security\\.kubernetes\\.io/enforce}",
        )
        assert psa == "privileged", f"Namespace PSA should be 'privileged', got '{psa}'"

        psa_annot = get_jsonpath(
            kubectl,
            "namespace",
            NAMESPACE,
            "{.metadata.annotations.opendatahub\\.io/psa-elevated-by}",
        )
        assert psa_annot == "kserve-modelcache", (
            f"PSA annotation should be 'kserve-modelcache', got '{psa_annot}'"
        )

        local_model_cfg = get_jsonpath(
            kubectl,
            "configmap",
            "inferenceservice-config",
            "{.data.localModel}",
            namespace=NAMESPACE,
        )
        cfg = json.loads(local_model_cfg)
        assert cfg.get("enabled") is True, (
            f"localModel.enabled should be true, got {cfg.get('enabled')}"
        )
        assert cfg.get("jobNamespace") == NAMESPACE, (
            f"localModel.jobNamespace should be '{NAMESPACE}', got {cfg.get('jobNamespace')}"
        )

        # Verify ModelCacheReady condition
        cr = get_cr(kubectl)
        conditions = {c["type"]: c for c in cr.get("status", {}).get("conditions", [])}
        assert "ModelCacheReady" in conditions, "ModelCacheReady condition should exist"
        assert conditions["ModelCacheReady"]["status"] == "True", (
            f"ModelCacheReady should be True, got {conditions['ModelCacheReady']['status']}"
        )


@pytest.mark.modelcache
@pytest.mark.ocp_only
class TestModelCacheDisable:
    """Verify disabling ModelCache cleans up all resources."""

    def test_disable_cleans_up_resources(self, kubectl, cluster_info, apply_kserve_cr):
        """After enabling then disabling ModelCache, all managed resources
        are removed and the Kserve CR remains Ready.
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

        disable_model_cache(kubectl)
        _poll_cr(
            kubectl,
            KSERVE_CR_NAME,
            generation_matches,
            TIMEOUT_120S,
            f"ModelCache disable not reconciled within {TIMEOUT_120S}s",
        )

        # Poll for resource cleanup — reconcile completes before GC finishes
        import time

        deadline = time.time() + TIMEOUT_120S
        while time.time() < deadline:
            if (
                not resource_exists(kubectl, "pv", PV_NAME)
                and not resource_exists(kubectl, "pvc", PVC_NAME, namespace=NAMESPACE)
                and not resource_exists(kubectl, LMNG_RESOURCE, LMNG_NAME)
            ):
                break
            time.sleep(5)

        assert not resource_exists(kubectl, "pv", PV_NAME), (
            f"PV {PV_NAME} should be deleted"
        )
        assert not resource_exists(kubectl, "pvc", PVC_NAME, namespace=NAMESPACE), (
            f"PVC {PVC_NAME} should be deleted"
        )
        assert not resource_exists(kubectl, LMNG_RESOURCE, LMNG_NAME), (
            f"LocalModelNodeGroup {LMNG_NAME} should be deleted"
        )

        label = get_jsonpath(
            kubectl, "node", worker, "{.metadata.labels.kserve/localmodel}"
        )
        assert label == "", (
            f"Node label kserve/localmodel should be removed, got '{label}'"
        )

        psa = get_jsonpath(
            kubectl,
            "namespace",
            NAMESPACE,
            "{.metadata.labels.pod-security\\.kubernetes\\.io/enforce}",
        )
        assert psa == "baseline", (
            f"Namespace PSA should revert to 'baseline', got '{psa}'"
        )

        psa_annot = get_jsonpath(
            kubectl,
            "namespace",
            NAMESPACE,
            "{.metadata.annotations.opendatahub\\.io/psa-elevated-by}",
        )
        assert psa_annot == "", f"PSA annotation should be removed, got '{psa_annot}'"

        local_model_cfg = get_jsonpath(
            kubectl,
            "configmap",
            "inferenceservice-config",
            "{.data.localModel}",
            namespace=NAMESPACE,
        )
        cfg = json.loads(local_model_cfg)
        assert cfg.get("enabled") is False, (
            f"localModel.enabled should be false, got {cfg.get('enabled')}"
        )

        cr = _poll_cr(
            kubectl,
            KSERVE_CR_NAME,
            is_cr_ready,
            TIMEOUT_120S,
            f"Kserve CR not Ready after ModelCache disable within {TIMEOUT_120S}s",
        )
        assert cr is not None, "Kserve CR should still be Ready"

        # Verify ModelCacheReady condition is cleared
        conditions = {c["type"]: c for c in cr.get("status", {}).get("conditions", [])}
        assert "ModelCacheReady" not in conditions, (
            "ModelCacheReady condition should be cleared when disabled"
        )


@pytest.mark.modelcache
class TestModelCacheCELValidation:
    """Verify CEL validation rules on ModelCache spec."""

    def test_rejects_managed_without_cache_size(self, kubectl, apply_kserve_cr):
        """managementState=Managed without cacheSize should be rejected."""
        # Use JSON patch to replace the entire modelCache field (merge would
        # keep stale cacheSize from previous tests).
        patch = json.dumps(
            [
                {
                    "op": "add",
                    "path": "/spec/modelCache",
                    "value": {"managementState": "Managed", "nodeNames": ["node1"]},
                }
            ]
        )
        result = run(
            [kubectl, "patch", "kserve", KSERVE_CR_NAME, "--type", "json", "-p", patch],
            check=False,
        )
        assert result.returncode != 0, "Patch without cacheSize should be rejected"

    def test_rejects_managed_without_node_selection(self, kubectl, apply_kserve_cr):
        """managementState=Managed without nodeNames or nodeSelector should be rejected."""
        patch = json.dumps(
            [
                {
                    "op": "add",
                    "path": "/spec/modelCache",
                    "value": {"managementState": "Managed", "cacheSize": "5Gi"},
                }
            ]
        )
        result = run(
            [kubectl, "patch", "kserve", KSERVE_CR_NAME, "--type", "json", "-p", patch],
            check=False,
        )
        assert result.returncode != 0, (
            "Patch without nodeNames or nodeSelector should be rejected"
        )

    def test_rejects_both_node_names_and_selector(self, kubectl, apply_kserve_cr):
        """Specifying both nodeNames and nodeSelector should be rejected."""
        patch = json.dumps(
            [
                {
                    "op": "add",
                    "path": "/spec/modelCache",
                    "value": {
                        "managementState": "Managed",
                        "cacheSize": "5Gi",
                        "nodeNames": ["n1"],
                        "nodeSelector": {"matchLabels": {"a": "b"}},
                    },
                }
            ]
        )
        result = run(
            [kubectl, "patch", "kserve", KSERVE_CR_NAME, "--type", "json", "-p", patch],
            check=False,
        )
        assert result.returncode != 0, (
            "Patch with both nodeNames and nodeSelector should be rejected"
        )


@pytest.mark.modelcache
@pytest.mark.ocp_only
class TestModelCacheRecovery:
    """Verify deleted ModelCache resources are recreated on reconcile."""

    def test_deleted_lmng_is_recreated(self, kubectl, model_cache_enabled):
        """Deleting the LocalModelNodeGroup triggers recreation on next reconcile."""
        assert resource_exists(kubectl, LMNG_RESOURCE, LMNG_NAME), (
            f"{LMNG_NAME} should exist before deletion"
        )

        run([kubectl, "delete", LMNG_RESOURCE, LMNG_NAME])

        def assert_lmng_deleted():
            assert not resource_exists(kubectl, LMNG_RESOURCE, LMNG_NAME), (
                f"{LMNG_NAME} should be deleted"
            )

        wait_for(assert_lmng_deleted, timeout=TIMEOUT_120S, interval=5)

        trigger_reconcile(kubectl)

        def assert_lmng_recreated():
            assert resource_exists(kubectl, LMNG_RESOURCE, LMNG_NAME), (
                f"{LMNG_NAME} should be recreated after reconcile"
            )

        wait_for(assert_lmng_recreated, timeout=TIMEOUT_120S, interval=5)
