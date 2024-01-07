# Copyright 2022 The KServe Authors.
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

import io
import json
import os
import pathlib
import re
from unittest import mock
from unittest.mock import patch

import avro
import pytest
import tomlkit
from cloudevents.conversion import to_binary, to_structured
from cloudevents.http import CloudEvent
from ray import serve
from kserve.protocol.rest.openai.types.openapi import (
    CreateChatCompletionResponse as ChatCompletion,
    CreateChatCompletionStreamResponse as ChatCompletionChunk,
    CreateCompletionResponse as Completion,
)
from typing import AsyncIterator, Union
from kserve.errors import InvalidInput, ModelNotFound
from kserve.model import PredictorProtocol
from kserve.protocol.dataplane import DataPlane
from kserve.protocol.rest.openai import CompletionRequest, OpenAIModel
from kserve.model_repository import ModelRepository
from kserve.ray import RayModel
from test.test_server import (
    DummyModel,
    dummy_cloud_event,
    DummyCEModel,
    DummyAvroCEModel,
    DummyServeModel,
)


@pytest.mark.asyncio
class TestDataPlane:
    MODEL_NAME = "TestModel"

    # Adding two 'params' to the below fixture causes all the tests using this fixture to run twice.
    # First time this fixture uses 'Model' class.
    # The second time it uses the 'RayServeHandle' class.
    @pytest.fixture(scope="class", params=["TEST_RAW_MODEL", "TEST_RAY_SERVE_MODEL"])
    def dataplane_with_model(self, request):  # pylint: disable=no-self-use
        dataplane = DataPlane(model_registry=ModelRepository())

        if request.param == "TEST_RAW_MODEL":
            model = DummyModel(self.MODEL_NAME)
            model.load()
            dataplane._model_registry.update(model)
            yield dataplane
        else:  # request.param == "TEST_RAY_SERVE_MODEL"
            serve.start(http_options={"host": "0.0.0.0", "port": 9071})

            # https://github.com/ray-project/ray/blob/releases/2.8.0/python/ray/serve/deployment.py#L256
            app = DummyServeModel.bind(name=self.MODEL_NAME)
            handle = serve.run(app, name=self.MODEL_NAME, route_prefix="/")

            model = RayModel(self.MODEL_NAME, handle=handle)
            model.load()
            dataplane._model_registry.update(model)
            yield dataplane
            serve.delete(name="TestModel")

    async def test_get_model_from_registry(self):
        dataplane = DataPlane(model_registry=ModelRepository())
        model_name = "FakeModel"
        with pytest.raises(ModelNotFound) as http_exec:
            dataplane.get_model_from_registry(model_name)
        # assert http_exec.value.status_code == 404
        assert http_exec.value.reason == f"Model with name {model_name} does not exist."

        ready_model = DummyModel("Model")
        ready_model.load()
        dataplane._model_registry.update(ready_model)
        dataplane.get_model_from_registry("Model")

    async def test_liveness(self):
        assert (await DataPlane.live()) == {"status": "alive"}

    async def test_server_readiness(self, dataplane_with_model):
        assert (await dataplane_with_model.ready()) is True

        not_ready_model = DummyModel("NotReadyModel")
        # model.load()  # Model not loaded, i.e. model not ready
        dataplane_with_model._model_registry.update(not_ready_model)
        # The model server readiness endpoint should return 'True' irrespective of the readiness
        # of the model loaded into it.
        assert (await dataplane_with_model.ready()) is True

    async def test_model_readiness(self):
        dataplane = DataPlane(model_registry=ModelRepository())

        ready_model = DummyModel("ReadyModel")
        ready_model.load()
        dataplane._model_registry.update(ready_model)
        is_ready = await dataplane.model_ready(ready_model.name)
        assert is_ready is True

        not_ready_model = DummyModel("NotReadyModel")
        # model.load()  # Model not loaded, i.e. not ready
        dataplane._model_registry.update(not_ready_model)
        is_ready = await dataplane.model_ready(not_ready_model.name)
        assert is_ready is False

    async def test_server_metadata(self):
        with open(pathlib.Path(__file__).parent.parent / "pyproject.toml") as toml_file:
            toml_config = tomlkit.load(toml_file)
            version = toml_config["tool"]["poetry"]["version"].strip()

        dataplane = DataPlane(model_registry=ModelRepository())
        expected_metadata = {
            "name": "kserve",
            "version": version,
            "extensions": ["model_repository_extension"],
        }
        assert dataplane.metadata() == expected_metadata

    async def test_model_metadata(self, dataplane_with_model):
        assert await dataplane_with_model.model_metadata(self.MODEL_NAME) == {
            "name": self.MODEL_NAME,
            "platform": "",
            "inputs": [],
            "outputs": [],
        }

    async def test_infer(self, dataplane_with_model):
        body = b'{"instances":[[1,2]]}'
        infer_request, req_attributes = dataplane_with_model.decode(body, None)
        resp, headers = await dataplane_with_model.infer(self.MODEL_NAME, infer_request)
        resp, headers = dataplane_with_model.encode(
            self.MODEL_NAME, resp, headers, req_attributes
        )
        assert (resp, headers) == ({"predictions": [[1, 2]]}, {})  # body

    async def test_explain(self, dataplane_with_model: DataPlane):
        body = b'{"instances":[[1,2]]}'
        infer_request, req_attributes = dataplane_with_model.decode(body, None)
        resp, headers = await dataplane_with_model.explain(
            self.MODEL_NAME, infer_request
        )
        resp, headers = dataplane_with_model.encode(
            self.MODEL_NAME, resp, headers, req_attributes
        )
        assert (resp, headers) == ({"predictions": [[1, 2]]}, {})


@pytest.mark.asyncio
class TestDataPlaneCloudEvent:
    MODEL_NAME = "TestModel"

    @pytest.fixture(scope="class")
    def dataplane_with_ce_model(self):  # pylint: disable=no-self-use
        dataplane = DataPlane(model_registry=ModelRepository())
        model = DummyCEModel(self.MODEL_NAME)
        model.load()
        dataplane._model_registry.update(model)
        return dataplane

    async def test_infer_ce_structured(self, dataplane_with_ce_model: DataPlane):
        event: CloudEvent = dummy_cloud_event({"instances": [[1, 2]]})
        headers, body = to_structured(event)
        infer_request, req_attributes = dataplane_with_ce_model.decode(body, headers)
        resp, response_headers = await dataplane_with_ce_model.infer(
            self.MODEL_NAME, infer_request, headers
        )
        resp, res_headers = dataplane_with_ce_model.encode(
            self.MODEL_NAME, resp, headers, req_attributes
        )
        response_headers.update(res_headers)
        body = json.loads(resp)

        assert response_headers["content-type"] == "application/cloudevents+json"

        assert body["id"] != "36077800-0c23-4f38-a0b4-01f4369f670a"
        assert body["data"] == {"predictions": [[1, 2]]}
        assert body["specversion"] == "1.0"
        assert body["source"] == "io.kserve.inference.TestModel"
        assert body["type"] == "io.kserve.inference.response"
        assert body["time"] > "2021-01-28T21:04:43.144141+00:00"

    async def test_infer_custom_ce_attributes(self, dataplane_with_ce_model):
        with mock.patch.dict(
            os.environ,
            {
                "CE_SOURCE": "io.kserve.inference.CustomSource",
                "CE_TYPE": "io.kserve.custom_type",
            },
        ):
            event = dummy_cloud_event({"instances": [[1, 2]]})
            headers, body = to_structured(event)

            infer_request, req_attributes = dataplane_with_ce_model.decode(
                body, headers
            )
            resp, response_headers = await dataplane_with_ce_model.infer(
                self.MODEL_NAME, infer_request, headers
            )
            resp, res_headers = dataplane_with_ce_model.encode(
                self.MODEL_NAME, resp, headers, req_attributes
            )
            response_headers.update(res_headers)
            body = json.loads(resp)

            assert response_headers["content-type"] == "application/cloudevents+json"

            assert body["id"] != "36077800-0c23-4f38-a0b4-01f4369f670a"
            assert body["data"] == {"predictions": [[1, 2]]}
            assert body["source"] == "io.kserve.inference.CustomSource"
            assert body["type"] == "io.kserve.custom_type"

    async def test_infer_merge_structured_ce_attributes(
        self, dataplane_with_ce_model: DataPlane
    ):
        with mock.patch.dict(os.environ, {"CE_MERGE": "true"}):
            event = dummy_cloud_event({"instances": [[1, 2]]}, add_extension=True)
            headers, body = to_structured(event)

            infer_request, req_attributes = dataplane_with_ce_model.decode(
                body, headers
            )
            resp, response_headers = await dataplane_with_ce_model.infer(
                self.MODEL_NAME, infer_request, headers
            )
            resp, res_headers = dataplane_with_ce_model.encode(
                self.MODEL_NAME, resp, headers, req_attributes
            )
            response_headers.update(res_headers)
            body = json.loads(resp)

            assert response_headers["content-type"] == "application/cloudevents+json"

            assert body["id"] != "36077800-0c23-4f38-a0b4-01f4369f670a"
            assert body["data"] == {"predictions": [[1, 2]]}
            assert body["source"] == "io.kserve.inference.TestModel"
            assert body["type"] == "io.kserve.inference.response"
            assert (
                body["custom-extension"] == "custom-value"
            )  # Added by add_extension=True in dummy_cloud_event
            assert body["time"] > "2021-01-28T21:04:43.144141+00:00"

    async def test_infer_merge_binary_ce_attributes(self, dataplane_with_ce_model):
        with mock.patch.dict(os.environ, {"CE_MERGE": "true"}):
            event = dummy_cloud_event(
                {"instances": [[1, 2]]}, set_contenttype=True, add_extension=True
            )
            headers, body = to_binary(event)

            infer_request, req_attributes = dataplane_with_ce_model.decode(
                body, headers
            )
            resp, response_headers = await dataplane_with_ce_model.infer(
                self.MODEL_NAME, infer_request, headers
            )
            resp, res_headers = dataplane_with_ce_model.encode(
                self.MODEL_NAME, resp, headers, req_attributes
            )
            response_headers.update(res_headers)

            assert response_headers["content-type"] == "application/json"
            assert response_headers["ce-specversion"] == "1.0"
            assert response_headers["ce-id"] != "36077800-0c23-4f38-a0b4-01f4369f670a"
            # Added by add_extension=True in dummy_cloud_event
            assert response_headers["ce-custom-extension"] == "custom-value"
            assert response_headers["ce-source"] == "io.kserve.inference.TestModel"
            assert response_headers["ce-type"] == "io.kserve.inference.response"
            assert response_headers["ce-time"] > "2021-01-28T21:04:43.144141+00:00"
            assert resp == b'{"predictions": [[1, 2]]}'

    async def test_infer_ce_binary_dict(self, dataplane_with_ce_model):
        event = dummy_cloud_event({"instances": [[1, 2]]}, set_contenttype=True)
        headers, body = to_binary(event)

        infer_request, req_attributes = dataplane_with_ce_model.decode(body, headers)
        resp, response_headers = await dataplane_with_ce_model.infer(
            self.MODEL_NAME, infer_request, headers
        )
        resp, res_headers = dataplane_with_ce_model.encode(
            self.MODEL_NAME, resp, headers, req_attributes
        )
        response_headers.update(res_headers)

        assert response_headers["content-type"] == "application/json"
        assert response_headers["ce-specversion"] == "1.0"
        assert response_headers["ce-id"] != "36077800-0c23-4f38-a0b4-01f4369f670a"
        assert response_headers["ce-source"] == "io.kserve.inference.TestModel"
        assert response_headers["ce-type"] == "io.kserve.inference.response"
        assert response_headers["ce-time"] > "2021-01-28T21:04:43.144141+00:00"
        assert resp == b'{"predictions": [[1, 2]]}'

    async def test_infer_ce_binary_bytes(self, dataplane_with_ce_model):
        event = dummy_cloud_event(b'{"instances":[[1,2]]}', set_contenttype=True)
        headers, body = to_binary(event)

        infer_request, req_attributes = dataplane_with_ce_model.decode(body, headers)
        resp, response_headers = await dataplane_with_ce_model.infer(
            self.MODEL_NAME, infer_request, headers
        )
        resp, res_headers = dataplane_with_ce_model.encode(
            self.MODEL_NAME, resp, headers, req_attributes
        )
        response_headers.update(res_headers)
        assert response_headers["content-type"] == "application/json"
        assert response_headers["ce-specversion"] == "1.0"
        assert response_headers["ce-id"] != "36077800-0c23-4f38-a0b4-01f4369f670a"
        assert response_headers["ce-source"] == "io.kserve.inference.TestModel"
        assert response_headers["ce-type"] == "io.kserve.inference.response"
        assert response_headers["ce-time"] > "2021-01-28T21:04:43.144141+00:00"
        assert resp == b'{"predictions": [[1, 2]]}'

    async def test_infer_ce_bytes_bad_format_exception(self, dataplane_with_ce_model):
        event = dummy_cloud_event(b"{", set_contenttype=True)
        headers, body = to_binary(event)

        with pytest.raises(InvalidInput) as err:
            infer_request, req_attributes = dataplane_with_ce_model.decode(
                body, headers
            )
            await dataplane_with_ce_model.infer(self.MODEL_NAME, infer_request, headers)

        error_regex = re.compile(
            "Failed to decode or parse binary json cloudevent: "
            "unexpected end of data:*"
        )
        assert error_regex.match(err.value.reason) is not None

    async def test_infer_ce_bytes_bad_hex_format_exception(
        self, dataplane_with_ce_model
    ):
        event = dummy_cloud_event(b"0\x80\x80\x06World!\x00\x00", set_contenttype=True)
        headers, body = to_binary(event)

        with pytest.raises(InvalidInput) as err:
            infer_request, req_attributes = dataplane_with_ce_model.decode(
                body, headers
            )
            await dataplane_with_ce_model.infer(self.MODEL_NAME, infer_request, headers)

        error_regex = re.compile(
            "Failed to decode or parse binary json cloudevent: 'utf-8' codec "
            "can't decode byte 0x80 in position 1: invalid start byte.*"
        )

        assert error_regex.match(err.value.reason) is not None


@pytest.mark.asyncio
class TestDataPlaneAvroCloudEvent:
    test_avsc_schema = """
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
            """

    MODEL_NAME = "TestModel"

    @pytest.fixture(scope="class")
    def dataplane_with_ce_model(self):  # pylint: disable=no-self-use
        dataplane = DataPlane(model_registry=ModelRepository())
        model = DummyAvroCEModel(self.MODEL_NAME)
        model.load()
        dataplane._model_registry.update(model)
        return dataplane

    async def test_infer_ce_avro_binary(self, dataplane_with_ce_model):
        schema = avro.schema.parse(self.test_avsc_schema)
        msg = {"name": "foo", "favorite_number": 1, "favorite_color": "pink"}

        writer = avro.io.DatumWriter(schema)
        bytes_writer = io.BytesIO()
        encoder = avro.io.BinaryEncoder(bytes_writer)
        writer.write(msg, encoder)
        data = bytes_writer.getvalue()

        event = dummy_cloud_event(
            data, set_contenttype=True, contenttype="application/avro"
        )
        # Creates the HTTP request representation of the CloudEvent in binary content mode
        headers, body = to_binary(event)

        infer_request, req_attributes = dataplane_with_ce_model.decode(body, headers)
        resp, response_headers = await dataplane_with_ce_model.infer(
            self.MODEL_NAME, infer_request, headers
        )

        resp, res_headers = dataplane_with_ce_model.encode(
            self.MODEL_NAME, resp, headers, req_attributes
        )
        response_headers.update(res_headers)

        assert response_headers["content-type"] == "application/json"
        assert response_headers["ce-specversion"] == "1.0"
        assert response_headers["ce-id"] != "36077800-0c23-4f38-a0b4-01f4369f670a"
        assert response_headers["ce-source"] == "io.kserve.inference.TestModel"
        assert response_headers["ce-type"] == "io.kserve.inference.response"
        assert response_headers["ce-time"] > "2021-01-28T21:04:43.144141+00:00"
        assert resp == b'{"predictions": [["foo", 1, "pink"]]}'


@pytest.mark.asyncio
class TestDataPlaneOpenAI:
    MODEL_NAME = "TestModel"

    class DummyOpenAIModel(OpenAIModel):
        async def create_completion(
            self, params: CompletionRequest
        ) -> Union[Completion, AsyncIterator[Completion]]:
            pass

        async def create_chat_completion(
            self, params: CompletionRequest
        ) -> Union[ChatCompletion, AsyncIterator[ChatCompletionChunk]]:
            pass

    async def test_infer_on_openai_model_raises(self):
        openai_model = self.DummyOpenAIModel(self.MODEL_NAME)
        repo = ModelRepository()
        repo.update(openai_model)
        dataplane = DataPlane(model_registry=repo)

        with pytest.raises(InvalidInput) as exc:
            await dataplane.infer(
                model_name=self.MODEL_NAME,
                request={},
            )

        assert (
            exc.value.reason
            == "Model TestModel is of type OpenAIModel. It does not support the infer method."
        )


@pytest.mark.asyncio
class TestDataplaneTransformer:
    @patch('kserve.protocol.dataplane.httpx.AsyncClient')
    async def test_server_liveness_v1(self, mock_httpx_async_client):
        predictor_host = "dummy.host"
        DataPlane.predictor_host = predictor_host
        DataPlane.predictor_protocol = PredictorProtocol.REST_V1.value
        DataPlane.predictor_use_ssl = False

        # scenario: getting a 2xx response from predictor
        mock_response = mock.Mock()
        mock_response.is_success = True
        mock_response.json.return_value = {"status": "alive"}
        mock_httpx_async_client.return_value.__aenter__.return_value.get.return_value = mock_response
        assert (await DataPlane.live()) == {"status": "alive"}
        mock_httpx_async_client.return_value.__aenter__.return_value.get.assert_called_with(f"http://{predictor_host}")

        # scenario: not a 2xx response from predictor
        mock_response = mock.Mock()
        mock_response.is_success = False
        mock_httpx_async_client.return_value.__aenter__.return_value.get.return_value = mock_response
        assert (await DataPlane.live()) == {"status": "error"}

        # scenario: 2xx response with server not alive response from predictor
        mock_response = mock.Mock()
        mock_response.is_success = True
        mock_httpx_async_client.return_value.__aenter__.return_value.get.return_value = mock_response
        mock_response.json.return_value = {"status": "error"}
        assert (await DataPlane.live()) == {"status": "error"}

        # scenario: getting a 2xx response from predictor with ssl enabled
        DataPlane.predictor_use_ssl = True
        mock_response = mock.Mock()
        mock_response.is_success = True
        mock_response.json.return_value = {"status": "alive"}
        mock_httpx_async_client.return_value.__aenter__.return_value.get.return_value = mock_response
        assert (await DataPlane.live()) == {"status": "alive"}
        mock_httpx_async_client.return_value.__aenter__.return_value.get.assert_called_with(f"https://{predictor_host}")

    @patch('kserve.protocol.dataplane.httpx.AsyncClient')
    async def test_server_liveness_v2(self, mock_httpx_async_client):
        predictor_host = "dummy.host"
        DataPlane.predictor_host = predictor_host
        DataPlane.predictor_protocol = PredictorProtocol.REST_V2.value
        DataPlane.predictor_use_ssl = False

        # scenario: getting a 2xx response from predictor
        mock_response = mock.Mock()
        mock_response.is_success = True
        mock_response.json.return_value = {"live": True}
        mock_httpx_async_client.return_value.__aenter__.return_value.get.return_value = mock_response
        assert (await DataPlane.live()) == {"status": "alive"}
        (mock_httpx_async_client.return_value.__aenter__.return_value.get
         .assert_called_with(f"http://{predictor_host}/{PredictorProtocol.REST_V2.value}/health/live"))

        # scenario: not a 2xx response from predictor
        mock_response = mock.Mock()
        mock_response.is_success = False
        mock_httpx_async_client.return_value.__aenter__.return_value.get.return_value = mock_response
        assert (await DataPlane.live()) == {"status": "error"}

        # scenario: 2xx response with server not alive response from predictor
        mock_response = mock.Mock()
        mock_response.is_success = True
        mock_response.json.return_value = {"live": False}
        mock_httpx_async_client.return_value.__aenter__.return_value.get.return_value = mock_response
        assert (await DataPlane.live()) == {"status": "error"}

        # scenario: getting a 2xx response from predictor with ssl enabled
        mock_response = mock.Mock()
        mock_response.is_success = True
        mock_response.json.return_value = {"live": True}
        mock_httpx_async_client.return_value.__aenter__.return_value.get.return_value = mock_response
        assert (await DataPlane.live()) == {"status": "alive"}
        (mock_httpx_async_client.return_value.__aenter__.return_value.get
         .assert_called_with(f"https://{predictor_host}/{PredictorProtocol.REST_V2.value}/health/live"))

    @patch('kserve.protocol.dataplane.grpc.aio.secure_channel')
    @patch('kserve.protocol.dataplane.grpc.aio.insecure_channel')
    @patch('kserve.protocol.dataplane.grpc_predict_v2_pb2_grpc.GRPCInferenceServiceStub')
    async def test_server_liveness_grpc_v2(self, mock_grpc_stub, mock_insecure_channel, mock_secure_channel):
        predictor_host = "dummy.host"
        DataPlane.predictor_host = predictor_host
        DataPlane.predictor_protocol = PredictorProtocol.GRPC_V2.value
        DataPlane.predictor_use_ssl = False

        mock_channel = mock.AsyncMock()
        mock_insecure_channel.return_value = mock_channel
        mock_secure_channel.return_value = mock_channel
        mock_grpc_stub.return_value = mock.AsyncMock()

        # scenario: server alive response from predictor with default port
        mock_grpc_stub.return_value.ServerLive.return_value.live = True
        assert (await DataPlane.live()) == {"status": "alive"}
        mock_insecure_channel.assert_called_with(predictor_host + ":80")

        # scenario: server not alive response from predictor
        mock_grpc_stub.return_value.ServerLive.return_value.live = False
        assert (await DataPlane.live()) == {"status": "error"}

        # scenario: server alive response from predictor with ssl enabled and default port
        DataPlane.predictor_use_ssl = True
        mock_grpc_stub.return_value.ServerLive.return_value.live = True
        assert (await DataPlane.live()) == {"status": "alive"}
        mock_secure_channel.assert_called_with(predictor_host + ":443", mock.ANY)

        # scenario: server alive response from predictor with ssl enabled and given port
        predictor_host = "dummy.host:8080"
        DataPlane.predictor_host = predictor_host
        DataPlane.predictor_use_ssl = True
        mock_grpc_stub.return_value.ServerLive.return_value.live = True
        assert (await DataPlane.live()) == {"status": "alive"}
        mock_secure_channel.assert_called_with(predictor_host, mock.ANY)

        # scenario: server alive response from predictor with given port and ssl disabled
        predictor_host = "dummy.host:8080"
        DataPlane.predictor_host = predictor_host
        DataPlane.predictor_use_ssl = False
        mock_grpc_stub.return_value.ServerLive.return_value.live = True
        assert (await DataPlane.live()) == {"status": "alive"}
        mock_insecure_channel.assert_called_with(predictor_host)

    @patch('kserve.protocol.dataplane.httpx.AsyncClient')
    async def test_server_readiness_v1(self, mock_httpx_async_client):
        predictor_host = "dummy.host"
        DataPlane.predictor_host = predictor_host
        DataPlane.predictor_protocol = PredictorProtocol.REST_V1.value
        DataPlane.predictor_use_ssl = False

        # scenario: getting a 2xx response from predictor
        mock_response = mock.Mock()
        mock_response.is_success = True
        mock_response.json.return_value = {"status": "alive"}
        mock_httpx_async_client.return_value.__aenter__.return_value.get.return_value = mock_response
        assert (await DataPlane.ready()) is True
        mock_httpx_async_client.return_value.__aenter__.return_value.get.assert_called_with(f"http://{predictor_host}")

        # scenario: not a 2xx response from predictor
        mock_response = mock.Mock()
        mock_response.is_success = False
        mock_httpx_async_client.return_value.__aenter__.return_value.get.return_value = mock_response
        assert (await DataPlane.ready()) is False

        # scenario: 2xx response with server not alive response from predictor
        mock_response = mock.Mock()
        mock_response.is_success = True
        mock_response.json.return_value = {"status": "error"}
        mock_httpx_async_client.return_value.__aenter__.return_value.get.return_value = mock_response
        assert (await DataPlane.ready()) is False

        # scenario: getting a 2xx response from predictor with ssl enabled
        DataPlane.predictor_use_ssl = True
        mock_response = mock.Mock()
        mock_response.is_success = True
        mock_response.json.return_value = {"status": "alive"}
        mock_httpx_async_client.return_value.__aenter__.return_value.get.return_value = mock_response
        assert (await DataPlane.ready()) is True
        mock_httpx_async_client.return_value.__aenter__.return_value.get.assert_called_with(f"https://{predictor_host}")

    @patch('kserve.protocol.dataplane.httpx.AsyncClient')
    async def test_server_readiness_v2(self, mock_httpx_async_client):
        predictor_host = "dummy.host"
        DataPlane.predictor_host = predictor_host
        DataPlane.predictor_protocol = PredictorProtocol.REST_V2.value
        DataPlane.predictor_use_ssl = False

        # scenario: getting a 2xx response from predictor
        mock_response = mock.Mock()
        mock_response.is_success = True
        mock_response.json.return_value = {"ready": True}
        mock_httpx_async_client.return_value.__aenter__.return_value.get.return_value = mock_response
        assert (await DataPlane.ready()) is True
        (mock_httpx_async_client.return_value.__aenter__.return_value.get
         .assert_called_with(f"http://{predictor_host}/{PredictorProtocol.REST_V2.value}/health/ready"))

        # scenario: not a 2xx response from predictor
        mock_response = mock.Mock()
        mock_response.is_success = False
        mock_httpx_async_client.return_value.__aenter__.return_value.get.return_value = mock_response
        assert (await DataPlane.ready()) is False

        # scenario: 2xx response with server not ready response from predictor
        mock_response = mock.Mock()
        mock_response.is_success = True
        mock_response.json.return_value = {"ready": False}
        mock_httpx_async_client.return_value.__aenter__.return_value.get.return_value = mock_response
        assert (await DataPlane.ready()) is False

        # scenario: getting a 2xx response from predictor with ssl enabled
        DataPlane.predictor_use_ssl = True
        mock_response = mock.Mock()
        mock_response.is_success = True
        mock_response.json.return_value = {"ready": True}
        mock_httpx_async_client.return_value.__aenter__.return_value.get.return_value = mock_response
        assert (await DataPlane.ready()) is True
        (mock_httpx_async_client.return_value.__aenter__.return_value.get
         .assert_called_with(f"https://{predictor_host}/{PredictorProtocol.REST_V2.value}/health/ready"))

    @patch('kserve.protocol.dataplane.grpc.aio.secure_channel')
    @patch('kserve.protocol.dataplane.grpc.aio.insecure_channel')
    @patch('kserve.protocol.dataplane.grpc_predict_v2_pb2_grpc.GRPCInferenceServiceStub')
    async def test_server_readiness_grpc_v2(self, mock_grpc_stub, mock_insecure_channel, mock_secure_channel):
        predictor_host = "dummy.host"
        DataPlane.predictor_host = predictor_host
        DataPlane.predictor_protocol = PredictorProtocol.GRPC_V2.value
        DataPlane.predictor_use_ssl = False

        mock_channel = mock.AsyncMock()
        mock_insecure_channel.return_value = mock_channel
        mock_secure_channel.return_value = mock_channel
        mock_grpc_stub.return_value = mock.AsyncMock()

        # scenario: server ready response from predictor with default port
        mock_grpc_stub.return_value.ServerReady.return_value.ready = True
        assert await DataPlane.ready() is True
        mock_insecure_channel.assert_called_with(predictor_host + ":80")

        # scenario: server not ready response from predictor
        mock_grpc_stub.return_value.ServerReady.return_value.ready = False
        assert await DataPlane.ready() is False

        # scenario: server alive response from predictor with ssl enabled and default port
        DataPlane.predictor_use_ssl = True
        mock_grpc_stub.return_value.ServerReady.return_value.ready = True
        assert await DataPlane.ready() is True
        mock_secure_channel.assert_called_with(predictor_host + ":443", mock.ANY)

        # scenario: server ready response from predictor with ssl enabled and given port
        predictor_host = "dummy.host:8080"
        DataPlane.predictor_host = predictor_host
        DataPlane.predictor_use_ssl = True
        mock_grpc_stub.return_value.ServerReady.return_value.ready = True
        assert await DataPlane.ready() is True
        mock_secure_channel.assert_called_with(predictor_host, mock.ANY)

        # scenario: server ready response from predictor with given port and ssl disabled
        predictor_host = "dummy.host:8080"
        DataPlane.predictor_host = predictor_host
        DataPlane.predictor_use_ssl = False
        mock_grpc_stub.return_value.ServerReady.return_value.ready = True
        assert await DataPlane.ready() is True
        mock_insecure_channel.assert_called_with(predictor_host)

    @patch('kserve.protocol.dataplane.httpx.AsyncClient')
    async def test_model_readiness_v1(self, mock_httpx_async_client):
        predictor_host = "dummy.host"
        dataplane = DataPlane(model_registry=ModelRepository())
        DataPlane.predictor_host = predictor_host
        DataPlane.predictor_protocol = PredictorProtocol.REST_V1.value
        DataPlane.predictor_use_ssl = False

        # Transformer model ready
        ready_model = DummyModel("ReadyModel")
        ready_model.load()
        dataplane._model_registry.update(ready_model)

        # scenario: transformer model is ready and getting a 2xx response from predictor
        mock_response = mock.Mock()
        mock_response.is_success = True
        mock_response.json.return_value = {"ready": "True"}
        mock_httpx_async_client.return_value.__aenter__.return_value.get.return_value = mock_response
        assert await dataplane.model_ready(ready_model.name) is True
        (mock_httpx_async_client.return_value.__aenter__.return_value.get
         .assert_called_with(f"http://{predictor_host}/{PredictorProtocol.REST_V1.value}/models/{ready_model.name}"))

        # scenario: transformer model is ready and getting not a 2xx response from predictor
        mock_response = mock.Mock()
        mock_response.is_success = False
        mock_httpx_async_client.return_value.__aenter__.return_value.get.return_value = mock_response
        assert await dataplane.model_ready(ready_model.name) is False

        # scenario: transformer model is ready and getting a 2xx response with model not ready from predictor
        mock_response = mock.Mock()
        mock_response.is_success = True
        mock_response.json.return_value = {"ready": "False"}
        mock_httpx_async_client.return_value.__aenter__.return_value.get.return_value = mock_response
        assert await dataplane.model_ready(ready_model.name) is False

        # Transformer model is not ready
        not_ready_model = DummyModel("NotReadyModel")
        # model.load()  # Model not loaded, i.e. not ready
        dataplane._model_registry.update(not_ready_model)

        # scenario: transformer model is not ready and getting a 2xx response from predictor
        mock_response = mock.Mock()
        mock_response.is_success = True
        mock_response.json.return_value = {"ready": "True"}
        mock_httpx_async_client.return_value.__aenter__.return_value.get.return_value = mock_response
        assert await dataplane.model_ready(not_ready_model.name) is False

        # scenario: transformer model is not ready and getting not a 2xx response from predictor
        mock_response = mock.Mock()
        mock_response.is_success = False
        mock_httpx_async_client.return_value.__aenter__.return_value.get.return_value = mock_response
        assert await dataplane.model_ready(not_ready_model.name) is False

        # scenario: transformer model is not ready and getting a 2xx response with model not ready from predictor
        mock_response = mock.Mock()
        mock_response.is_success = True
        mock_response.json.return_value = {"ready": "False"}
        mock_httpx_async_client.return_value.__aenter__.return_value.get.return_value = mock_response
        assert await dataplane.model_ready(not_ready_model.name) is False

        # scenario: transformer model is ready and getting a 2xx response from predictor with ssl enabled
        DataPlane.predictor_use_ssl = True
        mock_response = mock.Mock()
        mock_response.is_success = True
        mock_response.json.return_value = {"ready": "True"}
        mock_httpx_async_client.return_value.__aenter__.return_value.get.return_value = mock_response
        assert await dataplane.model_ready(ready_model.name) is True
        (mock_httpx_async_client.return_value.__aenter__.return_value.get
         .assert_called_with(f"https://{predictor_host}/{PredictorProtocol.REST_V1.value}/models/{ready_model.name}"))

    @patch('kserve.protocol.dataplane.httpx.AsyncClient')
    async def test_model_readiness_v2(self, mock_httpx_async_client):
        predictor_host = "dummy.host"
        dataplane = DataPlane(model_registry=ModelRepository())
        DataPlane.predictor_host = predictor_host
        DataPlane.predictor_protocol = PredictorProtocol.REST_V2.value
        DataPlane.predictor_use_ssl = False

        # Transformer model ready
        ready_model = DummyModel("ReadyModel")
        ready_model.load()
        dataplane._model_registry.update(ready_model)

        # scenario: transformer model is ready and getting a 2xx response from predictor
        mock_response = mock.Mock()
        mock_response.is_success = True
        mock_response.json.return_value = {"ready": True}
        mock_httpx_async_client.return_value.__aenter__.return_value.get.return_value = mock_response
        assert await dataplane.model_ready(ready_model.name) is True
        (mock_httpx_async_client.return_value.__aenter__.return_value.get
         .assert_called_with(f"http://{predictor_host}/{PredictorProtocol.REST_V2.value}/models/"
                             f"{ready_model.name}/ready"))

        # scenario: transformer model is ready and getting not a 2xx response from predictor
        mock_response = mock.Mock()
        mock_response.is_success = False
        mock_httpx_async_client.return_value.__aenter__.return_value.get.return_value = mock_response
        assert await dataplane.model_ready(ready_model.name) is False

        # scenario: transformer model is ready and getting a 2xx response with model not ready from predictor
        mock_response = mock.Mock()
        mock_response.is_success = True
        mock_response.json.return_value = {"ready": False}
        mock_httpx_async_client.return_value.__aenter__.return_value.get.return_value = mock_response
        assert await dataplane.model_ready(ready_model.name) is False

        # Transformer model is not ready
        not_ready_model = DummyModel("NotReadyModel")
        # model.load()  # Model not loaded, i.e. not ready
        dataplane._model_registry.update(not_ready_model)

        # scenario: transformer model is not ready and getting a 2xx response from predictor
        mock_response = mock.Mock()
        mock_response.is_success = True
        mock_response.json.return_value = {"ready": True}
        mock_httpx_async_client.return_value.__aenter__.return_value.get.return_value = mock_response
        assert await dataplane.model_ready(not_ready_model.name) is False

        # scenario: transformer model is not ready and getting not a 2xx response from predictor
        mock_response = mock.Mock()
        mock_response.is_success = False
        mock_httpx_async_client.return_value.__aenter__.return_value.get.return_value = mock_response
        assert await dataplane.model_ready(not_ready_model.name) is False

        # scenario: transformer model is not ready and getting a 2xx response with model not ready from predictor
        mock_response = mock.Mock()
        mock_response.is_success = True
        mock_response.json.return_value = {"ready": False}
        mock_httpx_async_client.return_value.__aenter__.return_value.get.return_value = mock_response
        assert await dataplane.model_ready(not_ready_model.name) is False

        # scenario: transformer model is ready and getting a 2xx response from predictor with ssl enabled
        DataPlane.predictor_use_ssl = True
        mock_response = mock.Mock()
        mock_response.is_success = True
        mock_response.json.return_value = {"ready": True}
        mock_httpx_async_client.return_value.__aenter__.return_value.get.return_value = mock_response
        assert await dataplane.model_ready(ready_model.name) is True
        (mock_httpx_async_client.return_value.__aenter__.return_value.get
         .assert_called_with(f"https://{predictor_host}/{PredictorProtocol.REST_V2.value}/models/"
                             f"{ready_model.name}/ready"))

    @patch('kserve.protocol.dataplane.grpc.aio.secure_channel')
    @patch('kserve.protocol.dataplane.grpc.aio.insecure_channel')
    @patch('kserve.protocol.dataplane.grpc_predict_v2_pb2_grpc.GRPCInferenceServiceStub')
    async def test_model_readiness_grpc_v2(self, mock_grpc_stub, mock_insecure_channel, mock_secure_channel):
        predictor_host = "dummy.host"
        DataPlane.predictor_host = predictor_host
        DataPlane.predictor_protocol = PredictorProtocol.GRPC_V2.value
        DataPlane.predictor_use_ssl = False
        dataplane = DataPlane(model_registry=ModelRepository())

        # Transformer model ready
        ready_model = DummyModel("ReadyModel")
        ready_model.load()
        dataplane._model_registry.update(ready_model)

        mock_channel = mock.AsyncMock()
        mock_insecure_channel.return_value = mock_channel
        mock_secure_channel.return_value = mock_channel
        mock_grpc_stub.return_value = mock.AsyncMock()

        # scenario: transformer model ready and predictor model ready response with default port
        mock_grpc_stub.return_value.ModelReady.return_value.ready = True
        assert await dataplane.model_ready(ready_model.name) is True
        mock_insecure_channel.assert_called_with(predictor_host + ":80")

        # scenario: transformer model ready and predictor model is not ready response with default port
        mock_grpc_stub.return_value.ModelReady.return_value.ready = False
        assert await dataplane.model_ready(ready_model.name) is False
        mock_insecure_channel.assert_called_with(predictor_host + ":80")

        # Transformer model is not ready
        not_ready_model = DummyModel("NotReadyModel")
        # model.load()  # Model not loaded, i.e. not ready
        dataplane._model_registry.update(not_ready_model)

        # scenario: transformer model not ready and predictor model ready response with default port
        mock_grpc_stub.return_value.ModelReady.return_value.ready = True
        assert await dataplane.model_ready(not_ready_model.name) is False
        mock_insecure_channel.assert_called_with(predictor_host + ":80")

        # scenario: transformer model not ready and predictor model not ready response with default port
        mock_grpc_stub.return_value.ModelReady.return_value.ready = False
        assert await dataplane.model_ready(not_ready_model.name) is False
        mock_insecure_channel.assert_called_with(predictor_host + ":80")

        # scenario: transformer model ready and predictor model ready with ssl enabled and default port
        DataPlane.predictor_use_ssl = True
        mock_grpc_stub.return_value.ModelReady.return_value.ready = True
        assert await dataplane.model_ready(ready_model.name) is True
        mock_secure_channel.assert_called_with(predictor_host + ":443", mock.ANY)

        # scenario: transformer model ready and predictor model ready with ssl enabled and given port
        predictor_host = "dummy.host:8080"
        DataPlane.predictor_host = predictor_host
        DataPlane.predictor_use_ssl = True
        mock_grpc_stub.return_value.ModelReady.return_value.ready = True
        assert await dataplane.model_ready(ready_model.name) is True
        mock_secure_channel.assert_called_with(predictor_host, mock.ANY)

        # scenario: transformer model ready and predictor model ready with given port and ssl disabled
        predictor_host = "dummy.host:8080"
        DataPlane.predictor_host = predictor_host
        DataPlane.predictor_use_ssl = False
        mock_grpc_stub.return_value.ModelReady.return_value.ready = True
        assert await dataplane.model_ready(ready_model.name) is True
        mock_insecure_channel.assert_called_with(predictor_host)
