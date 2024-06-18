from typing import List
from kserve import (Model, ModelServer, model_server, InferInput, InferRequest, InferOutput, InferResponse,
                    InferenceServerClient)
import asyncio


# gRPC client setup
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


async def main():
    print("starting main")
    output = await grpc_infer_request(1, "localhost:8081", False, [], [])
    print(f"output is: {output}")
    return output

if __name__ == "__main__":
    asyncio.run(main())
