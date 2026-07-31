"""Query-mode tests for the test selector engine.

Negative assertions use the stable marker names from CI workflows
(.github/workflows/e2e-test*.yaml):
  e2e-test.yml:           predictor, explainer, graph, transformer, mms,
                          collocation, raw, rawcipn, dual_protocol,
                          path_based_routing, kourier, helm, autoscaling,
                          llm, vllm, vllm_runtime
  e2e-test-llmisvc.yaml:  llminferenceservice, autoscaling_hpa,
                          autoscaling_keda, cluster_cpu, tracing
  e2e-test-modelcache.yaml: modelcache
"""

from __future__ import annotations

from pathlib import Path

from test_selector.mapping.schema import Mapping
from test_selector.selector.engine import select_tests

E2E_MARKERS = {
    "predictor",
    "explainer",
    "graph",
    "transformer",
    "mms",
    "collocation",
    "raw",
    "rawcipn",
    "dual_protocol",
    "path_based_routing",
    "kourier",
    "helm",
    "autoscaling",
    "llm",
    "vllm",
    "vllm_runtime",
}
LLMISVC_MARKERS = {
    "llminferenceservice",
    "autoscaling_hpa",
    "autoscaling_keda",
    "cluster_cpu",
    "tracing",
}
MODELCACHE_MARKERS = {"modelcache"}


def _query(mapping: Mapping, repo_root: Path, files: list[str]) -> dict:
    return select_tests(mapping, files, repo_root).to_dict()


# -- PR #5906: docs-only change (ignorable) ----------------------------------


def test_docs_only(mapping: Mapping, repo_root: Path) -> None:
    result = _query(
        mapping,
        repo_root,
        [
            "docs/samples/llmisvc/agentic-tool-calling/README.md",
        ],
    )
    assert result["go_tests"]["run"] is False
    assert result["python_tests"]["run"] is False
    assert result["e2e_tests"]["run"] is False
    assert result["reasons"] == []


# -- PR #5896: e2e test file in mapping --------------------------------------


def test_e2e_test_in_mapping(mapping: Mapping, repo_root: Path) -> None:
    result = _query(
        mapping,
        repo_root,
        [
            "test/e2e/llmisvc/test_llm_canary_lifecycle.py",
        ],
    )
    assert result["go_tests"]["run"] is False
    assert result["e2e_tests"]["run"] is True
    markers = set(result["e2e_tests"]["markers"])
    assert markers & LLMISVC_MARKERS
    assert not markers & E2E_MARKERS


# -- PR #5892: Go llmisvc controller -----------------------------------------


def test_go_llmisvc_controller(mapping: Mapping, repo_root: Path) -> None:
    result = _query(
        mapping,
        repo_root,
        [
            "pkg/controller/v1alpha2/llmisvc/scheduler.go",
        ],
    )
    assert result["go_tests"]["run"] is True
    assert "./cmd/llmisvc..." in result["go_tests"]["packages"]
    assert "./cmd/manager..." not in result["go_tests"]["packages"]
    assert result["e2e_tests"]["run"] is True
    markers = set(result["e2e_tests"]["markers"])
    assert "llminferenceservice" in markers
    assert not markers & E2E_MARKERS


# -- PR #5891: shared Go package (all-entrypoints escalation) -----------------


def test_shared_go_package(mapping: Mapping, repo_root: Path) -> None:
    result = _query(
        mapping,
        repo_root,
        [
            "pkg/utils/utils.go",
        ],
    )
    assert result["go_tests"]["run"] is True
    assert result["go_tests"]["all"] is True
    assert result["e2e_tests"]["run"] is True
    markers = set(result["e2e_tests"]["markers"])
    assert markers >= E2E_MARKERS
    assert markers >= LLMISVC_MARKERS
    assert markers >= MODELCACHE_MARKERS


# -- PR #5889: Python SDK package (kserve -> all_e2e) -------------------------


def test_python_sdk_package(mapping: Mapping, repo_root: Path) -> None:
    result = _query(
        mapping,
        repo_root,
        [
            "python/kserve/kserve/utils/numpy_codec.py",
        ],
    )
    assert result["python_tests"]["run"] is True
    assert "kserve" in result["python_tests"]["packages"]
    assert result["e2e_tests"]["run"] is True
    markers = set(result["e2e_tests"]["markers"])
    assert markers >= E2E_MARKERS
    assert markers >= LLMISVC_MARKERS


# -- PR #5888: Makefile override + config files -------------------------------


def test_makefile_override_with_config(mapping: Mapping, repo_root: Path) -> None:
    result = _query(
        mapping,
        repo_root,
        [
            "Makefile",
            "config/crd/full/llmisvc/serving.kserve.io_llminferenceserviceconfigs.yaml",
        ],
    )
    assert result["go_tests"]["run"] is True
    assert result["go_tests"]["all"] is True
    assert result["python_tests"]["run"] is True
    assert result["e2e_tests"]["run"] is True
    markers = set(result["e2e_tests"]["markers"])
    assert markers >= E2E_MARKERS
    assert markers >= LLMISVC_MARKERS


# -- PR #5884: e2e test file not in mapping (path inference) ------------------


def test_e2e_test_not_in_mapping(mapping: Mapping, repo_root: Path) -> None:
    result = _query(
        mapping,
        repo_root,
        [
            "test/e2e/predictor/test_canary_raw_deployment.py",
        ],
    )
    assert result["e2e_tests"]["run"] is True
    markers = set(result["e2e_tests"]["markers"])
    assert "predictor" in markers
    assert not markers & LLMISVC_MARKERS
    assert not markers & MODELCACHE_MARKERS


# -- PR #5880: Go controller with test file -----------------------------------


def test_go_controller_with_test(mapping: Mapping, repo_root: Path) -> None:
    result = _query(
        mapping,
        repo_root,
        [
            "pkg/controller/v1alpha2/llmisvc/config_merge.go",
            "pkg/controller/v1alpha2/llmisvc/config_merge_test.go",
        ],
    )
    assert result["go_tests"]["run"] is True
    assert result["e2e_tests"]["run"] is True
    markers = set(result["e2e_tests"]["markers"])
    assert "llminferenceservice" in markers
    assert "cluster_cpu" in markers
    assert not markers & E2E_MARKERS


# -- Edge case: unknown file -> conservative:all ------------------------------


def test_unknown_file_triggers_all(mapping: Mapping, repo_root: Path) -> None:
    result = _query(
        mapping,
        repo_root,
        [
            "random/unknown/file.xyz",
        ],
    )
    assert result["go_tests"]["run"] is True
    assert result["go_tests"]["all"] is True
    assert result["python_tests"]["run"] is True
    assert result["e2e_tests"]["run"] is True
    assert any("conservative:all" in r for r in result["reasons"])


# -- Edge case: ignorable file -----------------------------------------------


def test_ignorable_file(mapping: Mapping, repo_root: Path) -> None:
    result = _query(
        mapping,
        repo_root,
        [
            "CODEOWNERS",
        ],
    )
    assert result["go_tests"]["run"] is False
    assert result["python_tests"]["run"] is False
    assert result["e2e_tests"]["run"] is False
    assert result["reasons"] == []


# -- Edge case: config file with CRD mapping (narrow, not all) ----------------


def test_config_crd_mapping_narrow(mapping: Mapping, repo_root: Path) -> None:
    result = _query(
        mapping,
        repo_root,
        [
            "config/crd/full/llmisvc/serving.kserve.io_llminferenceserviceconfigs.yaml",
        ],
    )
    assert result["e2e_tests"]["run"] is True
    markers = set(result["e2e_tests"]["markers"])
    assert "llminferenceservice" in markers
    assert not markers & E2E_MARKERS
    assert not markers & MODELCACHE_MARKERS


# -- Edge case: framework-specific Go file (narrow selection) -----------------


def test_framework_specific_go_file(mapping: Mapping, repo_root: Path) -> None:
    result = _query(
        mapping,
        repo_root,
        [
            "pkg/apis/serving/v1beta1/predictor_sklearn.go",
        ],
    )
    assert result["go_tests"]["run"] is True
    assert result["e2e_tests"]["run"] is True
    markers = set(result["e2e_tests"]["markers"])
    assert "predictor" in markers
    assert not markers & LLMISVC_MARKERS


# -- Common packages ----------------------------------------------------------


def test_pkg_constants(mapping: Mapping, repo_root: Path) -> None:
    result = _query(
        mapping,
        repo_root,
        [
            "pkg/constants/constants.go",
        ],
    )
    assert result["go_tests"]["run"] is True
    assert result["go_tests"]["all"] is True
    assert result["e2e_tests"]["run"] is True
    markers = set(result["e2e_tests"]["markers"])
    assert markers >= E2E_MARKERS
    assert markers >= LLMISVC_MARKERS
    assert markers >= MODELCACHE_MARKERS


def test_pkg_webhook(mapping: Mapping, repo_root: Path) -> None:
    result = _query(
        mapping,
        repo_root,
        [
            "pkg/webhook/admission/pod/mutator.go",
        ],
    )
    assert result["go_tests"]["run"] is True
    assert result["e2e_tests"]["run"] is True
    markers = set(result["e2e_tests"]["markers"])
    assert "predictor" in markers
    assert "graph" in markers


def test_isvc_controller(mapping: Mapping, repo_root: Path) -> None:
    result = _query(
        mapping,
        repo_root,
        [
            "pkg/controller/v1beta1/inferenceservice/controller.go",
        ],
    )
    assert result["go_tests"]["run"] is True
    assert "./cmd/manager..." in result["go_tests"]["packages"]
    assert result["e2e_tests"]["run"] is True
    markers = set(result["e2e_tests"]["markers"])
    assert {"predictor", "graph", "transformer"} <= markers
    assert not markers & LLMISVC_MARKERS


def test_localmodel_controller(mapping: Mapping, repo_root: Path) -> None:
    result = _query(
        mapping,
        repo_root,
        [
            "pkg/controller/v1alpha1/localmodel/controller.go",
        ],
    )
    assert result["go_tests"]["run"] is True
    assert result["e2e_tests"]["run"] is True
    markers = set(result["e2e_tests"]["markers"])
    assert "modelcache" in markers
    assert "llminferenceservice" in markers
    assert "predictor" in markers


# -- Sidecar entrypoints (no CRDs, no e2e) ------------------------------------


def test_go_agent_sidecar(mapping: Mapping, repo_root: Path) -> None:
    result = _query(
        mapping,
        repo_root,
        [
            "pkg/agent/syncer.go",
        ],
    )
    assert result["go_tests"]["run"] is True
    assert result["go_tests"].get("all") is not True
    assert result["e2e_tests"]["run"] is False
    assert result["python_tests"]["run"] is False


# -- Python SDK model files (narrow by CRD kind) ------------------------------


def test_python_sdk_model_llmisvc(mapping: Mapping, repo_root: Path) -> None:
    result = _query(
        mapping,
        repo_root,
        [
            "python/kserve/kserve/models/v1alpha2_llm_inference_service.py",
        ],
    )
    assert result["python_tests"]["run"] is True
    assert "kserve" in result["python_tests"]["packages"]
    assert result["e2e_tests"]["run"] is True
    markers = set(result["e2e_tests"]["markers"])
    assert "llminferenceservice" in markers
    assert not markers & E2E_MARKERS


def test_python_sdk_model_isvc(mapping: Mapping, repo_root: Path) -> None:
    result = _query(
        mapping,
        repo_root,
        [
            "python/kserve/kserve/models/v1beta1_inference_service_spec.py",
        ],
    )
    assert result["python_tests"]["run"] is True
    assert result["e2e_tests"]["run"] is True
    markers = set(result["e2e_tests"]["markers"])
    assert "predictor" in markers
    assert not markers & LLMISVC_MARKERS


def test_python_sdk_model_init_all_e2e(mapping: Mapping, repo_root: Path) -> None:
    result = _query(
        mapping,
        repo_root,
        [
            "python/kserve/kserve/models/__init__.py",
        ],
    )
    assert result["python_tests"]["run"] is True
    assert result["e2e_tests"]["run"] is True
    markers = set(result["e2e_tests"]["markers"])
    assert markers >= E2E_MARKERS
    assert markers >= LLMISVC_MARKERS


def test_python_sdk_model_no_version_prefix(mapping: Mapping, repo_root: Path) -> None:
    result = _query(
        mapping,
        repo_root,
        [
            "python/kserve/kserve/models/knative_condition.py",
        ],
    )
    assert result["python_tests"]["run"] is True
    assert result["e2e_tests"]["run"] is True
    markers = set(result["e2e_tests"]["markers"])
    assert markers >= E2E_MARKERS
    assert markers >= LLMISVC_MARKERS


# -- Python server packages (framework narrowing) -----------------------------


def test_python_server_sklearn(mapping: Mapping, repo_root: Path) -> None:
    result = _query(
        mapping,
        repo_root,
        [
            "python/sklearnserver/sklearnserver/model.py",
        ],
    )
    assert result["python_tests"]["run"] is True
    assert "sklearnserver" in result["python_tests"]["packages"]
    assert result["e2e_tests"]["run"] is True
    markers = set(result["e2e_tests"]["markers"])
    assert "predictor" in markers
    assert not markers & LLMISVC_MARKERS


def test_python_server_predictive_multi_format(
    mapping: Mapping, repo_root: Path
) -> None:
    """predictiveserver supports sklearn, xgboost, lightgbm via runtime YAML."""
    assert "predictiveserver" in mapping.server_to_frameworks
    formats = mapping.server_to_frameworks["predictiveserver"]
    assert "sklearn" in formats
    assert "xgboost" in formats
    assert "lightgbm" in formats
    result = _query(
        mapping,
        repo_root,
        [
            "python/predictiveserver/predictiveserver/model.py",
        ],
    )
    assert result["python_tests"]["run"] is True
    assert "predictiveserver" in result["python_tests"]["packages"]
    assert result["e2e_tests"]["run"] is True
    markers = set(result["e2e_tests"]["markers"])
    assert "predictor" in markers
    assert not markers & LLMISVC_MARKERS


def test_python_server_huggingface(mapping: Mapping, repo_root: Path) -> None:
    result = _query(
        mapping,
        repo_root,
        [
            "python/huggingfaceserver/huggingfaceserver/encoder_model.py",
        ],
    )
    assert result["python_tests"]["run"] is True
    assert "huggingfaceserver" in result["python_tests"]["packages"]
    assert result["e2e_tests"]["run"] is True
    markers = set(result["e2e_tests"]["markers"])
    assert not markers & LLMISVC_MARKERS


def test_server_to_frameworks_excludes_non_python(mapping: Mapping) -> None:
    """Runtime servers without matching python/ dirs are excluded."""
    assert "tritonserver" not in mapping.server_to_frameworks
    assert "torchserve" not in mapping.server_to_frameworks
    assert "tensorflow-serving" not in mapping.server_to_frameworks
    assert "mlserver" not in mapping.server_to_frameworks


# -- Override-only triggers ----------------------------------------------------


def test_go_mod_override(mapping: Mapping, repo_root: Path) -> None:
    result = _query(mapping, repo_root, ["go.mod"])
    assert result["go_tests"]["run"] is True
    assert result["go_tests"]["all"] is True
    assert result["python_tests"]["run"] is True
    assert result["e2e_tests"]["run"] is True
    assert any("override" in r for r in result["reasons"])


def test_go_sum_override(mapping: Mapping, repo_root: Path) -> None:
    result = _query(mapping, repo_root, ["go.sum"])
    assert result["go_tests"]["run"] is True
    assert result["go_tests"]["all"] is True
    assert result["python_tests"]["run"] is True
    assert result["e2e_tests"]["run"] is True


# -- Config discovery (charts, config, hack) -----------------------------------


def test_chart_llmisvc_crd(mapping: Mapping, repo_root: Path) -> None:
    result = _query(
        mapping,
        repo_root,
        [
            "charts/kserve-llmisvc-crd/templates/serving.kserve.io_llminferenceservices.yaml",
        ],
    )
    assert result["go_tests"]["run"] is False
    assert result["python_tests"]["run"] is False
    assert result["e2e_tests"]["run"] is True
    markers = set(result["e2e_tests"]["markers"])
    assert "llminferenceservice" in markers
    assert not markers & E2E_MARKERS


def test_chart_isvc_crd(mapping: Mapping, repo_root: Path) -> None:
    result = _query(
        mapping,
        repo_root,
        [
            "charts/kserve-crd/templates/serving.kserve.io_inferenceservices.yaml",
        ],
    )
    assert result["go_tests"]["run"] is False
    assert result["e2e_tests"]["run"] is True
    markers = set(result["e2e_tests"]["markers"])
    assert "predictor" in markers
    assert "graph" in markers
    assert not markers & LLMISVC_MARKERS


def test_config_overlay_narrow(mapping: Mapping, repo_root: Path) -> None:
    result = _query(
        mapping,
        repo_root,
        [
            "config/overlays/test/s3-local-backend/seaweedfs-init-job.yaml",
        ],
    )
    assert result["e2e_tests"]["run"] is True
    markers = set(result["e2e_tests"]["markers"])
    assert "graph" in markers
    assert not markers & LLMISVC_MARKERS
    assert not markers & MODELCACHE_MARKERS


def test_config_rbac_narrow(mapping: Mapping, repo_root: Path) -> None:
    result = _query(
        mapping,
        repo_root,
        [
            "config/rbac/llmisvc/role.yaml",
        ],
    )
    assert result["go_tests"]["run"] is False
    assert result["e2e_tests"]["run"] is True
    markers = set(result["e2e_tests"]["markers"])
    assert "llminferenceservice" in markers
    assert not markers & E2E_MARKERS


def test_chart_llmisvc_resources(mapping: Mapping, repo_root: Path) -> None:
    result = _query(
        mapping,
        repo_root,
        [
            "charts/kserve-llmisvc-resources/values.yaml",
        ],
    )
    assert result["go_tests"]["run"] is False
    assert result["e2e_tests"]["run"] is True
    markers = set(result["e2e_tests"]["markers"])
    assert "llminferenceservice" in markers
    assert not markers & E2E_MARKERS


def test_chart_kserve_resources(mapping: Mapping, repo_root: Path) -> None:
    result = _query(
        mapping,
        repo_root,
        [
            "charts/kserve-resources/values.yaml",
        ],
    )
    assert result["go_tests"]["run"] is False
    assert result["e2e_tests"]["run"] is True
    markers = set(result["e2e_tests"]["markers"])
    assert "helm" in markers
    assert "predictor" in markers
    assert "graph" in markers
    assert not markers & LLMISVC_MARKERS


def test_hack_install_script(mapping: Mapping, repo_root: Path) -> None:
    result = _query(
        mapping,
        repo_root,
        [
            "hack/setup/quick-install/llmisvc-full-install-with-manifests.sh",
        ],
    )
    assert result["e2e_tests"]["run"] is True
    markers = set(result["e2e_tests"]["markers"])
    assert markers >= E2E_MARKERS
    assert markers >= LLMISVC_MARKERS
    assert markers >= MODELCACHE_MARKERS


# -- E2e helper files ----------------------------------------------------------


def test_e2e_helper_conftest(mapping: Mapping, repo_root: Path) -> None:
    result = _query(
        mapping,
        repo_root,
        [
            "test/e2e/llmisvc/conftest.py",
        ],
    )
    assert result["go_tests"]["run"] is False
    assert result["e2e_tests"]["run"] is True
    markers = set(result["e2e_tests"]["markers"])
    assert "llminferenceservice" in markers
    assert not markers & E2E_MARKERS


def test_e2e_helper_fixtures(mapping: Mapping, repo_root: Path) -> None:
    result = _query(
        mapping,
        repo_root,
        [
            "test/e2e/llmisvc/fixtures.py",
        ],
    )
    assert result["e2e_tests"]["run"] is True
    markers = set(result["e2e_tests"]["markers"])
    assert "llminferenceservice" in markers
    assert not markers & E2E_MARKERS


def test_e2e_helper_not_in_mapping_logger(mapping: Mapping, repo_root: Path) -> None:
    """Helper file not in mapping falls back to directory-based inference."""
    result = _query(
        mapping,
        repo_root,
        [
            "test/e2e/logger/format_verifiers.py",
        ],
    )
    assert result["e2e_tests"]["run"] is True
    markers = set(result["e2e_tests"]["markers"])
    assert "predictor" in markers
    assert not markers & LLMISVC_MARKERS


def test_e2e_helper_not_in_mapping_batcher(mapping: Mapping, repo_root: Path) -> None:
    """Batcher dir also infers predictor marker."""
    result = _query(
        mapping,
        repo_root,
        [
            "test/e2e/batcher/__init__.py",
        ],
    )
    assert result["e2e_tests"]["run"] is True
    markers = set(result["e2e_tests"]["markers"])
    assert "predictor" in markers
    assert not markers & LLMISVC_MARKERS


# -- Entrypoint main.go files -------------------------------------------------


def test_cmd_llmisvc_entrypoint(mapping: Mapping, repo_root: Path) -> None:
    result = _query(mapping, repo_root, ["cmd/llmisvc/main.go"])
    assert result["go_tests"]["run"] is True
    assert result["e2e_tests"]["run"] is True
    markers = set(result["e2e_tests"]["markers"])
    assert "llminferenceservice" in markers
    assert not markers & E2E_MARKERS


def test_cmd_manager_entrypoint(mapping: Mapping, repo_root: Path) -> None:
    result = _query(mapping, repo_root, ["cmd/manager/main.go"])
    assert result["go_tests"]["run"] is True
    assert result["e2e_tests"]["run"] is True
    markers = set(result["e2e_tests"]["markers"])
    assert {"predictor", "graph", "transformer"} <= markers
    assert not markers & LLMISVC_MARKERS


# -- Multi-file changes -------------------------------------------------------


def test_mixed_go_controller_and_e2e(mapping: Mapping, repo_root: Path) -> None:
    result = _query(
        mapping,
        repo_root,
        [
            "pkg/controller/v1alpha2/llmisvc/scheduler.go",
            "test/e2e/llmisvc/fixtures.py",
        ],
    )
    assert result["go_tests"]["run"] is True
    assert result["e2e_tests"]["run"] is True
    markers = set(result["e2e_tests"]["markers"])
    assert "llminferenceservice" in markers
    assert not markers & E2E_MARKERS
    assert len(result["reasons"]) >= 2
