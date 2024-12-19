# Copyright 2024 The KServe Authors.
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
from kserve.utils.inference_client_factory import InferenceClientFactory
from kserve.inference_client import InferenceRESTClient, RESTConfig


@pytest.fixture
def factory():
    return InferenceClientFactory()


def test_get_rest_client_v1(factory):
    config = RESTConfig(protocol="v1")
    client = factory.get_rest_client(config)
    assert client._config.protocol == "v1"
    assert factory._rest_v1_client is client


def test_get_rest_cleint_v1_config_none(factory):
    client = factory.get_rest_client()
    assert isinstance(client, InferenceRESTClient)
    assert client._config.protocol == "v1"
    assert factory._rest_v1_client is client


def test_get_rest_client_v2(factory):
    config = RESTConfig(protocol="v2")
    client = factory.get_rest_client(config)
    assert isinstance(client, InferenceRESTClient)
    assert client._config.protocol == "v2"
    assert factory._rest_v2_client is client


def test_get_rest_client_v1_cached(factory):
    config = RESTConfig(protocol="v1")
    client1 = factory.get_rest_client(config)
    client2 = factory.get_rest_client(config)
    assert client1._config.protocol == "v1"
    assert client1 is client2


def test_get_rest_client_v2_cached(factory):
    config = RESTConfig(protocol="v2")
    client1 = factory.get_rest_client(config)
    client2 = factory.get_rest_client(config)
    assert client1._config.protocol == "v2"
    assert client1 is client2
