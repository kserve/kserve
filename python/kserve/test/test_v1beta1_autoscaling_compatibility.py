# Copyright 2026 The KServe Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import inspect

import pytest

from kserve import V1beta1AutoScalingSpec, V1beta1KEDAScalingConfig, V1beta1MetricsSpec
from kserve.configuration import Configuration


def test_auto_scaling_spec_previous_positional_signature_is_preserved():
    configuration = Configuration()

    spec = V1beta1AutoScalingSpec("behavior", ["metric"], configuration)

    assert spec.behavior == "behavior"
    assert spec.metrics == ["metric"]
    assert spec.keda is None
    assert spec.local_vars_configuration is configuration


def test_metrics_spec_previous_positional_signature_is_preserved():
    configuration = Configuration()

    spec = V1beta1MetricsSpec(
        "external", "podmetric", "resource", "External", configuration
    )

    assert spec.external == "external"
    assert spec.podmetric == "podmetric"
    assert spec.resource == "resource"
    assert spec.type == "External"
    assert spec.name is None
    assert spec.local_vars_configuration is configuration


@pytest.mark.parametrize(
    "model, field",
    [
        (V1beta1AutoScalingSpec, "keda"),
        (V1beta1MetricsSpec, "name"),
        (V1beta1MetricsSpec, "use_cached_metrics"),
    ],
)
def test_new_fields_are_keyword_only(model, field):
    parameter = inspect.signature(model).parameters[field]

    assert parameter.kind is inspect.Parameter.KEYWORD_ONLY


def test_keda_scaling_config_is_exported_at_top_level():
    spec = V1beta1AutoScalingSpec(keda=V1beta1KEDAScalingConfig(polling_interval=15))

    assert spec.keda.polling_interval == 15
