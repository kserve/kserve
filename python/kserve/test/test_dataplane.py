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
import grpc
import httpx
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
from kserve.model import PredictorProtocol, PredictorConfig
from kserve.protocol.dataplane import DataPlane
from kserve.protocol.rest.openai import CompletionRequest, OpenAIGenerativeModel
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

    class DummyOpenAIModel(OpenAIGenerativeModel):
        async def create_completion(
            self, params: CompletionRequest
        ) -> Union[Completion, AsyncIterator[Completion]]:
            pass

        async def create_chat_completion(
            self, params: CompletionRequest
        ) -> Union[ChatCompletion, AsyncIterator[ChatCompletionChunk]]:
            pass

    async def test_infer_on_openai_completion_model_raises(self):
        openai_model = self.DummyOpenAIModel(self.MODEL_NAME)
        repo = ModelRepository()
        repo.update(openai_model)
        dataplane = DataPlane(model_registry=repo)

        with pytest.raises(ValueError) as exc:
            await dataplane.infer(
                model_name=self.MODEL_NAME,
                request={},
            )

        assert (
            exc.value.args[0]
            == "Model of type DummyOpenAIModel does not support inference"
        )


@pytest.mark.asyncio
class TestDataplaneTransformer:

    async def test_dataplane_rest_with_ssl_enabled(self, httpx_mock):
        # scenario: getting a 2xx response from predictor with ssl enabled
        predictor_host = "ready.host"
        httpx_mock.add_response(
            url=re.compile(f"https://{predictor_host}/*"), json={"status": "alive"}
        )
        dataplane = DataPlane(
            model_registry=ModelRepository(),
            predictor_config=PredictorConfig(
                predictor_host=predictor_host,
                predictor_protocol=PredictorProtocol.REST_V1.value,
                predictor_request_retries=2,
                predictor_request_timeout_seconds=5,
                predictor_health_check=True,
                predictor_use_ssl=True,
            ),
        )
        assert (await dataplane.ready()) is True

    @patch("kserve.protocol.dataplane.InferenceClientFactory.get_grpc_client")
    async def test_dataplane_grpc_with_ssl_enabled(self, mock_grpc_client):
        # scenario: getting a 2xx response from predictor with ssl enabled
        predictor_host = "ready.host"
        dataplane = DataPlane(
            model_registry=ModelRepository(),
            predictor_config=PredictorConfig(
                predictor_host=predictor_host,
                predictor_protocol=PredictorProtocol.GRPC_V2.value,
                predictor_request_retries=2,
                predictor_request_timeout_seconds=5,
                predictor_health_check=True,
                predictor_use_ssl=True,
            ),
        )
        mock_is_server_ready = mock.AsyncMock(return_value=True)
        mock_grpc_client.return_value.is_server_ready = mock_is_server_ready
        assert (await dataplane.ready()) is True
        mock_grpc_client.assert_called_with(
            url=predictor_host, timeout=5, retries=2, use_ssl=True
        )

    async def test_server_readiness_v1(self, httpx_mock):
        # scenario: getting a 2xx response from predictor
        predictor_host = "ready.host"
        httpx_mock.add_response(
            url=re.compile(f"http://{predictor_host}/*"), json={"status": "alive"}
        )
        dataplane = DataPlane(
            model_registry=ModelRepository(),
            predictor_config=PredictorConfig(
                predictor_host=predictor_host,
                predictor_protocol=PredictorProtocol.REST_V1.value,
                predictor_request_retries=2,
                predictor_request_timeout_seconds=5,
                predictor_health_check=True,
            ),
        )
        assert (await dataplane.ready()) is True

        # scenario: not a 2xx response from predictor
        predictor_host = "not-ready.host"
        httpx_mock.add_response(
            url=re.compile(f"http://{predictor_host}/*"), status_code=500
        )
        dataplane = DataPlane(
            model_registry=ModelRepository(),
            predictor_config=PredictorConfig(
                predictor_host=predictor_host,
                predictor_protocol=PredictorProtocol.REST_V1.value,
                predictor_request_retries=2,
                predictor_request_timeout_seconds=5,
                predictor_health_check=True,
            ),
        )
        with pytest.raises(httpx.HTTPStatusError):
            await dataplane.ready()

    async def test_server_readiness_v2(self, httpx_mock):
        # scenario: getting a 2xx response from predictor
        predictor_host = "ready.host"
        httpx_mock.add_response(
            url=re.compile(f"http://{predictor_host}/v2/*"), json={"ready": True}
        )
        dataplane = DataPlane(
            model_registry=ModelRepository(),
            predictor_config=PredictorConfig(
                predictor_host=predictor_host,
                predictor_protocol=PredictorProtocol.REST_V2.value,
                predictor_request_retries=2,
                predictor_request_timeout_seconds=5,
                predictor_health_check=True,
            ),
        )
        assert (await dataplane.ready()) is True

        # scenario: getting a 2xx response from predictor and server not ready
        predictor_host = "not-ready.host"
        httpx_mock.add_response(
            url=re.compile(f"http://{predictor_host}/v2/*"), json={"ready": False}
        )
        dataplane = DataPlane(
            model_registry=ModelRepository(),
            predictor_config=PredictorConfig(
                predictor_host=predictor_host,
                predictor_protocol=PredictorProtocol.REST_V2.value,
                predictor_request_retries=2,
                predictor_request_timeout_seconds=5,
                predictor_health_check=True,
            ),
        )
        assert (await dataplane.ready()) is False

        # scenario: not a 2xx response from predictor
        predictor_host = "not-ready.host"
        httpx_mock.add_response(
            url=re.compile(f"http://{predictor_host}/v2/*"), status_code=500
        )
        dataplane = DataPlane(
            model_registry=ModelRepository(),
            predictor_config=PredictorConfig(
                predictor_host=predictor_host,
                predictor_protocol=PredictorProtocol.REST_V2.value,
                predictor_request_retries=2,
                predictor_request_timeout_seconds=5,
                predictor_health_check=True,
            ),
        )
        with pytest.raises(httpx.HTTPStatusError):
            await dataplane.ready()

    @patch("kserve.protocol.dataplane.InferenceClientFactory.get_grpc_client")
    async def test_server_readiness_grpc_v2(self, mock_grpc_client):
        # scenario: getting a 2xx response from predictor
        predictor_host = "ready.host"
        dataplane = DataPlane(
            model_registry=ModelRepository(),
            predictor_config=PredictorConfig(
                predictor_host=predictor_host,
                predictor_protocol=PredictorProtocol.GRPC_V2.value,
                predictor_request_retries=2,
                predictor_request_timeout_seconds=5,
                predictor_health_check=True,
            ),
        )
        mock_is_server_ready = mock.AsyncMock(return_value=True)
        mock_grpc_client.return_value.is_server_ready = mock_is_server_ready
        assert (await dataplane.ready()) is True
        mock_grpc_client.assert_called_with(
            url=predictor_host, timeout=5, retries=2, use_ssl=False
        )

        # scenario: getting a 2xx response from predictor and server not ready
        predictor_host = "not-ready.host"
        dataplane = DataPlane(
            model_registry=ModelRepository(),
            predictor_config=PredictorConfig(
                predictor_host=predictor_host,
                predictor_protocol=PredictorProtocol.GRPC_V2.value,
                predictor_request_retries=2,
                predictor_request_timeout_seconds=5,
                predictor_health_check=True,
            ),
        )
        mock_is_server_ready = mock.AsyncMock(return_value=False)
        mock_grpc_client.return_value.is_server_ready = mock_is_server_ready
        assert (await dataplane.ready()) is False
        mock_grpc_client.assert_called_with(
            url=predictor_host, timeout=5, retries=2, use_ssl=False
        )

    async def test_model_readiness_v1(self, httpx_mock):
        # scenario: getting a 2xx response from predictor
        predictor_host = "ready.host"
        httpx_mock.add_response(
            url=re.compile(f"http://{predictor_host}/v1/*"), json={"ready": True}
        )
        dataplane = DataPlane(
            model_registry=ModelRepository(),
            predictor_config=PredictorConfig(
                predictor_host=predictor_host,
                predictor_protocol=PredictorProtocol.REST_V1.value,
                predictor_request_retries=2,
                predictor_request_timeout_seconds=5,
                predictor_health_check=True,
            ),
        )
        # Transformer model ready
        ready_model = DummyModel("ReadyModel")
        dataplane._model_registry.update(ready_model)
        assert (await dataplane.model_ready(ready_model.name)) is True

        # scenario: getting a 2xx response from predictor and model not ready
        predictor_host = "ready.host"
        httpx_mock.add_response(
            url=re.compile(f"http://{predictor_host}/v1/*"), json={"ready": False}
        )
        dataplane = DataPlane(
            model_registry=ModelRepository(),
            predictor_config=PredictorConfig(
                predictor_host=predictor_host,
                predictor_protocol=PredictorProtocol.REST_V1.value,
                predictor_request_retries=2,
                predictor_request_timeout_seconds=5,
                predictor_health_check=True,
            ),
        )
        # Transformer model ready
        not_ready_model = DummyModel("NotReadyModel")
        dataplane._model_registry.update(not_ready_model)
        assert (await dataplane.model_ready(not_ready_model.name)) is False

        # scenario: not a 2xx response from predictor
        predictor_host = "not-ready.host"
        httpx_mock.add_response(
            url=re.compile(f"http://{predictor_host}/v1/*"), status_code=503
        )
        dataplane = DataPlane(
            model_registry=ModelRepository(),
            predictor_config=PredictorConfig(
                predictor_host=predictor_host,
                predictor_protocol=PredictorProtocol.REST_V1.value,
                predictor_request_retries=2,
                predictor_request_timeout_seconds=5,
                predictor_health_check=True,
            ),
        )
        # Transformer model is not ready
        not_ready_model = DummyModel("NotReadyModel")
        dataplane._model_registry.update(not_ready_model)
        assert await dataplane.model_ready(not_ready_model.name) is False

    async def test_model_readiness_v2(self, httpx_mock):
        # scenario: getting a 2xx response from predictor
        predictor_host = "ready.host"
        httpx_mock.add_response(url=re.compile(f"http://{predictor_host}/v2/*"))
        dataplane = DataPlane(
            model_registry=ModelRepository(),
            predictor_config=PredictorConfig(
                predictor_host=predictor_host,
                predictor_protocol=PredictorProtocol.REST_V2.value,
                predictor_request_retries=2,
                predictor_request_timeout_seconds=5,
                predictor_health_check=True,
            ),
        )
        # Transformer model ready
        ready_model = DummyModel("ReadyModel")
        dataplane._model_registry.update(ready_model)
        assert (await dataplane.model_ready(ready_model.name)) is True

        # scenario: getting a 400 response from predictor and model not ready
        predictor_host = "triton-not-ready.host"
        # Triton returns a non-200 response if model is not ready
        httpx_mock.add_response(
            url=re.compile(f"http://{predictor_host}/v2/*"), status_code=400
        )
        dataplane = DataPlane(
            model_registry=ModelRepository(),
            predictor_config=PredictorConfig(
                predictor_host=predictor_host,
                predictor_protocol=PredictorProtocol.REST_V2.value,
                predictor_request_retries=2,
                predictor_request_timeout_seconds=5,
                predictor_health_check=True,
            ),
        )
        # Transformer model ready
        not_ready_model = DummyModel("NotReadyModel")
        dataplane._model_registry.update(not_ready_model)
        assert (await dataplane.model_ready(not_ready_model.name)) is False

        # scenario: 503 response from predictor
        # According to V2 protocol, 200 status code indicates true and a 4xx status code indicates false.
        # The HTTP response body should be empty.
        # However, KServe returns 503 when not ready
        predictor_host = "not-ready.host"
        httpx_mock.add_response(
            url=re.compile(f"http://{predictor_host}/v2/*"), status_code=503
        )
        dataplane = DataPlane(
            model_registry=ModelRepository(),
            predictor_config=PredictorConfig(
                predictor_host=predictor_host,
                predictor_protocol=PredictorProtocol.REST_V2.value,
                predictor_request_retries=2,
                predictor_request_timeout_seconds=5,
                predictor_health_check=True,
            ),
        )
        # Transformer model is not ready
        not_ready_model = DummyModel("NotReadyModel")
        dataplane._model_registry.update(not_ready_model)
        assert await dataplane.model_ready(not_ready_model.name) is False

        # Connection error
        predictor_host = "not-reachable.host"
        dataplane = DataPlane(
            model_registry=ModelRepository(),
            predictor_config=PredictorConfig(
                predictor_host=predictor_host,
                predictor_protocol=PredictorProtocol.REST_V2.value,
                predictor_request_retries=2,
                predictor_request_timeout_seconds=5,
                predictor_health_check=True,
            ),
        )
        # Transformer model is not ready
        not_ready_model = DummyModel("NotReadyModel")
        dataplane._model_registry.update(not_ready_model)
        httpx_mock.add_exception(
            url=re.compile(f"http://{predictor_host}/v2/*"),
            exception=httpx.ConnectError("All connection attempts failed"),
        )
        assert (await dataplane.model_ready(not_ready_model.name)) is False

    @patch("kserve.protocol.dataplane.InferenceClientFactory.get_grpc_client")
    async def test_model_readiness_grpc_v2(self, mock_grpc_client):
        # scenario: getting a 2xx response from predictor
        predictor_host = "ready.host"
        dataplane = DataPlane(
            model_registry=ModelRepository(),
            predictor_config=PredictorConfig(
                predictor_host=predictor_host,
                predictor_protocol=PredictorProtocol.GRPC_V2.value,
                predictor_request_retries=2,
                predictor_request_timeout_seconds=5,
                predictor_health_check=True,
            ),
        )
        # Transformer model ready
        ready_model = DummyModel("ReadyModel")
        dataplane._model_registry.update(ready_model)
        mock_is_model_ready = mock.AsyncMock(return_value=True)
        mock_grpc_client.return_value.is_model_ready = mock_is_model_ready
        assert (await dataplane.model_ready(ready_model.name)) is True
        mock_grpc_client.assert_called_with(
            url=predictor_host, timeout=5, retries=2, use_ssl=False
        )

        # scenario: getting a 2xx response from predictor and server not ready
        predictor_host = "not-ready.host"
        dataplane = DataPlane(
            model_registry=ModelRepository(),
            predictor_config=PredictorConfig(
                predictor_host=predictor_host,
                predictor_protocol=PredictorProtocol.GRPC_V2.value,
                predictor_request_retries=2,
                predictor_request_timeout_seconds=5,
                predictor_health_check=True,
            ),
        )
        # Transformer model is not ready
        not_ready_model = DummyModel("NotReadyModel")
        dataplane._model_registry.update(not_ready_model)
        mock_is_model_ready = mock.AsyncMock(return_value=False)
        mock_grpc_client.return_value.is_model_ready = mock_is_model_ready
        assert (await dataplane.model_ready(not_ready_model.name)) is False
        mock_grpc_client.assert_called_with(
            url=predictor_host, timeout=5, retries=2, use_ssl=False
        )

        # Connection error
        predictor_host = "not-reachable.host"
        dataplane = DataPlane(
            model_registry=ModelRepository(),
            predictor_config=PredictorConfig(
                predictor_host=predictor_host,
                predictor_protocol=PredictorProtocol.GRPC_V2.value,
                predictor_request_retries=2,
                predictor_request_timeout_seconds=5,
                predictor_health_check=True,
            ),
        )
        dataplane._model_registry.update(ready_model)
        mock_grpc_client.side_effect = grpc.RpcError("Mocked exception")
        assert (await dataplane.model_ready(ready_model.name)) is False

    @patch("kserve.protocol.dataplane.InferenceClientFactory.get_grpc_client")
    @patch("kserve.protocol.dataplane.InferenceClientFactory.get_rest_client")
    async def test_dataplane_with_predictor_health_check_false(
        self, mock_rest_client, mock_grpc_client
    ):
        # Inference client should not be created when predictor_health_check is False
        predictor_host = "ready.host"
        _ = DataPlane(
            model_registry=ModelRepository(),
            predictor_config=PredictorConfig(
                predictor_host=predictor_host,
                predictor_protocol=PredictorProtocol.GRPC_V2.value,
                predictor_request_retries=2,
                predictor_request_timeout_seconds=5,
                predictor_health_check=False,
            ),
        )
        mock_grpc_client.assert_not_called()

        _ = DataPlane(
            model_registry=ModelRepository(),
            predictor_config=PredictorConfig(
                predictor_host=predictor_host,
                predictor_protocol=PredictorProtocol.REST_V2.value,
                predictor_request_retries=2,
                predictor_request_timeout_seconds=5,
                predictor_health_check=False,
            ),
        )
        mock_rest_client.assert_not_called()
