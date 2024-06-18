import unittest
import pytest
from typing import List, Union
from concurrent import futures
from concurrent.futures import ThreadPoolExecutor
import time
import asyncio
import kserve

# Import Kserve
from typing import Dict, Union
from kserve import (Model, ModelServer, model_server, InferInput, InferRequest, InferOutput, InferResponse,
                    InferenceServerClient)
from kserve.utils.utils import generate_uuid


# from ..kserve import InferRequest, InferResponse
# from ..kserve.model_server import Model, ModelServer
# from ..kserve.protocol.grpc.grpc_predict_v2_pb2 import ModelInferRequest


async def run_model(secure_grpc_server, server_key, server_cert, ca_cert, models):
    # asyncio.set_event_loop(asyncio.new_event_loop())  # Create a new event loop for this thread
    if secure_grpc_server:
        server = await ModelServer(
            secure_grpc_server=secure_grpc_server,
            server_key=server_key,
            server_cert=server_cert,
            ca_cert=ca_cert
        ).start(models)
    else:
        server = await ModelServer().start(models)
    print("Starting the model server...")
    # loop.run_until_complete(server.start(models))
    return server


async def run_model_loop(secure_grpc_server, server_key, server_cert, ca_cert, models):
    loop = asyncio.new_event_loop()
    asyncio.set_event_loop(loop)
    try:
        return loop.run_until_complete(await run_model(secure_grpc_server, server_key, server_cert, ca_cert, models))
    except Exception as e:
        print(f"Caught exception in run_model_loop: {e}")
    finally:
        loop.close()


# Minimal Kserve Model solely to return data to verify secure grpc, data irrelevant
class TestModel(kserve.Model):  # Test model
    def __init__(self, name: str):
        super().__init__(name)
        self.name = name
        self.ready = False

    def load(self):
        self.ready = True
        pass

    # Returns a number + 1
    def predict(self, payload: InferRequest, headers: Dict[str, str] = None) -> InferResponse:
        req = payload.inputs[0]
        print(req)
        input_number = req.data[0]  # Input should be a single number
        assert isinstance(input_number, (int, float)), "Data is not a number or float"
        result = [float(input_number + 1)]

        response_id = generate_uuid()
        infer_output = InferOutput(name="output-0", shape=[1], datatype="FP32", data=result)
        infer_response = InferResponse(model_name=self.name, infer_outputs=[infer_output], response_id=response_id)
        return infer_response


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


@pytest.mark.asyncio
class TestGrpcSecureServer:
    '''
    async def test_secure_server_returns(self):
        # TODO: create certs
        server_key = "test"
        server_cert = "test"
        ca_cert = "test"
        models = [TestModel("test-model")]

        # test_model = run_model(False, server_key, server_cert, ca_cert, models)
        # run_model(True, server_key, server_cert, ca_cert, models)

        server_future = None
        print("Server future about to start...")
        try:
            with ThreadPoolExecutor() as executor:
                server_future = executor.submit(run_model_loop, False, server_key, server_cert, ca_cert, models)
                print("Server future started. Waiting for server to be ready...")
                await asyncio.sleep(5)
                print("After sleep, attempting to connect to the gRPC server...")
        except Exception as e:
            print(f"Caught error launching server: {e}")

        # try:
        #     print("Creating server task...")
        #     server_task = asyncio.create_task(run_model(False, server_key, server_cert, ca_cert, models))
        #     print("Server task created. Waiting for server to be ready...")
        #     await asyncio.sleep(5)
        #     print("After sleep, attempting to connect to the gRPC server...")
        # except Exception as e:
        #     print(f"Caught error launching server: {e}")
        #     raise

        try:
            # TODO: ping the model endpoint
            grpc_output = await grpc_infer_request(1, "http://0.0.0.0:8081", False, [], [])
            print(f"grpc_output is: {grpc_output}")
            # TODO: test that outputs are as desired
        except Exception as e:
            print(f"Caught error creating channel or making request: {e}")
        finally:
            if not server_future:
                print(f"server_future is None")
                return
            print(f"starting finally block - server_future is {server_future}")
            if server_future.done():
                if server_future.exception():
                    print(f"Server future raised an exception: {server_future.exception()}")
                else:
                    print("Server future completed without exception")
            if server_future.running():
                print(f"server_future in stop block - server_future is: {server_future}")
                server_future.result().stop()
            executor.shutdown(wait=True)

        # print("Starting finally block")
        # if server_task.done():
        #     if server_task.exception():
        #         print(f"Server task raised an exception: {server_task.exception()}")
        #     else:
        #         print("Server task completed without exception")
        # else:
        #     print("Shutting down the server...")
        #     server = await server_task
        #     server.stop()
        # print("Server shut down.")
        assert False
    '''

    async def test_secure_server_returns(self):
        server_key = "test"
        server_cert = "test"
        ca_cert = "test"
        models = [TestModel("test-model")]

        async def function1():
            try:
                print("Creating server task...")
                server = await run_model_loop(False, server_key, server_cert, ca_cert, models)
                print("Server started. Waiting for server to be ready...")
                await asyncio.sleep(5)  # Simulate waiting for the server to start
                print("After sleep, attempting to connect to the gRPC server...")
            except Exception as e:
                print(f"Caught error launching server: {e}")
                raise

        async def function2():
            try:
                print("Making gRPC request...")
                # TODO: Make gRPC request
                grpc_output = await grpc_infer_request(1, "http://0.0.0.0:8081", False, [], [])
                print(f"grpc_output is: {grpc_output}")
                # TODO: Test gRPC response
                pass
            except Exception as e:
                print(f"Caught error making gRPC request: {e}")
                raise

        task1 = await asyncio.create_task(function1())
        task2 = await asyncio.create_task(function2())

        await asyncio.gather(task1, task2)

