import unittest
from typing import Union, Dict

import pytest

from ..kserve import InferRequest, InferResponse
from ..kserve.model_server import Model, ModelServer
from ..kserve.protocol.grpc.grpc_predict_v2_pb2 import ModelInferRequest


class TestGrpcSecureServer:

    class TestModel(Model):
        def __init__(self, name: str):
            super().__init__(name)
            self.name = name
            self.ready = False
            self.load()

        def load(self):
            self.ready = True
            pass

        def predict(self, payload: Union[Dict, InferRequest, ModelInferRequest],
                      headers: Dict[str, str] = None) -> Union[Dict, InferResponse]:
            req = payload.inputs[0]


    @pytest.fixture



    @pytest.fixture
    def run_model(self, secure_grpc_server, server_key, server_cert, ca_cert, models):
        return ModelServer(
            secure_grpc_server=secure_grpc_server,
            server_key=server_key,
            server_cert=server_cert,
            ca_cert=ca_cert
        ).start([models])


