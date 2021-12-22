# Copyright 2021 The KServe Authors.
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
import json
import os
import re
from unittest import mock

import avro.io
import avro.schema
import io
import pytest
from cloudevents.http import CloudEvent, to_binary, to_structured
from kserve import Model
from kserve import ModelServer
from kserve import ModelRepository
from tornado.httpclient import HTTPClientError
from ray import serve


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


def dummy_cloud_event(data, set_contenttype=False, add_extension=False):
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
    if add_extension:
        attributes["custom-extension"] = "custom-value"

    event = CloudEvent(attributes, data)
    return event


class DummyModel(Model):
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


@serve.deployment
class DummyServeModel(Model):
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


class DummyCEModel(Model):
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


class DummyAvroCEModel(Model):
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
        if isinstance(request, CloudEvent):
            attributes = request._attributes
            assert attributes["specversion"] == "1.0"
            assert attributes["source"] == "https://example.com/event-producer"
            assert attributes["type"] == "com.example.sampletype1"
            assert attributes["datacontenttype"] == "application/x-www-form-urlencoded"
            assert attributes["content-type"] == "application/json"
            return self._parserequest(request.data)

    async def predict(self, request):
        return {"predictions": [[request['name'], request['favorite_number'], request['favorite_color']]]}

    async def explain(self, request):
        return {"predictions": [[request['name'], request['favorite_number'], request['favorite_color']]]}


class DummyModelRepository(ModelRepository):
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


class TestTFHttpServer:

    @pytest.fixture(scope="class")
    def app(self):  # pylint: disable=no-self-use
        model = DummyModel("TestModel")
        model.load()
        server = ModelServer()
        server.register_model(model)
        return server.create_application()

    async def test_liveness(self, http_server_client):
        resp = await http_server_client.fetch('/')
        assert resp.code == 200
        assert resp.body == b'{"status": "alive"}'

    async def test_model(self, http_server_client):
        resp = await http_server_client.fetch('/v1/models/TestModel')
        assert resp.code == 200

    async def test_unknown_model(self, http_server_client):
        with pytest.raises(HTTPClientError) as err:
            _ = await http_server_client.fetch('/v1/models/InvalidModel')
        assert err.value.code == 404
        assert err.value.response.body == b'{"error": "Model with name InvalidModel does not exist."}'

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

    async def test_list(self, http_server_client):
        resp = await http_server_client.fetch('/v1/models')
        assert resp.code == 200
        assert resp.body == b'{"models": ["TestModel"]}'

    async def test_unknown_path(self, http_server_client):
        with pytest.raises(HTTPClientError) as err:
            _ = await http_server_client.fetch('/unknown_path')
        assert err.value.code == 404
        assert err.value.response.body == b'{"error": "invalid path"}'


class TestRayServer:
    @pytest.fixture(scope="class")
    def app(self):  # pylint: disable=no-self-use
        serve.start(detached=False, http_options={"host": "0.0.0.0", "port": 9071})

        DummyServeModel.deploy("TestModel")
        handle = DummyServeModel.get_handle()
        handle.load.remote()

        server = ModelServer()
        server.register_model_handle("TestModel", handle)

        return server.create_application()

    async def test_liveness_handler(self, http_server_client):
        resp = await http_server_client.fetch('/')
        assert resp.code == 200
        assert resp.body == b'{"status": "alive"}'

    async def test_list_handler(self, http_server_client):
        resp = await http_server_client.fetch('/v1/models')
        assert resp.code == 200
        assert resp.body == b'{"models": ["TestModel"]}'

    async def test_health_handler(self, http_server_client):
        resp = await http_server_client.fetch('/v1/models/TestModel')
        assert resp.code == 200
        assert resp.body == b'{"name": "TestModel", "ready": true}'

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


class TestTFHttpServerLoadAndUnLoad:
    @pytest.fixture(scope="class")
    def app(self):  # pylint: disable=no-self-use
        server = ModelServer(registered_models=DummyModelRepository(test_load_success=True))
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


class TestTFHttpServerLoadAndUnLoadFailure:
    @pytest.fixture(scope="class")
    def app(self):  # pylint: disable=no-self-use
        server = ModelServer(registered_models=DummyModelRepository(test_load_success=False))
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


class TestTFHttpServerModelNotLoaded:

    @pytest.fixture(scope="class")
    def app(self):  # pylint: disable=no-self-use
        model = DummyModel("TestModel")
        server = ModelServer()
        server.register_model(model)
        return server.create_application()

    async def test_model_not_ready_error(self, http_server_client):
        with pytest.raises(HTTPClientError) as excinfo:
            _ = await http_server_client.fetch('/v1/models/TestModel')
        assert excinfo.value.code == 503


class TestTFHttpServerCloudEvent:
    @pytest.fixture(scope="class")
    def app(self):  # pylint: disable=no-self-use
        model = DummyCEModel("TestModel")
        server = ModelServer()
        server.register_model(model)
        return server.create_application()

    async def test_predict_ce_structured(self, http_server_client):
        event = dummy_cloud_event({"instances": [[1, 2]]})
        headers, body = to_structured(event)

        resp = await http_server_client.fetch('/v1/models/TestModel:predict',
                                              method="POST",
                                              headers=headers,
                                              body=body)
        body = json.loads(resp.body)

        assert resp.code == 200
        assert resp.headers['content-type'] == "application/cloudevents+json"

        assert body["id"] != "36077800-0c23-4f38-a0b4-01f4369f670a"
        assert body["data"] == {"predictions": [[1, 2]]}
        assert body['specversion'] == "1.0"
        assert body['source'] == "io.kserve.kfserver.TestModel"
        assert body['type'] == "io.kserve.inference.response"
        assert body['time'] > "2021-01-28T21:04:43.144141+00:00"

    async def test_predict_custom_ce_attributes(self, http_server_client):
        with mock.patch.dict(os.environ,
                             {"CE_SOURCE": "io.kserve.kfserver.CustomSource", "CE_TYPE": "io.kserve.custom_type"}):
            event = dummy_cloud_event({"instances": [[1, 2]]})
            headers, body = to_structured(event)

            resp = await http_server_client.fetch('/v1/models/TestModel:predict',
                                                  method="POST",
                                                  headers=headers,
                                                  body=body)
            body = json.loads(resp.body)

            assert resp.code == 200
            assert resp.headers['content-type'] == "application/cloudevents+json"

            assert body["id"] != "36077800-0c23-4f38-a0b4-01f4369f670a"
            assert body["data"] == {"predictions": [[1, 2]]}
            assert body['source'] == "io.kserve.kfserver.CustomSource"
            assert body['type'] == "io.kserve.custom_type"

    async def test_predict_merge_structured_ce_attributes(self, http_server_client):
        with mock.patch.dict(os.environ, {"CE_MERGE": "true"}):
            event = dummy_cloud_event({"instances": [[1, 2]]}, add_extension=True)
            headers, body = to_structured(event)

            resp = await http_server_client.fetch('/v1/models/TestModel:predict',
                                                  method="POST",
                                                  headers=headers,
                                                  body=body)
            body = json.loads(resp.body)

            assert resp.code == 200
            assert resp.headers['content-type'] == "application/cloudevents+json"

            assert body["id"] != "36077800-0c23-4f38-a0b4-01f4369f670a"
            assert body["data"] == {"predictions": [[1, 2]]}
            assert body['source'] == "io.kserve.kfserver.TestModel"
            assert body['type'] == "io.kserve.inference.response"
            assert body["custom-extension"] == "custom-value"  # Added by add_extension=True in dummy_cloud_event
            assert body['time'] > "2021-01-28T21:04:43.144141+00:00"

    async def test_predict_merge_binary_ce_attributes(self, http_server_client):
        with mock.patch.dict(os.environ, {"CE_MERGE": "true"}):
            event = dummy_cloud_event({"instances": [[1, 2]]}, set_contenttype=True, add_extension=True)
            headers, body = to_binary(event)

            resp = await http_server_client.fetch('/v1/models/TestModel:predict',
                                                  method="POST",
                                                  headers=headers,
                                                  body=body)

            assert resp.code == 200
            assert resp.headers['content-type'] == "application/json"
            assert resp.headers['ce-specversion'] == "1.0"
            assert resp.headers["ce-id"] != "36077800-0c23-4f38-a0b4-01f4369f670a"
            # Added by add_extension=True in dummy_cloud_event
            assert resp.headers['ce-custom-extension'] == 'custom-value'
            assert resp.headers['ce-source'] == "io.kserve.kfserver.TestModel"
            assert resp.headers['ce-type'] == "io.kserve.inference.response"
            assert resp.headers['ce-time'] > "2021-01-28T21:04:43.144141+00:00"
            assert resp.body == b'{"predictions": [[1, 2]]}'

    async def test_predict_ce_binary_dict(self, http_server_client):
        event = dummy_cloud_event({"instances": [[1, 2]]}, set_contenttype=True)
        headers, body = to_binary(event)
        resp = await http_server_client.fetch('/v1/models/TestModel:predict',
                                              method="POST",
                                              headers=headers,
                                              body=body)

        assert resp.code == 200
        assert resp.headers['content-type'] == "application/json"
        assert resp.headers['ce-specversion'] == "1.0"
        assert resp.headers["ce-id"] != "36077800-0c23-4f38-a0b4-01f4369f670a"
        assert resp.headers['ce-source'] == "io.kserve.kfserver.TestModel"
        assert resp.headers['ce-type'] == "io.kserve.inference.response"
        assert resp.headers['ce-time'] > "2021-01-28T21:04:43.144141+00:00"
        assert resp.body == b'{"predictions": [[1, 2]]}'

    async def test_predict_ce_binary_bytes(self, http_server_client):
        event = dummy_cloud_event(b'{"instances":[[1,2]]}', set_contenttype=True)
        headers, body = to_binary(event)
        resp = await http_server_client.fetch('/v1/models/TestModel:predict',
                                              method="POST",
                                              headers=headers,
                                              body=body)

        assert resp.code == 200
        assert resp.headers['content-type'] == "application/json"
        assert resp.headers['ce-specversion'] == "1.0"
        assert resp.headers["ce-id"] != "36077800-0c23-4f38-a0b4-01f4369f670a"
        assert resp.headers['ce-source'] == "io.kserve.kfserver.TestModel"
        assert resp.headers['ce-type'] == "io.kserve.inference.response"
        assert resp.headers['ce-time'] > "2021-01-28T21:04:43.144141+00:00"
        assert resp.body == b'{"predictions": [[1, 2]]}'

    async def test_predict_ce_bytes_bad_format_exception(self, http_server_client):
        event = dummy_cloud_event(b'{', set_contenttype=True)
        headers, body = to_binary(event)

        with pytest.raises(HTTPClientError) as err:
            _ = await http_server_client.fetch('/v1/models/TestModel:predict',
                                               method="POST",
                                               headers=headers,
                                               body=body)
        assert err.value.code == 400
        error_regex = re.compile("Failed to decode or parse binary json cloudevent: "
                                 "Expecting property name enclosed in double quotes.*")
        response = json.loads(err.value.response.body)
        assert error_regex.match(response["error"]) is not None

    async def test_predict_ce_bytes_bad_hex_format_exception(self, http_server_client):
        event = dummy_cloud_event(b'0\x80\x80\x06World!\x00\x00', set_contenttype=True)
        headers, body = to_binary(event)

        with pytest.raises(HTTPClientError) as err:
            _ = await http_server_client.fetch('/v1/models/TestModel:predict',
                                               method="POST",
                                               headers=headers,
                                               body=body)
        assert err.value.code == 400
        error_regex = re.compile("Failed to decode or parse binary json cloudevent: "
                                 "'utf-8' codec can't decode byte 0x80 in position 1: invalid start byte.*")
        response = json.loads(err.value.response.body)
        assert error_regex.match(response["error"]) is not None


class TestTFHttpServerAvroCloudEvent:

    @pytest.fixture(scope="class")
    def app(self):  # pylint: disable=no-self-use
        model = DummyAvroCEModel("TestModel")
        server = ModelServer()
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
        assert resp.headers['content-type'] == "application/json"
        assert resp.headers['ce-specversion'] == "1.0"
        assert resp.headers["ce-id"] != "36077800-0c23-4f38-a0b4-01f4369f670a"
        assert resp.headers['ce-source'] == "io.kserve.kfserver.TestModel"
        assert resp.headers['ce-type'] == "io.kserve.inference.response"
        assert resp.headers['ce-time'] > "2021-01-28T21:04:43.144141+00:00"
        assert resp.body == b'{"predictions": [["foo", 1, "pink"]]}'
