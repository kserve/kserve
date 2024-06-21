import pytest
from typing import List, Union
import asyncio
import kserve
import multiprocessing
import grpc
import os

# Import Kserve
from typing import Dict, Union
from kserve import (Model, ModelServer, model_server, InferInput, InferRequest, InferOutput, InferResponse,
                    InferenceServerClient)
from kserve.utils.utils import generate_uuid


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
async def run_model(secure_grpc_server, models):
    if secure_grpc_server:
        server_key = open("./test/kserve_test_certs/server-key.pem", "rb").read()
        server_cert = open("./test/kserve_test_certs/server-cert.pem", "rb").read()
        ca_cert = open("./test/kserve_test_certs/ca-cert.pem", "rb").read()
        server = ModelServer(
            secure_grpc_server=secure_grpc_server,
            server_key=server_key,
            server_cert=server_cert,
            ca_cert=ca_cert
        )
        await server.start(models)
    else:
        server = ModelServer()
        await server.start(models)


async def grpc_infer_request(ssl: bool, port: str, integer: int, queue: multiprocessing.Queue):
    await asyncio.sleep(1)
    if ssl:
        channel_args = ('grpc.ssl_target_name_override', 'localhost')
        client_key = open("./test/kserve_test_certs/client-key.pem", "rb").read()
        client_cert = open("./test/kserve_test_certs/client-cert.pem", "rb").read()
        ca_cert = open("./test/kserve_test_certs/ca-cert.pem", "rb").read()

        channel_creds = grpc.ssl_channel_credentials(
            root_certificates=ca_cert, private_key=client_key, certificate_chain=client_cert
        )
        creds = channel_creds
        client = InferenceServerClient(url=port,
                                       ssl=ssl,
                                       creds=creds,
                                       channel_args=(
                                           # grpc.ssl_target_name_override must be set to match CN used in cert gen
                                           channel_args,)
                                       )
    else:  # not ssl
        client = InferenceServerClient(url=port,
                                       ssl=ssl,
                                       )
    data = float(integer)
    infer_input = InferInput(name="input-0", shape=[1], datatype="FP32", data=[data])
    request = InferRequest(infer_inputs=[infer_input], model_name="test-model")
    res = client.infer(infer_request=request)
    print(f"res is: {res}")
    queue.put(res.outputs[0].contents.fp32_contents[0])


def run_model_sync(secure_grpc_server, models):
    asyncio.run(run_model(secure_grpc_server, models))


def grpc_infer_request_sync(ssl: bool, port: str, integer: int, queue: multiprocessing.Queue):
    return asyncio.run(grpc_infer_request(ssl, port, integer, queue))


class TestGrpcSecureServer:
    def test_insecure_grpc_server_returns(self):
        models = [TestModel("test-model")]

        queue = multiprocessing.Queue()

        server_process = multiprocessing.Process(target=run_model_sync,
                                                 args=(False, models))
        client_process = multiprocessing.Process(target=grpc_infer_request_sync,
                                                 args=(False, "localhost:8081", 1, queue))

        server_process.start()
        client_process.start()

        client_process.join()

        output = queue.get()

        server_process.terminate()
        assert output == 2.0

    def test_secure_grpc_server_returns(self):
        models = [TestModel("test-model")]

        queue = multiprocessing.Queue()

        server_process = multiprocessing.Process(target=run_model_sync,
                                                 args=(True, models))
        client_process = multiprocessing.Process(target=grpc_infer_request_sync,
                                                 args=(True, "localhost:8081", 1, queue))

        server_process.start()
        client_process.start()

        client_process.join()

        output = queue.get()

        server_process.terminate()
        assert output == 2.0

    def test_secure_grpc_server_insecure_client_fails(self):
        models = [TestModel("test-model")]

        queue = multiprocessing.Queue()

        server_process = multiprocessing.Process(target=run_model_sync,
                                                 args=(True, models))
        client_process = multiprocessing.Process(target=grpc_infer_request_sync,
                                                 args=(False, "localhost:8081", 1, queue))

        server_process.start()
        client_process.start()

        client_process.join(timeout=5)

        if not queue.empty():
            output = queue.get()
        else:
            output = None

        server_process.terminate()
        assert output is None

    def test_insecure_grpc_server_secure_client_fails(self):
        models = [TestModel("test-model")]

        queue = multiprocessing.Queue()

        server_process = multiprocessing.Process(target=run_model_sync,
                                                 args=(False, models))
        client_process = multiprocessing.Process(target=grpc_infer_request_sync,
                                                 args=(True, "localhost:8081", 1, queue))

        server_process.start()
        client_process.start()

        client_process.join(timeout=5)

        if not queue.empty():
            output = queue.get()
        else:
            output = None

        server_process.terminate()
        assert output is None
