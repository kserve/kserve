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

import pytest

from .fixtures import (
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
