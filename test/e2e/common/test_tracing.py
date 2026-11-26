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

from types import SimpleNamespace
from unittest.mock import Mock

import pytest

from . import tracing
from .tracing import (
    assert_connected_trace,
    assert_trace_descends_from_parent,
)


def _trace(
    trace_id="trace-id",
    transformer_parent=None,
    predictor_parent=None,
    include_transformer=True,
    include_predictor=True,
):
    processes = {
        "transformer-process": {"serviceName": "test-transformer"},
        "predictor-process": {"serviceName": "test-predictor"},
    }
    transformer = {"spanID": "transformer-span", "processID": "transformer-process"}
    predictor = {"spanID": "predictor-span", "processID": "predictor-process"}
    if transformer_parent:
        transformer["references"] = [
            {
                "refType": "CHILD_OF",
                "spanID": transformer_parent,
                "traceID": trace_id,
            }
        ]
    if predictor_parent:
        predictor["references"] = [
            {
                "refType": "CHILD_OF",
                "spanID": predictor_parent,
                "traceID": trace_id,
            }
        ]
    spans = []
    if include_transformer:
        spans.append(transformer)
    if include_predictor:
        spans.append(predictor)
    return {
        "traceID": trace_id,
        "spans": spans,
        "processes": processes,
    }


def test_connected_trace_rejects_disconnected_component_traces():
    with pytest.raises(AssertionError, match="No trace contains"):
        assert_connected_trace(
            [
                _trace("transformer-trace", include_predictor=False),
                _trace("predictor-trace", include_transformer=False),
            ],
            "test-transformer",
            "test-predictor",
        )


def test_connected_trace_rejects_shared_trace_without_lineage():
    with pytest.raises(AssertionError, match="No trace contains"):
        assert_connected_trace(
            [_trace("shared-trace")], "test-transformer", "test-predictor"
        )


def test_connected_trace_accepts_predictor_child_of_transformer():
    trace = _trace("shared-trace", predictor_parent="transformer-span")
    assert (
        assert_connected_trace([trace], "test-transformer", "test-predictor") == trace
    )


@pytest.mark.parametrize(
    "trace_id,parent,predictor_parent",
    [
        ("wrong-trace", "external-parent", "transformer-span"),
        ("supplied-trace", "wrong-parent", "transformer-span"),
        ("supplied-trace", "external-parent", "external-parent"),
    ],
)
def test_parent_trace_rejects_wrong_context_or_bypassed_transformer(
    trace_id, parent, predictor_parent
):
    trace = _trace(
        trace_id, transformer_parent=parent, predictor_parent=predictor_parent
    )
    with pytest.raises(AssertionError):
        assert_trace_descends_from_parent(
            trace,
            "supplied-trace",
            "external-parent",
            "test-transformer",
            "test-predictor",
        )


def test_parent_trace_requires_supplied_trace_and_parent():
    trace = _trace("supplied-trace", transformer_parent="external-parent")
    trace["spans"][1]["references"] = [
        {
            "refType": "CHILD_OF",
            "spanID": "transformer-span",
            "traceID": "supplied-trace",
        }
    ]
    assert_trace_descends_from_parent(
        trace,
        "supplied-trace",
        "external-parent",
        "test-transformer",
        "test-predictor",
    )


def test_diagnostics_collect_previous_logs_after_restarts():
    kserve_client = Mock()
    core = kserve_client.core_api
    core.list_namespaced_pod.return_value.items = [
        SimpleNamespace(
            metadata=SimpleNamespace(name="iris-predictor"),
            status=SimpleNamespace(
                phase="Running",
                conditions=[],
                init_container_statuses=[],
                container_statuses=[
                    SimpleNamespace(
                        name="kserve-container",
                        restart_count=1,
                        state="failed",
                        last_state="failed",
                    )
                ],
            ),
            spec=SimpleNamespace(
                init_containers=[],
                containers=[SimpleNamespace(name="kserve-container")],
            ),
        )
    ]
    core.list_namespaced_event.return_value = SimpleNamespace(
        items=[], metadata=SimpleNamespace(_continue=None)
    )
    tracing._collect_isvc_diagnostics(kserve_client, "iris", "worker-test")
    assert (
        kserve_client.api_instance.get_namespaced_custom_object.call_args.args[2]
        == "worker-test"
    )
    assert core.list_namespaced_pod.call_args.args[0] == "worker-test"
    assert all(
        c.kwargs["namespace"] == "worker-test"
        for c in core.read_namespaced_pod_log.call_args_list
    )
    assert (
        kserve_client.api_instance.get_namespaced_custom_object.call_args.kwargs[
            "_request_timeout"
        ]
        == 10
    )
    assert [
        c.kwargs["previous"] for c in core.read_namespaced_pod_log.call_args_list
    ] == [False, True]
    assert all(
        c.kwargs["tail_lines"] == 200
        for c in core.read_namespaced_pod_log.call_args_list
    )


def test_diagnostics_report_admission_events_without_pods(caplog):
    kserve_client = Mock()
    core = kserve_client.core_api
    core.list_namespaced_pod.return_value.items = []
    # A busy namespace can put the relevant admission event on a later page.
    core.list_namespaced_event.side_effect = [
        SimpleNamespace(items=[], metadata=SimpleNamespace(_continue="next-page")),
        SimpleNamespace(
            items=[
                SimpleNamespace(
                    involved_object=SimpleNamespace(name="iris-predictor-abcd"),
                    reason="FailedCreate",
                    message="admission denied",
                    last_timestamp=None,
                )
            ],
            metadata=SimpleNamespace(_continue=None),
        ),
    ]
    with caplog.at_level("INFO"):
        tracing._collect_isvc_diagnostics(kserve_client, "iris", "worker-test")
    assert all(
        c.args[0] == "worker-test" for c in core.list_namespaced_event.call_args_list
    )
    assert "admission denied" in caplog.text
    assert core.list_namespaced_event.call_count == 2
    assert core.list_namespaced_event.call_args.kwargs["_continue"] == "next-page"
    core.read_namespaced_pod_log.assert_not_called()


@pytest.mark.parametrize(
    "waiter,args",
    [
        ("_wait_for_connected_trace", ("iris", "trace-id", "worker-test")),
        ("_wait_for_parent_trace", ("iris", "trace-id", "parent", "worker-test")),
    ],
)
def test_trace_waits_query_the_selected_namespace(monkeypatch, waiter, args):
    query = Mock(return_value=[])
    monkeypatch.setattr(tracing, "_get_jaeger_traces", query)
    with pytest.raises(AssertionError, match="Timed out"):
        getattr(tracing, waiter)(*args, timeout=0)
    query.assert_called_once_with("iris", "worker-test")


def test_trace_absence_queries_the_selected_namespace(monkeypatch):
    query = Mock(return_value=[])
    monkeypatch.setattr(tracing, "_get_jaeger_traces", query)
    tracing._assert_no_traces_for_service("iris", "worker-test", 0)
    query.assert_called_once_with("iris", "worker-test")


def test_cleanup_deletes_from_the_selected_namespace():
    kserve_client = Mock()
    tracing._safe_delete(kserve_client, "iris", "worker-test")
    kserve_client.delete.assert_called_once_with("iris", "worker-test")
