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

import grpc
import grpc_testing
import numpy as np
import pandas as pd
import pytest
from google.protobuf.json_format import MessageToDict
from unittest.mock import patch

from kserve import Model, ModelServer
from kserve.errors import InvalidInput
from kserve.protocol.grpc import grpc_predict_v2_pb2, servicer
from kserve.protocol.infer_type import serialize_byte_tensor, InferResponse
from kserve.utils.utils import get_predict_response


class DummyModel(Model):
    def __init__(self, name):
        super().__init__(name)
        self.name = name
        self.ready = False

    def load(self):
        self.ready = True

    async def predict(self, request, headers=None):
        outputs = pd.DataFrame(
            {
                "fp32_output": request.get_input_by_name("fp32_input")
                .as_numpy()
                .flatten(),
                "int32_output": request.get_input_by_name("int32_input")
                .as_numpy()
                .flatten(),
                "string_output": request.get_input_by_name("string_input")
                .as_numpy()
                .flatten(),
                "uint8_output": request.get_input_by_name("uint8_input")
                .as_numpy()
                .flatten(),
                "bool_input": request.get_input_by_name("bool_input")
                .as_numpy()
                .flatten(),
            }
        )
        # Fixme: Gets only the 1st element of the input
        # inputs = get_predict_input(request)
        infer_response = get_predict_response(request, outputs, self.name)
        if request.parameters:
            infer_response.parameters = request.parameters
        if request.inputs[0].parameters:
            infer_response.outputs[0].parameters = request.inputs[0].parameters
        return infer_response


class DummyFP16OutputModel(Model):
    def __init__(self, name):
        super().__init__(name)
        self.name = name
        self.ready = False

    def load(self):
        self.ready = True

    async def predict(self, request, headers=None):
        outputs = pd.DataFrame(
            {
                "fp16_output": request.get_input_by_name("fp32_input")
                .as_numpy()
                .astype(np.float16)
                .flatten(),
                "fp32_output": request.get_input_by_name("fp32_input")
                .as_numpy()
                .flatten(),
            }
        )
        # Fixme: Gets only the 1st element of the input
        # inputs = get_predict_input(request)
        infer_response = get_predict_response(request, outputs, self.name)
        if request.parameters:
            infer_response.parameters = request.parameters
        if request.inputs[0].parameters:
            infer_response.outputs[0].parameters = request.inputs[0].parameters
        return infer_response


class DummyFP16InputModel(Model):
    def __init__(self, name):
        super().__init__(name)
        self.name = name
        self.ready = False

    def load(self):
        self.ready = True

    async def predict(self, request, headers=None):
        outputs = pd.DataFrame(
            {
                "int32_output": np.array([1, 2, 3, 4, 5, 6, 7, 8]),
                "fp16_output": request.get_input_by_name("fp16_input")
                .as_numpy()
                .flatten(),
            }
        )
        # Fixme: Gets only the 1st element of the input
        # inputs = get_predict_input(request)
        infer_response = get_predict_response(request, outputs, self.name)
        if request.parameters:
            infer_response.parameters = request.parameters
        if request.inputs[0].parameters:
            infer_response.outputs[0].parameters = request.inputs[0].parameters
        return infer_response


@pytest.fixture(scope="class")
def server():
    server = ModelServer()
    model = DummyModel("TestModel")
    model.load()
    server.register_model(model)
    fp16_output_model = DummyFP16OutputModel("FP16OutputModel")
    fp16_output_model.load()
    server.register_model(fp16_output_model)
    fp16_input_model = DummyFP16InputModel("FP16InputModel")
    fp16_input_model.load()
    server.register_model(fp16_input_model)
    servicers = {
        grpc_predict_v2_pb2.DESCRIPTOR.services_by_name[
            "GRPCInferenceService"
        ]: servicer.InferenceServicer(
            server.dataplane, server.model_repository_extension
        )
    }
    test_server = grpc_testing.server_from_dictionary(
        servicers,
        grpc_testing.strict_real_time(),
    )
    return test_server


@pytest.mark.asyncio
@patch(
    "kserve.protocol.grpc.servicer.to_headers", return_value=[]
)  # To avoid NotImplementedError from trailing_metadata function
async def test_grpc_inputs(mock_to_headers, server):
    request = grpc_predict_v2_pb2.ModelInferRequest(
        model_name="TestModel",
        id="123",
        inputs=[
            {
                "name": "fp32_input",
                "shape": [2, 4],
                "datatype": "FP32",
                "contents": {"fp32_contents": [6.8, 2.8, 4.8, 1.4, 6.0, 3.4, 4.5, 1.6]},
            },
            {
                "name": "int32_input",
                "shape": [2, 4],
                "datatype": "INT32",
                "contents": {"int_contents": [6, 2, 4, 1, 6, 3, 4, 1]},
            },
            {
                "name": "string_input",
                "shape": [8],
                "datatype": "BYTES",
                "contents": {
                    "bytes_contents": [
                        b"Cat",
                        b"Dog",
                        b"Wolf",
                        b"Cat",
                        b"Dog",
                        b"Wolf",
                        b"Dog",
                        b"Wolf",
                    ]
                },
            },
            {
                "name": "uint8_input",
                "shape": [2, 4],
                "datatype": "UINT8",
                "contents": {"uint_contents": [6, 2, 4, 1, 6, 3, 4, 1]},
            },
            {
                "name": "bool_input",
                "shape": [8],
                "datatype": "BOOL",
                "contents": {
                    "bool_contents": [
                        True,
                        False,
                        True,
                        False,
                        True,
                        False,
                        True,
                        False,
                    ]
                },
            },
        ],
    )

    model_infer_method = server.invoke_unary_unary(
        method_descriptor=(
            grpc_predict_v2_pb2.DESCRIPTOR.services_by_name[
                "GRPCInferenceService"
            ].methods_by_name["ModelInfer"]
        ),
        invocation_metadata={},
        request=request,
        timeout=20,
    )

    response, _, code, _ = model_infer_method.termination()
    response = await response
    response_dict = MessageToDict(
        response,
        preserving_proto_field_name=True,
    )
    assert code == grpc.StatusCode.OK
    assert response_dict == {
        "model_name": "TestModel",
        "id": "123",
        "outputs": [
            {
                "name": "fp32_output",
                "datatype": "FP32",
                "shape": ["8"],
                "contents": {"fp32_contents": [6.8, 2.8, 4.8, 1.4, 6.0, 3.4, 4.5, 1.6]},
            },
            {
                "name": "int32_output",
                "datatype": "INT32",
                "shape": ["8"],
                "contents": {"int_contents": [6, 2, 4, 1, 6, 3, 4, 1]},
            },
            {
                "name": "string_output",
                "datatype": "BYTES",
                "shape": ["8"],
                "contents": {
                    "bytes_contents": [
                        "Q2F0",
                        "RG9n",
                        "V29sZg==",
                        "Q2F0",
                        "RG9n",
                        "V29sZg==",
                        "RG9n",
                        "V29sZg==",
                    ]
                },
            },
            {
                "name": "uint8_output",
                "datatype": "UINT8",
                "shape": ["8"],
                "contents": {"uint_contents": [6, 2, 4, 1, 6, 3, 4, 1]},
            },
            {
                "name": "bool_input",
                "datatype": "BOOL",
                "shape": ["8"],
                "contents": {
                    "bool_contents": [
                        True,
                        False,
                        True,
                        False,
                        True,
                        False,
                        True,
                        False,
                    ]
                },
            },
        ],
    }


@pytest.mark.asyncio
@patch(
    "kserve.protocol.grpc.servicer.to_headers", return_value=[]
)  # To avoid NotImplementedError from trailing_metadata function
async def test_grpc_raw_inputs(mock_to_headers, server):
    """
    If we receive raw inputs then, the response also should be in raw output format.
    """
    fp32_data = np.array([6.8, 2.8, 4.8, 1.4, 6.0, 3.4, 4.5, 1.6], dtype=np.float32)
    int32_data = np.array([6, 2, 4, 1, 6, 3, 4, 1], dtype=np.int32)
    str_data = np.array(
        [b"Cat", b"Dog", b"Wolf", b"Cat", b"Dog", b"Wolf", b"Dog", b"Wolf"],
        dtype=np.object_,
    )
    uint8_data = np.array([6, 2, 4, 1, 6, 3, 4, 1], dtype=np.uint8)
    bool_data = np.array(
        [True, False, True, False, True, False, True, False], dtype=np.bool_
    )
    raw_input_contents = [
        fp32_data.tobytes(),
        int32_data.tobytes(),
        serialize_byte_tensor(str_data).item(),
        uint8_data.tobytes(),
        bool_data.tobytes(),
    ]
    request = grpc_predict_v2_pb2.ModelInferRequest(
        model_name="TestModel",
        id="123",
        inputs=[
            {
                "name": "fp32_input",
                "shape": [2, 4],
                "datatype": "FP32",
            },
            {
                "name": "int32_input",
                "shape": [2, 4],
                "datatype": "INT32",
            },
            {
                "name": "string_input",
                "shape": [8],
                "datatype": "BYTES",
            },
            {
                "name": "uint8_input",
                "shape": [2, 4],
                "datatype": "UINT8",
            },
            {
                "name": "bool_input",
                "shape": [8],
                "datatype": "BOOL",
            },
        ],
        raw_input_contents=raw_input_contents,
    )

    model_infer_method = server.invoke_unary_unary(
        method_descriptor=(
            grpc_predict_v2_pb2.DESCRIPTOR.services_by_name[
                "GRPCInferenceService"
            ].methods_by_name["ModelInfer"]
        ),
        invocation_metadata={},
        request=request,
        timeout=20,
    )

    response, _, code, _ = model_infer_method.termination()
    response = await response
    response_dict = MessageToDict(
        response,
        preserving_proto_field_name=True,
    )
    assert code == grpc.StatusCode.OK
    assert response_dict == {
        "model_name": "TestModel",
        "id": "123",
        "outputs": [
            {
                "name": "fp32_output",
                "datatype": "FP32",
                "shape": ["8"],
                "parameters": {"binary_data_size": {"int64_param": "32"}},
            },
            {
                "name": "int32_output",
                "datatype": "INT32",
                "shape": ["8"],
                "parameters": {"binary_data_size": {"int64_param": "32"}},
            },
            {
                "name": "string_output",
                "datatype": "BYTES",
                "shape": ["8"],
                "parameters": {"binary_data_size": {"int64_param": "59"}},
            },
            {
                "name": "uint8_output",
                "datatype": "UINT8",
                "shape": ["8"],
                "parameters": {"binary_data_size": {"int64_param": "8"}},
            },
            {
                "name": "bool_input",
                "datatype": "BOOL",
                "shape": ["8"],
                "parameters": {"binary_data_size": {"int64_param": "8"}},
            },
        ],
        "raw_output_contents": [
            "mpnZQDMzM0CamZlAMzOzPwAAwECamVlAAACQQM3MzD8=",
            "BgAAAAIAAAAEAAAAAQAAAAYAAAADAAAABAAAAAEAAAA=",
            "AwAAAENhdAMAAABEb2cEAAAAV29sZgMAAABDYXQDAAAARG9nBAAAAFdvbGYDAAAARG9nBAAAAFdvbGY=",
            "BgIEAQYDBAE=",
            "AQABAAEAAQA=",
        ],
    }
    infer_response = InferResponse.from_grpc(response)
    assert np.array_equal(infer_response.outputs[0].as_numpy(), fp32_data)
    assert np.array_equal(infer_response.outputs[1].as_numpy(), int32_data)
    assert np.array_equal(infer_response.outputs[2].as_numpy(), str_data)
    assert np.array_equal(infer_response.outputs[3].as_numpy(), uint8_data)
    assert np.array_equal(infer_response.outputs[4].as_numpy(), bool_data)


@pytest.mark.asyncio
@patch(
    "kserve.protocol.grpc.servicer.to_headers", return_value=[]
)  # To avoid NotImplementedError from trailing_metadata function
async def test_grpc_fp16_output(mock_to_headers, server):
    """
    If the output contains FP16 datatype, then the outputs should be returned as raw outputs.
    """
    fp32_data = [6.8, 2.8, 4.8, 1.4, 6.0, 3.4, 4.5, 1.6]
    request = grpc_predict_v2_pb2.ModelInferRequest(
        model_name="FP16OutputModel",
        id="123",
        inputs=[
            {
                "name": "fp32_input",
                "shape": [2, 4],
                "datatype": "FP32",
                "contents": {"fp32_contents": fp32_data},
            },
        ],
    )

    model_infer_method = server.invoke_unary_unary(
        method_descriptor=(
            grpc_predict_v2_pb2.DESCRIPTOR.services_by_name[
                "GRPCInferenceService"
            ].methods_by_name["ModelInfer"]
        ),
        invocation_metadata={},
        request=request,
        timeout=20,
    )

    response, _, code, _ = model_infer_method.termination()
    response = await response
    response_dict = MessageToDict(
        response,
        preserving_proto_field_name=True,
    )
    assert code == grpc.StatusCode.OK
    assert response_dict == {
        "model_name": "FP16OutputModel",
        "id": "123",
        "outputs": [
            {
                "name": "fp16_output",
                "datatype": "FP16",
                "shape": ["8"],
                "parameters": {"binary_data_size": {"int64_param": "16"}},
            },
            {
                "name": "fp32_output",
                "datatype": "FP32",
                "shape": ["8"],
                "parameters": {"binary_data_size": {"int64_param": "32"}},
            },
        ],
        "raw_output_contents": [
            "zUaaQc1Emj0ARs1CgERmPg==",
            "mpnZQDMzM0CamZlAMzOzPwAAwECamVlAAACQQM3MzD8=",
        ],
    }
    infer_response = InferResponse.from_grpc(response)
    assert np.array_equal(
        infer_response.outputs[0].as_numpy(), np.array(fp32_data, dtype=np.float16)
    )
    assert np.array_equal(
        infer_response.outputs[1].as_numpy(), np.array(fp32_data, dtype=np.float32)
    )


@pytest.mark.asyncio
@patch(
    "kserve.protocol.grpc.servicer.to_headers", return_value=[]
)  # To avoid NotImplementedError from trailing_metadata function
async def test_grpc_fp16_input(mock_to_headers, server):
    fp16_data = np.array([6.8, 2.8, 4.8, 1.4, 6.0, 3.4, 4.5, 1.6], dtype=np.float16)
    raw_input_contents = [fp16_data.tobytes()]
    request = grpc_predict_v2_pb2.ModelInferRequest(
        model_name="FP16InputModel",
        id="123",
        inputs=[
            {
                "name": "fp16_input",
                "shape": [2, 4],
                "datatype": "FP16",
            },
        ],
        raw_input_contents=raw_input_contents,
    )

    model_infer_method = server.invoke_unary_unary(
        method_descriptor=(
            grpc_predict_v2_pb2.DESCRIPTOR.services_by_name[
                "GRPCInferenceService"
            ].methods_by_name["ModelInfer"]
        ),
        invocation_metadata={},
        request=request,
        timeout=20,
    )

    response, _, code, _ = model_infer_method.termination()
    response = await response
    response_dict = MessageToDict(
        response,
        preserving_proto_field_name=True,
    )
    assert code == grpc.StatusCode.OK
    assert response_dict == {
        "model_name": "FP16InputModel",
        "id": "123",
        "outputs": [
            {
                "name": "int32_output",
                "datatype": "INT64",
                "shape": ["8"],
                "parameters": {"binary_data_size": {"int64_param": "64"}},
            },
            {
                "name": "fp16_output",
                "datatype": "FP16",
                "shape": ["8"],
                "parameters": {"binary_data_size": {"int64_param": "16"}},
            },
        ],
        "raw_output_contents": [
            "AQAAAAAAAAACAAAAAAAAAAMAAAAAAAAABAAAAAAAAAAFAAAAAAAAAAYAAAAAAAAABwAAAAAAAAAIAAAAAAAAAA==",
            "zUaaQc1Emj0ARs1CgERmPg==",
        ],
    }
    infer_response = InferResponse.from_grpc(response)
    assert np.array_equal(infer_response.outputs[1].as_numpy(), fp16_data)


@pytest.mark.asyncio
@patch(
    "kserve.protocol.grpc.servicer.to_headers", return_value=[]
)  # To avoid NotImplementedError from trailing_metadata function
async def test_grpc_raw_inputs_with_missing_input_data(mock_to_headers, server):
    """
    Server should raise InvalidInput if raw_input_contents missing some input data.
    """
    raw_input_contents = [
        np.array([6.8, 2.8, 4.8, 1.4, 6.0, 3.4, 4.5, 1.6], dtype=np.float32).tobytes(),
        np.array([6, 2, 4, 1, 6, 3, 4, 1], dtype=np.int32).tobytes(),
    ]
    request = grpc_predict_v2_pb2.ModelInferRequest(
        model_name="TestModel",
        id="123",
        inputs=[
            {
                "name": "fp32_input",
                "shape": [2, 4],
                "datatype": "FP32",
            },
            {
                "name": "int32_input",
                "shape": [2, 4],
                "datatype": "INT32",
            },
            {
                "name": "string_input",
                "shape": [8],
                "datatype": "BYTES",
            },
        ],
        raw_input_contents=raw_input_contents,
    )

    model_infer_method = server.invoke_unary_unary(
        method_descriptor=(
            grpc_predict_v2_pb2.DESCRIPTOR.services_by_name[
                "GRPCInferenceService"
            ].methods_by_name["ModelInfer"]
        ),
        invocation_metadata={},
        request=request,
        timeout=20,
    )

    with pytest.raises(InvalidInput):
        response, _, _, _ = model_infer_method.termination()
        _ = await response


@pytest.mark.asyncio
@patch(
    "kserve.protocol.grpc.servicer.to_headers", return_value=[]
)  # To avoid NotImplementedError from trailing_metadata function
async def test_grpc_raw_inputs_with_contents_specified(mock_to_headers, server):
    """
    Server should raise InvalidInput if both contents and raw_input_contents specified.
    """
    raw_input_contents = [
        np.array([6.8, 2.8, 4.8, 1.4, 6.0, 3.4, 4.5, 1.6], dtype=np.float32).tobytes(),
        np.array([6, 2, 4, 1, 6, 3, 4, 1], dtype=np.int32).tobytes(),
        serialize_byte_tensor(
            np.array(
                [b"Cat", b"Dog", b"Wolf", b"Cat", b"Dog", b"Wolf", b"Dog", b"Wolf"],
                dtype=np.object_,
            )
        ).item(),
        np.array([6, 2, 4, 1, 6, 3, 4, 1], dtype=np.uint8).tobytes(),
        np.array(
            [True, False, True, False, True, False, True, False], dtype=np.bool_
        ).tobytes(),
    ]
    request = grpc_predict_v2_pb2.ModelInferRequest(
        model_name="TestModel",
        id="123",
        inputs=[
            {
                "name": "fp32_input",
                "shape": [2, 4],
                "datatype": "FP32",
            },
            {
                "name": "int32_input",
                "shape": [2, 4],
                "datatype": "INT32",
            },
            {
                "name": "string_input",
                "shape": [8],
                "datatype": "BYTES",
            },
            {
                "name": "uint8_input",
                "shape": [2, 4],
                "datatype": "UINT8",
                "contents": {
                    "uint_contents": [6, 2, 4, 1, 6, 3, 4, 1],
                },
            },
            {
                "name": "bool_input",
                "shape": [8],
                "datatype": "BOOL",
            },
        ],
        raw_input_contents=raw_input_contents,
    )

    model_infer_method = server.invoke_unary_unary(
        method_descriptor=(
            grpc_predict_v2_pb2.DESCRIPTOR.services_by_name[
                "GRPCInferenceService"
            ].methods_by_name["ModelInfer"]
        ),
        invocation_metadata={},
        request=request,
        timeout=20,
    )

    with pytest.raises(InvalidInput):
        response, _, _, _ = model_infer_method.termination()
        _ = await response
