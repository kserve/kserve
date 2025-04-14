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

import asyncio
import datetime
import io
import json
import os
import re
from typing import Dict
from unittest import mock

import avro.io
import avro.schema
import httpx
from kserve.protocol.dataplane import DataPlane
from kserve.protocol.model_repository_extension import ModelRepositoryExtension
from kserve.protocol.rest.multiprocess.server import RESTServerMultiProcess
import numpy as np
import pandas as pd
import pytest
import pytest_asyncio
from cloudevents.conversion import to_binary, to_structured
from cloudevents.http import CloudEvent
from fastapi.testclient import TestClient
from ray import serve

from kserve import Model, ModelRepository, ModelServer
from kserve.constants.constants import (
    FASTAPI_APP_IMPORT_STRING,
    INFERENCE_CONTENT_LENGTH_HEADER,
)
from kserve.errors import InvalidInput, NoModelReady
from kserve.model import PredictorProtocol
from kserve.model_server import app as kserve_app
from kserve.ray import RayModel
from kserve.protocol.infer_type import (
    InferInput,
    InferOutput,
    InferRequest,
    InferResponse,
    RequestedOutput,
)
from kserve.protocol.rest.v2_datamodels import is_pydantic_2
from kserve.utils.utils import generate_uuid, get_predict_input, get_predict_response

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

fake_stream_data = "some streamed data"


def dummy_cloud_event(
    data,
    set_contenttype: bool = False,
    add_extension: bool = False,
    contenttype: str = "application/json",
):
    # This data defines a binary cloudevent
    attributes = {
        "type": "com.example.sampletype1",
        "source": "https://example.com/event-producer",
        "specversion": "1.0",
        "id": "36077800-0c23-4f38-a0b4-01f4369f670a",
        "time": "2021-01-28T21:04:43.144141+00:00",
    }
    if set_contenttype:
        attributes["content-type"] = contenttype
    if add_extension:
        attributes["custom-extension"] = "custom-value"

    event = CloudEvent(attributes, data)
    return event


async def fake_data_streamer():
    for _ in range(10):
        yield fake_stream_data.encode()
        await asyncio.sleep(0.5)  # sleep 1/2 second


class DummyStreamModel(Model):
    def __init__(self, name):
        super().__init__(name)
        self.name = name
        self.ready = False

    def load(self):
        self.ready = True

    async def predict(self, request, headers=None):
        return fake_data_streamer()


class TestStreamPredict:

    @pytest_asyncio.fixture(scope="class")
    async def app(self, server):  # pylint: disable=no-self-use
        model = DummyStreamModel("TestModel")
        model.load()
        server.register_model(model)
        yield kserve_app
        await server.model_repository_extension.unload("TestModel")

    @pytest.fixture(scope="class")
    def http_server_client(self, app):
        return TestClient(app)

    def test_predict_stream(self, http_server_client):
        with http_server_client.stream(
            "POST", "/v1/models/TestModel:predict", content=b'{"instances":[[1,2]]}'
        ) as response:
            response: httpx.Response
            all_data = []
            for value in response.iter_bytes():
                data = value.decode()
                assert fake_stream_data in data
                all_data.append(data)
        assert all(
            [fake_stream_data in data for data in all_data]
        ), "Unexpected number of streamed responses"


class DummyModel(Model):
    def __init__(self, name):
        super().__init__(name)
        self.name = name
        self.ready = False

    def load(self):
        self.ready = True

    async def predict(self, request, headers=None):
        if isinstance(request, InferRequest):
            inputs = get_predict_input(request)
            infer_response = get_predict_response(request, inputs, self.name)
            if request.parameters:
                infer_response.parameters = request.parameters
            if request.inputs[0].parameters:
                infer_response.outputs[0].parameters = request.inputs[0].parameters
            return infer_response
        else:
            if "inputs" in request:
                return {"predictions": request["inputs"]}
            else:
                return {"predictions": request["instances"]}

    async def explain(self, request, headers=None):
        if "inputs" in request:
            return {"predictions": request["inputs"]}
        else:
            return {"predictions": request["instances"]}


@serve.deployment
class DummyServeModel(Model):
    async def predict(self, request, headers=None):
        if isinstance(request, InferRequest):
            inputs = get_predict_input(request)
            infer_response = get_predict_response(request, inputs, self.name)
            return infer_response
        else:
            return {"predictions": request["instances"]}

    async def explain(self, request, headers=None):
        return {"predictions": request["instances"]}


class DummyCEModel(Model):
    def __init__(self, name):
        super().__init__(name)
        self.name = name
        self.ready = False

    def load(self):
        self.ready = True

    async def predict(self, request, headers=None):
        return {"predictions": request["instances"]}

    async def explain(self, request, headers=None):
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

    def preprocess(self, request, headers: Dict[str, str] = None):
        assert headers["ce-specversion"] == "1.0"
        assert headers["ce-source"] == "https://example.com/event-producer"
        assert headers["ce-type"] == "com.example.sampletype1"
        assert headers["ce-content-type"] == "application/avro"
        return self._parserequest(request)

    async def predict(self, request, headers=None):
        return {
            "predictions": [
                [request["name"], request["favorite_number"], request["favorite_color"]]
            ]
        }

    async def explain(self, request, headers=None):
        return {
            "predictions": [
                [request["name"], request["favorite_number"], request["favorite_color"]]
            ]
        }


class DummyModelRepository(ModelRepository):
    def __init__(self, test_load_success: bool, fail_with_exception: bool = False):
        super().__init__()
        self.test_load_success = test_load_success
        self.fail_with_exception = fail_with_exception

    async def load(self, name: str) -> bool:
        if self.test_load_success:
            model = DummyModel(name)
            model.load()
            self.update(model)
            return model.ready
        else:
            if self.fail_with_exception:
                raise Exception(f"Could not load model {name}.")
            else:
                return False


class DummyNeverReadyModel(Model):
    def __init__(self, name):
        super().__init__(name)
        self.name = name
        self.ready = False


class DummyFP16OutputModel(Model):
    def __init__(self, name):
        super().__init__(name)
        self.name = name
        self.ready = False

    def load(self):
        self.ready = True

    async def predict(self, request, headers=None):
        outputs = pd.DataFrame(
            {
                "fp16_output": request.get_input_by_name("fp32_input")
                .as_numpy()
                .astype(np.float16)
                .flatten(),
                "fp32_output": request.get_input_by_name("fp32_input")
                .as_numpy()
                .flatten(),
            }
        )
        # Fixme: Gets only the 1st element of the input
        # inputs = get_predict_input(request)
        infer_response = get_predict_response(request, outputs, self.name)
        if request.parameters:
            infer_response.parameters = request.parameters
            infer_response.parameters.pop("binary_data_output", None)
        if request.inputs[0].parameters:
            infer_response.outputs[0].parameters = request.inputs[0].parameters
            infer_response.outputs[0].parameters.pop("binary_data", None)
        return infer_response


class DummyFP16InputModel(Model):
    def __init__(self, name):
        super().__init__(name)
        self.name = name
        self.ready = False

    def load(self):
        self.ready = True

    async def predict(self, request, headers=None):
        outputs = pd.DataFrame(
            {
                "str_output": request.get_input_by_name("str_input")
                .as_numpy()
                .flatten(),
                "fp32_output": request.get_input_by_name("fp16_input")
                .as_numpy()
                .astype(np.float32)
                .flatten(),
            }
        )
        # Fixme: Gets only the 1st element of the input
        # inputs = get_predict_input(request)
        infer_response = get_predict_response(request, outputs, self.name)
        if request.parameters:
            infer_response.parameters = request.parameters
            infer_response.parameters.pop("binary_data_output", None)
        if request.inputs[0].parameters:
            infer_response.outputs[0].parameters = request.inputs[0].parameters
            infer_response.outputs[0].parameters.pop("binary_data", None)
        return infer_response


class DateTimeModel(Model):
    def __init__(self, name):
        super().__init__(name)
        self.ready = False

    def load(self):
        self.ready = True

    async def predict(self, payload, headers=None, response_headers=None):
        if isinstance(payload, InferRequest):
            response_id = generate_uuid()
            infer_output = InferOutput(
                name="output-0",
                shape=[1],
                datatype="BYTES",
            )
            infer_output.set_data_from_numpy(
                np.array(
                    [datetime.datetime.now(tz=datetime.timezone.utc)], dtype=np.object_
                ),
                binary_data=False,
            )
            return InferResponse(
                model_name=self.name,
                infer_outputs=[infer_output],
                response_id=response_id,
            )
        return {"predictions": [datetime.datetime.now(tz=datetime.timezone.utc)]}

    async def explain(self, payload, headers=None):
        return {"predictions": [datetime.datetime.now(tz=datetime.timezone.utc)]}


@pytest.fixture(scope="module")
def http_server_client():
    return TestClient(kserve_app, headers={"content-type": "application/json"})


@pytest.mark.asyncio
class TestModel:
    async def test_validate(self):
        model = DummyModel("TestModel")
        good_request = {"instances": []}
        validated_request = model.validate(good_request)
        assert validated_request == good_request
        bad_request = {"instances": "invalid"}
        with pytest.raises(InvalidInput):
            model.validate(bad_request)

        model.protocol = PredictorProtocol.REST_V2.value
        good_request = {"inputs": []}
        validated_request = model.validate(good_request)
        assert validated_request == good_request
        bad_request = {"inputs": "invalid"}
        with pytest.raises(InvalidInput):
            model.validate(bad_request)


# Separate out v1 and v2 endpoint unit tests in
# https://github.com/kserve/kserve/blob/master/python/kserve/test/test_server.py.


class TestV1Endpoints:

    @pytest_asyncio.fixture(scope="class", autouse=True)
    async def app(self, server):
        model = DummyModel("TestModel")
        model.load()
        server.register_model(model)
        datetime_model = DateTimeModel("DateTimeModel")
        datetime_model.load()
        server.register_model(datetime_model)
        yield kserve_app
        await server.model_repository_extension.unload("TestModel")
        await server.model_repository_extension.unload("DateTimeModel")

    def test_liveness_v1(self, http_server_client):
        resp = http_server_client.get("/")
        assert resp.status_code == 200
        assert resp.json() == {"status": "alive"}

    def test_model_v1(self, http_server_client):
        resp = http_server_client.get("/v1/models/TestModel")
        assert resp.status_code == 200

    def test_unknown_model_v1(self, http_server_client):
        resp = http_server_client.get("/v1/models/InvalidModel")
        assert resp.status_code == 404
        assert resp.json() == {"error": "Model with name InvalidModel does not exist."}

    def test_list_models_v1(self, http_server_client):
        resp = http_server_client.get("/v1/models")
        assert resp.status_code == 200
        assert resp.json() == {"models": ["TestModel", "DateTimeModel"]}

    def test_predict_v1(self, http_server_client):
        resp = http_server_client.post(
            "/v1/models/TestModel:predict", content=b'{"instances":[[1,2]]}'
        )
        assert resp.status_code == 200
        assert resp.content == b'{"predictions":[[1,2]]}'
        assert resp.headers["content-type"] == "application/json"

    def test_explain_v1(self, http_server_client):
        resp = http_server_client.post(
            "/v1/models/TestModel:explain", content=b'{"instances":[[1,2]]}'
        )
        assert resp.status_code == 200
        assert resp.content == b'{"predictions":[[1,2]]}'
        assert resp.headers["content-type"] == "application/json"

    def test_unknown_path_v1(self, http_server_client):
        resp = http_server_client.get("/unknown_path")
        assert resp.status_code == 404
        assert resp.json() == {"detail": "Not Found"}

    def test_metrics_v1(self, http_server_client):
        resp = http_server_client.get("/metrics")
        assert resp.status_code == 200
        assert resp.content is not None

    def test_datetime_output_v1(self, http_server_client):
        resp = http_server_client.post(
            "/v1/models/DateTimeModel:predict", content=b'{"instances":[[1,2]]}'
        )
        assert resp.status_code == 200
        assert resp.headers["content-type"] == "application/json"
        result = json.loads(resp.content)
        assert isinstance(result["predictions"][0], str)
        result_datetime = datetime.datetime.fromisoformat(result["predictions"][0])
        datetime_now = datetime.datetime.now(tz=datetime.timezone.utc)
        assert result_datetime.date() == datetime_now.date()
        assert result_datetime.tzinfo == datetime_now.tzinfo


class TestV2Endpoints:

    @pytest_asyncio.fixture(scope="class", autouse=True)
    async def app(self, server):
        model = DummyModel("TestModel")
        model.load()
        server.register_model(model)
        fp16_input_model = DummyFP16InputModel("FP16InputModel")
        fp16_input_model.load()
        server.register_model(fp16_input_model)
        fp16_output_model = DummyFP16OutputModel("FP16OutputModel")
        fp16_output_model.load()
        server.register_model(fp16_output_model)
        datetime_model = DateTimeModel("DateTimeModel")
        datetime_model.load()
        server.register_model(datetime_model)
        yield kserve_app
        await server.model_repository_extension.unload("TestModel")
        await server.model_repository_extension.unload("FP16InputModel")
        await server.model_repository_extension.unload("FP16OutputModel")
        await server.model_repository_extension.unload("DateTimeModel")

    def test_list_models_v2(self, http_server_client):
        resp = http_server_client.get("/v2/models")
        assert resp.status_code == 200
        assert resp.json() == {
            "models": [
                "TestModel",
                "FP16InputModel",
                "FP16OutputModel",
                "DateTimeModel",
            ]
        }

    def test_infer_v2(self, http_server_client):
        input_data = b'{"inputs": [{"name": "input-0","shape": [1, 2],"datatype": "INT32","data": [[1,2]]}]}'
        resp = http_server_client.post(
            "/v2/models/TestModel/infer",
            content=input_data,
            headers={"content-type": "application/json"},
        )

        result = json.loads(resp.content)
        assert resp.status_code == 200
        assert result["outputs"][0]["data"] == [1, 2]
        assert resp.headers["content-type"] == "application/json"

    def test_infer_parameters_v2(self, http_server_client):
        model_name = "TestModel"
        req = InferRequest(
            model_name=model_name,
            request_id="123",
            parameters={
                "test-str": "dummy",
                "test-bool": True,
                "test-int": 100,
                "test-float": 1.3,
            },
            infer_inputs=[
                InferInput(
                    name="input-0",
                    datatype="INT32",
                    shape=[1, 2],
                    data=[1, 2],
                    parameters={
                        "test-str": "dummy",
                        "test-bool": True,
                        "test-int": 100,
                        "test-float": 1.3,
                    },
                )
            ],
        )
        infer_dict, _ = req.to_rest()
        input_data = json.dumps(infer_dict).encode("utf-8")
        expected_res = InferResponse(
            model_name=model_name,
            response_id="123",
            parameters={
                "test-str": "dummy",
                "test-bool": True,
                "test-int": 100,
                "test-float": 1.3,
            },
            infer_outputs=[
                InferOutput(
                    name="output-0",
                    datatype="INT32",
                    shape=[1, 2],
                    data=[1, 2],
                    parameters={
                        "test-str": "dummy",
                        "test-bool": True,
                        "test-int": 100,
                        "test-float": 1.3,
                    },
                )
            ],
        )
        resp = http_server_client.post("/v2/models/TestModel/infer", content=input_data)
        assert resp.status_code == 200
        assert resp.headers["content-type"] == "application/json"
        result = InferResponse.from_rest(response=json.loads(resp.content))
        assert result == expected_res

    def test_fp16_input_as_binary_data(self, http_server_client):
        fp16_data = np.array(
            [[6.8, 2.8, 4.8, 1.4], [6.0, 3.4, 4.5, 1.6]], dtype=np.float16
        )
        str_data = np.array(
            [["cat", "dog", "cat", "dog"], ["cat", "dog", "cat", "dog"]],
            dtype=np.object_,
        )
        fp16_input = InferInput(
            name="fp16_input",
            shape=[2, 4],
            datatype="FP16",
        )
        fp16_input.set_data_from_numpy(fp16_data, binary_data=True)
        request = InferRequest(
            model_name="FP16InputModel",
            request_id="123",
            infer_inputs=[
                fp16_input,
                InferInput(
                    name="str_input",
                    shape=[2, 4],
                    datatype="BYTES",
                    data=str_data.tolist(),
                ),
            ],
        )
        req_bytes, json_length = request.to_rest()
        assert isinstance(req_bytes, bytes)
        resp = http_server_client.post(
            "/v2/models/FP16InputModel/infer",
            content=req_bytes,
            headers={
                INFERENCE_CONTENT_LENGTH_HEADER: str(json_length),
                "Content-Type": "application/octet-stream",
            },
        )
        assert resp.status_code == 200
        assert (
            resp.content
            == b'{"model_name":"FP16InputModel","model_version":null,"id":"123","parameters":null,"outputs":[{"name":"str_output","shape":[8],"datatype":"BYTES","parameters":null,"data":["cat","dog","cat","dog","cat","dog","cat","dog"]},{"name":"fp32_output","shape":[8],"datatype":"FP32","parameters":null,"data":[6.80078125,2.80078125,4.80078125,1.400390625,6.0,3.400390625,4.5,1.599609375]}]}'
        )

    def test_fp16_input_not_binary_data(self, http_server_client):
        fp16_data = np.array(
            [[6.8, 2.8, 4.8, 1.4], [6.0, 3.4, 4.5, 1.6]], dtype=np.float16
        )
        str_data = np.array(
            [["cat", "dog", "cat", "dog"], ["cat", "dog", "cat", "dog"]],
            dtype=np.object_,
        )
        req_dict = {
            "model_name": "FP16InputModel",
            "request_id": "123",
            "inputs": [
                {
                    "name": "fp16_input",
                    "shape": [2, 4],
                    "datatype": "FP16",
                    "data": fp16_data.tolist(),
                },
                {
                    "name": "str_input",
                    "shape": [2, 4],
                    "datatype": "BYTES",
                    "data": str_data.tolist(),
                },
            ],
        }
        resp = http_server_client.post(
            "/v2/models/FP16InputModel/infer",
            json=req_dict,
            headers={
                "Content-Type": "application/json",
            },
        )
        assert resp.status_code == 400

    def test_fp16_output_as_binary_data(self, http_server_client):
        fp32_data = np.array(
            [[6.8, 2.8, 4.8, 1.4], [6.0, 3.4, 4.5, 1.6]], dtype=np.float32
        )
        request = InferRequest(
            model_name="FP16OutputModel",
            request_id="123",
            infer_inputs=[
                InferInput(
                    name="fp32_input",
                    shape=[2, 4],
                    datatype="FP32",
                    data=fp32_data.tolist(),
                )
            ],
            request_outputs=[
                RequestedOutput(
                    name="fp16_output",
                    parameters={"binary_data": True},
                ),
                RequestedOutput(
                    name="fp32_output",
                    parameters={"binary_data": False},
                ),
            ],
        )
        req_dict, _ = request.to_rest()
        resp = http_server_client.post(
            "/v2/models/FP16OutputModel/infer",
            json=req_dict,
            headers={
                "Content-Type": "application/json",
            },
        )
        assert resp.status_code == 200
        assert (
            resp.content
            == b'{"id":"123","model_name":"FP16OutputModel","model_version":null,"outputs":[{"name":"fp16_output","shape":[8],"datatype":"FP16","parameters":{"binary_data_size":16}},{"name":"fp32_output","shape":[8],"datatype":"FP32","data":[6.800000190734863,2.799999952316284,4.800000190734863,1.399999976158142,6.0,3.4000000953674316,4.5,1.600000023841858]}]}\xcdF\x9aA\xcdD\x9a=\x00F\xcdB\x80Df>'
        )
        assert resp.headers.get(INFERENCE_CONTENT_LENGTH_HEADER) == "345"

    def test_fp16_output_not_binary_data(self, http_server_client):
        fp32_data = np.array(
            [[6.8, 2.8, 4.8, 1.4], [6.0, 3.4, 4.5, 1.6]], dtype=np.float32
        )
        req_dict = {
            "model_name": "FP16OutputModel",
            "request_id": "123",
            "inputs": [
                {
                    "name": "fp32_input",
                    "shape": [2, 4],
                    "datatype": "FP32",
                    "data": fp32_data.tolist(),
                }
            ],
            "outputs": [
                {
                    "name": "fp16_output",
                },
                {
                    "name": "fp32_output",
                    "parameters": {"binary_data": False},
                },
            ],
        }
        resp = http_server_client.post(
            "/v2/models/FP16OutputModel/infer",
            json=req_dict,
            headers={
                "Content-Type": "application/json",
            },
        )
        assert resp.status_code == 400

    def test_requested_output(self, http_server_client):
        fp32_data = np.array(
            [[6.8, 2.8, 4.8, 1.4], [6.0, 3.4, 4.5, 1.6]], dtype=np.float32
        )
        request = InferRequest(
            model_name="FP16OutputModel",
            request_id="123",
            infer_inputs=[
                InferInput(
                    name="fp32_input",
                    shape=[2, 4],
                    datatype="FP32",
                    data=fp32_data.tolist(),
                )
            ],
            request_outputs=[
                RequestedOutput(
                    name="fp32_output",
                    parameters={"binary_data": False},
                )
            ],
        )
        req_dict, _ = request.to_rest()
        resp = http_server_client.post(
            "/v2/models/FP16OutputModel/infer",
            json=req_dict,
            headers={
                "Content-Type": "application/json",
            },
        )
        assert resp.status_code == 200
        assert (
            resp.content
            == b'{"model_name":"FP16OutputModel","model_version":null,"id":"123","parameters":null,"outputs":[{"name":"fp32_output","shape":[8],"datatype":"FP32","parameters":null,"data":[6.800000190734863,2.799999952316284,4.800000190734863,1.399999976158142,6.0,3.4000000953674316,4.5,1.600000023841858]}]}'
        )

    def test_all_output_as_binary_data(self, http_server_client):
        fp32_data = np.array(
            [[6.8, 2.8, 4.8, 1.4], [6.0, 3.4, 4.5, 1.6]], dtype=np.float32
        )
        request = InferRequest(
            model_name="FP16OutputModel",
            request_id="123",
            infer_inputs=[
                InferInput(
                    name="fp32_input",
                    shape=[2, 4],
                    datatype="FP32",
                    data=fp32_data.tolist(),
                )
            ],
            parameters={"binary_data_output": True},
        )
        req_dict, _ = request.to_rest()
        resp = http_server_client.post(
            "/v2/models/FP16OutputModel/infer",
            json=req_dict,
            headers={
                "Content-Type": "application/json",
            },
        )
        assert resp.status_code == 200
        assert (
            resp.content
            == b'{"id":"123","model_name":"FP16OutputModel","model_version":null,"outputs":[{"name":"fp16_output","shape":[8],"datatype":"FP16","parameters":{"binary_data_size":16}},{"name":"fp32_output","shape":[8],"datatype":"FP32","parameters":{"binary_data_size":32}}]}\xcdF\x9aA\xcdD\x9a=\x00F\xcdB\x80Df>\x9a\x99\xd9@333@\x9a\x99\x99@33\xb3?\x00\x00\xc0@\x9a\x99Y@\x00\x00\x90@\xcd\xcc\xcc?'
        )
        assert resp.headers.get(INFERENCE_CONTENT_LENGTH_HEADER) == "256"

    def test_binary_data_parameter_precedence(self, http_server_client):
        fp32_data = np.array(
            [[6.8, 2.8, 4.8, 1.4], [6.0, 3.4, 4.5, 1.6]], dtype=np.float32
        )
        request = InferRequest(
            model_name="FP16OutputModel",
            request_id="123",
            infer_inputs=[
                InferInput(
                    name="fp32_input",
                    shape=[2, 4],
                    datatype="FP32",
                    data=fp32_data.tolist(),
                )
            ],
            parameters={"binary_data_output": True},
            request_outputs=[
                RequestedOutput(
                    name="fp16_output",
                    parameters={"binary_data": True},
                ),
                RequestedOutput(
                    name="fp32_output",
                    parameters={"binary_data": False},
                ),
            ],
        )
        req_dict, _ = request.to_rest()
        resp = http_server_client.post(
            "/v2/models/FP16OutputModel/infer",
            json=req_dict,
            headers={
                "Content-Type": "application/json",
            },
        )
        assert resp.status_code == 200
        assert (
            resp.content
            == b'{"id":"123","model_name":"FP16OutputModel","model_version":null,"outputs":[{"name":"fp16_output","shape":[8],"datatype":"FP16","parameters":{"binary_data_size":16}},{"name":"fp32_output","shape":[8],"datatype":"FP32","data":[6.800000190734863,2.799999952316284,4.800000190734863,1.399999976158142,6.0,3.4000000953674316,4.5,1.600000023841858]}]}\xcdF\x9aA\xcdD\x9a=\x00F\xcdB\x80Df>'
        )
        assert resp.headers.get(INFERENCE_CONTENT_LENGTH_HEADER) == "345"

    def test_datetime_output(self, http_server_client):
        input_data = b'{"inputs": [{"name": "input-0","shape": [1, 2],"datatype": "INT32","data": [[1,2]]}]}'
        resp = http_server_client.post(
            "/v2/models/DateTimeModel/infer",
            content=input_data,
        )
        assert resp.status_code == 200
        assert resp.headers["content-type"] == "application/json"
        result = json.loads(resp.content)
        assert isinstance(result["outputs"][0]["data"][0], str)
        result_datetime = datetime.datetime.fromisoformat(
            result["outputs"][0]["data"][0]
        )
        datetime_now = datetime.datetime.now(tz=datetime.timezone.utc)
        assert result_datetime.date() == datetime_now.date()
        assert result_datetime.tzinfo == datetime_now.tzinfo


class TestRayServer:

    @pytest_asyncio.fixture(scope="class", autouse=True)
    async def app(self, server):  # pylint: disable=no-self-use
        serve.start(http_options={"host": "0.0.0.0", "port": 9071})

        # https://github.com/ray-project/ray/blob/releases/2.8.0/python/ray/serve/deployment.py#L256
        model_name = "TestModel"
        ray_app = DummyServeModel.bind(name=model_name)
        handle = serve.run(ray_app, name=model_name, route_prefix="/")

        model = RayModel(model_name, handle=handle)
        model.load()
        server.register_model(model)
        yield kserve_app
        await server.model_repository_extension.unload("TestModel")
        serve.shutdown()

    def test_liveness_handler(self, http_server_client):
        resp = http_server_client.get("/")
        assert resp.status_code == 200
        assert resp.content == b'{"status":"alive"}'

    def test_list_handler(self, http_server_client):
        resp = http_server_client.get("/v1/models")
        assert resp.status_code == 200
        assert resp.content == b'{"models":["TestModel"]}'

    def test_health_handler(self, http_server_client):
        resp = http_server_client.get("/v1/models/TestModel")
        assert resp.status_code == 200
        # for some reason the RayServer responds with the stringified python bool
        # when run on pydantic < 2 and the bool when run on pydantic >= 2
        # eg {"name":"TestModel","ready":"True"} vs {"name":"TestModel","ready":true}
        if is_pydantic_2:
            expected_content = b'{"name":"TestModel","ready":true}'
        else:
            expected_content = b'{"name":"TestModel","ready":"True"}'
        assert resp.content == expected_content

    def test_predict(self, http_server_client):
        resp = http_server_client.post(
            "/v1/models/TestModel:predict", content=b'{"instances":[[1,2]]}'
        )
        assert resp.status_code == 200
        assert resp.content == b'{"predictions":[[1,2]]}'
        assert resp.headers["content-type"] == "application/json"

    def test_infer(self, http_server_client):
        input_data = b'{"inputs": [{"name": "input-0","shape": [1, 2],"datatype": "INT32","data": [[1,2]]}]}'
        resp = http_server_client.post("/v2/models/TestModel/infer", content=input_data)

        result = json.loads(resp.content)
        assert resp.status_code == 200
        assert result["outputs"][0]["data"] == [1, 2]
        assert resp.headers["content-type"] == "application/json"

    def test_explain(self, http_server_client):
        resp = http_server_client.post(
            "/v1/models/TestModel:explain", content=b'{"instances":[[1,2]]}'
        )
        assert resp.status_code == 200
        assert resp.content == b'{"predictions":[[1,2]]}'
        assert resp.headers["content-type"] == "application/json"


class TestTFHttpServerModelNotLoaded:

    @pytest_asyncio.fixture(scope="class", autouse=True)
    async def app(self, server):  # pylint: disable=no-self-use
        model = DummyModel("TestModel")
        server.register_model(model)
        yield kserve_app
        await server.model_repository_extension.unload("TestModel")

    def test_model_not_ready_error(self, http_server_client):
        resp = http_server_client.get("/v1/models/TestModel")
        assert resp.status_code == 503


class TestTFHttpServerCloudEvent:

    @pytest_asyncio.fixture(scope="class", autouse=True)
    async def app(self, server):  # pylint: disable=no-self-use
        model = DummyCEModel("TestModel")
        model.load()
        server.register_model(model)
        yield kserve_app
        await server.model_repository_extension.unload("TestModel")

    def test_predict_ce_structured(self, http_server_client):
        event = dummy_cloud_event({"instances": [[1, 2]]})
        headers, body = to_structured(event)

        resp = http_server_client.post(
            "/v1/models/TestModel:predict", headers=headers, content=body
        )
        body = json.loads(resp.content)

        assert resp.status_code == 200
        assert resp.headers["content-type"] == "application/cloudevents+json"

        assert body["id"] != "36077800-0c23-4f38-a0b4-01f4369f670a"
        assert body["data"] == {"predictions": [[1, 2]]}
        assert body["specversion"] == "1.0"
        assert body["source"] == "io.kserve.inference.TestModel"
        assert body["type"] == "io.kserve.inference.response"
        assert body["time"] > "2021-01-28T21:04:43.144141+00:00"

    def test_predict_custom_ce_attributes(self, http_server_client):
        with mock.patch.dict(
            os.environ,
            {
                "CE_SOURCE": "io.kserve.inference.CustomSource",
                "CE_TYPE": "io.kserve.custom_type",
            },
        ):
            event = dummy_cloud_event({"instances": [[1, 2]]})
            headers, body = to_structured(event)

            resp = http_server_client.post(
                "/v1/models/TestModel:predict", headers=headers, content=body
            )
            body = json.loads(resp.content)

            assert resp.status_code == 200
            assert resp.headers["content-type"] == "application/cloudevents+json"

            assert body["id"] != "36077800-0c23-4f38-a0b4-01f4369f670a"
            assert body["data"] == {"predictions": [[1, 2]]}
            assert body["source"] == "io.kserve.inference.CustomSource"
            assert body["type"] == "io.kserve.custom_type"

    def test_predict_merge_structured_ce_attributes(self, http_server_client):
        with mock.patch.dict(os.environ, {"CE_MERGE": "true"}):
            event = dummy_cloud_event({"instances": [[1, 2]]}, add_extension=True)
            headers, body = to_structured(event)

            resp = http_server_client.post(
                "/v1/models/TestModel:predict", headers=headers, content=body
            )
            body = json.loads(resp.content)

            assert resp.status_code == 200
            assert resp.headers["content-type"] == "application/cloudevents+json"

            assert body["id"] != "36077800-0c23-4f38-a0b4-01f4369f670a"
            assert body["data"] == {"predictions": [[1, 2]]}
            assert body["source"] == "io.kserve.inference.TestModel"
            assert body["type"] == "io.kserve.inference.response"
            assert (
                body["custom-extension"] == "custom-value"
            )  # Added by add_extension=True in dummy_cloud_event
            assert body["time"] > "2021-01-28T21:04:43.144141+00:00"

    def test_predict_merge_binary_ce_attributes(self, http_server_client):
        with mock.patch.dict(os.environ, {"CE_MERGE": "true"}):
            event = dummy_cloud_event(
                {"instances": [[1, 2]]}, set_contenttype=True, add_extension=True
            )
            headers, body = to_binary(event)

            resp = http_server_client.post(
                "/v1/models/TestModel:predict", headers=headers, content=body
            )

            assert resp.status_code == 200
            assert resp.headers["content-type"] == "application/json"
            assert resp.headers["ce-specversion"] == "1.0"
            assert resp.headers["ce-id"] != "36077800-0c23-4f38-a0b4-01f4369f670a"
            # Added by add_extension=True in dummy_cloud_event
            assert resp.headers["ce-custom-extension"] == "custom-value"
            assert resp.headers["ce-source"] == "io.kserve.inference.TestModel"
            assert resp.headers["ce-type"] == "io.kserve.inference.response"
            assert resp.headers["ce-time"] > "2021-01-28T21:04:43.144141+00:00"
            assert resp.content == b'{"predictions": [[1, 2]]}'

    def test_predict_ce_binary_dict(self, http_server_client):
        event = dummy_cloud_event({"instances": [[1, 2]]}, set_contenttype=True)
        headers, body = to_binary(event)

        resp = http_server_client.post(
            "/v1/models/TestModel:predict", headers=headers, content=body
        )

        assert resp.status_code == 200
        assert resp.headers["content-type"] == "application/json"
        assert resp.headers["ce-specversion"] == "1.0"
        assert resp.headers["ce-id"] != "36077800-0c23-4f38-a0b4-01f4369f670a"
        assert resp.headers["ce-source"] == "io.kserve.inference.TestModel"
        assert resp.headers["ce-type"] == "io.kserve.inference.response"
        assert resp.headers["ce-time"] > "2021-01-28T21:04:43.144141+00:00"
        assert resp.content == b'{"predictions": [[1, 2]]}'

    def test_predict_ce_binary_bytes(self, http_server_client):
        event = dummy_cloud_event(b'{"instances":[[1,2]]}', set_contenttype=True)
        headers, body = to_binary(event)
        resp = http_server_client.post(
            "/v1/models/TestModel:predict", headers=headers, content=body
        )

        assert resp.status_code == 200
        assert resp.headers["content-type"] == "application/json"
        assert resp.headers["ce-specversion"] == "1.0"
        assert resp.headers["ce-id"] != "36077800-0c23-4f38-a0b4-01f4369f670a"
        assert resp.headers["ce-source"] == "io.kserve.inference.TestModel"
        assert resp.headers["ce-type"] == "io.kserve.inference.response"
        assert resp.headers["ce-time"] > "2021-01-28T21:04:43.144141+00:00"
        assert resp.content == b'{"predictions": [[1, 2]]}'

    def test_predict_ce_bytes_bad_format_exception(self, http_server_client):
        event = dummy_cloud_event(b"{", set_contenttype=True)
        headers, body = to_binary(event)

        resp = http_server_client.post(
            "/v1/models/TestModel:predict", headers=headers, content=body
        )

        assert resp.status_code == 400
        error_regex = re.compile(
            "Failed to decode or parse binary json cloudevent: "
            "unexpected end of data:*"
        )
        response = json.loads(resp.content)
        assert error_regex.match(response["error"]) is not None

    def test_predict_ce_bytes_bad_hex_format_exception(self, http_server_client):
        event = dummy_cloud_event(b"0\x80\x80\x06World!\x00\x00", set_contenttype=True)
        headers, body = to_binary(event)

        resp = http_server_client.post(
            "/v1/models/TestModel:predict", headers=headers, content=body
        )

        assert resp.status_code == 400
        error_regex = re.compile(
            "Failed to decode or parse binary json cloudevent: "
            "'utf-8' codec can't decode byte 0x80 in position 1: invalid start byte.*"
        )
        response = json.loads(resp.content)
        assert error_regex.match(response["error"]) is not None


class TestTFHttpServerAvroCloudEvent:

    @pytest_asyncio.fixture(scope="class", autouse=True)
    async def app(self, server):  # pylint: disable=no-self-use
        model = DummyAvroCEModel("TestModel")
        model.load()
        server.register_model(model)
        yield kserve_app
        await server.model_repository_extension.unload("TestModel")

    def test_predict_ce_avro_binary(self, http_server_client):
        schema = avro.schema.parse(test_avsc_schema)
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
        resp = http_server_client.post(
            "/v1/models/TestModel:predict", headers=headers, content=body
        )

        assert resp.status_code == 200
        assert resp.headers["content-type"] == "application/json"
        assert resp.headers["ce-specversion"] == "1.0"
        assert resp.headers["ce-id"] != "36077800-0c23-4f38-a0b4-01f4369f670a"
        assert resp.headers["ce-source"] == "io.kserve.inference.TestModel"
        assert resp.headers["ce-type"] == "io.kserve.inference.response"
        assert resp.headers["ce-time"] > "2021-01-28T21:04:43.144141+00:00"
        assert resp.content == b'{"predictions": [["foo", 1, "pink"]]}'


class TestTFHttpServerLoadAndUnLoad:
    @pytest_asyncio.fixture(scope="class", autouse=True)
    def app(self, server):
        mp = pytest.MonkeyPatch()
        mp.setattr(
            server.model_repository_extension,
            "_model_registry",
            DummyModelRepository(test_load_success=True),
        )
        yield
        mp.undo()

    def test_load(self, http_server_client):
        resp = http_server_client.post("/v2/repository/models/model/load", content=b"")
        assert resp.status_code == 200
        assert resp.content == b'{"name":"model","load":true}'

    def test_unload(self, http_server_client):
        resp = http_server_client.post(
            "/v2/repository/models/model/unload", content=b""
        )
        assert resp.status_code == 200
        assert resp.content == b'{"name":"model","unload":true}'


class TestTFHttpServerLoadAndUnLoadFailure:
    @pytest.fixture(scope="class", autouse=True)
    def app(self, server):
        mp = pytest.MonkeyPatch()
        mp.setattr(
            server.model_repository_extension,
            "_model_registry",
            DummyModelRepository(test_load_success=False),
        )
        yield
        mp.undo()

    def test_load_fail(self, http_server_client):
        resp = http_server_client.post("/v2/repository/models/model/load", content=b"")
        assert resp.status_code == 503

    def test_unload_fail(self, http_server_client):
        resp = http_server_client.post(
            "/v2/repository/models/model/unload", content=b""
        )
        assert resp.status_code == 404


class TestTFHttpServerModelNotReady:

    @pytest_asyncio.fixture(scope="class", autouse=True)
    async def app(self, server):  # pylint: disable=no-self-use
        model = DummyModel("TestModel")
        server.register_model(model)
        yield kserve_app
        await server.model_repository_extension.unload("TestModel")

    def test_model_not_ready_v1(self, http_server_client):
        resp = http_server_client.get("/v1/models/TestModel")
        assert resp.status_code == 503

    def test_model_not_ready_v2(self, http_server_client):
        resp = http_server_client.get("/v2/models/TestModel/ready")
        assert resp.status_code == 503

    def test_predict(self, http_server_client):
        resp = http_server_client.post(
            "/v1/models/TestModel:predict", content=b'{"instances":[[1,2]]}'
        )
        assert resp.status_code == 503

    def test_infer(self, http_server_client):
        input_data = b'{"inputs": [{"name": "input-0","shape": [1, 2],"datatype": "INT32","data": [[1,2]]}]}'
        resp = http_server_client.post("/v2/models/TestModel/infer", content=input_data)
        assert resp.status_code == 503

    def test_explain(self, http_server_client):
        resp = http_server_client.post(
            "/v1/models/TestModel:explain", content=b'{"instances":[[1,2]]}'
        )
        assert resp.status_code == 503


class TestWithUnhealthyModel:
    def test_with_not_ready_model(self):
        model = DummyNeverReadyModel("Dummy")
        server = ModelServer()
        with pytest.raises(NoModelReady) as exc_info:
            server.start([model])
        assert exc_info.type == NoModelReady


class TestMutiProcessServer:

    @pytest.mark.asyncio
    async def test_rest_server_multiprocess(self):
        model_repository = ModelRepository()
        dummy_model = DummyModel("TestModel")
        dummy_model.load()
        model_repository.update(dummy_model)
        model_repository.load("TestModel")
        data_plane = DataPlane(model_registry=model_repository)
        model_repository_extension = ModelRepositoryExtension(
            model_registry=model_repository
        )
        http_port = 8080
        workers = 4

        server = RESTServerMultiProcess(
            FASTAPI_APP_IMPORT_STRING,
            data_plane,
            model_repository_extension,
            http_port,
            workers=workers,
        )

        # Start server
        task = asyncio.create_task(server.start())

        # Wait for processes to start and become ready
        max_attempts = 30
        for attempt in range(max_attempts):
            await asyncio.sleep(1)
            if len(server._processes) == workers and all(
                p.is_alive() for p in server._processes
            ):
                break
            if attempt == max_attempts - 1:
                raise RuntimeError("Server worker processes did not start properly")

        try:
            # Constantly poll to check if the port is open
            for _ in range(30):
                try:
                    resp = httpx.get("http://localhost:8080/")
                    if resp.status_code == 200:
                        break
                except httpx.RequestError:
                    await asyncio.sleep(1)
            else:
                raise RuntimeError("Server did not start in time")

            # Send predict request
            input_data = b'{"inputs": [{"name": "input-0","shape": [1, 2],"datatype": "INT32","data": [[1,2]]}]}'
            resp = httpx.post(
                "http://localhost:8080/v2/models/TestModel/infer",
                content=input_data,
                headers={"content-type": "application/json"},
            )
            result = json.loads(resp.content)
            assert resp.status_code == 200
            assert result["outputs"][0]["data"] == [1, 2]
            assert resp.headers["content-type"] == "application/json"

            # Kill a process and check if it's restarted
            old_pid = server._processes[0].pid
            server._processes[0].kill()
            await server._processes[0].wait_for_termination()

            # Wait for respawn
            await asyncio.sleep(5)

            new_pid = server._processes[0].pid
            assert old_pid != new_pid, "Process was not restarted"
        finally:
            # Cleanup
            await server.stop()
            await asyncio.sleep(5)
            task.cancel()
