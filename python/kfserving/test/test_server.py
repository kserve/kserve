# Copyright 2019 kubeflow.org.
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
from tornado.httpclient import HTTPClientError
import kfserving
from kfserving.server import Protocol  # pylint: disable=no-name-in-module
from kfserving.transformer import Transformer
from unittest import mock


class DummyModel(kfserving.KFModel):
    def __init__(self, name):
        super().__init__(name)
        self.name = name
        self.ready = False

    def load(self):
        self.ready = True

    def predict(self, inputs):
        return inputs


class TestTFHttpServer(object):

    @pytest.fixture(scope="class")
    def app(self):  # pylint: disable=no-self-use
        model = DummyModel("TestModel")
        model.load()
        server = kfserving.KFServer(Protocol.tensorflow_http)
        server.register_model(model)
        return server.create_application()

    async def test_liveness(self, http_server_client):
        resp = await http_server_client.fetch('/')
        assert resp.code == 200

    async def test_protocol(self, http_server_client):
        resp = await http_server_client.fetch('/protocol')
        assert resp.code == 200
        assert resp.body == b"tensorflow.http"

    async def test_model(self, http_server_client):
        resp = await http_server_client.fetch('/v1/models/TestModel')
        assert resp.code == 200

    async def test_predict(self, http_server_client):
        resp = await http_server_client.fetch('/v1/models/TestModel:predict', method="POST",
                                              body=b'{"instances":[[1,2]]}')
        assert resp.code == 200
        assert resp.body == b'{"predictions": [[1, 2]]}'


class TestSeldonHttpServer(object):

    @pytest.fixture(scope="class")
    def app(self):  # pylint: disable=no-self-use
        model = DummyModel("TestModelSeldon")
        model.load()
        server = kfserving.KFServer(Protocol.seldon_http)
        server.register_model(model)
        return server.create_application()

    async def test_liveness(self, http_server_client):
        resp = await http_server_client.fetch('/')
        assert resp.code == 200

    async def test_protocol(self, http_server_client):
        resp = await http_server_client.fetch('/protocol')
        assert resp.code == 200
        assert resp.body == b"seldon.http"

    async def test_model_ndarray(self, http_server_client):
        resp = await http_server_client.fetch('/v1/models/TestModelSeldon:predict', method="POST",
                                              body=b'{"data":{"ndarray":[[1,2]]}}')
        assert resp.code == 200
        assert resp.body == b'{"data": {"ndarray": [[1, 2]]}}'

    async def test_model_tensor(self, http_server_client):
        resp = await http_server_client.fetch('/v1/models/TestModelSeldon:predict', method="POST",
                                              body=b'{"data":{"tensor":{"shape":[1,2],\
                                                      "values":[1,2]}}}')
        assert resp.code == 200
        assert resp.body == b'{"data": {"tensor": {"shape": [1, 2], "values": [1, 2]}}}'

    async def test_model_tftensor(self, http_server_client):
        with pytest.raises(HTTPClientError):
            resp = await http_server_client.fetch('/v1/models/TestModelSeldon:predict', method="POST",
                                                  body=b'{"data":{"tftensor":{}}}')
            assert resp.code == 400


class TestTransformerHttpServer(object):

    @pytest.fixture(scope="class")
    def app(self):  # pylint: disable=no-self-use
        model = Transformer("TestModel", predictor_host='localhost', protocol=Protocol.tensorflow_http)
        model.load()
        server = kfserving.KFServer(Protocol.tensorflow_http)
        server.register_model(model)
        return server.create_application()

    async def test_liveness(self, http_server_client):
        resp = await http_server_client.fetch('/')
        assert resp.code == 200

    async def test_protocol(self, http_server_client):
        resp = await http_server_client.fetch('/protocol')
        assert resp.code == 200
        assert resp.body == b"tensorflow.http"

    async def test_model(self, http_server_client):
        resp = await http_server_client.fetch('/v1/models/TestModel')
        assert resp.code == 200

    @staticmethod
    def _mock_response(
            status=200,
            content="CONTENT",
            json_data=None,
            raise_for_status=None):
        """
                since we typically test a bunch of different
                requests calls for a service, we are going to do
                a lot of mock responses, so its usually a good idea
                to have a helper function that builds these things
                """

        mock_resp = mock.Mock()
        # mock raise_for_status call w/optional error
        mock_resp.raise_for_status = mock.Mock()

        if raise_for_status:
            mock_resp.raise_for_status.side_effect = raise_for_status
            # set status code and content
            mock_resp.status_code = status
            mock_resp.content = content
        # add json data if provided
        if json_data:
            mock_resp.json = mock.Mock(return_value=json_data)
        return mock_resp

    @mock.patch('requests.post')
    async def test_predict(self, mock_post, http_server_client):
        mock_resp = self._mock_response(content='{"predictions": [[1, 2]]}')
        mock_post.return_value = mock_resp
        resp = await http_server_client.fetch('/v1/models/TestModel:predict', method="POST",
                                              body=b'{"instances":[[1,2]]}')
        print(resp)
        assert resp.code == 200
        assert resp.body == b'{"predictions": [[1, 2]]}'
