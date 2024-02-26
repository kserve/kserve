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

import logging

import httpx
import pytest

from kserve import ModelServer, InferenceRESTClient, InferRequest, InferInput
from kserve.errors import UnsupportedProtocol
from kserve.inference_client import RESTConfig
from kserve.model import PredictorProtocol
from kserve.protocol.rest.server import RESTServer
from test.test_server import DummyModel

logging.basicConfig(level=logging.INFO)


@pytest.mark.asyncio
class TestInferenceRESTClient:
    @pytest.fixture(scope="class")
    def rest_client(self):
        model = DummyModel("TestModel")
        model.load()
        not_ready_model = DummyModel("NotReadyModel")
        # model.load()  # Model not loaded, i.e. not ready
        server = ModelServer()
        server.register_model(model)
        server.register_model(not_ready_model)
        rest_server = RESTServer(server.dataplane, server.model_repository_extension)
        app = rest_server.create_application()
        config = RESTConfig(transport=httpx.ASGITransport(app=app), verbose=True)
        return InferenceRESTClient(config=config)

    async def test_is_server_ready(self, rest_client):
        assert await rest_client.is_server_ready("http://test-server/v2/health/ready",
                                                 headers={"Host": "test-server.com"}, timeout=1.3) is True

    async def test_is_server_live(self, rest_client):
        # Open inference protocol / V2
        assert await rest_client.is_server_live("http://test-server/v2/health/live",
                                                protocol_version=PredictorProtocol.REST_V2,
                                                headers={"Host": "test-server.com"}, timeout=1.3) is True
        assert await rest_client.is_server_live("http://test-server/v2/health/live",
                                                protocol_version="v2",
                                                headers={"Host": "test-server.com"}, timeout=1.3) is True
        assert await rest_client.is_server_live("http://test-server/v2/health/live",
                                                protocol_version="V2",
                                                headers={"Host": "test-server.com"}, timeout=1.3) is True
        # V1 protocol
        assert await rest_client.is_server_live("http://test-server/", protocol_version=PredictorProtocol.REST_V1,
                                                headers={"Host": "test-server.com"}, timeout=1.3) is True
        assert await rest_client.is_server_live("http://test-server/",
                                                protocol_version="v1",
                                                headers={"Host": "test-server.com"}, timeout=1.3) is True
        assert await rest_client.is_server_live("http://test-server/",
                                                protocol_version="V1",
                                                headers={"Host": "test-server.com"}, timeout=1.3) is True

        # Unsupported protocol
        with pytest.raises(UnsupportedProtocol, match="Unsupported protocol v3"):
            await rest_client.is_server_live("http://test-server/v2/health/live", protocol_version="v3",
                                             headers={"Host": "test-server.com"}, timeout=1.3)

    async def test_is_model_ready(self, rest_client):
        # Open inference protocol / V2
        assert await rest_client.is_model_ready("http://test-server/v2/models/TestModel/ready",
                                                protocol_version=PredictorProtocol.REST_V2,
                                                headers={"Host": "test-server.com"}, timeout=1.3) is True
        assert await rest_client.is_model_ready("http://test-server/v2/models/TestModel/ready",
                                                protocol_version="v2",
                                                headers={"Host": "test-server.com"}, timeout=1.3) is True
        assert await rest_client.is_model_ready("http://test-server/v2/models/TestModel/ready",
                                                protocol_version="V2",
                                                headers={"Host": "test-server.com"}, timeout=1.3) is True

        with pytest.raises(httpx.HTTPStatusError):
            assert await rest_client.is_model_ready("http://test-server/v2/models/NotReadyModel/ready",
                                                    protocol_version="V2",
                                                    headers={"Host": "test-server.com"}, timeout=1.3) is False
        # V1 protocol
        assert await rest_client.is_model_ready("http://test-server/v1/models/TestModel",
                                                protocol_version=PredictorProtocol.REST_V1,
                                                headers={"Host": "test-server.com"}, timeout=1.3) is True
        assert await rest_client.is_model_ready("http://test-server/v1/models/TestModel",
                                                protocol_version="v1",
                                                headers={"Host": "test-server.com"}, timeout=1.3) is True
        assert await rest_client.is_model_ready("http://test-server/v1/models/TestModel",
                                                protocol_version="V1",
                                                headers={"Host": "test-server.com"}, timeout=1.3) is True
        with pytest.raises(httpx.HTTPStatusError):
            assert await rest_client.is_model_ready("http://test-server/v1/models/NotReadyModel",
                                                    protocol_version="V1",
                                                    headers={"Host": "test-server.com"}, timeout=1.3) is False

        # Unsupported protocol
        with pytest.raises(UnsupportedProtocol, match="Unsupported protocol v3"):
            await rest_client.is_server_live("http://test-server/v2/models/TestModel/ready", protocol_version="v3",
                                             headers={"Host": "test-server.com"}, timeout=1.3)

    async def test_predict(self, rest_client):
        input_data = {"instances": [1, 2]}
        res = await rest_client.predict("http://test-server/v1/models/TestModel:predict", data=input_data,
                                        headers={"Host": "test-server.com"}, timeout=2)
        assert res["predictions"] == [1, 2]

        input_data = {"inputs": [1, 2]}
        res = await rest_client.predict("http://test-server/v1/models/TestModel:predict", data=input_data,
                                        headers={"Host": "test-server.com"}, timeout=2)
        assert res["predictions"] == [1, 2]

    async def test_infer(self, rest_client):
        request_id = "2ja0ls9j1309"
        input_data = {
            "id": request_id,
            "parameters": {
                "test-param": "abc"
            },
            "inputs": [
                {
                    "name": "input-0",
                    "datatype": "INT32",
                    "shape": [2, 2],
                    "data": [[1, 2], [3, 4]],
                    "parameters": {
                        "test-param": "abc"
                    },
                }
            ]
        }
        res = await rest_client.infer("http://test-server/v2/models/TestModel/infer", data=input_data,
                                      headers={"Host": "test-server.com"}, timeout=2)
        assert res.outputs[0].data == [1, 2, 3, 4]
        assert res.id == request_id

        input_data = InferRequest(model_name="TestModel", request_id=request_id,
                                  infer_inputs=[InferInput(name="input-0", datatype="INT32",
                                                           shape=[2, 2], data=[[1, 2], [3, 4]],
                                                           parameters={"test-param": "abc"})],
                                  parameters={"test-param": "abc"})

        res = await rest_client.infer("http://test-server/v2/models/TestModel/infer", data=input_data,
                                      headers={"Host": "test-server.com"}, timeout=2)
        assert res.outputs[0].data == [1, 2, 3, 4]
        assert res.id == request_id

    async def test_explain(self, rest_client):
        input_data = {"instances": [1, 2]}
        res = await rest_client.explain("http://test-server/v1/models/TestModel:explain", data=input_data,
                                        headers={"Host": "test-server.com"}, timeout=2)
        assert res == {"predictions": [1, 2]}

        input_data = {"inputs": [1, 2]}
        res = await rest_client.predict("http://test-server/v1/models/TestModel:explain", data=input_data,
                                        headers={"Host": "test-server.com"}, timeout=2)
        assert res == {"predictions": [1, 2]}
