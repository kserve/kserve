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
import unittest
from unittest import mock
from unittest.mock import patch

from kserve.constants.constants import PredictorProtocol
from kserve.utils import utils
from kserve.utils.utils import get_liveness_endpoint, get_readiness_endpoint, get_model_ready_endpoint


class TestUtils(unittest.TestCase):
    def setUp(self):
        utils._http_client = None
        utils._grpc_client = None
        utils._channel = None

    @patch("grpc.aio.secure_channel")
    @patch("grpc.aio.insecure_channel")
    def test_get_grpc_client_with_default_port(self, mock_insecure_channel, mock_secure_channel):
        host = "dummy.host"
        assert utils._channel is None
        assert utils._grpc_client is None
        client = utils.get_grpc_client(host, False)
        mock_insecure_channel.assert_called_with(host + ":80")
        mock_secure_channel.assert_not_called()
        assert utils._channel is not None
        assert utils._grpc_client is client
        # verify the client is instantiated only once
        client_2 = utils.get_grpc_client(host, False)
        assert client_2 is client

    @patch("grpc.aio.secure_channel")
    @patch("grpc.aio.insecure_channel")
    def test_get_grpc_client_with_ssl_default_port(self, mock_insecure_channel, mock_secure_channel):
        host = "dummy.host"
        assert utils._channel is None
        assert utils._grpc_client is None
        client = utils.get_grpc_client(host, True)
        mock_secure_channel.assert_called_with(host + ":443", mock.ANY)
        mock_insecure_channel.assert_not_called()
        assert utils._channel is not None
        assert utils._grpc_client is client

    @patch("grpc.aio.secure_channel")
    @patch("grpc.aio.insecure_channel")
    def test_get_grpc_client_with_given_port(self, mock_insecure_channel, mock_secure_channel):
        host = "dummy.host:8080"
        assert utils._channel is None
        assert utils._grpc_client is None
        client = utils.get_grpc_client(host, False)
        mock_insecure_channel.assert_called_with(host)
        mock_secure_channel.assert_not_called()
        assert utils._channel is not None
        assert utils._grpc_client is client

    async def test_close_grpc_channel(self):
        utils.get_grpc_client("dummy.host", False)
        await utils.close_grpc_channel()
        assert utils._channel is None
        assert utils._grpc_client is None

    def test_get_http_client(self):
        assert utils._http_client is None
        client = utils.get_http_client()
        assert utils._http_client is client
        # verify the client is instantiated only once
        client_2 = utils.get_http_client()
        assert client_2 is client

    async def test_close_http_client(self):
        utils.get_http_client()
        await utils.close_http_client()
        assert utils._http_client is None

    def test_get_liveness_endpoint(self):
        host = "dummy.host:80"
        protocol = PredictorProtocol.REST_V1.value
        # v1 protocol without ssl
        url = get_liveness_endpoint(host, protocol, False)
        assert url == f"http://{host}"

        # v1 protocol with ssl
        url = get_liveness_endpoint(host, protocol, True)
        assert url == f"https://{host}"

        protocol = PredictorProtocol.REST_V2.value
        # v2 protocol without ssl
        url = get_liveness_endpoint(host, protocol, False)
        assert url == f"http://{host}/{protocol}/health/live"

        # v2 protocol with ssl
        url = get_liveness_endpoint(host, protocol, True)
        assert url == f"https://{host}/{protocol}/health/live"

    def test_get_readiness_endpoint(self):
        host = "dummy.host:80"
        protocol = PredictorProtocol.REST_V1.value
        # v1 protocol without ssl
        url = get_readiness_endpoint(host, protocol, False)
        assert url == f"http://{host}"

        # v1 protocol with ssl
        url = get_readiness_endpoint(host, protocol, True)
        assert url == f"https://{host}"

        protocol = PredictorProtocol.REST_V2.value
        # v2 protocol without ssl
        url = get_readiness_endpoint(host, protocol, False)
        assert url == f"http://{host}/{protocol}/health/ready"

        # v2 protocol with ssl
        url = get_readiness_endpoint(host, protocol, True)
        assert url == f"https://{host}/{protocol}/health/ready"

    def test_get_model_ready_endpoint(self):
        host = "dummy.host:80"
        model_name = "dummy_model"
        protocol = PredictorProtocol.REST_V1.value

        # v1 protocol without ssl
        url = get_model_ready_endpoint(host, protocol, False, model_name)
        assert url == f"http://{host}/{protocol}/models/{model_name}"

        # v1 protocol with ssl
        url = get_model_ready_endpoint(host, protocol, True, model_name)
        assert url == f"https://{host}/{protocol}/models/{model_name}"

        protocol = PredictorProtocol.REST_V2.value
        # v2 protocol without ssl
        url = get_model_ready_endpoint(host, protocol, False, model_name)
        assert url == f"http://{host}/{protocol}/models/{model_name}/ready"

        # v2 protocol with ssl
        url = get_model_ready_endpoint(host, protocol, True, model_name)
        assert url == f"https://{host}/{protocol}/models/{model_name}/ready"
