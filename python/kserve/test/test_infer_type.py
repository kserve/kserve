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
import copy
import json

import numpy as np
import pytest
from orjson import orjson

from kserve import InferRequest, InferInput, InferResponse, InferOutput
from kserve.errors import InvalidInput
from kserve.protocol.grpc.grpc_predict_v2_pb2 import (
    ModelInferRequest,
    InferParameter,
    ModelInferResponse,
)
from kserve.protocol.infer_type import (
    serialize_byte_tensor,
    _contains_fp16_datatype,
    RequestedOutput,
)
from kserve.protocol.rest.v2_datamodels import (
    InferenceRequest,
    RequestInput,
    RequestOutput,
)


class TestInferRequest:
    def test_to_grpc(self):
        infer_req = InferRequest(
            model_name="TestModel",
            model_version="v1",
            request_id="123",
            parameters={"test-str": "dummy", "test-bool": True, "test-int": 100},
            infer_inputs=[
                InferInput(
                    name="input-0",
                    datatype="INT32",
                    shape=[1, 2],
                    data=[1, 2],
                    parameters={
                        "test-str": "dummy",
                        "test-bool": True,
                        "test-int": 100,
                    },
                )
            ],
            request_outputs=[
                RequestedOutput(
                    name="output-0",
                    parameters={
                        "test-str": "dummy",
                        "test-bool": True,
                        "test-int": 100,
                    },
                ),
                RequestedOutput(
                    name="output-1",
                    parameters={
                        "test-str": "dummy",
                        "test-bool": True,
                        "test-int": 100,
                    },
                ),
            ],
        )
        expected = ModelInferRequest(
            model_name="TestModel",
            model_version="v1",
            id="123",
            parameters={
                "test-str": InferParameter(string_param="dummy"),
                "test-bool": InferParameter(bool_param=True),
                "test-int": InferParameter(int64_param=100),
            },
            inputs=[
                {
                    "name": "input-0",
                    "shape": [1, 2],
                    "datatype": "INT32",
                    "contents": {"int_contents": [1, 2]},
                    "parameters": {
                        "test-str": InferParameter(string_param="dummy"),
                        "test-bool": InferParameter(bool_param=True),
                        "test-int": InferParameter(int64_param=100),
                    },
                }
            ],
            outputs=[
                {
                    "name": "output-0",
                    "parameters": {
                        "test-str": InferParameter(string_param="dummy"),
                        "test-bool": InferParameter(bool_param=True),
                        "test-int": InferParameter(int64_param=100),
                    },
                },
                {
                    "name": "output-1",
                    "parameters": {
                        "test-str": InferParameter(string_param="dummy"),
                        "test-bool": InferParameter(bool_param=True),
                        "test-int": InferParameter(int64_param=100),
                    },
                },
            ],
        )
        res = infer_req.to_grpc()
        assert res == expected

    def test_from_grpc(self):
        infer_req = ModelInferRequest(
            model_name="TestModel",
            id="123",
            parameters={
                "test-str": InferParameter(string_param="dummy"),
                "test-bool": InferParameter(bool_param=True),
                "test-int": InferParameter(int64_param=100),
            },
            inputs=[
                {
                    "name": "input-0",
                    "shape": [1, 2],
                    "datatype": "INT32",
                    "contents": {"int_contents": [1, 2]},
                    "parameters": {
                        "test-str": InferParameter(string_param="dummy"),
                        "test-bool": InferParameter(bool_param=True),
                        "test-int": InferParameter(int64_param=100),
                    },
                }
            ],
            outputs=[
                {
                    "name": "output-0",
                    "parameters": {
                        "test-str": InferParameter(string_param="dummy"),
                        "test-bool": InferParameter(bool_param=True),
                        "test-int": InferParameter(int64_param=100),
                    },
                },
                {
                    "name": "output-1",
                    "parameters": {
                        "test-str": InferParameter(string_param="dummy"),
                        "test-bool": InferParameter(bool_param=True),
                        "test-int": InferParameter(int64_param=100),
                    },
                },
            ],
        )
        expected = InferRequest(
            model_name="TestModel",
            request_id="123",
            parameters={
                "test-str": InferParameter(string_param="dummy"),
                "test-bool": InferParameter(bool_param=True),
                "test-int": InferParameter(int64_param=100),
            },
            infer_inputs=[
                InferInput(
                    name="input-0",
                    datatype="INT32",
                    shape=[1, 2],
                    data=[1, 2],
                    parameters={
                        "test-str": "dummy",
                        "test-bool": True,
                        "test-int": 100,
                    },
                )
            ],
            request_outputs=[
                RequestedOutput(
                    name="output-0",
                    parameters={
                        "test-str": "dummy",
                        "test-bool": True,
                        "test-int": 100,
                    },
                ),
                RequestedOutput(
                    name="output-1",
                    parameters={
                        "test-str": "dummy",
                        "test-bool": True,
                        "test-int": 100,
                    },
                ),
            ],
            from_grpc=True,
        )
        res = InferRequest.from_grpc(infer_req)
        assert res == expected

    def test_to_rest_with_mixed_binary_data(self):
        infer_input_1 = InferInput(
            name="input1",
            shape=[3],
            datatype="INT32",
            data=np.array([1, 2, 3], dtype=np.int32),
            parameters={
                "test-str": "dummy",
            },
        )
        infer_input_2 = InferInput(
            name="input2",
            shape=[1],
            datatype="BYTES",
            data=None,
            parameters={
                "test-int": 2,
            },
        )
        infer_input_2.set_data_from_numpy(
            np.array(["test"], dtype=np.object_), binary_data=True
        )
        infer_input_3 = InferInput(
            name="input3",
            shape=[3],
            datatype="FP16",
            data=None,
        )
        infer_input_3.set_data_from_numpy(
            np.array([1.2, 2.2, 3.2], dtype=np.float16), binary_data=True
        )
        infer_request = InferRequest(
            request_id="4be4e82f-5500-420a-a5c5-ac86841e271b",
            model_name="test_model",
            infer_inputs=[infer_input_1, infer_input_2, infer_input_3],
            request_outputs=[
                RequestedOutput(
                    name="output-0",
                    parameters={
                        "test-str": "dummy",
                        "test-bool": True,
                        "test-int": 100,
                    },
                ),
                RequestedOutput(
                    name="output-1",
                    parameters={
                        "test-str": "dummy",
                        "test-bool": True,
                        "test-int": 100,
                    },
                ),
            ],
        )
        infer_request_bytes, json_length = infer_request.to_rest()
        assert isinstance(infer_request_bytes, bytes)
        assert (
            infer_request_bytes
            == b'{"id":"4be4e82f-5500-420a-a5c5-ac86841e271b","model_name":"test_model","inputs":[{"name":"input1","shape":[3],"datatype":"INT32","parameters":{"test-str":"dummy"},"data":[1,2,3]},{"name":"input2","shape":[1],"datatype":"BYTES","parameters":{"test-int":2,"binary_data_size":8}},{"name":"input3","shape":[3],"datatype":"FP16","parameters":{"binary_data_size":6}}],"outputs":[{"name":"output-0","parameters":{"test-str":"dummy","test-bool":true,"test-int":100}},{"name":"output-1","parameters":{"test-str":"dummy","test-bool":true,"test-int":100}}]}\x04\x00\x00\x00test\xcd<f@fB'
        )
        assert json_length == 546

    def test_to_rest_with_fp16_not_as_binary_data(self):
        infer_input_1 = InferInput(
            name="input1",
            shape=[3],
            datatype="INT32",
            data=None,
        )
        infer_input_1.set_data_from_numpy(
            np.array([1, 2, 3], dtype=np.int32), binary_data=False
        )
        infer_input_2 = InferInput(
            name="input2",
            shape=[1],
            datatype="BYTES",
            data=None,
        )
        infer_input_2.set_data_from_numpy(
            np.array(["test"], dtype=np.object_), binary_data=True
        )
        infer_input_3 = InferInput(
            name="input3",
            shape=[3],
            datatype="FP16",
            data=None,
        )
        infer_input_3.set_data_from_numpy(
            np.array([1.2, 2.2, 3.2], dtype=np.float16), binary_data=False
        )
        infer_request = InferRequest(
            request_id="4be4e82f-5500-420a-a5c5-ac86841e271b",
            model_name="test_model",
            infer_inputs=[infer_input_1, infer_input_2, infer_input_3],
        )
        with pytest.raises(InvalidInput):
            infer_request_bytes, json_length = infer_request.to_rest()

    def test_to_rest_with_no_binary_data(self):
        infer_input_1 = InferInput(
            name="input1",
            shape=[3],
            datatype="INT32",
            data=[1, 2, 3],
        )
        infer_input_2 = InferInput(
            name="input2",
            shape=[1],
            datatype="BYTES",
            data=["test"],
        )
        infer_input_3 = InferInput(
            name="input3",
            shape=[3],
            datatype="FP32",
            data=[1.2, 2.2, 3.2],
            parameters={
                "test-int": 2,
            },
        )
        infer_request = InferRequest(
            request_id="4be4e82f-5500-420a-a5c5-ac86841e271b",
            model_name="test_model",
            infer_inputs=[infer_input_1, infer_input_2, infer_input_3],
            request_outputs=[
                RequestedOutput(
                    name="output-0",
                    parameters={
                        "test-str": "dummy",
                        "test-bool": True,
                        "test-int": 100,
                    },
                ),
                RequestedOutput(
                    name="output-1",
                    parameters={
                        "test-str": "dummy",
                        "test-bool": True,
                        "test-int": 100,
                    },
                ),
            ],
        )
        infer_request_dict, json_length = infer_request.to_rest()
        assert isinstance(infer_request_dict, dict)
        assert infer_request_dict == {
            "id": "4be4e82f-5500-420a-a5c5-ac86841e271b",
            "model_name": "test_model",
            "inputs": [
                {
                    "name": "input1",
                    "shape": [3],
                    "datatype": "INT32",
                    "data": [1, 2, 3],
                },
                {"name": "input2", "shape": [1], "datatype": "BYTES", "data": ["test"]},
                {
                    "name": "input3",
                    "shape": [3],
                    "datatype": "FP32",
                    "parameters": {"test-int": 2},
                    "data": [1.2, 2.2, 3.2],
                },
            ],
            "outputs": [
                {
                    "name": "output-0",
                    "parameters": {
                        "test-str": "dummy",
                        "test-bool": True,
                        "test-int": 100,
                    },
                },
                {
                    "name": "output-1",
                    "parameters": {
                        "test-str": "dummy",
                        "test-bool": True,
                        "test-int": 100,
                    },
                },
            ],
        }
        assert json_length is None

    def test_to_rest_with_parameters(self):
        infer_input = InferInput(
            name="input1",
            shape=[3],
            datatype="INT32",
            data=None,
            parameters={"param1": "value1"},
        )
        infer_input.set_data_from_numpy(
            np.array([1, 2, 3], dtype=np.int32), binary_data=True
        )
        infer_request = InferRequest(
            model_name="test_model",
            infer_inputs=[infer_input],
            parameters={"request_param1": "request_value1"},
        )
        infer_request_bytes, json_length = infer_request.to_rest()
        assert infer_request_bytes is not None
        assert json_length > 0
        infer_request_dict = orjson.loads(infer_request_bytes[:json_length])
        assert infer_request_dict["parameters"]["request_param1"] == "request_value1"
        assert infer_request_dict["inputs"][0]["parameters"]["param1"] == "value1"

    def test_infer_request_to_rest_missing_data_field(self):
        infer_input = InferInput(
            name="input1",
            shape=[1],
            datatype="INT32",
            parameters={"binary_data_size": 12},
        )
        infer_request = InferRequest(
            model_name="test_model",
            infer_inputs=[infer_input],
        )
        with pytest.raises(InvalidInput):
            infer_request_bytes, json_length = infer_request.to_rest()

    def test_infer_request_from_bytes_valid_input(self):
        infer_input_1 = InferInput(
            name="input1",
            shape=[3],
            datatype="INT32",
            data=[1, 2, 3],
        )
        infer_input_2 = InferInput(
            name="input2",
            shape=[3],
            datatype="FP16",
            data=None,
            parameters={},
        )
        infer_input_2.set_data_from_numpy(
            np.array([1.1, 2.9, 3.4], dtype=np.float16), binary_data=True
        )
        infer_request = InferRequest(
            request_id="abc",
            model_name="test_model",
            infer_inputs=[infer_input_1, infer_input_2],
        )
        expected = copy.deepcopy(infer_request)
        infer_request_bytes, json_length = infer_request.to_rest()
        infer_request_from_bytes = InferRequest.from_bytes(
            infer_request_bytes, json_length, "test_model"
        )
        infer_request_from_bytes.inputs[1].set_data_from_numpy(
            infer_request_from_bytes.inputs[1].as_numpy(), binary_data=True
        )
        assert expected == infer_request_from_bytes

    def test_infer_request_from_bytes_with_requested_outputs(self):
        infer_input_1 = InferInput(
            name="input1",
            shape=[3],
            datatype="INT32",
            data=[1, 2, 3],
        )
        infer_input_2 = InferInput(
            name="input2",
            shape=[3],
            datatype="FP16",
            data=None,
            parameters={},
        )
        infer_input_2.set_data_from_numpy(
            np.array([1.1, 2.9, 3.4], dtype=np.float16), binary_data=True
        )
        infer_request = InferRequest(
            request_id="abc",
            model_name="test_model",
            infer_inputs=[infer_input_1, infer_input_2],
            request_outputs=[
                RequestedOutput(
                    name="output-0",
                    parameters={
                        "test-str": "dummy",
                        "test-bool": True,
                        "test-int": 100,
                    },
                ),
                RequestedOutput(
                    name="output-1",
                    parameters={
                        "test-str": "dummy",
                        "test-bool": True,
                        "test-int": 100,
                    },
                ),
            ],
        )
        expected = copy.deepcopy(infer_request)
        infer_request_bytes, json_length = infer_request.to_rest()
        infer_request_from_bytes = InferRequest.from_bytes(
            infer_request_bytes, json_length, "test_model"
        )
        infer_request_from_bytes.inputs[1].set_data_from_numpy(
            infer_request_from_bytes.inputs[1].as_numpy(), binary_data=True
        )
        assert expected == infer_request_from_bytes

    def test_infer_request_from_bytes_invalid_json(self):
        with pytest.raises(InvalidInput):
            InferRequest.from_bytes(
                b'{"id": "1", "inputs": [{"name": "input1", "shape": [1], "datatype": "INT32", "data": [1]}',
                100,
                "test_model",
            )

    def test_infer_request_from_bytes_missing_data_field(self):
        infer_request_bytes = b'{"id": "1", "inputs": [{"name": "input1", "shape": [1], "datatype": "INT32"'
        with pytest.raises(InvalidInput):
            InferRequest.from_bytes(
                infer_request_bytes, len(infer_request_bytes), "test_model"
            )

    def test_infer_request_from_bytes_missing_binary_data_size(self):
        infer_request_bytes = b'{"id":"509c5da9-80d4-46e8-a50c-0bba2b9d76f8","inputs":[{"name":"input1","shape":[3],"datatype":"INT32"}]}\x01\x00\x00\x00\x02\x00\x00\x00\x03\x00\x00\x00'

        with pytest.raises(InvalidInput):
            InferRequest.from_bytes(
                infer_request_bytes,
                len(
                    b'{"id":"509c5da9-80d4-46e8-a50c-0bba2b9d76f8","inputs":[{"name":"input1","shape":[3],"datatype":"INT32"}]}'
                ),
                "test_model",
            )

    def test_infer_request_from_bytes_fp16_data_via_json(self):

        infer_request_bytes = b'{"id": "1", "inputs": [{"name": "input1", "shape": [1], "datatype": "FP16", "data": [1]}'
        with pytest.raises(InvalidInput):
            InferRequest.from_bytes(
                infer_request_bytes, len(infer_request_bytes), "test_model"
            )

    def test_from_inference_request_with_valid_input(self):
        inference_request = InferenceRequest(
            id="test_id",
            inputs=[
                RequestInput(
                    name="input1",
                    shape=[3],
                    datatype="INT32",
                    data=[1, 2, 3],
                    parameters={
                        "test-str": "dummy",
                    },
                ),
                RequestInput(
                    name="input2",
                    shape=[3],
                    datatype="INT32",
                    data=[1, 2, 3],
                    parameters={
                        "test-int": 1,
                    },
                ),
            ],
            outputs=[
                RequestOutput(name="output1", parameters={"binary_data": True}),
                RequestOutput(name="output2", parameters={"binary_data": False}),
            ],
            parameters={"test-bool": True},
        )
        infer_request = InferRequest.from_inference_request(
            inference_request, "test_model"
        )
        assert infer_request.model_name == "test_model"
        assert infer_request.id == "test_id"
        assert infer_request.parameters == {"test-bool": True}
        assert len(infer_request.inputs) == 2
        assert infer_request.inputs[0].name == "input1"
        assert infer_request.inputs[0].shape == [3]
        assert infer_request.inputs[0].datatype == "INT32"
        assert infer_request.inputs[0].data == [1, 2, 3]
        assert infer_request.inputs[0].parameters == {"test-str": "dummy"}
        assert len(infer_request.request_outputs) == 2
        assert infer_request.request_outputs[0].name == "output1"
        assert infer_request.request_outputs[0].parameters == {"binary_data": True}
        assert infer_request.request_outputs[1].name == "output2"
        assert infer_request.request_outputs[1].parameters == {"binary_data": False}

    def test_from_inference_request_with_fp16_data(self):
        inference_request = InferenceRequest(
            id="test_id",
            inputs=[
                RequestInput(
                    name="input1", shape=[3], datatype="FP16", data=[1.0, 2.0, 3.0]
                )
            ],
        )
        with pytest.raises(InvalidInput):
            InferRequest.from_inference_request(inference_request, "test_model")

    def test_from_inference_request_with_no_requested_outputs(self):
        inference_request = InferenceRequest(
            id="test_id",
            inputs=[
                RequestInput(name="input1", shape=[3], datatype="INT32", data=[1, 2, 3])
            ],
        )
        infer_request = InferRequest.from_inference_request(
            inference_request, "test_model"
        )
        assert infer_request.model_name == "test_model"
        assert infer_request.id == "test_id"
        assert len(infer_request.inputs) == 1
        assert infer_request.inputs[0].name == "input1"
        assert infer_request.inputs[0].shape == [3]
        assert infer_request.inputs[0].datatype == "INT32"
        assert infer_request.inputs[0].data == [1, 2, 3]
        assert infer_request.request_outputs is None


class TestInferResponse:
    def test_to_grpc(self):
        infer_res = InferResponse(
            model_name="TestModel",
            response_id="123",
            model_version="v1",
            parameters={"test-str": "dummy", "test-bool": True, "test-int": 100},
            infer_outputs=[
                InferOutput(
                    name="output-0",
                    datatype="INT32",
                    shape=[1, 2],
                    data=[1, 2],
                    parameters={
                        "test-str": "dummy",
                        "test-bool": True,
                        "test-int": 100,
                    },
                )
            ],
        )
        expected = ModelInferResponse(
            model_name="TestModel",
            id="123",
            model_version="v1",
            parameters={
                "test-str": InferParameter(string_param="dummy"),
                "test-bool": InferParameter(bool_param=True),
                "test-int": InferParameter(int64_param=100),
            },
            outputs=[
                {
                    "name": "output-0",
                    "shape": [1, 2],
                    "datatype": "INT32",
                    "contents": {"int_contents": [1, 2]},
                    "parameters": {
                        "test-str": InferParameter(string_param="dummy"),
                        "test-bool": InferParameter(bool_param=True),
                        "test-int": InferParameter(int64_param=100),
                    },
                }
            ],
        )
        res = infer_res.to_grpc()
        assert res == expected

    def test_from_grpc(self):
        infer_res = ModelInferResponse(
            model_name="TestModel",
            id="123",
            model_version="v1",
            parameters={
                "test-str": InferParameter(string_param="dummy"),
                "test-bool": InferParameter(bool_param=True),
                "test-int": InferParameter(int64_param=100),
            },
            outputs=[
                {
                    "name": "output-0",
                    "shape": [1, 2],
                    "datatype": "INT32",
                    "contents": {"int_contents": [1, 2]},
                    "parameters": {
                        "test-str": InferParameter(string_param="dummy"),
                        "test-bool": InferParameter(bool_param=True),
                        "test-int": InferParameter(int64_param=100),
                    },
                }
            ],
        )
        expected = InferResponse(
            model_name="TestModel",
            response_id="123",
            model_version="v1",
            parameters={
                "test-str": InferParameter(string_param="dummy"),
                "test-bool": InferParameter(bool_param=True),
                "test-int": InferParameter(int64_param=100),
            },
            infer_outputs=[
                InferOutput(
                    name="output-0",
                    datatype="INT32",
                    shape=[1, 2],
                    data=[1, 2],
                    parameters={
                        "test-str": InferParameter(string_param="dummy"),
                        "test-bool": InferParameter(bool_param=True),
                        "test-int": InferParameter(int64_param=100),
                    },
                )
            ],
            from_grpc=True,
        )
        res = InferResponse.from_grpc(infer_res)
        assert res == expected

    def test_infer_response_to_rest_with_binary_data(self):
        data = np.array(
            [[1.2, 2.2, 3.2, 4.1], [1.5, 2.6, 3.787, 4.54]], dtype=np.float16
        )
        fp16_output = InferOutput(
            name="fp16_output",
            shape=list(data.shape),
            datatype="FP16",
            data=None,
            parameters=None,
        )
        fp32_output = InferOutput(
            name="fp32_output",
            shape=list(data.shape),
            datatype="FP32",
            data=None,
            parameters=None,
        )
        fp16_output.set_data_from_numpy(data, binary_data=True)
        fp32_output.set_data_from_numpy(data.astype(np.float32), binary_data=True)
        infer_response = InferResponse(
            response_id="1",
            model_name="test_model",
            infer_outputs=[fp16_output, fp32_output],
            requested_outputs=[
                RequestedOutput(name="fp16_output", parameters={"binary_data": True}),
                RequestedOutput(name="fp32_output", parameters={"binary_data": True}),
            ],
        )
        result, json_length = infer_response.to_rest()
        assert isinstance(result, bytes)
        assert (
            result
            == b'{"id":"1","model_name":"test_model","model_version":null,"outputs":[{"name":"fp16_output","shape":[2,4],"datatype":"FP16","parameters":{"binary_data_size":16}},{"name":"fp32_output","shape":[2,4],"datatype":"FP32","parameters":{"binary_data_size":32}}]}\xcd<f@fB\x1aD\x00>3A\x93C\x8aD\x00\xa0\x99?\x00\xc0\x0c@\x00\xc0L@\x00@\x83@\x00\x00\xc0?\x00`&@\x00`r@\x00@\x91@'
        )
        assert json_length == 253

    def test_infer_response_to_rest_without_binary_data(self):
        int32_output = InferOutput(
            name="int32_output",
            shape=[1],
            datatype="INT32",
            data=[1],
            parameters=None,
        )
        str_output = InferOutput(
            name="str_output",
            shape=[1],
            datatype="BYTES",
            data=["test"],
            parameters=None,
        )
        uint_data = np.array([[1, 2]], dtype=np.uint32)
        uint_output = InferOutput(
            name="uint_output",
            shape=list(uint_data.shape),
            datatype="UINT32",
            parameters=None,
            data=uint_data,
        )
        infer_response = InferResponse(
            response_id="1",
            model_name="test_model",
            infer_outputs=[int32_output, str_output, uint_output],
        )

        result, json_length = infer_response.to_rest()
        assert isinstance(result, dict)
        assert result == {
            "id": "1",
            "model_name": "test_model",
            "model_version": None,
            "outputs": [
                {
                    "name": "int32_output",
                    "shape": [1],
                    "datatype": "INT32",
                    "data": [1],
                },
                {
                    "name": "str_output",
                    "shape": [1],
                    "datatype": "BYTES",
                    "data": ["test"],
                },
                {
                    "name": "uint_output",
                    "shape": [1, 2],
                    "datatype": "UINT32",
                    "data": [1, 2],
                },
            ],
        }
        assert json_length is None

    def test_infer_response_to_rest_with_mixed_binary_data(self):
        infer_output1 = InferOutput(
            name="output1",
            shape=[1],
            datatype="FP16",
            data=np.array([1], dtype=np.float16),
            parameters=None,
        )
        infer_output1.set_data_from_numpy(
            np.array([1], dtype=np.float16), binary_data=True
        )
        infer_output2 = InferOutput(
            name="output2",
            shape=[1],
            datatype="INT32",
            data=[1],
            parameters=None,
        )
        infer_output2.set_data_from_numpy(
            np.array([1], dtype=np.int32), binary_data=False
        )
        infer_output3 = InferOutput(
            name="output3",
            shape=[1],
            datatype="BYTES",
            data=None,
            parameters=None,
        )
        infer_output3.set_data_from_numpy(
            np.array(["test"], dtype=np.object_), binary_data=True
        )
        infer_response = InferResponse(
            response_id="1",
            model_name="test_model",
            infer_outputs=[infer_output1, infer_output2, infer_output3],
            requested_outputs=[
                RequestedOutput(name="output1", parameters={"binary_data": True}),
                RequestedOutput(name="output2", parameters={"binary_data": True}),
                RequestedOutput(name="output3", parameters={"binary_data": False}),
            ],
        )
        result, json_length = infer_response.to_rest()
        assert isinstance(result, bytes)
        assert (
            result
            == b'{"id":"1","model_name":"test_model","model_version":null,"outputs":[{"name":"output1","shape":[1],"datatype":"FP16","parameters":{"binary_data_size":2}},{"name":"output2","shape":[1],"datatype":"INT32","parameters":{"binary_data_size":4}},{"name":"output3","shape":[1],"datatype":"BYTES","data":["test"]}]}\x00<\x01\x00\x00\x00'
        )
        assert json_length == 306

    def test_infer_response_to_rest_with_binary_output_data_true(self):
        infer_output1 = InferOutput(
            name="output1",
            shape=[1],
            datatype="FP16",
            data=[1],
            parameters=None,
        )
        infer_output2 = InferOutput(
            name="output2",
            shape=[1],
            datatype="FP16",
            data=None,
            parameters=None,
        )
        infer_output2.set_data_from_numpy(
            np.array([1], dtype=np.float16), binary_data=True
        )
        infer_output3 = InferOutput(
            name="output3",
            shape=[1],
            datatype="BYTES",
            data=[b"test"],
            parameters=None,
        )
        infer_response = InferResponse(
            response_id="1",
            model_name="test_model",
            infer_outputs=[infer_output1, infer_output2, infer_output3],
            use_binary_outputs=True,
        )
        result, json_length = infer_response.to_rest()
        assert isinstance(result, bytes)
        assert (
            result
            == b'{"id":"1","model_name":"test_model","model_version":null,"outputs":[{"name":"output1","shape":[1],"datatype":"FP16","parameters":{"binary_data_size":2}},{"name":"output2","shape":[1],"datatype":"FP16","parameters":{"binary_data_size":2}},{"name":"output3","shape":[1],"datatype":"BYTES","parameters":{"binary_data_size":8}}]}\x00<\x00<\x04\x00\x00\x00test'
        )
        assert json_length == 325

    def test_infer_response_to_rest_with_binary_output_data_precedence(
        self,
    ):
        infer_output1 = InferOutput(
            name="output1",
            shape=[1],
            datatype="FP16",
            data=np.array([1], dtype=np.float16),
            parameters=None,
        )
        infer_output1.set_data_from_numpy(
            np.array([1], dtype=np.float16), binary_data=True
        )
        infer_output2 = InferOutput(
            name="output2",
            shape=[1],
            datatype="INT32",
            data=[1],
            parameters=None,
        )
        infer_output3 = InferOutput(
            name="output3",
            shape=[1],
            datatype="BYTES",
            data=["test"],
            parameters=None,
        )
        infer_response = InferResponse(
            response_id="1",
            model_name="test_model",
            infer_outputs=[infer_output1, infer_output2, infer_output3],
            use_binary_outputs=True,
            requested_outputs=[
                RequestedOutput(name="output1", parameters={"binary_data": True}),
                RequestedOutput(name="output2", parameters={"binary_data": False}),
                RequestedOutput(name="output3", parameters={"binary_data": True}),
            ],
        )
        result, json_length = infer_response.to_rest()
        assert isinstance(result, bytes)
        assert (
            result
            == b'{"id":"1","model_name":"test_model","model_version":null,"outputs":[{"name":"output1","shape":[1],"datatype":"FP16","parameters":{"binary_data_size":2}},{"name":"output2","shape":[1],"datatype":"INT32","data":[1]},{"name":"output3","shape":[1],"datatype":"BYTES","parameters":{"binary_data_size":8}}]}\x00<\x04\x00\x00\x00test'
        )
        assert json_length == 301

    def test_infer_response_to_rest_with_raw_data_with_binary_data_false(
        self,
    ):
        infer_output = InferOutput(
            name="output1",
            shape=[3],
            datatype="INT32",
        )
        raw_data = b"\x01\x00\x00\x00\x02\x00\x00\x00\x03\x00\x00\x00"
        infer_output._raw_data = raw_data
        infer_request = InferResponse(
            response_id="4be4e82f-5500-420a-a5c5-ac86841e271b",
            model_name="test_model",
            infer_outputs=[infer_output],
        )
        infer_response, json_length = infer_request.to_rest()
        assert infer_response == {
            "id": "4be4e82f-5500-420a-a5c5-ac86841e271b",
            "model_name": "test_model",
            "model_version": None,
            "outputs": [
                {
                    "name": "output1",
                    "shape": [3],
                    "datatype": "INT32",
                    "data": [1, 2, 3],
                }
            ],
        }
        assert json_length is None

    def test_infer_response_to_rest_missing_data_field(self):
        infer_output = InferOutput(
            name="input1",
            shape=[1],
            datatype="INT32",
            parameters={"binary_data_size": 12},
        )
        infer_response = InferResponse(
            response_id="1",
            model_name="test_model",
            infer_outputs=[infer_output],
        )
        with pytest.raises(InvalidInput):
            _, _ = infer_response.to_rest()

    def test_infer_response_from_bytes_happy_path(self):
        model_name = "test_model"
        response_bytes = b'{"id": "1", "model_name": "test_model", "outputs": [{"name": "output1", "shape": [1], "datatype": "INT32", "data": [1]}]}'
        json_length = len(response_bytes)

        infer_response = InferResponse.from_bytes(
            response_bytes,
            json_length,
        )

        assert infer_response.id == "1"
        assert infer_response.model_name == model_name
        assert len(infer_response.outputs) == 1
        assert infer_response.outputs[0].name == "output1"
        assert infer_response.outputs[0].shape == [1]
        assert infer_response.outputs[0].datatype == "INT32"
        assert infer_response.outputs[0].data == [1]

    def test_infer_response_from_bytes_with_binary_data(self):
        serialized_str_data = serialize_byte_tensor(
            np.array([b"cat", b"dog", b"bird", b"fish"], dtype=np.object_)
        ).item()
        model_name = "test_model"
        response_bytes = json.dumps(
            {
                "model_name": model_name,
                "id": "1",
                "outputs": [
                    {
                        "name": "output1",
                        "shape": [4],
                        "datatype": "BYTES",
                        "parameters": {"binary_data_size": len(serialized_str_data)},
                    }
                ],
            }
        ).encode()
        json_length = len(response_bytes)

        infer_response = InferResponse.from_bytes(
            response_bytes + serialized_str_data,
            json_length,
        )

        assert infer_response.id == "1"
        assert infer_response.model_name == model_name
        assert len(infer_response.outputs) == 1
        assert infer_response.outputs[0].name == "output1"
        assert infer_response.outputs[0].shape == [4]
        assert infer_response.outputs[0].datatype == "BYTES"
        assert infer_response.outputs[0].data == ["cat", "dog", "bird", "fish"]

    def test_infer_response_from_bytes_with_missing_data(self):
        response_bytes = b'{"id": "1", "model_name": "test_model", "outputs": [{"name": "output1", "shape": [1], "datatype": "INT32"}]}'
        json_length = len(response_bytes)

        with pytest.raises(InvalidInput):
            InferResponse.from_bytes(response_bytes, json_length)

    def test_infer_response_get_output_by_name_returns_correct_output(self):
        infer_output1 = InferOutput(
            name="output1", shape=[1], datatype="INT32", data=[1]
        )
        infer_output2 = InferOutput(
            name="output2", shape=[1], datatype="INT32", data=[2]
        )
        infer_response = InferResponse(
            response_id="1",
            model_name="test_model",
            infer_outputs=[infer_output1, infer_output2],
        )

        result = infer_response.get_output_by_name("output2")

        assert result == infer_output2

    def test_infer_response_get_output_by_name_returns_none_for_non_existent_output(
        self,
    ):
        infer_output1 = InferOutput(
            name="output1", shape=[1], datatype="INT32", data=[1]
        )
        infer_output2 = InferOutput(
            name="output2", shape=[1], datatype="INT32", data=[2]
        )
        infer_response = InferResponse(
            response_id="1",
            model_name="test_model",
            infer_outputs=[infer_output1, infer_output2],
        )

        result = infer_response.get_output_by_name("output3")

        assert result is None


def test_contains_fp16_datatype_with_fp16_output():
    infer_output1 = InferOutput(name="output1", shape=[1], datatype="FP16", data=[1])
    infer_output2 = InferOutput(name="output2", shape=[1], datatype="INT32", data=[2])
    infer_response = InferResponse(
        response_id="1",
        model_name="test_model",
        infer_outputs=[infer_output1, infer_output2],
    )

    assert _contains_fp16_datatype(infer_response) is True


def test_contains_fp16_datatype_without_fp16_output():
    infer_output = InferOutput(name="output1", shape=[1], datatype="INT32", data=[1])
    infer_response = InferResponse(
        response_id="1", model_name="test_model", infer_outputs=[infer_output]
    )

    assert _contains_fp16_datatype(infer_response) is False


def test_contains_fp16_datatype_with_no_outputs():
    infer_response = InferResponse(
        response_id="1", model_name="test_model", infer_outputs=[]
    )

    assert _contains_fp16_datatype(infer_response) is False
