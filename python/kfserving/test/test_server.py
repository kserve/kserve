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

import avro.io, avro.schema, io, json, pytest, requests
from cloudevents.http import CloudEvent, to_binary, to_structured
from kfserving import kfmodel
from kfserving import kfserver
from tornado.httpclient import HTTPClientError
from kfserving.kfmodel_repository import KFModelRepository

test_avsc_schema = '''
        {
        "namespace": "example.avro",
         "type": "record",
         "name": "User",
         "fields": [
             {"name": "name", "type": "string"},
             {"name": "favorite_number",  "type": ["int", "null"]},
             {"name": "favorite_color", "type": ["string", "null"]}
         ]
        }
        '''

def dummy_cloud_event(data, set_contenttype=False):
    # This data defines a binary cloudevent
    attributes = {
        "type": "com.example.sampletype1",
        "source": "https://example.com/event-producer",
        "specversion": "1.0",
        "id": "36077800-0c23-4f38-a0b4-01f4369f670a",
        "time": "2021-01-28T21:04:43.144141+00:00"
    }
    if set_contenttype:
        attributes["content-type"] = "application/json"

    event = CloudEvent(attributes, data)
    return event


class DummyModel(kfmodel.KFModel):
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

class DummyCEModel(kfmodel.KFModel):
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

class DummyAvroCEModel(kfmodel.KFModel):
    def __init__(self, name):
        super().__init__(name)
        self.name = name
        self.ready = False

    def load(self):
        self.ready = True

    def _parserequest(self, request):
        schema = avro.schema.parse(test_avsc_schema)
        raw_bytes = request
        bytes_reader = io.BytesIO(raw_bytes)
        decoder = avro.io.BinaryDecoder(bytes_reader)
        reader = avro.io.DatumReader(schema)
        record1 = reader.read(decoder)
        return record1

    def preprocess(self, request):
        if(isinstance(request, CloudEvent)):
            attributes = request._attributes
            assert attributes["specversion"] == "1.0"
            assert attributes["source"] == "https://example.com/event-producer"
            assert attributes["type"] == "com.example.sampletype1"
            assert attributes["datacontenttype"] == "application/x-www-form-urlencoded"
            assert attributes["content-type"] == "application/json"
            return request.data

    async def predict(self, request):
        record1 = self._parserequest(request)
        return {"predictions": [[record1['name'] , record1['favorite_number'], record1['favorite_color']]]}

    async def explain(self, request):
        record1 = self._parserequest(request)
        return {"predictions": [[record1['name'] , record1['favorite_number'], record1['favorite_color']]]}


class DummyKFModelRepository(KFModelRepository):
    def __init__(self, test_load_success: bool):
        super().__init__()
        self.test_load_success = test_load_success

    async def load(self, name: str) -> bool:
        if self.test_load_success:
            model = DummyModel(name)
            model.load()
            self.update(model)
            return model.ready
        else:
            return False


class TestTFHttpServer():

    @pytest.fixture(scope="class")
    def app(self):  # pylint: disable=no-self-use
        model = DummyModel("TestModel")
        model.load()
        server = kfserver.KFServer()
        server.register_model(model)
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

    async def test_predict_ce_structured(self, http_server_client):

        event = dummy_cloud_event({"instances":[[1,2]]})
        headers, body = to_structured(event)
        resp = await http_server_client.fetch('/v1/models/TestModel:predict',
                                              method="POST",
                                              headers=headers,
                                              body=body)

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

    async def test_list(self, http_server_client):
        resp = await http_server_client.fetch('/v1/models')
        assert resp.code == 200
        assert resp.body == b'["TestModel"]'


class TestTFHttpServerLoadAndUnLoad():
    @pytest.fixture(scope="class")
    def app(self):  # pylint: disable=no-self-use
        server = kfserver.KFServer(registered_models=DummyKFModelRepository(test_load_success=True))
        return server.create_application()

    async def test_load(self, http_server_client):
        resp = await http_server_client.fetch('/v2/repository/models/model/load',
                                              method="POST", body=b'')
        assert resp.code == 200
        assert resp.body == b'{"name": "model", "load": true}'

    async def test_unload(self, http_server_client):
        resp = await http_server_client.fetch('/v2/repository/models/model/unload',
                                              method="POST", body=b'')
        assert resp.code == 200
        assert resp.body == b'{"name": "model", "unload": true}'


class TestTFHttpServerLoadAndUnLoadFailure():
    @pytest.fixture(scope="class")
    def app(self):  # pylint: disable=no-self-use
        server = kfserver.KFServer(registered_models=DummyKFModelRepository(test_load_success=False))
        return server.create_application()

    async def test_load_fail(self, http_server_client):
        with pytest.raises(HTTPClientError) as err:
            _ = await http_server_client.fetch('/v2/repository/models/model/load',
                                               method="POST", body=b'')
        assert err.value.code == 503

    async def test_unload_fail(self, http_server_client):
        with pytest.raises(HTTPClientError) as err:
            _ = await http_server_client.fetch('/v2/repository/models/model/unload',
                                               method="POST", body=b'')
        assert err.value.code == 404


class TestTFHttpServerModelNotLoaded():

    @pytest.fixture(scope="class")
    def app(self):  # pylint: disable=no-self-use
        model = DummyModel("TestModel")
        server = kfserver.KFServer()
        server.register_model(model)
        return server.create_application()

    async def test_model_not_ready_error(self, http_server_client):
        with pytest.raises(HTTPClientError) as excinfo:
            _ = await http_server_client.fetch('/v1/models/TestModel')
        assert excinfo.value.code == 503

class TestTFHttpServerCloudEvent():

    @pytest.fixture(scope="class")
    def app(self):  # pylint: disable=no-self-use
        model = DummyCEModel("TestModel")
        server = kfserver.KFServer()
        server.register_model(model)
        return server.create_application()

    async def test_predict_ce_binary_dict(self, http_server_client):
        event = dummy_cloud_event({"instances":[[1,2]]}, set_contenttype=True)
        headers, body = to_binary(event)
        resp = await http_server_client.fetch('/v1/models/TestModel:predict',
                                              method="POST",
                                              headers=headers,
                                              body=body)
    
        assert resp.code == 200
        assert resp.body == b'{"predictions": [[1, 2]]}'
        assert resp.headers['content-type'] == "application/x-www-form-urlencoded"
        assert resp.headers['ce-specversion'] == "1.0"
        assert resp.headers['ce-id'] == "36077800-0c23-4f38-a0b4-01f4369f670a"
        assert resp.headers['ce-source'] == "https://example.com/event-producer"
        assert resp.headers['ce-type'] == "com.example.sampletype1"
        assert resp.headers['ce-datacontenttype'] == "application/x-www-form-urlencoded"
        assert resp.headers['ce-time'] > "2021-01-28T21:04:43.144141+00:00"

    async def test_predict_ce_binary_bytes(self, http_server_client):
        event = dummy_cloud_event(b'{"instances":[[1,2]]}', set_contenttype=True)
        headers, body = to_binary(event)
        resp = await http_server_client.fetch('/v1/models/TestModel:predict',
                                              method="POST",
                                              headers=headers,
                                              body=body)
    
        assert resp.code == 200
        assert resp.body == b'{"predictions": [[1, 2]]}'
        assert resp.headers['content-type'] == "application/x-www-form-urlencoded"
        assert resp.headers['ce-specversion'] == "1.0"
        assert resp.headers['ce-id'] == "36077800-0c23-4f38-a0b4-01f4369f670a"
        assert resp.headers['ce-source'] == "https://example.com/event-producer"
        assert resp.headers['ce-type'] == "com.example.sampletype1"
        assert resp.headers['ce-datacontenttype'] == "application/x-www-form-urlencoded"
        assert resp.headers['ce-time'] > "2021-01-28T21:04:43.144141+00:00"

    async def test_predict_ce_bytes_bad_format_exception(self, http_server_client):
        event = dummy_cloud_event(b'{', set_contenttype=True)
        headers, body = to_binary(event)
        with pytest.raises(
            HTTPClientError, match=r".*HTTP 400: Unrecognized request format: Expecting property name enclosed in double quotes.*"
        ):
            resp = await http_server_client.fetch('/v1/models/TestModel:predict',
                                              method="POST",
                                              headers=headers,
                                              body=body)

    async def test_predict_ce_bytes_bad_hex_format_exception(self, http_server_client):
        event = dummy_cloud_event(b'0\x80\x80\x06World!\x00\x00', set_contenttype=True)
        headers, body = to_binary(event)
        with pytest.raises(
            HTTPClientError, match=r".*HTTP 400: Unrecognized request format: 'utf-8' codec can't decode byte 0x80 in position 1: invalid start byte.*"
        ):
            resp = await http_server_client.fetch('/v1/models/TestModel:predict',
                                              method="POST",
                                              headers=headers,
                                              body=body)
    
class TestTFHttpServerAvroCloudEvent():

    @pytest.fixture(scope="class")
    def app(self):  # pylint: disable=no-self-use
        model = DummyAvroCEModel("TestModel")
        server = kfserver.KFServer()
        server.register_model(model)
        return server.create_application()

    async def test_predict_ce_avro_binary(self, http_server_client):
        schema = avro.schema.parse(test_avsc_schema)
        msg = {"name": "foo", "favorite_number": 1, "favorite_color": "pink"}

        writer = avro.io.DatumWriter(schema)
        bytes_writer = io.BytesIO()
        encoder = avro.io.BinaryEncoder(bytes_writer)
        writer.write(msg, encoder)
        data = bytes_writer.getvalue()

        event = dummy_cloud_event(data, set_contenttype=True)

        # Creates the HTTP request representation of the CloudEvent in binary content mode
        headers, body = to_binary(event)
        resp = await http_server_client.fetch('/v1/models/TestModel:predict',
                                              method="POST",
                                              headers=headers,
                                              body=body)

        assert resp.code == 200
        assert resp.body == b'{"predictions": [["foo", 1, "pink"]]}'
        assert resp.headers['content-type'] == "application/x-www-form-urlencoded"
        assert resp.headers['ce-specversion'] == "1.0"
        assert resp.headers['ce-id'] == "36077800-0c23-4f38-a0b4-01f4369f670a"
        assert resp.headers['ce-source'] == "https://example.com/event-producer"
        assert resp.headers['ce-type'] == "com.example.sampletype1"
        assert resp.headers['ce-datacontenttype'] == "application/x-www-form-urlencoded"
        assert resp.headers['ce-time'] > "2021-01-28T21:04:43.144141+00:00"
