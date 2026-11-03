import grpc
import inference_pb2
import inference_pb2_grpc
import management_pb2
import management_pb2_grpc
import argparse


def get_inference_stub(host, port, hostname):
    channel = grpc.insecure_channel(
        host + ":" + str(port),
        options=(
            (
                "grpc.ssl_target_name_override",
                hostname,
            ),
        ),
    )
    stub = inference_pb2_grpc.InferenceAPIsServiceStub(channel)
    return stub


def get_management_stub(host, port, hostname):
    channel = grpc.insecure_channel(
        host + ":" + str(port),
        options=(
            (
                "grpc.ssl_target_name_override",
                hostname,
            ),
        ),
    )
    stub = management_pb2_grpc.ManagementAPIsServiceStub(channel)
    return stub


def infer(stub, model_name, model_input):
    with open(model_input, "rb") as f:
        data = f.read()

    input_data = {"data": data}
    response = stub.Predictions(
        inference_pb2.PredictionsRequest(
            model_name=model_name,
            input=input_data,
        )
    )

    try:
        prediction = response.prediction.decode("utf-8")
        print(prediction)
    except grpc.RpcError:
        exit(1)


def ping(stub):
    response = stub.Ping(inference_pb2.TorchServeHealthResponse())
    try:
        health = response
        print("Ping Response:", health)
    except grpc.RpcError:
        exit(1)


def register(stub, model_name, mar_set_str):
    mar_set = set()
    if mar_set_str:
        mar_set = set(mar_set_str.split(","))
    marfile = f"{model_name}.mar"
    print(
        f"## Check {marfile} in mar_set :",
        mar_set,
    )
    if marfile not in mar_set:
        marfile = "https://torchserve.s3.amazonaws.com/mar_files/{}.mar".format(
            model_name
        )

    print(f"## Register marfile:{marfile}\n")
    params = {
        "url": marfile,
        "initial_workers": 1,
        "synchronous": True,
        "model_name": model_name,
    }
    try:
        stub.RegisterModel(management_pb2.RegisterModelRequest(**params))
        print(f"Model {model_name} registered successfully")
    except grpc.RpcError as e:
        print(f"Failed to register model {model_name}.")
        print(str(e.details()))
        exit(1)


def unregister(stub, model_name):
    try:
        stub.UnregisterModel(
            management_pb2.UnregisterModelRequest(model_name=model_name)
        )
        print(f"Model {model_name} unregistered successfully")
    except grpc.RpcError as e:
        print(f"Failed to unregister model {model_name}.")
        print(str(e.details()))
        exit(1)


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--host",
        help="Ingress Host Name",
        default="localhost",
        type=str,
    )
    parser.add_argument(
        "--port",
        help="Ingress Port",
        default=80,
        type=int,
    )
    parser.add_argument(
        "--hostname",
        help="Service Host Name",
        default="",
        type=str,
    )
    parser.add_argument(
        "--model",
        help="Torchserve Model Name",
        type=str,
    )
    parser.add_argument(
        "--api_name",
        help="API Name",
        default="ping",
        type=str,
    )
    parser.add_argument(
        "--input_path",
        help="Prediction data input path",
        default="mnist.json",
        type=str,
    )

    args = parser.parse_args()
    stub = get_inference_stub(args.host, args.port, args.hostname)
    if args.api_name == "infer":
        infer(stub, args.model, args.input_path)
    elif args.api_name == "ping":
        ping(stub)
    else:
        print("Invalid API name")
        exit(1)
