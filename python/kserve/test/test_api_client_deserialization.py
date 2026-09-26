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

import json
from types import SimpleNamespace

import pytest
from kubernetes import client as kubernetes_client

from kserve import ApiClient, V1beta1InferenceService, V1beta1PredictorSpec


@pytest.fixture
def api_client():
    with ApiClient() as client:
        yield client


def test_deserialize_inference_service_with_kubernetes_models(api_client):
    payload = {
        "apiVersion": "serving.kserve.io/v1beta1",
        "kind": "InferenceService",
        "metadata": {"name": "example", "labels": {"app": "predictor"}},
        "spec": {
            "predictor": {
                "containers": [
                    {
                        "name": "predictor",
                        "image": "example/predictor:latest",
                        "env": [
                            {
                                "name": "TOKEN",
                                "valueFrom": {
                                    "secretKeyRef": {
                                        "name": "credentials",
                                        "key": "token",
                                    }
                                },
                            }
                        ],
                        "resources": {"requests": {"cpu": "100m", "memory": "128Mi"}},
                    }
                ],
                "volumes": [{"name": "scratch", "emptyDir": {}}],
            }
        },
    }

    service = api_client.deserialize(
        SimpleNamespace(data=json.dumps(payload)), "V1beta1InferenceService"
    )

    assert isinstance(service, V1beta1InferenceService)
    assert isinstance(service.metadata, kubernetes_client.V1ObjectMeta)
    assert isinstance(service.spec.predictor, V1beta1PredictorSpec)
    container = service.spec.predictor.containers[0]
    assert isinstance(container, kubernetes_client.V1Container)
    assert isinstance(container.env[0], kubernetes_client.V1EnvVar)
    assert isinstance(container.env[0].value_from, kubernetes_client.V1EnvVarSource)
    assert isinstance(
        container.env[0].value_from.secret_key_ref,
        kubernetes_client.V1SecretKeySelector,
    )
    assert container.env[0].value_from.secret_key_ref.key == "token"
    assert isinstance(container.resources, kubernetes_client.V1ResourceRequirements)
    assert container.resources.requests == {"cpu": "100m", "memory": "128Mi"}
    volume = service.spec.predictor.volumes[0]
    assert isinstance(volume, kubernetes_client.V1Volume)
    assert isinstance(volume.empty_dir, kubernetes_client.V1EmptyDirVolumeSource)
    assert api_client.sanitize_for_serialization(service) == payload


@pytest.mark.parametrize(
    "response_type,payload",
    [
        ("V1ObjectMeta", {"name": "example"}),
        ("list[V1ObjectMeta]", [{"name": "example"}]),
        ("dict(str, V1ObjectMeta)", {"item": {"name": "example"}}),
        ("dict[str, V1ObjectMeta]", {"item": {"name": "example"}}),
    ],
)
def test_deserialize_external_model_response(api_client, response_type, payload):
    result = api_client.deserialize(
        SimpleNamespace(data=json.dumps(payload)), response_type
    )

    model = result[0] if isinstance(result, list) else result
    if isinstance(model, dict):
        model = model["item"]
    assert isinstance(model, kubernetes_client.V1ObjectMeta)
    assert model.name == "example"
    assert api_client.sanitize_for_serialization(result) == payload


def test_deserialize_unknown_model_raises(api_client):
    with pytest.raises(AttributeError, match="UnknownApiModel"):
        api_client.deserialize(SimpleNamespace(data="{}"), "UnknownApiModel")


def test_deserialize_null_external_model(api_client):
    assert api_client.deserialize(SimpleNamespace(data="null"), "V1ObjectMeta") is None
