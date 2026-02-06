import pytest

import kserve.protocol.grpc.grpc_predict_v2_pb2 as pb
import kserve.protocol.grpc.grpc_predict_v2_pb2_grpc as pb_grpc

def test_proto_module_loads():
    assert pb.DESCRIPTOR is not None
    assert pb.DESCRIPTOR.name == "grpc_predict_v2.proto"

@pytest.mark.parametrize(
    "message_name",
    [
        "ServerLiveRequest",
        "ServerLiveResponse",
        "ServerReadyRequest",
        "ServerReadyResponse",
        "ModelReadyRequest",
        "ModelReadyResponse",
        "ModelInferRequest",
        "ModelInferResponse",
        "ServerMetadataRequest",
        "ServerMetadataResponse",
        "ModelMetadataRequest",
        "ModelMetadataResponse",
        "RepositoryModelLoadRequest",
        "RepositoryModelLoadResponse",
        "RepositoryModelUnloadRequest",
        "RepositoryModelUnloadResponse",
    ],
)
def test_message_classes_exist(message_name):
    assert hasattr(pb, message_name)

def test_model_infer_request_instantiation():
    req = pb.ModelInferRequest(
        model_name="test-model",
        model_version="1",
        id="123",
    )

    assert req.model_name == "test-model"
    assert req.model_version == "1"
    assert req.id == "123"

def test_nested_tensor_messages_exist():
    assert hasattr(pb.ModelInferRequest, "InferInputTensor")
    assert hasattr(pb.ModelInferRequest, "InferRequestedOutputTensor")
    assert hasattr(pb.ModelInferResponse, "InferOutputTensor")

def test_grpc_service_descriptor_exists():
    service = pb.DESCRIPTOR.services_by_name.get("GRPCInferenceService")
    assert service is not None

def test_grpc_service_methods():
    service = pb.DESCRIPTOR.services_by_name["GRPCInferenceService"]

    rpc_names = {method.name for method in service.methods}

    assert rpc_names == {
        "ServerLive",
        "ServerReady",
        "ModelReady",
        "ServerMetadata",
        "ModelMetadata",
        "ModelInfer",
        "RepositoryModelLoad",
        "RepositoryModelUnload",
    }

def test_grpc_stub_creation():
    channel = pytest.importorskip("grpc").insecure_channel("localhost:50051")
    stub = pb_grpc.GRPCInferenceServiceStub(channel)

    assert stub is not None
