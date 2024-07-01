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


from kserve import InferRequest, InferInput, InferResponse, InferOutput
from kserve.protocol.grpc.grpc_predict_v2_pb2 import (
    ModelInferRequest,
    InferParameter,
    ModelInferResponse,
)


class TestInferRequest:
    def test_to_rest(self):
        infer_req = InferRequest(
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
                        "test-str": InferParameter(string_param="dummy"),
                        "test-bool": InferParameter(bool_param=True),
                        "test-int": InferParameter(int64_param=100),
                    },
                )
            ],
        )
        # model_name should not be present for rest
        expected = {
            "id": "123",
            "inputs": [
                {
                    "name": "input-0",
                    "shape": [1, 2],
                    "datatype": "INT32",
                    "data": [1, 2],
                    "parameters": {
                        "test-str": "dummy",
                        "test-bool": True,
                        "test-int": 100,
                    },
                }
            ],
            "parameters": {"test-str": "dummy", "test-bool": True, "test-int": 100},
        }
        res = infer_req.to_rest()
        assert res == expected

    def test_to_grpc(self):
        infer_req = InferRequest(
            model_name="TestModel",
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
        )
        expected = ModelInferRequest(
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
                        "test-str": InferParameter(string_param="dummy"),
                        "test-bool": InferParameter(bool_param=True),
                        "test-int": InferParameter(int64_param=100),
                    },
                )
            ],
            from_grpc=True,
        )
        res = InferRequest.from_grpc(infer_req)
        assert res == expected

    class TestInferResponse:
        def test_to_rest(self):
            infer_res = InferResponse(
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
            )
            expected = {
                "id": "123",
                "model_name": "TestModel",
                "model_version": "v1",
                "outputs": [
                    {
                        "name": "output-0",
                        "shape": [1, 2],
                        "datatype": "INT32",
                        "data": [1, 2],
                        "parameters": {
                            "test-str": "dummy",
                            "test-bool": True,
                            "test-int": 100,
                        },
                    }
                ],
                "parameters": {"test-str": "dummy", "test-bool": True, "test-int": 100},
            }
            res = infer_res.to_rest()
            assert res == expected

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
