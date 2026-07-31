"""Tests for marker expression parsing and matching."""

from __future__ import annotations

import argparse
import json
from pathlib import Path

import pytest

from test_selector.cli import cmd_learn, cmd_query
from test_selector.selector.matcher import expression_matches, extract_positive_markers


# -- extract_positive_markers -------------------------------------------------


@pytest.mark.parametrize(
    "expr, expected",
    [
        ("predictor", {"predictor"}),
        ("transformer or mms or collocation", {"transformer", "mms", "collocation"}),
        (
            "llminferenceservice and llmisvc_core and cluster_cpu",
            {"llminferenceservice", "llmisvc_core", "cluster_cpu"},
        ),
        ("autoscaling and not llminferenceservice", {"autoscaling"}),
        ("(predictor or graph) and not slow", {"predictor", "graph"}),
        ("not storage", set()),
        ("", set()),
    ],
    ids=[
        "simple",
        "or",
        "and",
        "and_not",
        "parens_and_not",
        "only_negated",
        "empty",
    ],
)
def test_extract_positive_markers(expr: str, expected: set[str]) -> None:
    assert extract_positive_markers(expr) == expected


# -- expression_matches: simple -----------------------------------------------


def test_simple_match() -> None:
    assert expression_matches("predictor", {"predictor", "graph"}) is True


def test_simple_no_match() -> None:
    assert expression_matches("predictor", {"llminferenceservice"}) is False


# -- expression_matches: or ---------------------------------------------------


def test_or_match_second() -> None:
    assert expression_matches("transformer or mms or collocation", {"mms"}) is True


def test_or_no_match() -> None:
    assert (
        expression_matches("transformer or mms or collocation", {"predictor"}) is False
    )


# -- expression_matches: and --------------------------------------------------


def test_and_match_any() -> None:
    assert (
        expression_matches(
            "llminferenceservice and llmisvc_core and cluster_cpu",
            {"llminferenceservice", "cluster_cpu"},
        )
        is True
    )


def test_and_no_match() -> None:
    assert (
        expression_matches(
            "llminferenceservice and llmisvc_core and cluster_cpu",
            {"predictor"},
        )
        is False
    )


# -- expression_matches: not --------------------------------------------------


def test_not_positive_matches() -> None:
    assert (
        expression_matches(
            "autoscaling and not llminferenceservice",
            {"autoscaling"},
        )
        is True
    )


def test_not_only_negated_marker() -> None:
    assert (
        expression_matches(
            "autoscaling and not llminferenceservice",
            {"llminferenceservice"},
        )
        is False
    )


# -- expression_matches: parens -----------------------------------------------


def test_parens_match() -> None:
    assert (
        expression_matches(
            "(predictor or graph) and not slow",
            {"graph"},
        )
        is True
    )


def test_parens_no_match() -> None:
    assert (
        expression_matches(
            "(predictor or graph) and not slow",
            {"slow"},
        )
        is False
    )


# -- edge cases ---------------------------------------------------------------


def test_empty_expression() -> None:
    assert expression_matches("", {"predictor"}) is False


def test_empty_selected() -> None:
    assert expression_matches("predictor", set()) is False


# -- all 20 CI expressions against known selected sets -----------------------


_ISVC_MARKERS = {
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
_LLMISVC_MARKERS = {
    "llminferenceservice",
    "autoscaling_hpa",
    "autoscaling_keda",
    "cluster_cpu",
    "tracing",
    "llmisvc_core",
    "conversion",
}
_MODELCACHE_MARKERS = {"modelcache"}


@pytest.mark.parametrize(
    "expr, selected, expected",
    [
        ("predictor", _ISVC_MARKERS, True),
        ("predictor", _LLMISVC_MARKERS, False),
        ("transformer or mms or collocation", _ISVC_MARKERS, True),
        ("transformer or mms or collocation", _LLMISVC_MARKERS, False),
        ("explainer", _ISVC_MARKERS, True),
        ("explainer", _MODELCACHE_MARKERS, False),
        ("graph", _ISVC_MARKERS, True),
        ("graph", _LLMISVC_MARKERS, False),
        ("path_based_routing", _ISVC_MARKERS, True),
        ("helm", _ISVC_MARKERS, True),
        ("raw", _ISVC_MARKERS, True),
        ("dual_protocol", _ISVC_MARKERS, True),
        ("rawcipn", _ISVC_MARKERS, True),
        ("autoscaling and not llminferenceservice", _ISVC_MARKERS, True),
        ("autoscaling and not llminferenceservice", _LLMISVC_MARKERS, False),
        ("kourier", _ISVC_MARKERS, True),
        ("llm", _ISVC_MARKERS, True),
        ("vllm", _ISVC_MARKERS, True),
        ("vllm_runtime", _ISVC_MARKERS, True),
        (
            "llminferenceservice and llmisvc_core and cluster_cpu",
            _LLMISVC_MARKERS,
            True,
        ),
        ("llminferenceservice and llmisvc_core and cluster_cpu", _ISVC_MARKERS, False),
        ("llminferenceservice and conversion and cluster_cpu", _LLMISVC_MARKERS, True),
        (
            "llminferenceservice and autoscaling_hpa and cluster_cpu",
            _LLMISVC_MARKERS,
            True,
        ),
        (
            "llminferenceservice and autoscaling_keda and cluster_cpu",
            _LLMISVC_MARKERS,
            True,
        ),
        ("llminferenceservice and tracing and cluster_cpu", _LLMISVC_MARKERS, True),
        ("llminferenceservice and tracing and cluster_cpu", _ISVC_MARKERS, False),
        ("modelcache", _MODELCACHE_MARKERS, True),
        ("modelcache", _ISVC_MARKERS, False),
    ],
    ids=[
        "predictor-isvc",
        "predictor-llmisvc",
        "transformer_or-isvc",
        "transformer_or-llmisvc",
        "explainer-isvc",
        "explainer-modelcache",
        "graph-isvc",
        "graph-llmisvc",
        "path_based_routing-isvc",
        "helm-isvc",
        "raw-isvc",
        "dual_protocol-isvc",
        "rawcipn-isvc",
        "autoscaling_not-isvc",
        "autoscaling_not-llmisvc",
        "kourier-isvc",
        "llm-isvc",
        "vllm-isvc",
        "vllm_runtime-isvc",
        "llmisvc_core-llmisvc",
        "llmisvc_core-isvc",
        "conversion-llmisvc",
        "autoscaling_hpa-llmisvc",
        "autoscaling_keda-llmisvc",
        "tracing-llmisvc",
        "tracing-isvc",
        "modelcache-modelcache",
        "modelcache-isvc",
    ],
)
def test_ci_expressions(expr: str, selected: set[str], expected: bool) -> None:
    assert expression_matches(expr, selected) is expected


# -- --match-jobs CLI output ---------------------------------------------------


def _repo_root() -> Path:
    """Find the repo root."""
    here = Path(__file__).resolve()
    for parent in here.parents:
        if (parent / "go.mod").exists():
            return parent
    pytest.skip("repo root not found")


@pytest.fixture(scope="module")
def learned_repo() -> Path:
    """Run learn once for the module and return the repo root."""
    repo = _repo_root()
    args = argparse.Namespace(repo=str(repo))
    cmd_learn(args)
    return repo


def test_match_jobs_output(
    learned_repo: Path, capsys: pytest.CaptureFixture[str]
) -> None:
    """Verify --match-jobs prints job=true/false lines to stdout."""
    jobs = {
        "predictor": "predictor",
        "autoscaling": "autoscaling and not llminferenceservice",
        "llmisvc-core": "llminferenceservice and llmisvc_core and cluster_cpu",
    }
    args = argparse.Namespace(
        repo=str(learned_repo),
        changed_files=["python/sklearnserver/setup.py"],
        match=None,
        match_jobs=json.dumps(jobs),
        format="json",
    )
    cmd_query(args)

    captured = capsys.readouterr()
    lines = captured.out.strip().splitlines()
    parsed = dict(line.split("=", 1) for line in lines)
    assert set(parsed.keys()) == {"predictor", "autoscaling", "llmisvc-core"}
    assert parsed["predictor"] == "true"
    assert parsed["llmisvc-core"] == "false"
    for val in parsed.values():
        assert val in ("true", "false")
