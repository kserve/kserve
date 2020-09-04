# Copyright 2020 kubeflow.org.
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

import nest_asyncio
import pytest
import os
import kfserving
from tornado.httpclient import HTTPClientError


class DummyModel(kfserving.KFModel):
    def __init__(self, name):
        super().__init__(name)
        self.name = name
        self.ready = False

    def load(self):
        self.ready = True

    async def predict(self, request):
        return {"predictions": request["instances"]}

    async def explain(self, request):
        return {"predictions": request["instances"]}


class TestTFHttpServer():

    @pytest.fixture(scope="class")
    def app(self):  # pylint: disable=no-self-use
        model = DummyModel("TestModel")
        model.load()
        server = kfserving.KFServer()
        server.register_model(model)
        models_dir = os.path.join(os.path.dirname(__file__), "example_models", "xgboost")
        server.registered_models.set_models_dir(models_dir)
        return server.create_application()

    async def test_liveness(self, http_server_client):
        resp = await http_server_client.fetch('/')
        assert resp.code == 200

    async def test_model(self, http_server_client):
        resp = await http_server_client.fetch('/v1/models/TestModel')
        assert resp.code == 200

    async def test_predict(self, http_server_client):
        resp = await http_server_client.fetch('/v1/models/TestModel:predict',
                                              method="POST",
                                              body=b'{"instances":[[1,2]]}')
        assert resp.code == 200
        assert resp.body == b'{"predictions": [[1, 2]]}'
        assert resp.headers['content-type'] == "application/json; charset=UTF-8"

    async def test_explain(self, http_server_client):
        resp = await http_server_client.fetch('/v1/models/TestModel:explain',
                                              method="POST",
                                              body=b'{"instances":[[1,2]]}')
        assert resp.code == 200
        assert resp.body == b'{"predictions": [[1, 2]]}'
        assert resp.headers['content-type'] == "application/json; charset=UTF-8"

    async def test_load(self, http_server_client):
        nest_asyncio.apply()
        resp = await http_server_client.fetch('/v1/models/model/load',
                                              method="POST", body=b'')
        assert resp.code == 200
        assert resp.body == b'succeed to load model model'

    async def test_unload(self, http_server_client):
        resp = await http_server_client.fetch('/v1/models/model/unload',
                                              method="POST", body=b'')
        assert resp.code == 200
        assert resp.body == b'succeed to unload model model'

    async def test_load_fail(self, http_server_client):
        nest_asyncio.apply()
        with pytest.raises(HTTPClientError) as err:
            _ = await http_server_client.fetch('/v1/models/Model1/load',
                                               method="POST", body=b'')
        assert err.value.code == 404

    async def test_unload_fail(self, http_server_client):
        with pytest.raises(HTTPClientError) as err:
            _ = await http_server_client.fetch('/v1/models/ModelNotExist/unload',
                                               method="POST", body=b'')
        assert err.value.code == 404

    async def test_list(self, http_server_client):
        resp = await http_server_client.fetch('/v1/models')
        assert resp.code == 200
        assert resp.body == b'["TestModel"]'


class TestTFHttpServerModelNotLoaded():

    @pytest.fixture(scope="class")
    def app(self):  # pylint: disable=no-self-use
        model = DummyModel("TestModel")
        server = kfserving.KFServer()
        server.register_model(model)
        return server.create_application()

    async def test_model_not_ready_error(self, http_server_client):
        with pytest.raises(HTTPClientError) as excinfo:
            _ = await http_server_client.fetch('/v1/models/TestModel')
        assert excinfo.value.code == 503
