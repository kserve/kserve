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
import kfserving
from tornado.httpclient import HTTPClientError


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
    def app(self):
        import kfserving
        model = DummyModel("TestModel")
        model.load()
        server = kfserving.KFServer(kfserving.server.TFSERVING_HTTP_PROTOCOL)
        server.register_model(model)
        return server.createApplication()

    async def test_liveness(self, http_server_client):
        resp = await http_server_client.fetch('/')
        assert resp.code == 200

    async def test_protocol(self, http_server_client):
        resp = await http_server_client.fetch('/protocol')
        assert resp.code == 200
        assert resp.body == b"tensorflow.http"

    async def test_model(self, http_server_client):
        resp = await http_server_client.fetch('/models/TestModel')
        assert resp.code == 200

    async def test_predict(selfself, http_server_client):
        resp = await http_server_client.fetch('/models/TestModel:predict', method="POST", body=b'{"instances":[[1,2]]}')
        assert resp.code == 200
        assert resp.body == b'{"predictions": [[1, 2]]}'


class TestSeldonHttpServer(object):

    @pytest.fixture(scope="class")
    def app(self):
        import kfserving
        model = DummyModel("TestModelSeldon")
        model.load()
        server = kfserving.KFServer(kfserving.server.SELDON_HTTP_PROTOCOL)
        server.register_model(model)
        return server.createApplication()

    async def test_liveness(self, http_server_client):
        resp = await http_server_client.fetch('/')
        assert resp.code == 200

    async def test_protocol(self, http_server_client):
        resp = await http_server_client.fetch('/protocol')
        assert resp.code == 200
        assert resp.body == b"seldon.http"

    async def test_model_ndarray(self, http_server_client):
        resp = await http_server_client.fetch('/models/TestModelSeldon:predict', method="POST",
                                              body=b'{"data":{"ndarray":[[1,2]]}}')
        assert resp.code == 200
        assert resp.body == b'{"data": {"ndarray": [[1, 2]]}}'

    async def test_model_tensor(self, http_server_client):
        resp = await http_server_client.fetch('/models/TestModelSeldon:predict', method="POST",
                                              body=b'{"data":{"tensor":{"shape":[1,2],"values":[1,2]}}}')
        assert resp.code == 200
        assert resp.body == b'{"data": {"tensor": {"shape": [1, 2], "values": [1, 2]}}}'

    async def test_model_tftensor(self, http_server_client):
        with pytest.raises(HTTPClientError):
            resp = await http_server_client.fetch('/models/TestModelSeldon:predict', method="POST",
                                                  body=b'{"data":{"tftensor":{}}}')
            assert resp.code == 400
