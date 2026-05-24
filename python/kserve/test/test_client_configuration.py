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

from kubernetes import client
from unittest.mock import patch
from kserve import KServeClient

cfg = client.Configuration()
cfg.host = "https://test-cluster.example.com"
cfg.api_key = {"authorization": "Bearer test-token"}

kserve_client = KServeClient(client_configuration=cfg)

def test_client_configuration_is_wired_to_all_api_clients():
    """client_configuration must be passed directly to every underlying API
    client without reading ~/.kube/config or setting a global default."""
    assert kserve_client.core_api.api_client.configuration.host == cfg.host
    assert kserve_client.app_api.api_client.configuration.host == cfg.host
    assert kserve_client.api_instance.api_client.configuration.host == cfg.host
    assert kserve_client.hpa_v2_api.api_client.configuration.host == cfg.host
    assert kserve_client.core_api.api_client.configuration.api_key == cfg.api_key
    assert kserve_client.app_api.api_client.configuration.api_key == cfg.api_key
    assert kserve_client.api_instance.api_client.configuration.api_key == cfg.api_key
    assert kserve_client.hpa_v2_api.api_client.configuration.api_key == cfg.api_key

def test_config_dict_takes_priority_over_client_configuration():
    """When config_dict and client_configuration are both provided,
    config_dict wins and client_configuration is ignored."""
    with patch("kubernetes.config.load_kube_config_from_dict") as mock_load:
        KServeClient(config_dict={"apiVersion": "v1"}, client_configuration=cfg)
        mock_load.assert_called_once()
        _, kwargs = mock_load.call_args
        assert kwargs["client_configuration"] is None

def test_config_file_takes_priority_over_client_configuration():
    """When config_file and client_configuration are both provided,
    config_file wins and load_kube_config is called instead of using
    client_configuration directly."""
    with patch("kubernetes.config.load_kube_config") as mock_load:
        KServeClient(config_file="./kserve/test/kubeconfig", client_configuration=cfg)
        mock_load.assert_called_once()
        _, kwargs = mock_load.call_args
        assert kwargs["config_file"] == "./kserve/test/kubeconfig"
