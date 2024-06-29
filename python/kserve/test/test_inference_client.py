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
import pytest

from kserve import ModelServer, InferenceRESTClient, InferRequest, InferInput
from kserve.errors import UnsupportedProtocol
from kserve.inference_client import RESTConfig
from kserve.protocol.rest.server import RESTServer
from test.test_server import DummyModel


@pytest.mark.asyncio
class TestInferenceRESTClient:
    @pytest.fixture(scope="class")
    def rest_client(self, request):
        model = DummyModel("TestModel")
        model.load()
        not_ready_model = DummyModel("NotReadyModel")
        # model.load()  # Model not loaded, i.e. not ready
        server = ModelServer()
        server.register_model(model)
        server.register_model(not_ready_model)
        rest_server = RESTServer(server.dataplane, server.model_repository_extension)
        app = rest_server.create_application()
        config = RESTConfig(
            transport=httpx.ASGITransport(app=app), verbose=True, protocol=request.param
        )
        return InferenceRESTClient(config=config)

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

    # Because no versions of pytest-httpx match >v0.22.0,<0.23.0
    # and pytest-httpx (0.22.0) depends on httpx (==0.24.*), pytest-httpx (>=v0.22.0,<0.23.0) requires httpx (==0.24.*).
    # So, because kserve depends on both httpx (^0.26.0) and pytest_httpx (~v0.22.0), version solving failed.
    @pytest.mark.skip("pytest_httpx requires python >= 3.9")
    @pytest.mark.parametrize(
        "rest_client", ["v1", "v2", "v3"], indirect=["rest_client"]
    )
    async def test_infer_graph_endpoint(self, rest_client, httpx_mock):
        request_id = "2ja0ls9j1309"
        httpx_mock.add_response(
            url="http://test-server-v1", json={"predictions": [1, 2]}
        )
        httpx_mock.add_response(
            url="http://test-server-v2",
            json={
                "id": request_id,
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
                "http://test-server-v2",
                data=input_data,
                headers={"Host": "test-server.com"},
                timeout=2,
                is_graph_endpoint=True,
            )
            assert res["outputs"][0]["data"] == [1, 2, 3, 4]
            assert res["id"] == request_id

    # Because no versions of pytest-httpx match >v0.22.0,<0.23.0
    # and pytest-httpx (0.22.0) depends on httpx (==0.24.*), pytest-httpx (>=v0.22.0,<0.23.0) requires httpx (==0.24.*).
    # So, because kserve depends on both httpx (^0.26.0) and pytest_httpx (~v0.22.0), version solving failed.
    @pytest.mark.skip("pytest_httpx requires python >= 3.9")
    @pytest.mark.parametrize(
        "rest_client, protocol",
        [("v1", "v1"), ("v2", "v2"), ("v3", "v3")],
        indirect=["rest_client"],
    )
    async def test_infer_path_based_routing(self, rest_client, protocol, httpx_mock):
        request_id = "2ja0ls9j1309"
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
                input_data = {"instances": [1, 2]}
                await rest_client.explain(
                    "http://test-server",
                    "TestModel",
                    data=input_data,
                    headers={"Host": "test-server.com"},
                    timeout=2,
                )
