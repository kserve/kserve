import unittest
import pytest
import pytest_asyncio
from typing import List, Union
from concurrent import futures
from concurrent.futures import ThreadPoolExecutor
import threading
import time
import asyncio
import kserve
import multiprocessing

# Import Kserve
from typing import Dict, Union
from kserve import (Model, ModelServer, model_server, InferInput, InferRequest, InferOutput, InferResponse,
                    InferenceServerClient)
from kserve.utils.utils import generate_uuid


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
async def run_model(secure_grpc_server, server_key, server_cert, ca_cert, models):
    if secure_grpc_server:
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


async def grpc_infer_request(integer: int, port: str, ssl: bool, creds: List, channel_args: any, queue: multiprocessing.Queue):
    await asyncio.sleep(1.5)
    print("After sleep")
    if channel_args:
        client = InferenceServerClient(url=port,
                                       ssl=ssl,
                                       creds=creds,
                                       channel_args=(
                                           # grpc.ssl_target_name_override must be set to match CN used in cert gen
                                           channel_args,)
                                       )
    else:  # not channel_args or channel_args == []:
        client = InferenceServerClient(url=port,
                                       ssl=ssl,
                                       creds=creds)
    data = float(integer)
    infer_input = InferInput(name="input-0", shape=[1], datatype="FP32", data=[data])
    request = InferRequest(infer_inputs=[infer_input], model_name="test-model")
    res = client.infer(infer_request=request)
    # assert res.outputs[0].contents.fp32_contents[0] == 2.0
    queue.put(res.outputs[0].contents.fp32_contents[0])


def run_model_sync(secure_grpc_server, server_key, server_cert, ca_cert, models):
    asyncio.run(run_model(secure_grpc_server, server_key, server_cert, ca_cert, models))


def grpc_infer_request_sync(integer: int, port: str, ssl: bool, creds: List, channel_args: any, queue: multiprocessing.Queue):
    return asyncio.run(grpc_infer_request(integer, port, ssl, creds, channel_args, queue))


def main():
    # TODO: create better certs
    server_key = "test"
    server_cert = "test"
    ca_cert = "test"
    models = [TestModel("test-model")]

    queue = multiprocessing.Queue()

    server_process = multiprocessing.Process(target=run_model_sync, args=(False, server_key, server_cert, ca_cert, models))
    client_process = multiprocessing.Process(target=grpc_infer_request_sync, args=(1, "localhost:8081", False, [], [], queue))

    server_process.start()
    client_process.start()

    client_process.join()

    output = queue.get()
    print(f"output value from queue is: {output}")

    server_process.terminate()


if __name__ == "__main__":
    main()
    # asyncio.run(main())
    # server_key = "test"
    # server_cert = "test"
    # ca_cert = "test"
    # models = [TestModel("test-model")]
    #
    # pool = multiprocessing.Pool()
    # service = pool.apply_async(run_model, args=(False, server_key, server_cert, ca_cert, models))
