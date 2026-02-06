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
import json
from unittest.mock import patch, MagicMock, AsyncMock, mock_open, ANY

from kserve import InferenceRESTClient, InferRequest, InferInput, InferenceGRPCClient, InferResponse
from kserve.model_server import app as kserve_app
from kserve.errors import UnsupportedProtocol
from kserve.inference_client import USE_CLIENT_DEFAULT, RESTConfig
from kserve.protocol.infer_type import RequestedOutput
from kserve.protocol.grpc.grpc_predict_v2_pb2 import (
    ServerReadyResponse,
    ServerLiveResponse,
    ModelReadyResponse,
    ServerReadyRequest,
    ServerLiveRequest,
    ModelReadyRequest,
)
import grpc
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

########################################################
# Tests for __init__ of InferenceGRPCClient
########################################################

@pytest.mark.asyncio
@patch("grpc.aio.insecure_channel")
def test_inference_grpc_client_init_insecure_channel(mock_insecure_channel):
    # Arrange
    mock_channel = MagicMock()
    mock_insecure_channel.return_value = mock_channel

    url = "localhost"
    retries = 3
    timeout = 30

    # Act
    client = InferenceGRPCClient(
        url=url,
        use_ssl=False,
        retries=retries,
        timeout=timeout,
    )

    # Assert
    # Port should be auto-appended for insecure channel
    mock_insecure_channel.assert_called_once()
    called_url = mock_insecure_channel.call_args[0][0]
    assert called_url == "localhost:80"

    # Validate channel options
    options = mock_insecure_channel.call_args[1]["options"]
    assert ("grpc.enable_retries", 1) in options

    # Validate retry service config exists
    service_config = [
        opt for opt in options if opt[0] == "grpc.service_config"
    ]
    assert len(service_config) == 1

    config_json = json.loads(service_config[0][1])
    assert config_json["methodConfig"][0]["retryPolicy"]["maxAttempts"] == retries

    # Validate client attributes
    assert client._channel == mock_channel
    assert client._timeout == timeout
    assert client._verbose is False
    assert client._client_stub is not None


@patch("grpc.aio.secure_channel")
def test_inference_grpc_client_init_secure_channel(mock_secure_channel):
    # Arrange
    mock_channel = MagicMock()
    mock_secure_channel.return_value = mock_channel

    url = "example.com"

    # Act
    client = InferenceGRPCClient(
        url=url,
        use_ssl=True,
    )

    # Assert
    # Port should be auto-appended for secure channel
    mock_secure_channel.assert_called_once()
    called_url = mock_secure_channel.call_args[0][0]
    assert called_url == "example.com:443"

    assert client._channel == mock_channel
    assert client._client_stub is not None

@patch("grpc.aio.insecure_channel")
def test_init_with_channel_args_adds_retry_and_service_config(
    mock_insecure_channel,
):
    # Arrange
    mock_channel = MagicMock()
    mock_insecure_channel.return_value = mock_channel

    # channel_args provided WITHOUT grpc.enable_retries and grpc.service_config
    channel_args = [
        ("grpc.max_send_message_length", -1),
    ]

    retries = 3

    # Act
    InferenceGRPCClient(
        url="localhost",
        use_ssl=False,
        channel_args=channel_args,
        retries=retries,
    )

    # Assert
    mock_insecure_channel.assert_called_once()

    # Extract final channel options passed to gRPC
    passed_options = mock_insecure_channel.call_args[1]["options"]

    # Original option should remain
    assert ("grpc.max_send_message_length", -1) in passed_options

    # grpc.enable_retries should be added
    assert ("grpc.enable_retries", 1) in passed_options

    # grpc.service_config should be added
    service_configs = [
        opt for opt in passed_options if opt[0] == "grpc.service_config"
    ]
    assert len(service_configs) == 1

    # Validate retry count inside service config
    service_config_json = json.loads(service_configs[0][1])
    retry_policy = service_config_json["methodConfig"][0]["retryPolicy"]
    assert retry_policy["maxAttempts"] == retries

def test_init_channel_args_service_config_exists():
    channel_args = [("grpc.service_config", "{}")]
    
    with patch("grpc.aio.secure_channel") as mock_secure_channel:
        mock_creds = MagicMock(spec=grpc.ChannelCredentials)
        client = InferenceGRPCClient(
            url="localhost",
            creds=mock_creds,
            channel_args=channel_args,
        )
    
        # Assert grpc.aio.secure_channel called with modified options
        mock_secure_channel.assert_called_once()
        assert ("grpc.enable_retries", 1) in mock_secure_channel.call_args.kwargs["options"]
        # service_config should not be appended again
        assert sum(1 for k, _ in mock_secure_channel.call_args.kwargs["options"] if k=="grpc.service_config") == 1

def test_init_uses_secure_channel_with_creds():
    with patch("grpc.aio.secure_channel") as mock_secure_channel:
        mock_creds = MagicMock(spec=grpc.ChannelCredentials)
        client = InferenceGRPCClient(url="localhost", creds=mock_creds)

        # Use ANY for the 'options' argument
        mock_secure_channel.assert_called_once_with(
            "localhost:80",
            mock_creds,
            options=ANY
        )


def test_init_reads_cert_key_chain_files():
    m_open = mock_open(read_data=b"dummy-bytes")
    with patch("builtins.open", m_open), patch("grpc.ssl_channel_credentials") as mock_ssl_creds, patch("grpc.aio.secure_channel") as mock_secure_channel:
        client = InferenceGRPCClient(
            url="localhost",
            use_ssl=True,
            root_certificates="/fake/root.crt",
            private_key="/fake/private.key",
            certificate_chain="/fake/chain.crt"
        )

        # Assert open called for each file
        m_open.assert_any_call("/fake/root.crt", "rb")
        m_open.assert_any_call("/fake/private.key", "rb")
        m_open.assert_any_call("/fake/chain.crt", "rb")

        # Assert grpc.ssl_channel_credentials called with file contents
        mock_ssl_creds.assert_called_once_with(
            root_certificates=b"dummy-bytes",
            private_key=b"dummy-bytes",
            certificate_chain=b"dummy-bytes",
        )

        # Assert secure_channel called with ssl creds
        mock_secure_channel.assert_called_once()

################################################

@pytest.mark.asyncio
async def test_aenter_returns_self():
    # Arrange
    client = InferenceGRPCClient(url="localhost")
    client._channel = MagicMock()
    client._channel.close = AsyncMock()

    # Act
    result = await client.__aenter__()

    # Assert
    assert result is client


@pytest.mark.asyncio
async def test_close_closes_channel():
    # Arrange
    client = InferenceGRPCClient(url="localhost")
    client._channel = MagicMock()
    client._channel.close = AsyncMock()

    # Act
    await client.close()

    # Assert
    client._channel.close.assert_awaited_once()


@pytest.mark.asyncio
async def test_aexit_calls_close():
    # Arrange
    client = InferenceGRPCClient(url="localhost")
    client._channel = MagicMock()
    client._channel.close = AsyncMock()

    # Act
    await client.__aexit__(None, None, None)

    # Assert
    client._channel.close.assert_awaited_once()

#########################################################
# Tests for infer method
##########################################################

@pytest.mark.asyncio
@patch("grpc.aio.insecure_channel")
async def test_infer_success(mock_insecure_channel):
    # Arrange
    mock_channel = MagicMock()
    mock_insecure_channel.return_value = mock_channel

    client = InferenceGRPCClient(
        url="localhost",
        timeout=30,
    )

    # Mock InferRequest
    mock_infer_request = MagicMock(spec=InferRequest)
    grpc_request = MagicMock()
    mock_infer_request.to_grpc.return_value = grpc_request

    # Mock gRPC stub call
    grpc_response = MagicMock()
    client._client_stub.ModelInfer = AsyncMock(return_value=grpc_response)

    # Mock InferResponse conversion
    expected_response = MagicMock(spec=InferResponse)
    with patch.object(
        InferResponse, "from_grpc", return_value=expected_response
    ) as mock_from_grpc:

        # Act
        response = await client.infer(
            infer_request=mock_infer_request,
            timeout=USE_CLIENT_DEFAULT,
            headers=(("x-test", "true"),),
        )

    # Assert
    mock_infer_request.to_grpc.assert_called_once()

    client._client_stub.ModelInfer.assert_awaited_once_with(
        request=grpc_request,
        metadata=(("x-test", "true"),),
        timeout=30,  # client default timeout used
    )

    mock_from_grpc.assert_called_once_with(grpc_response)
    assert response is expected_response

@pytest.mark.asyncio
@patch("grpc.aio.insecure_channel")
async def test_infer_invalid_input_raises(mock_insecure_channel):
    mock_insecure_channel.return_value = MagicMock()
    client = InferenceGRPCClient(url="localhost")

    with pytest.raises(Exception):
        await client.infer(infer_request={"not": "valid"})

@pytest.mark.asyncio
@patch("grpc.aio.insecure_channel")
async def test_infer_propagates_grpc_error(mock_insecure_channel):
    mock_insecure_channel.return_value = MagicMock()
    client = InferenceGRPCClient(url="localhost")

    mock_infer_request = MagicMock(spec=InferRequest)
    mock_infer_request.to_grpc.return_value = MagicMock()

    client._client_stub.ModelInfer = AsyncMock(
        side_effect=grpc.RpcError("boom")
    )

    with pytest.raises(grpc.RpcError):
        await client.infer(mock_infer_request)

@pytest.mark.asyncio
@patch("grpc.aio.insecure_channel")
async def test_is_server_ready_true(mock_insecure_channel):
    # Arrange
    mock_insecure_channel.return_value = MagicMock()
    client = InferenceGRPCClient(url="localhost")

    grpc_response = ServerReadyResponse(ready=True)
    client._client_stub.ServerReady = AsyncMock(return_value=grpc_response)

    # Act
    result = await client.is_server_ready()

    # Assert
    assert result is True
    client._client_stub.ServerReady.assert_awaited_once()

@pytest.mark.asyncio
@patch("grpc.aio.insecure_channel")
async def test_is_server_ready_false(mock_insecure_channel):
    # Arrange
    mock_insecure_channel.return_value = MagicMock()
    client = InferenceGRPCClient(url="localhost")

    grpc_response = ServerReadyResponse(ready=False)
    client._client_stub.ServerReady = AsyncMock(return_value=grpc_response)

    # Act
    result = await client.is_server_ready()

    # Assert
    assert result is False
    client._client_stub.ServerReady.assert_awaited_once()

@pytest.mark.asyncio
@patch("grpc.aio.insecure_channel")
async def test_is_server_ready_propagates_grpc_error(mock_insecure_channel):
    # Arrange
    mock_insecure_channel.return_value = MagicMock()
    client = InferenceGRPCClient(url="localhost")

    client._client_stub.ServerReady = AsyncMock(
        side_effect=grpc.RpcError("boom")
    )

    # Act + Assert
    with pytest.raises(grpc.RpcError):
        await client.is_server_ready()

    client._client_stub.ServerReady.assert_awaited_once()


##################################################
# Tests for is_server_live method
##################################################
@pytest.mark.asyncio
@patch("grpc.aio.insecure_channel")
async def test_is_server_live_true(mock_insecure_channel):
    # Arrange
    mock_insecure_channel.return_value = MagicMock()
    client = InferenceGRPCClient(url="localhost")

    grpc_response = ServerLiveResponse(live=True)
    client._client_stub.ServerLive = AsyncMock(return_value=grpc_response)

    # Act
    result = await client.is_server_live()

    # Assert
    assert result is True
    client._client_stub.ServerLive.assert_awaited_once()

@pytest.mark.asyncio
@patch("grpc.aio.insecure_channel")
async def test_is_server_live_false(mock_insecure_channel):
    # Arrange
    mock_insecure_channel.return_value = MagicMock()
    client = InferenceGRPCClient(url="localhost")

    grpc_response = ServerLiveResponse(live=False)
    client._client_stub.ServerLive = AsyncMock(return_value=grpc_response)

    # Act
    result = await client.is_server_live()

    # Assert
    assert result is False
    client._client_stub.ServerLive.assert_awaited_once()

@pytest.mark.asyncio
@patch("grpc.aio.insecure_channel")
async def test_is_server_live_propagates_grpc_error(mock_insecure_channel):
    # Arrange
    mock_insecure_channel.return_value = MagicMock()
    client = InferenceGRPCClient(url="localhost")

    client._client_stub.ServerLive = AsyncMock(
        side_effect=grpc.RpcError("boom")
    )

    # Act + Assert
    with pytest.raises(grpc.RpcError):
        await client.is_server_live()

    client._client_stub.ServerLive.assert_awaited_once()

##################################################
# Tests for is_model_ready method  
##################################################
@pytest.mark.asyncio
@patch("grpc.aio.insecure_channel")
async def test_is_model_ready_true(mock_insecure_channel):
    # Arrange
    mock_insecure_channel.return_value = MagicMock()
    client = InferenceGRPCClient(url="localhost")

    grpc_response = ModelReadyResponse(ready=True)
    client._client_stub.ModelReady = AsyncMock(return_value=grpc_response)

    # Act
    result = await client.is_model_ready(model_name="test-model")

    # Assert
    assert result is True
    client._client_stub.ModelReady.assert_awaited_once()

    # Optional: verify correct request content
    args, kwargs = client._client_stub.ModelReady.call_args
    assert isinstance(args[0], ModelReadyRequest)
    assert args[0].name == "test-model"

@pytest.mark.asyncio
@patch("grpc.aio.insecure_channel")
async def test_is_model_ready_false(mock_insecure_channel):
    # Arrange
    mock_insecure_channel.return_value = MagicMock()
    client = InferenceGRPCClient(url="localhost")

    grpc_response = ModelReadyResponse(ready=False)
    client._client_stub.ModelReady = AsyncMock(return_value=grpc_response)

    # Act
    result = await client.is_model_ready(model_name="test-model")

    # Assert
    assert result is False
    client._client_stub.ModelReady.assert_awaited_once()

@pytest.mark.asyncio
@patch("grpc.aio.insecure_channel")
async def test_is_model_ready_propagates_grpc_error(mock_insecure_channel):
    # Arrange
    mock_insecure_channel.return_value = MagicMock()
    client = InferenceGRPCClient(url="localhost")

    client._client_stub.ModelReady = AsyncMock(
        side_effect=grpc.RpcError("boom")
    )

    # Act + Assert
    with pytest.raises(grpc.RpcError):
        await client.is_model_ready(model_name="missing-model")

    client._client_stub.ModelReady.assert_awaited_once()

