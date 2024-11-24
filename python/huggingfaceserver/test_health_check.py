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

import unittest
import requests
from unittest.mock import patch, MagicMock
import health_check


class TestHealthCheck(unittest.TestCase):

    @patch("health_check.ray.init")
    def test_initialize_ray_cluster(self, mock_ray_init):
        mock_ray_init.return_value = MagicMock()
        result = health_check.initialize_ray_cluster()
        # mock_ray_init.assert_called_once_with(address="auto")
        self.assertEqual(result, "Ray initialized")

    @patch("health_check.ray.init")
    def test_perform_health_check_success(self, mock_ray_init):
        mock_ray_init.return_value = MagicMock()
        result = health_check.check_startup()
        self.assertEqual(result, "Healthy")

    @patch("health_check.ray.init")
    def test_perform_health_check_failure(self, mock_ray_init):
        mock_ray_init.side_effect = Exception("Ray init failed")
        result = health_check.check_startup()
        self.assertEqual(result, "Unhealthy")

    # Test check_gpu_usage with healthy GPU usage
    @patch("health_check.ray.init")
    @patch("health_check.ray.nodes")
    def test_check_gpu_usage_healthy(mock_ray_init, mock_ray_nodes, capsys):
        mock_ray_init.return_value = MagicMock()
        mock_ray_nodes.return_value = [
            {
                "NodeID": "node_1",
                "Resources": {
                    "GPU": 1,
                    "GPU_group_0": 1,
                },
            },
            {
                "NodeID": "node_2",
                "Resources": {
                    "GPU": 1,
                    "GPU_group_0": 1,
                },
            },
        ]
        status = health_check.check_gpu_usage("Test GPU Usage")
        assert status == "Healthy"

    # Test check_gpu_usage with unhealthy GPU usage
    @patch("health_check.ray.init")
    @patch("health_check.ray.nodes")
    def test_check_gpu_usage_ungihealthy(mock_ray_init, mock_ray_nodes, capsys):
        mock_ray_init.return_value = MagicMock()
        mock_ray_nodes.return_value = [
            {
                "NodeID": "node_1",
                "Resources": {
                    "GPU": 1,
                    "GPU_group_0": 0,
                },
            },
            {
                "NodeID": "node_2",
                "Resources": {
                    "GPU": 1,
                    "GPU_group_0": 1,
                },
            },
        ]
        status = health_check.check_gpu_usage("Test GPU Usage")
        assert status == "Unhealthy"

    # Test check_registered_nodes with correct number of nodes
    @patch("health_check.ray.init")
    @patch("health_check.ray.nodes")
    def test_check_registered_nodes_healthy(mock_ray_init, mock_ray_nodes, capsys):
        mock_ray_init.return_value = MagicMock()
        mock_ray_nodes.return_value = [
            {
                "NodeID": "node_1",
                "Alive": True,
            },
            {
                "NodeID": "node_2",
                "Alive": True,
            },
        ]
        status = health_check.check_registered_nodes(2)
        assert status == "Healthy"

    # Test check_registered_nodes with incorrect number of nodes
    @patch("health_check.ray.init")
    @patch("health_check.ray.nodes")
    def test_check_registered_nodes_unhealthy(mock_ray_init, mock_ray_nodes, capsys):
        mock_ray_init.return_value = MagicMock()
        mock_ray_nodes.return_value = [
            {
                "NodeID": "node_1",
                "Alive": True,
            }
        ]
        status = health_check.check_registered_nodes(2)
        assert status == "Unhealthy"

    @patch("health_check.requests.get")
    def test_check_runtime_health_healthy(self, mock_get):
        mock_get.return_value.status_code = 200
        health_check_url = "http://example.com/health"
        status = health_check.check_runtime_health(health_check_url)

        assert status == "Healthy"
        mock_get.assert_called_once_with(health_check_url, timeout=5)

    @patch("health_check.requests.get")
    def test_check_runtime_health_unhealthy_status_code(self, mock_get):
        mock_get.return_value.status_code = 500
        health_check_url = "http://example.com/health"
        status = health_check.check_runtime_health(health_check_url)

        assert status == "Unhealthy"
        mock_get.assert_called_once_with(health_check_url, timeout=5)

    @patch("health_check.requests.get")
    def test_check_runtime_health_request_exception(self, mock_get):
        mock_get.side_effect = requests.RequestException
        health_check_url = "http://example.com/health"
        status = health_check.check_runtime_health(health_check_url)

        assert status == "Unhealthy"
        mock_get.assert_called_once_with(health_check_url, timeout=5)


if __name__ == "__main__":
    unittest.main()
