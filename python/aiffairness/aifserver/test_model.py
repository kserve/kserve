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

"""Unit tests for AIFModel.explain().

aif360/kserve/nest_asyncio are replaced with lightweight stand-ins in
conftest.py, so these tests exercise AIFModel's own request handling
(specifically the "outputs" fallback) rather than the aif360 metric math.
"""

from unittest import mock

import numpy as np
import pytest

from aifserver.model import AIFModel

INSTANCES = [[25, 50000], [40, 80000], [30, 60000], [35, 70000]]


def _make_model():
    return AIFModel(
        name="test",
        feature_names=["age", "income"],
        label_names=["label"],
        favorable_label=1.0,
        unfavorable_label=0.0,
        privileged_groups=[{"age": 1}],
        unprivileged_groups=[{"age": 0}],
    )


def test_explain_missing_outputs_falls_back_to_predict():
    """explain() must not raise KeyError when "outputs" is absent (issue #5850)."""
    model = _make_model()
    with mock.patch.object(
        model, "_predict", return_value=np.array([1, 0, 1, 0])
    ) as mocked_predict:
        result = model.explain({"instances": INSTANCES})

    mocked_predict.assert_called_once_with(INSTANCES)
    assert result["predictions"] == [1, 0, 1, 0]


def test_explain_uses_provided_outputs_without_predicting():
    """When "outputs" is present it is used directly; no prediction is run."""
    model = _make_model()
    with mock.patch.object(model, "_predict") as mocked_predict:
        result = model.explain({"instances": INSTANCES, "outputs": [1, 0, 1, 0]})

    mocked_predict.assert_not_called()
    assert result["predictions"] == [1, 0, 1, 0]


def test_explain_requires_instances():
    """A payload without "instances" is still a hard error."""
    model = _make_model()
    with pytest.raises(KeyError):
        model.explain({"outputs": [1, 0, 1, 0]})
