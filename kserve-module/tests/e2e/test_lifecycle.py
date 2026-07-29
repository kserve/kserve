"""E2E tests for Kserve CR lifecycle: create, update, delete, CEL validation."""

import json
import yaml
import pytest

from conftest import (
    run,
    _poll_cr,
    get_conditions,
    get_jsonpath,
    resource_exists,
    wait_for,
    wait_consistently,
    wait_for_kserve_cleanup,
    trigger_reconcile,
    wait_for_deployment,
    wait_for_deployment_gone,
    operand_deployments,
    KSERVE_CR_NAME,
    NAMESPACE,
    OPERATOR_DEPLOYMENT,
    WVA_DEPLOYMENT,
    WVA_CONFIGMAP,
    MODEL_CONTROLLER_DEPLOYMENT,
    TIMEOUT_120S,
    TIMEOUT_60S,
)


def _generation_matches(cr):
    gen = cr.get("metadata", {}).get("generation", -1)
    observed = cr.get("status", {}).get("observedGeneration", -2)
    return gen == observed


def _verify_deployments_available(kubectl, is_openshift):
    expected = operand_deployments(is_openshift)
    for name in expected:
        wait_for_deployment(kubectl, name)


@pytest.mark.sanity
class TestCreate:
    """Verify CR creation triggers operand resource deployment."""

    def test_create_deploys_operands(self, kubectl, cluster_info, apply_kserve_cr):
        """Kserve CR creation deploys managed deployments with Available status.

        apply_kserve_cr fixture creates the CR and waits for Ready=True.
        This test verifies the actual cluster state matches.
        """
        _verify_deployments_available(kubectl, is_openshift=cluster_info.is_openshift)


@pytest.mark.sanity
class TestDelete:
    """Verify CR deletion removes managed resources and preserves CRDs."""

    def test_delete_cleans_up_managed_resources(self, kubectl, cluster_info, apply_kserve_cr):
        """Kserve CR deletion removes managed deployments but keeps the operator running.

        Verifies GC cleans up operand deployments via ownerReference,
        while the operator deployment itself remains.
        """
        run([kubectl, "delete", "kserve", KSERVE_CR_NAME])
        wait_for_kserve_cleanup(kubectl, is_openshift=cluster_info.is_openshift)

        result = run([kubectl, "get", "kserve", KSERVE_CR_NAME], check=False)
        assert result.returncode != 0, "Kserve CR should be deleted"

        expected = operand_deployments(cluster_info.is_openshift)
        result = run([kubectl, "get", "deployments", "-n", NAMESPACE, "-o", "yaml"])
        deployments = yaml.safe_load(result.stdout)
        dep_names = [d["metadata"]["name"] for d in deployments.get("items", [])]

        for operand in expected:
            assert operand not in dep_names, \
                f"{operand} should be deleted. Found: {dep_names}"

        assert OPERATOR_DEPLOYMENT in dep_names, \
            "Operator deployment should still be running"


@pytest.mark.sanity
class TestUpdate:
    """Verify spec changes trigger reconcile and apply new config."""

    def test_spec_change_triggers_reconcile(self, kubectl, apply_kserve_cr):
        """Patching spec.rawDeploymentServiceConfig triggers reconcile.

        Verifies generation increments, observedGeneration catches up,
        and the new spec value is persisted.
        """
        gen_before = apply_kserve_cr["metadata"]["generation"]

        result = run([
            kubectl, "get", "configmap", "inferenceservice-config",
            "-n", NAMESPACE, "-o", "jsonpath={.data.service}",
        ])
        assert '"serviceClusterIPNone": true' in result.stdout, \
            "ConfigMap should have serviceClusterIPNone=true before update"

        patch = json.dumps({"spec": {"rawDeploymentServiceConfig": "Headed"}})
        run([kubectl, "patch", "kserve", KSERVE_CR_NAME, "--type", "merge", "-p", patch])

        cr_after = _poll_cr(kubectl, KSERVE_CR_NAME, _generation_matches, TIMEOUT_120S,
                            f"observedGeneration not matching within {TIMEOUT_120S}s")
        gen_after = cr_after["metadata"]["generation"]

        assert gen_after > gen_before, \
            f"Generation should increment: {gen_before} -> {gen_after}"
        assert cr_after["spec"]["rawDeploymentServiceConfig"] == "Headed", \
            "Spec should reflect update: expected Headed"

        expected_headless = "false"
        result = run([
            kubectl, "get", "configmap", "inferenceservice-config",
            "-n", NAMESPACE, "-o", "jsonpath={.data.service}",
        ])
        assert f'"serviceClusterIPNone": {expected_headless}' in result.stdout, \
            f"ConfigMap should reflect serviceClusterIPNone={expected_headless}"


@pytest.mark.sanity
class TestCELValidation:
    """Verify CEL validation rules on the Kserve CRD."""

    def test_rejects_invalid_cr_name(self, kubectl):
        """CRD-level CEL rule enforces singleton name 'default-kserve'.

        Attempts to create a CR with name 'invalid-name' and verifies
        the API server rejects it before it reaches the controller.
        """
        invalid_cr = (
            "apiVersion: components.platform.opendatahub.io/v1alpha1\n"
            "kind: Kserve\n"
            "metadata:\n"
            "  name: invalid-name\n"
            "spec:\n"
            "  managementState: Managed\n"
        )

        result = run(
            [kubectl, "apply", "-f", "-"], check=False, input_text=invalid_cr
        )

        assert result.returncode != 0, \
            "CR with invalid name should be rejected"
        assert "Kserve name must be 'default-kserve'" in result.stderr, \
            f"Error should reference CEL name validation. stderr: {result.stderr}"

        result = run([kubectl, "get", "kserve", "invalid-name"], check=False)
        assert result.returncode != 0, "Invalid CR should not exist"


@pytest.mark.sanity
@pytest.mark.ocp_only
class TestManagementState:
    """Verify managementState transitions for sub-components (WVA, NIM)."""

    def test_wva_default_removed_has_no_deployment(self, kubectl, cluster_info, apply_kserve_cr):
        """WVA defaults to Removed — no WVA deployment should exist."""
        patch = json.dumps({"spec": {"wva": {"managementState": "Removed"}}})
        run([kubectl, "patch", "kserve", KSERVE_CR_NAME, "--type", "merge", "-p", patch])
        _poll_cr(kubectl, KSERVE_CR_NAME, _generation_matches, TIMEOUT_120S,
                 f"observedGeneration not matching within {TIMEOUT_120S}s")
        wait_for_deployment_gone(kubectl, WVA_DEPLOYMENT)

        result = run(
            [kubectl, "get", "deployment", WVA_DEPLOYMENT, "-n", NAMESPACE],
            check=False,
        )
        assert result.returncode != 0, \
            f"{WVA_DEPLOYMENT} should not exist when WVA is Removed"

    def test_wva_managed_deploys_resources(self, kubectl, cluster_info, apply_kserve_cr):
        """Setting wva.managementState to Managed deploys WVA resources."""
        patch = json.dumps({"spec": {"wva": {"managementState": "Managed"}}})
        run([kubectl, "patch", "kserve", KSERVE_CR_NAME, "--type", "merge", "-p", patch])

        _poll_cr(kubectl, KSERVE_CR_NAME, _generation_matches, TIMEOUT_120S,
                 f"observedGeneration not matching within {TIMEOUT_120S}s")

        wait_for_deployment(kubectl, WVA_DEPLOYMENT)
        _verify_deployments_available(kubectl, is_openshift=True)

    def test_wva_managed_to_removed_cleans_up(self, kubectl, cluster_info, apply_kserve_cr):
        """Switching WVA from Managed to Removed removes WVA deployment but keeps others."""
        patch = json.dumps({"spec": {"wva": {"managementState": "Managed"}}})
        run([kubectl, "patch", "kserve", KSERVE_CR_NAME, "--type", "merge", "-p", patch])
        wait_for_deployment(kubectl, WVA_DEPLOYMENT)

        patch = json.dumps({"spec": {"wva": {"managementState": "Removed"}}})
        run([kubectl, "patch", "kserve", KSERVE_CR_NAME, "--type", "merge", "-p", patch])

        _poll_cr(kubectl, KSERVE_CR_NAME, _generation_matches, TIMEOUT_120S,
                 f"observedGeneration not matching within {TIMEOUT_120S}s")

        wait_for_deployment_gone(kubectl, WVA_DEPLOYMENT)

        _verify_deployments_available(kubectl, is_openshift=True)

    def test_nim_default_managed_env_var(self, kubectl, cluster_info, apply_kserve_cr):
        """NIM defaults to Managed — odh-model-controller should have NIM_STATE=managed."""
        _poll_cr(kubectl, KSERVE_CR_NAME, _generation_matches, TIMEOUT_120S,
                 f"observedGeneration not matching within {TIMEOUT_120S}s")
        wait_for_deployment(kubectl, MODEL_CONTROLLER_DEPLOYMENT)

        result = run([
            kubectl, "get", "deployment", MODEL_CONTROLLER_DEPLOYMENT,
            "-n", NAMESPACE, "-o",
            "jsonpath={.spec.template.spec.containers[?(@.name=='manager')].env[?(@.name=='NIM_STATE')].value}",
        ])
        assert result.stdout.strip() == "managed", \
            f"NIM_STATE should be 'managed' by default, got '{result.stdout.strip()}'"

    def test_nim_managed_to_removed_updates_env(self, kubectl, cluster_info, apply_kserve_cr):
        """Switching NIM to Removed updates odh-model-controller NIM_STATE env var."""
        wait_for_deployment(kubectl, MODEL_CONTROLLER_DEPLOYMENT)

        patch = json.dumps({"spec": {"nim": {"managementState": "Removed"}}})
        run([kubectl, "patch", "kserve", KSERVE_CR_NAME, "--type", "merge", "-p", patch])

        _poll_cr(kubectl, KSERVE_CR_NAME, _generation_matches, TIMEOUT_120S,
                 f"observedGeneration not matching within {TIMEOUT_120S}s")

        wait_for_deployment(kubectl, MODEL_CONTROLLER_DEPLOYMENT)

        result = run([
            kubectl, "get", "deployment", MODEL_CONTROLLER_DEPLOYMENT,
            "-n", NAMESPACE, "-o",
            "jsonpath={.spec.template.spec.containers[?(@.name=='manager')].env[?(@.name=='NIM_STATE')].value}",
        ])
        assert result.stdout.strip() == "removed", \
            f"NIM_STATE should be 'removed' after patch, got '{result.stdout.strip()}'"

    def test_nim_removed_to_managed_updates_env(self, kubectl, cluster_info, apply_kserve_cr):
        """Switching NIM back to Managed updates odh-model-controller NIM_STATE env var."""
        patch = json.dumps({"spec": {"nim": {"managementState": "Removed"}}})
        run([kubectl, "patch", "kserve", KSERVE_CR_NAME, "--type", "merge", "-p", patch])
        _poll_cr(kubectl, KSERVE_CR_NAME, _generation_matches, TIMEOUT_120S,
                 f"observedGeneration not matching within {TIMEOUT_120S}s")

        patch = json.dumps({"spec": {"nim": {"managementState": "Managed"}}})
        run([kubectl, "patch", "kserve", KSERVE_CR_NAME, "--type", "merge", "-p", patch])
        _poll_cr(kubectl, KSERVE_CR_NAME, _generation_matches, TIMEOUT_120S,
                 f"observedGeneration not matching within {TIMEOUT_120S}s")

        wait_for_deployment(kubectl, MODEL_CONTROLLER_DEPLOYMENT)

        result = run([
            kubectl, "get", "deployment", MODEL_CONTROLLER_DEPLOYMENT,
            "-n", NAMESPACE, "-o",
            "jsonpath={.spec.template.spec.containers[?(@.name=='manager')].env[?(@.name=='NIM_STATE')].value}",
        ])
        assert result.stdout.strip() == "managed", \
            f"NIM_STATE should be 'managed' after reverting, got '{result.stdout.strip()}'"


@pytest.mark.sanity
class TestDriftCorrection:
    """SSA drift correction — manual edits reverted on reconcile."""

    def test_configmap_edit_reverted(self, kubectl, apply_kserve_cr):
        """Manual ConfigMap edit is reverted by SSA on next reconcile."""
        cm_name = "inferenceservice-config"

        run([
            kubectl, "patch", "configmap", cm_name, "-n", NAMESPACE,
            "--type", "merge",
            "-p", '{"data":{"ingress":"{\\"ingressClassName\\":\\"TAMPERED\\"}"}}',
        ])

        result = run([
            kubectl, "get", "configmap", cm_name, "-n", NAMESPACE,
            "-o", "jsonpath={.data.ingress}",
        ])
        assert "TAMPERED" in result.stdout

        trigger_reconcile(kubectl, trigger_id="drift-001")

        def assert_tampered_reverted():
            result = run([
                kubectl, "get", "configmap", cm_name, "-n", NAMESPACE,
                "-o", "jsonpath={.data.ingress}",
            ])
            assert "TAMPERED" not in result.stdout

        wait_for(assert_tampered_reverted, timeout=TIMEOUT_60S, interval=5)

        def assert_provisioning_succeeded():
            conditions = get_conditions(kubectl)
            assert conditions["ProvisioningSucceeded"]["status"] == "True"

        wait_for(assert_provisioning_succeeded, timeout=TIMEOUT_60S, interval=5)


@pytest.mark.sanity
class TestDeletionRecovery:
    """Owned resources are recreated after external deletion on next reconcile."""

    def test_configmap_recovered_after_deletion(self, kubectl, apply_kserve_cr):
        """Deleting an owned ConfigMap triggers recreation with a new UID."""
        cm_name = "inferenceservice-config"
        uid_before = get_jsonpath(
            kubectl, "configmap", cm_name, "{.metadata.uid}", namespace=NAMESPACE
        )
        assert uid_before, f"{cm_name} should exist before deletion"

        run([kubectl, "delete", "configmap", cm_name, "-n", NAMESPACE])

        def assert_cm_deleted():
            assert not resource_exists(kubectl, "configmap", cm_name, namespace=NAMESPACE), (
                f"{cm_name} should be deleted"
            )

        wait_for(assert_cm_deleted, timeout=TIMEOUT_120S, interval=5)

        def assert_recreated_with_new_uid():
            uid_after = get_jsonpath(
                kubectl, "configmap", cm_name, "{.metadata.uid}", namespace=NAMESPACE
            )
            assert uid_after, f"{cm_name} should be recreated"
            assert uid_after != uid_before, (
                f"{cm_name} should have a new UID after recreation"
            )

        wait_for(assert_recreated_with_new_uid, timeout=TIMEOUT_120S, interval=5)

    def test_deployment_recovered_after_deletion(self, kubectl, cluster_info, apply_kserve_cr):
        """Deleting owned Deployments triggers recreation with new UIDs."""
        targets = list(operand_deployments(cluster_info.is_openshift))

        if cluster_info.is_openshift:
            patch = json.dumps({"spec": {"wva": {"managementState": "Managed"}}})
            run([kubectl, "patch", "kserve", KSERVE_CR_NAME, "--type", "merge", "-p", patch])
            _poll_cr(kubectl, KSERVE_CR_NAME, _generation_matches, TIMEOUT_120S,
                     f"observedGeneration not matching within {TIMEOUT_120S}s")
            wait_for_deployment(kubectl, WVA_DEPLOYMENT)
            targets.append(WVA_DEPLOYMENT)

        try:
            for dep_name in targets:
                uid_before = get_jsonpath(
                    kubectl, "deployment", dep_name, "{.metadata.uid}", namespace=NAMESPACE
                )
                assert uid_before, f"{dep_name} should exist before deletion"

                run([kubectl, "delete", "deployment", dep_name, "-n", NAMESPACE])

                def assert_recreated(name=dep_name, expected_old_uid=uid_before):
                    uid_after = get_jsonpath(
                        kubectl, "deployment", name, "{.metadata.uid}", namespace=NAMESPACE
                    )
                    assert uid_after, f"{name} should be recreated"
                    assert uid_after != expected_old_uid, (
                        f"{name} should have a new UID after recreation"
                    )

                wait_for(assert_recreated, timeout=TIMEOUT_120S, interval=5)
                wait_for_deployment(kubectl, dep_name)
        finally:
            if cluster_info.is_openshift:
                patch = json.dumps({"spec": {"wva": {"managementState": "Removed"}}})
                run([kubectl, "patch", "kserve", KSERVE_CR_NAME, "--type", "merge", "-p", patch],
                    check=False)
                _poll_cr(kubectl, KSERVE_CR_NAME, _generation_matches, TIMEOUT_120S,
                         f"observedGeneration not matching within {TIMEOUT_120S}s")
                wait_for_deployment_gone(kubectl, WVA_DEPLOYMENT)


def _enable_wva(kubectl):
    """Enable WVA by patching managementState to Managed."""
    patch = json.dumps({"spec": {"wva": {"managementState": "Managed"}}})
    run([kubectl, "patch", "kserve", KSERVE_CR_NAME, "--type", "merge", "-p", patch])
    _poll_cr(kubectl, KSERVE_CR_NAME, _generation_matches, TIMEOUT_120S,
             f"observedGeneration not matching within {TIMEOUT_120S}s")
    wait_for_deployment(kubectl, WVA_DEPLOYMENT)


def _disable_wva(kubectl):
    """Disable WVA by patching managementState to Removed."""
    patch = json.dumps({"spec": {"wva": {"managementState": "Removed"}}})
    run([kubectl, "patch", "kserve", KSERVE_CR_NAME, "--type", "merge", "-p", patch],
        check=False)
    _poll_cr(kubectl, KSERVE_CR_NAME, _generation_matches, TIMEOUT_120S,
             f"observedGeneration not matching within {TIMEOUT_120S}s")
    wait_for_deployment_gone(kubectl, WVA_DEPLOYMENT)


@pytest.mark.sanity
@pytest.mark.ocp_only
class TestWVAConfigMap:
    """Verify WVA saturation-scaling-config ConfigMap lifecycle.

    The ConfigMap is annotated with opendatahub.io/managed=false in the
    WVA kustomize overlay, so the deployer creates it once but never
    overwrites it via SSA — user modifications are preserved.
    """

    def test_wva_configmap_deployed_with_defaults(self, kubectl, apply_kserve_cr):
        """WVA ConfigMap is deployed with default queueSpareTrigger value."""
        try:
            _enable_wva(kubectl)

            assert resource_exists(kubectl, "configmap", WVA_CONFIGMAP, namespace=NAMESPACE), \
                f"{WVA_CONFIGMAP} should exist after WVA is Managed"

            data = get_jsonpath(kubectl, "configmap", WVA_CONFIGMAP,
                                "{.data.default}", namespace=NAMESPACE)
            assert "queueSpareTrigger: 3" in data, \
                f"Expected queueSpareTrigger: 3 in ConfigMap data, got: {data}"
        finally:
            _disable_wva(kubectl)

    def test_wva_configmap_preserves_user_modifications(self, kubectl, apply_kserve_cr):
        """User modifications to ConfigMap data persist across reconciles.

        The deployer skips SSA for resources annotated with
        opendatahub.io/managed=false — only creates if missing.
        """
        try:
            _enable_wva(kubectl)

            data = get_jsonpath(kubectl, "configmap", WVA_CONFIGMAP,
                                "{.data.default}", namespace=NAMESPACE)
            assert "queueSpareTrigger: 3" in data, \
                f"Expected default queueSpareTrigger: 3 before patch, got: {data}"

            new_data = data.replace("queueSpareTrigger: 3", "queueSpareTrigger: 2")
            escaped = json.dumps({"data": {"default": new_data}})
            run([kubectl, "patch", "configmap", WVA_CONFIGMAP, "-n", NAMESPACE,
                 "--type", "merge", "-p", escaped])

            def assert_value_preserved():
                current = get_jsonpath(kubectl, "configmap", WVA_CONFIGMAP,
                                       "{.data.default}", namespace=NAMESPACE)
                assert "queueSpareTrigger: 2" in current, \
                    f"Expected queueSpareTrigger: 2 to persist, got: {current}"

            wait_consistently(assert_value_preserved, duration=30.0, interval=5.0)
        finally:
            _disable_wva(kubectl)

    def test_wva_configmap_recreated_with_defaults_after_deletion(self, kubectl, apply_kserve_cr):
        """Deleted ConfigMap is recreated with default values via Owns() watch."""
        try:
            _enable_wva(kubectl)

            uid_before = get_jsonpath(kubectl, "configmap", WVA_CONFIGMAP,
                                      "{.metadata.uid}", namespace=NAMESPACE)
            assert uid_before, f"{WVA_CONFIGMAP} should exist before deletion"

            run([kubectl, "delete", "configmap", WVA_CONFIGMAP, "-n", NAMESPACE])

            def assert_recreated_with_defaults():
                if not resource_exists(kubectl, "configmap", WVA_CONFIGMAP, namespace=NAMESPACE):
                    raise AssertionError(f"{WVA_CONFIGMAP} not yet recreated")
                uid_after = get_jsonpath(kubectl, "configmap", WVA_CONFIGMAP,
                                         "{.metadata.uid}", namespace=NAMESPACE)
                assert uid_after != uid_before, \
                    "ConfigMap UID should change after recreation"
                data = get_jsonpath(kubectl, "configmap", WVA_CONFIGMAP,
                                    "{.data.default}", namespace=NAMESPACE)
                assert "queueSpareTrigger: 3" in data, \
                    f"Recreated ConfigMap should have default queueSpareTrigger: 3, got: {data}"

            wait_for(assert_recreated_with_defaults, timeout=TIMEOUT_120S, interval=5)
        finally:
            _disable_wva(kubectl)


@pytest.mark.sanity
class TestOwnerReferences:
    """Verify operand deployments have correct ownerReferences to the Kserve CR."""

    def test_owned_deployments_have_kserve_owner_reference(self, kubectl, cluster_info, apply_kserve_cr):
        """Each operand deployment should reference the Kserve CR as owner."""
        expected = operand_deployments(cluster_info.is_openshift)
        cr_uid = apply_kserve_cr["metadata"]["uid"]

        for name in expected:
            wait_for_deployment(kubectl, name)
            result = run([
                kubectl, "get", "deployment", name, "-n", NAMESPACE, "-o", "json",
            ])
            dep = json.loads(result.stdout)

            owner_refs = dep.get("metadata", {}).get("ownerReferences", [])
            assert owner_refs, \
                f"Deployment {name} should have ownerReferences"

            kserve_owners = [r for r in owner_refs if r.get("kind") == "Kserve"]
            assert kserve_owners, \
                f"Deployment {name} should have a Kserve ownerReference, got: {owner_refs}"
            assert kserve_owners[0]["name"] == KSERVE_CR_NAME, \
                f"Deployment {name} ownerReference name should be {KSERVE_CR_NAME}, " \
                f"got: {kserve_owners[0]['name']}"
            assert kserve_owners[0]["uid"] == cr_uid, \
                f"Deployment {name} ownerReference uid should be {cr_uid}, " \
                f"got: {kserve_owners[0].get('uid')}"
