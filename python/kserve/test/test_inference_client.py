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

import re

import httpx
import numpy as np
import pytest
import pytest_asyncio

from kserve import InferenceRESTClient, InferRequest, InferInput
from kserve.model_server import app as kserve_app
from kserve.errors import UnsupportedProtocol
from kserve.inference_client import RESTConfig
from kserve.protocol.infer_type import RequestedOutput
from test.test_server import DummyModel


@pytest.mark.asyncio
class TestInferenceRESTClient:

    @pytest_asyncio.fixture(scope="class")
    async def app(self, server):
        model = DummyModel("TestModel")
        model.load()
        not_ready_model = DummyModel("NotReadyModel")
        # model.load()  # Model not loaded, i.e. not ready
        server.register_model(model)
        server.register_model(not_ready_model)
        yield kserve_app
        await server.model_repository_extension.unload("TestModel")
        await server.model_repository_extension.unload("NotReadyModel")

    @pytest_asyncio.fixture(scope="class")
    async def rest_client(self, request, app):
        config = RESTConfig(
            transport=httpx.ASGITransport(app=app), verbose=True, protocol=request.param
        )
        client = InferenceRESTClient(config=config)
        yield client
        await client.close()

    @pytest.mark.parametrize(
        "rest_client, protocol", [("v1", "v1"), ("v2", "v2")], indirect=["rest_client"]
    )
    async def test_is_server_ready(self, rest_client, protocol):
        if protocol == "v2":
            assert (
                await rest_client.is_server_ready(
                    "http://test-server",
                    headers={"Host": "test-server.com"},
                    timeout=1.3,
                )
                is True
            )
        else:
            # Unsupported protocol
            with pytest.raises(UnsupportedProtocol, match="Unsupported protocol v1"):
                await rest_client.is_server_ready(
                    "http://test-server",
                    headers={"Host": "test-server.com"},
                    timeout=1.3,
                )

    @pytest.mark.parametrize(
        "rest_client, protocol",
        [("v1", "v1"), ("v2", "v2"), ("v3", "v3")],
        indirect=["rest_client"],
    )
    async def test_is_server_live(self, rest_client, protocol):
        if protocol == "v3":
            # Unsupported protocol
            with pytest.raises(UnsupportedProtocol, match="Unsupported protocol v3"):
                await rest_client.is_server_live(
                    "http://test-server",
                    headers={"Host": "test-server.com"},
                    timeout=1.3,
                )
        else:
            assert (
                await rest_client.is_server_live(
                    "http://test-server",
                    headers={"Host": "test-server.com"},
                    timeout=1.3,
                )
                is True
            )

    @pytest.mark.parametrize(
        "rest_client, protocol",
        [("v1", "v1"), ("v2", "v2"), ("v3", "v3")],
        indirect=["rest_client"],
    )
    async def test_is_model_ready(self, rest_client, protocol):
        if protocol == "v3":
            # Unsupported protocol
            with pytest.raises(UnsupportedProtocol, match="Unsupported protocol v3"):
                await rest_client.is_model_ready(
                    "http://test-server",
                    "TestModel",
                    headers={"Host": "test-server.com"},
                    timeout=1.3,
                )
        else:

            # Ready model
            assert (
                await rest_client.is_model_ready(
                    "http://test-server/",
                    "TestModel",
                    headers={"Host": "test-server.com"},
                    timeout=1.3,
                )
                is True
            )
            # Not ready model
            assert (
                await rest_client.is_model_ready(
                    "http://test-server",
                    "NotReadyModel",
                    headers={"Host": "test-server.com"},
                    timeout=1.3,
                )
                is False
            )

    @pytest.mark.parametrize(
        "rest_client, protocol",
        [("v1", "v1"), ("v2", "v2"), ("v3", "v3")],
        indirect=["rest_client"],
    )
    async def test_infer(self, rest_client, protocol):
        if protocol == "v1":
            input_data = {"instances": [1, 2]}
            res = await rest_client.infer(
                "http://test-server/",
                model_name="TestModel",
                data=input_data,
                headers={"Host": "test-server.com"},
                timeout=2,
            )
            assert res["predictions"] == [1, 2]

            input_data = {"inputs": [1, 2]}
            res = await rest_client.infer(
                "http://test-server/",
                model_name="TestModel",
                data=input_data,
                headers={"Host": "test-server.com"},
                timeout=2,
            )
            assert res["predictions"] == [1, 2]

        elif protocol == "v2":
            request_id = "2ja0ls9j1309"
            input_data = {
                "id": request_id,
                "parameters": {"test-param": "abc"},
                "inputs": [
                    {
                        "name": "input-0",
                        "datatype": "INT32",
                        "shape": [2, 2],
                        "data": [[1, 2], [3, 4]],
                        "parameters": {"test-param": "abc"},
                    }
                ],
            }
            res = await rest_client.infer(
                "http://test-server/",
                model_name="TestModel",
                data=input_data,
                headers={"Host": "test-server.com"},
                timeout=2,
            )
            assert res.outputs[0].data == [1, 2, 3, 4]
            assert res.id == request_id

            input_data = InferRequest(
                model_name="TestModel",
                request_id=request_id,
                infer_inputs=[
                    InferInput(
                        name="input-0",
                        datatype="INT32",
                        shape=[2, 2],
                        data=[[1, 2], [3, 4]],
                        parameters={"test-param": "abc"},
                    )
                ],
                parameters={"test-param": "abc"},
            )

            res = await rest_client.infer(
                "http://test-server/",
                model_name="TestModel",
                data=input_data,
                headers={"Host": "test-server.com"},
                timeout=2,
            )
            assert res.outputs[0].data == [1, 2, 3, 4]
            assert res.id == request_id
        else:
            # Unsupported protocol
            input_data = {"instances": [1, 2]}
            with pytest.raises(UnsupportedProtocol, match="Unsupported protocol v3"):
                await rest_client.infer(
                    "http://test-server/",
                    model_name="TestModel",
                    data=input_data,
                    headers={"Host": "test-server.com"},
                    timeout=2,
                )

        # model_name not provided
        input_data = {"instances": [1, 2]}
        with pytest.raises(ValueError):
            await rest_client.infer(
                "http://test-server/",
                data=input_data,
                headers={"Host": "test-server.com"},
                timeout=2,
            )

    @pytest.mark.parametrize(
        "rest_client, protocol",
        [("v2", "v2")],
        indirect=["rest_client"],
    )
    async def test_infer_with_all_binary_data(self, rest_client, protocol):
        request_id = "2ja0ls9j1309"
        fp16_data = np.array([[1.1, 2.22], [3.345, 4.34343]], dtype=np.float16)
        input_data = InferRequest(
            model_name="TestModel",
            request_id=request_id,
            infer_inputs=[
                InferInput(
                    name="input-0",
                    datatype="INT32",
                    shape=[2, 2],
                    data=[[1, 2], [3, 4]],
                    parameters={"test-param": "abc"},
                ),
                InferInput(
                    name="fp16_data",
                    datatype="FP16",
                    shape=[2, 2],
                    parameters={"test-param": "abc"},
                ),
            ],
            parameters={"test-param": "abc", "binary_data_output": True},
        )
        input_data.inputs[1].set_data_from_numpy(fp16_data, binary_data=True)

        res = await rest_client.infer(
            "http://test-server/",
            model_name="TestModel",
            data=input_data,
            headers={"Host": "test-server.com"},
            timeout=2,
        )
        assert res.outputs[0].data == [1, 2, 3, 4]
        assert res.id == request_id

    @pytest.mark.parametrize(
        "rest_client, protocol",
        [("v2", "v2")],
        indirect=["rest_client"],
    )
    async def test_infer_with_binary_data(self, rest_client, protocol):
        request_id = "2ja0ls9j1309"
        fp16_data = np.array([[1.1, 2.22], [3.345, 4.34343]], dtype=np.float16)
        input_data = InferRequest(
            model_name="TestModel",
            request_id=request_id,
            infer_inputs=[
                InferInput(
                    name="input-0",
                    datatype="INT32",
                    shape=[2, 2],
                    data=[[1, 2], [3, 4]],
                    parameters={"test-param": "abc"},
                ),
                InferInput(
                    name="fp16_data",
                    datatype="FP16",
                    shape=[2, 2],
                    parameters={"test-param": "abc"},
                ),
            ],
            parameters={"test-param": "abc"},
            request_outputs=[
                RequestedOutput(
                    name="output-0", parameters={"binary_data_output": True}
                )
            ],
        )
        input_data.inputs[1].set_data_from_numpy(fp16_data, binary_data=True)

        res = await rest_client.infer(
            "http://test-server/",
            model_name="TestModel",
            data=input_data,
            headers={"Host": "test-server.com"},
            timeout=2,
        )
        assert res.outputs[0].data == [1, 2, 3, 4]
        assert res.id == request_id

    @pytest.mark.parametrize(
        "rest_client", ["v1", "v2", "v3"], indirect=["rest_client"]
    )
    async def test_infer_graph_endpoint(self, rest_client, httpx_mock):
        request_id = "2ja0ls9j1309"
        model_name = "TestModel"
        httpx_mock.add_response(
            url="http://test-server-v1", json={"predictions": [1, 2]}
        )
        httpx_mock.add_response(
            url="http://test-server-v2",
            json={
                "id": request_id,
                "model_name": model_name,
                "outputs": [
                    {
                        "name": "output-0",
                        "datatype": "INT32",
                        "shape": [2, 2],
                        "data": [1, 2, 3, 4],
                    }
                ],
            },
        )

        async with httpx.AsyncClient() as client:
            rest_client._client = client
            input_data = {"instances": [1, 2]}
            res = await rest_client.infer(
                "http://test-server-v1",
                data=input_data,
                headers={"Host": "test-server.com"},
                timeout=2,
                is_graph_endpoint=True,
            )
            assert res["predictions"] == [1, 2]

            input_data = {
                "id": request_id,
                "parameters": {"test-param": "abc"},
                "inputs": [
                    {
                        "name": "input-0",
                        "datatype": "INT32",
                        "shape": [2, 2],
                        "data": [[1, 2], [3, 4]],
                        "parameters": {"test-param": "abc"},
                    }
                ],
            }
            res = await rest_client.infer(
                "http://test-server-v2",
                data=input_data,
                headers={"Host": "test-server.com"},
                timeout=2,
                is_graph_endpoint=True,
            )
            assert res["outputs"][0]["data"] == [1, 2, 3, 4]
            assert res["id"] == request_id

            input_data = InferRequest(
                model_name=model_name,
                request_id=request_id,
                infer_inputs=[
                    InferInput(
                        name="input-0",
                        datatype="INT32",
                        shape=[2, 2],
                        data=[[1, 2], [3, 4]],
                        parameters={"test-param": "abc"},
                    )
                ],
                parameters={"test-param": "abc"},
            )

            res = await rest_client.infer(
                "http://test-server-v2",
                data=input_data,
                headers={"Host": "test-server.com"},
                timeout=2,
                is_graph_endpoint=True,
            )
            assert res["outputs"][0]["data"] == [1, 2, 3, 4]
            assert res["id"] == request_id

    @pytest.mark.parametrize(
        "rest_client, protocol",
        [("v1", "v1"), ("v2", "v2"), ("v3", "v3")],
        indirect=["rest_client"],
    )
    async def test_infer_path_based_routing(self, rest_client, protocol, httpx_mock):
        request_id = "2ja0ls9j1309"
        model_name = "TestModel"
        async with httpx.AsyncClient() as client:
            rest_client._client = client
            if protocol == "v1":
                httpx_mock.add_response(
                    url=re.compile(r"http://test-server/serving/test/test-isvc/v1/*"),
                    json={"predictions": [1, 2]},
                )
                input_data = {"instances": [1, 2]}
                res = await rest_client.infer(
                    "http://test-server/serving/test/test-isvc",
                    model_name="TestModel",
                    data=input_data,
                    headers={"Host": "test-server.com"},
                    timeout=2,
                )
                assert res["predictions"] == [1, 2]
            elif protocol == "v2":
                httpx_mock.add_response(
                    url=re.compile(r"http://test-server/serving/test/test-isvc/v2/*"),
                    json={
                        "id": request_id,
                        "model_name": model_name,
                        "outputs": [
                            {
                                "name": "output-0",
                                "datatype": "INT32",
                                "shape": [2, 2],
                                "data": [1, 2, 3, 4],
                            }
                        ],
                    },
                )
                input_data = {
                    "id": request_id,
                    "parameters": {"test-param": "abc"},
                    "inputs": [
                        {
                            "name": "input-0",
                            "datatype": "INT32",
                            "shape": [2, 2],
                            "data": [[1, 2], [3, 4]],
                            "parameters": {"test-param": "abc"},
                        }
                    ],
                }
                res = await rest_client.infer(
                    "http://test-server/serving/test/test-isvc",
                    model_name="TestModel",
                    data=input_data,
                    headers={"Host": "test-server.com"},
                    timeout=2,
                )
                assert res.outputs[0].data == [1, 2, 3, 4]
                assert res.id == request_id

                input_data = InferRequest(
                    model_name="TestModel",
                    request_id=request_id,
                    infer_inputs=[
                        InferInput(
                            name="input-0",
                            datatype="INT32",
                            shape=[2, 2],
                            data=[[1, 2], [3, 4]],
                            parameters={"test-param": "abc"},
                        )
                    ],
                    parameters={"test-param": "abc"},
                )

                res = await rest_client.infer(
                    "http://test-server/serving/test/test-isvc",
                    model_name=model_name,
                    data=input_data,
                    headers={"Host": "test-server.com"},
                    timeout=2,
                )
                assert res.outputs[0].data == [1, 2, 3, 4]
                assert res.id == request_id
            else:
                # Unsupported protocol
                input_data = {"instances": [1, 2]}
                with pytest.raises(
                    UnsupportedProtocol, match="Unsupported protocol v3"
                ):
                    await rest_client.infer(
                        "http://test-server/serving/test/test-isvc",
                        model_name="TestModel",
                        data=input_data,
                        headers={"Host": "test-server.com"},
                        timeout=2,
                    )

    @pytest.mark.parametrize(
        "rest_client, protocol", [("v1", "v1"), ("v2", "v2")], indirect=["rest_client"]
    )
    async def test_explain(self, rest_client, protocol):
        if protocol == "v1":
            input_data = {"instances": [1, 2]}
            res = await rest_client.explain(
                "http://test-server",
                "TestModel",
                data=input_data,
                headers={"Host": "test-server.com"},
                timeout=2,
            )
            assert res == {"predictions": [1, 2]}

            input_data = {"inputs": [1, 2]}
            res = await rest_client.explain(
                "http://test-server/",
                "TestModel",
                data=input_data,
                headers={"Host": "test-server.com"},
                timeout=2,
            )
            assert res == {"predictions": [1, 2]}
        else:
            # Unsupported protocol
            with pytest.raises(UnsupportedProtocol, match="Unsupported protocol v2"):
                input_data = input_data = {
                    "parameters": {"test-param": "abc"},
                    "inputs": [
                        {
                            "name": "input-0",
                            "datatype": "INT32",
                            "shape": [2, 2],
                            "data": [[1, 2], [3, 4]],
                            "parameters": {"test-param": "abc"},
                        }
                    ],
                }
                await rest_client.explain(
                    "http://test-server",
                    "TestModel",
                    data=input_data,
                    headers={"Host": "test-server.com"},
                    timeout=2,
                )

    @pytest.mark.parametrize("rest_client", ["v2"], indirect=["rest_client"])
    async def test_base_url_with_no_scheme(self, rest_client):
        with pytest.raises(
            httpx.InvalidURL,
            match="Base url should have 'http://' or 'https://' protocol",
        ):
            await rest_client.is_server_ready(
                "test-server.com",
                headers={"Host": "test-server.com"},
                timeout=1.3,
            )
