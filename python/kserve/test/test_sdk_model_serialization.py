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

"""Handwritten wire contracts complement generated constructor smoke tests."""

from kubernetes.client import V1EnvVar

from kserve import (
    ApiClient,
    V1alpha1BuiltInAdapter,
    V1beta1ModelFormat,
    V1beta1ModelSpec,
)


def test_model_spec_serializes_nested_models_and_camel_case_fields():
    model = V1beta1ModelSpec(
        name="predictor",
        model_format=V1beta1ModelFormat(name="sklearn", version="1"),
        storage_uri="s3://models/classifier",
        runtime_version="v1",
        protocol_version="v2",
    )

    with ApiClient() as client:
        payload = client.sanitize_for_serialization(model)

    assert payload == {
        "name": "predictor",
        "modelFormat": {"name": "sklearn", "version": "1"},
        "storageUri": "s3://models/classifier",
        "runtimeVersion": "v1",
        "protocolVersion": "v2",
    }


def test_adapter_serialization_omits_unset_fields_and_preserves_falsy_values():
    adapter = V1alpha1BuiltInAdapter(
        mem_buffer_bytes=0,
        runtime_management_port=9000,
        env=[V1EnvVar(name="OPTION", value="")],
    )

    with ApiClient() as client:
        assert client.sanitize_for_serialization(V1alpha1BuiltInAdapter()) == {}
        payload = client.sanitize_for_serialization(adapter)

    assert payload == {
        "memBufferBytes": 0,
        "runtimeManagementPort": 9000,
        "env": [{"name": "OPTION", "value": ""}],
    }
