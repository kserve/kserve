import pytest
import kfserving
from kfserving.protocols.tensorflow_http import tensorflow_request_to_list, ndarray_to_tensorflow_response
from kfserving.protocols.seldon_http import seldon_request_to_ndarray, ndarray_to_seldon_response
import numpy as np
from tornado.httpclient import HTTPClientError
from typing import Dict

class DummyModel(kfserving.KFModel):
    def __init__(self, name,protocol):
        self.name = name
        self.ready = False
        self.protocol = protocol

    def _extract_data(self,body):
        if self.protocol == kfserving.server.TFSERVING_HTTP_PROTOCOL:
            return tensorflow_request_to_list(body)
        elif self.protocol == kfserving.server.SELDON_HTTP_PROTOCOL:
            return seldon_request_to_ndarray(body)

    def _create_response(self,request : Dict, prediction : np.ndarray) -> Dict:
        if self.protocol == kfserving.server.TFSERVING_HTTP_PROTOCOL:
            return ndarray_to_tensorflow_response(prediction)
        elif self.protocol == kfserving.server.SELDON_HTTP_PROTOCOL:
            return ndarray_to_seldon_response(request,prediction)
        else:
            raise Exception("Invalid protocol %s" % self.protocol)


    def load(self):
        self.ready = True

    def predict(self, inputs):
        data = self._extract_data(inputs)
        data = np.array(data) # Do nothing but return request data
        response = self._create_response(inputs,data)
        return response


class TestTFHttpServer(object):

    @pytest.fixture(scope="class")
    def app(self):
        import kfserving
        model = DummyModel("TestModel",kfserving.server.TFSERVING_HTTP_PROTOCOL)
        model.load()
        server = kfserving.KFServer(kfserving.server.TFSERVING_HTTP_PROTOCOL)
        server.register_model(model)
        return server.createApplication()

    async def test_liveness(self,http_server_client):
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
        resp = await http_server_client.fetch('/models/TestModel:predict',method="POST",body=b'{"instances":[[1,2]]}')
        assert resp.code == 200
        assert resp.body == b"{'predictions': [[1, 2]]}"

class TestSeldonHttpServer(object):

    @pytest.fixture(scope="class")
    def app(self):
        import kfserving
        model = DummyModel("TestModelSeldon",kfserving.server.SELDON_HTTP_PROTOCOL)
        model.load()
        server = kfserving.KFServer(kfserving.server.SELDON_HTTP_PROTOCOL)
        server.register_model(model)
        return server.createApplication()

    async def test_liveness(self,http_server_client):
        resp = await http_server_client.fetch('/')
        assert resp.code == 200

    async def test_protocol(self, http_server_client):
        resp = await http_server_client.fetch('/protocol')
        assert resp.code == 200
        assert resp.body == b"seldon.http"

    async def test_model(self, http_server_client):
        resp = await http_server_client.fetch('/models/TestModelSeldon:predict',method="POST",body=b'{"data":{"ndarray":[[1,2]]}}')
        assert resp.code == 200

    async def test_model_tftensor(self, http_server_client):
        with pytest.raises(HTTPClientError):
            resp = await http_server_client.fetch('/models/TestModelSeldon:predict',method="POST",body=b'{"data":{"tftensor":{}}}')
            assert resp.code == 400




