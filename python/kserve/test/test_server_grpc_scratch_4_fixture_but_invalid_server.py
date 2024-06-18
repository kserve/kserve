import unittest
import pytest
from typing import List, Union
from concurrent import futures
from concurrent.futures import ThreadPoolExecutor
import threading
import time
import asyncio
import kserve

# Import Kserve
from typing import Dict, Union
from kserve import (Model, ModelServer, model_server, InferInput, InferRequest, InferOutput, InferResponse,
                    InferenceServerClient)
from kserve.utils.utils import generate_uuid


# Assuming ModelServer class is defined somewhere, which includes the gRPC server logic

# Minimal Kserve Model solely to return data to verify secure grpc, data irrelevant
class TestModel(Model):  # Test model
    def __init__(self, name: str):
        super().__init__(name)
        self.name = name
        self.ready = False

    def load(self):
        self.ready = True

    # Returns a number + 1
    def predict(self, payload: InferRequest, headers: Dict[str, str] = None) -> InferResponse:
        req = payload.inputs[0]
        input_number = req.data[0]  # Input should be a single number
        assert isinstance(input_number, (int, float)), "Data is not a number or float"
        result = [float(input_number + 1)]

        response_id = generate_uuid()
        infer_output = InferOutput(name="output-0", shape=[1], datatype="FP32", data=result)
        infer_response = InferResponse(model_name=self.name, infer_outputs=[infer_output], response_id=response_id)
        return infer_response


# Function to run the model server
# regular version
def run_model(secure_grpc_server, server_key, server_cert, ca_cert, models):
    if secure_grpc_server:
        return ModelServer(
            secure_grpc_server=secure_grpc_server,
            server_key=server_key,
            server_cert=server_cert,
            ca_cert=ca_cert
        ).start(models)
    else:
        return ModelServer().start(models)

# async version
# async def run_model(secure_grpc_server, server_key, server_cert, ca_cert, models):
#     if secure_grpc_server:
#         server = ModelServer(
#             secure_grpc_server=secure_grpc_server,
#             server_key=server_key,
#             server_cert=server_cert,
#             ca_cert=ca_cert
#         )
#         await server.start(models)
#         return server
#     else:
#         server = ModelServer()
#         await server.start(models)
#         return server


# gRPC client setup (assuming appropriate stubs are defined)
async def grpc_infer_request(integer: int, port: str, ssl: bool, creds: List, channel_args: any):
    if channel_args:
        client = InferenceServerClient(url=port,
                                       ssl=ssl,
                                       creds=creds,
                                       channel_args=(
                                           # grpc.ssl_target_name_override must be set to match CN used in cert gen
                                           channel_args,)
                                       )
    elif not channel_args or channel_args == []:
        client = InferenceServerClient(url=port,
                                       ssl=ssl,
                                       creds=creds)
    data = float(integer)
    infer_input = InferInput(name="input-0", shape=[1], datatype="FP32", data=[data])
    request = InferRequest(infer_inputs=[infer_input], model_name="test-model")
    res = client.infer(infer_request=request)
    return res


@pytest.fixture(scope="module")
async def start_server():
    # Create server
    server_key = "test"
    server_cert = "test"
    ca_cert = "test"
    models = [TestModel("test-model")]

    # Start the server in a new thread
    server = run_model(False, server_key, server_cert, ca_cert, models)
    server_thread = threading.Thread(target=server.start)
    server_thread.start()

    server.wait_for_server()

    yield

    server.stop()


class TestGrpcSecureServer:
    # @pytest.mark.asyncio
    def test_secure_server_returns(self, start_server):
        # TODO: create certs
        # server_key = "test"
        # server_cert = "test"
        # ca_cert = "test"
        # models = [TestModel("test-model")]
        #
        # # Start the model server in a separate thread or process
        # server = run_model(False, server_key, server_cert, ca_cert, models)
        #
        # # Give the server some time to start
        # asyncio.sleep(25)  # Adjust as necessary

        # Create gRPC channel and stub
        time.sleep(10)
        # grpc_output = asyncio.run(grpc_infer_request(1, "localhost:8081", False, [], []))
        # print(f"grpc_output is: {grpc_output}")
        # assert grpc_output is not None
        max_retries = 10
        retries = 0
        while retries < max_retries:
            try:
                grpc_output = asyncio.run(grpc_infer_request(1, "localhost:8081", False, [], []))
                print(f"grpc_output is: {grpc_output}")
                break
            except Exception as e:
                print(f"Failed to connect to the server. Retrying... {e}")
                retries += 1
                time.sleep(1)  # Wait for 1 second before retrying
        else:
            raise Exception("Maximum retries exceeded. Unable to connect to the server.")
        assert False
