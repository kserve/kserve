# Copyright 2023 The KServe Authors.
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
from unittest.mock import patch
import sys
import requests
import health_check  # Assume your script is named health_check.py


# Mocking sys.exit to prevent the script from exiting during tests
@pytest.fixture(autouse=True)
def no_exit(monkeypatch):
    monkeypatch.setattr(sys, "exit", lambda x: None)


# Test check_gpu_usage with healthy GPU usage
@patch("health_check.ray.cluster_resources", return_value={"GPU": 4, "GPU_group_0": 4})
def test_check_gpu_usage_healthy(mock_cluster_resources, capsys):
    health_check.check_gpu_usage("Test GPU Usage")
    captured = capsys.readouterr()
    assert "Healthy" in captured.out


# Test check_gpu_usage with unhealthy GPU usage
@patch("health_check.ray.cluster_resources", return_value={"GPU": 4, "GPU_group_0": 2})
def test_check_gpu_usage_unhealthy(mock_cluster_resources, capsys):
    health_check.check_gpu_usage("Test GPU Usage")
    captured = capsys.readouterr()
    assert "Unhealthy" in captured.out


# Test check_registered_nodes with correct number of nodes
@patch("health_check.ray.nodes", return_value=[{"Alive": True}, {"Alive": True}])
def test_check_registered_nodes_healthy(mock_nodes, capsys):
    health_check.check_registered_nodes(2)
    captured = capsys.readouterr()
    assert "Unhealthy" not in captured.out


# Test check_registered_nodes with incorrect number of nodes
@patch("health_check.ray.nodes", return_value=[{"Alive": True}])
def test_check_registered_nodes_unhealthy(mock_nodes, capsys):
    health_check.check_registered_nodes(2)
    captured = capsys.readouterr()
    assert "Unhealthy" in captured.out


# Test check_readiness with healthy conditions
@patch("health_check.ray.nodes", return_value=[{"Alive": True}, {"Alive": True}])
@patch("health_check.ray.cluster_resources", return_value={"GPU": 4, "GPU_group_0": 4})
@patch("health_check.requests.get")
def test_check_readiness_healthy(mock_get, mock_cluster_resources, mock_nodes, capsys):
    mock_get.return_value.status_code = 200
    health_check.check_readiness(2, "http://localhost:8080")
    captured = capsys.readouterr()
    assert "Healthy" in captured.out


# Test check_readiness with unhealthy Huggingface server
@patch("health_check.ray.nodes", return_value=[{"Alive": True}, {"Alive": True}])
@patch("health_check.ray.cluster_resources", return_value={"GPU": 4, "GPU_group_0": 4})
@patch("health_check.requests.get", side_effect=requests.RequestException)
def test_check_readiness_unhealthy_server(
    mock_get, mock_cluster_resources, mock_nodes, capsys
):
    health_check.check_readiness(2, "http://localhost:8080")
    captured = capsys.readouterr()
    assert "Unhealthy - Hugging Face server is not reachable" in captured.out


# Test check_startup with Ray running
@patch("health_check.ray.nodes", return_value=[{"Alive": True}, {"Alive": True}])
def test_check_startup(mock_nodes, capsys):
    health_check.check_startup()
    captured = capsys.readouterr()
    assert "Ray is running" in captured.out


# Test check_startup with Ray not running
@patch("health_check.ray.nodes", side_effect=Exception("Failed to get Ray status"))
def test_check_startup_error(mock_nodes, capsys):
    health_check.check_startup()
    captured = capsys.readouterr()
    assert "Failed to get Ray status" in captured.out
