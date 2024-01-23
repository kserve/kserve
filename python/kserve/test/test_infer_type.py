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
from kserve.protocol.grpc.grpc_predict_v2_pb2 import ModelInferRequest, InferParameter, ModelInferResponse


class TestInferRequest:
    def test_to_rest(self):
        infer_req = InferRequest(model_name="TestModel", request_id="123",
                                 parameters={
                                     "test-str": InferParameter(string_param="dummy"),
                                     "test-bool": InferParameter(bool_param=True),
                                     "test-int": InferParameter(int64_param=100)
                                 },
                                 infer_inputs=[
                                     InferInput(name="input-0", datatype="INT32", shape=[1, 2], data=[1, 2],
                                                parameters={
                                                    "test-str": InferParameter(string_param="dummy"),
                                                    "test-bool": InferParameter(bool_param=True),
                                                    "test-int": InferParameter(int64_param=100)
                                                })]
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
                        "test-int": 100
                    }
                }
            ],
            "parameters": {
                "test-str": "dummy",
                "test-bool": True,
                "test-int": 100
            }
        }
        res = infer_req.to_rest()
        assert res == expected

    def test_to_grpc(self):
        infer_req = InferRequest(model_name="TestModel", request_id="123",
                                 parameters={
                                     "test-str": "dummy",
                                     "test-bool": True,
                                     "test-int": 100
                                 },
                                 infer_inputs=[
                                     InferInput(name="input-0", datatype="INT32", shape=[1, 2], data=[1, 2],
                                                parameters={
                                                    "test-str": "dummy",
                                                    "test-bool": True,
                                                    "test-int": 100
                                                })]
                                 )
        expected = ModelInferRequest(model_name="TestModel", id="123",
                                     parameters={
                                         "test-str": InferParameter(string_param="dummy"),
                                         "test-bool": InferParameter(bool_param=True),
                                         "test-int": InferParameter(int64_param=100)
                                     },
                                     inputs=[
                                         {
                                             "name": "input-0",
                                             "shape": [1, 2],
                                             "datatype": "INT32",
                                             "contents": {
                                                 "int_contents": [1, 2]
                                             },
                                             "parameters": {
                                                 "test-str": InferParameter(string_param="dummy"),
                                                 "test-bool": InferParameter(bool_param=True),
                                                 "test-int": InferParameter(int64_param=100)
                                             },
                                         }]
                                     )
        res = infer_req.to_grpc()
        assert res == expected

    def test_from_grpc(self):
        infer_req = ModelInferRequest(model_name="TestModel", id="123",
                                      parameters={
                                          "test-str": InferParameter(string_param="dummy"),
                                          "test-bool": InferParameter(bool_param=True),
                                          "test-int": InferParameter(int64_param=100)
                                      },
                                      inputs=[
                                          {
                                              "name": "input-0",
                                              "shape": [1, 2],
                                              "datatype": "INT32",
                                              "contents": {
                                                  "int_contents": [1, 2]
                                              },
                                              "parameters": {
                                                  "test-str": InferParameter(string_param="dummy"),
                                                  "test-bool": InferParameter(bool_param=True),
                                                  "test-int": InferParameter(int64_param=100)
                                              },
                                          }]
                                      )
        expected = InferRequest(model_name="TestModel", request_id="123",
                                parameters={
                                    "test-str": InferParameter(string_param="dummy"),
                                    "test-bool": InferParameter(bool_param=True),
                                    "test-int": InferParameter(int64_param=100)
                                },
                                infer_inputs=[
                                    InferInput(name="input-0", datatype="INT32", shape=[1, 2], data=[1, 2],
                                               parameters={
                                                   "test-str": InferParameter(string_param="dummy"),
                                                   "test-bool": InferParameter(bool_param=True),
                                                   "test-int": InferParameter(int64_param=100)
                                               })],
                                from_grpc=True
                                )
        res = InferRequest.from_grpc(infer_req)
        assert InferRequestMatcher(expected).__eq__(res) is True

    class TestInferResponse:
        def test_to_rest(self):
            infer_res = InferResponse(model_name="TestModel", response_id="123",
                                      parameters={
                                          "test-str": InferParameter(string_param="dummy"),
                                          "test-bool": InferParameter(bool_param=True),
                                          "test-int": InferParameter(int64_param=100)
                                      },
                                      infer_outputs=[
                                          InferOutput(name="output-0", datatype="INT32", shape=[1, 2], data=[1, 2],
                                                      parameters={
                                                          "test-str": InferParameter(string_param="dummy"),
                                                          "test-bool": InferParameter(bool_param=True),
                                                          "test-int": InferParameter(int64_param=100)
                                                      })]
                                      )
            expected = {
                "id": "123",
                "model_name": "TestModel",
                "outputs": [
                    {
                        "name": "output-0",
                        "shape": [1, 2],
                        "datatype": "INT32",
                        "data": [1, 2],
                        "parameters": {
                            "test-str": "dummy",
                            "test-bool": True,
                            "test-int": 100
                        }
                    }
                ],
                "parameters": {
                    "test-str": "dummy",
                    "test-bool": True,
                    "test-int": 100
                }
            }
            res = infer_res.to_rest()
            assert res == expected

        def test_to_grpc(self):
            infer_res = InferResponse(model_name="TestModel", response_id="123",
                                      parameters={
                                          "test-str": "dummy",
                                          "test-bool": True,
                                          "test-int": 100
                                      },
                                      infer_outputs=[
                                          InferOutput(name="output-0", datatype="INT32", shape=[1, 2], data=[1, 2],
                                                      parameters={
                                                          "test-str": "dummy",
                                                          "test-bool": True,
                                                          "test-int": 100
                                                      })]
                                      )
            expected = ModelInferResponse(model_name="TestModel", id="123",
                                          parameters={
                                              "test-str": InferParameter(string_param="dummy"),
                                              "test-bool": InferParameter(bool_param=True),
                                              "test-int": InferParameter(int64_param=100)
                                          },
                                          outputs=[
                                              {
                                                  "name": "output-0",
                                                  "shape": [1, 2],
                                                  "datatype": "INT32",
                                                  "contents": {
                                                      "int_contents": [1, 2]
                                                  },
                                                  "parameters": {
                                                      "test-str": InferParameter(string_param="dummy"),
                                                      "test-bool": InferParameter(bool_param=True),
                                                      "test-int": InferParameter(int64_param=100)
                                                  },
                                              }]
                                          )
            res = infer_res.to_grpc()
            assert res == expected

        def test_from_grpc(self):
            infer_res = ModelInferResponse(model_name="TestModel", id="123",
                                           parameters={
                                               "test-str": InferParameter(string_param="dummy"),
                                               "test-bool": InferParameter(bool_param=True),
                                               "test-int": InferParameter(int64_param=100)
                                           },
                                           outputs=[
                                               {
                                                   "name": "output-0",
                                                   "shape": [1, 2],
                                                   "datatype": "INT32",
                                                   "contents": {
                                                       "int_contents": [1, 2]
                                                   },
                                                   "parameters": {
                                                       "test-str": InferParameter(string_param="dummy"),
                                                       "test-bool": InferParameter(bool_param=True),
                                                       "test-int": InferParameter(int64_param=100)
                                                   },
                                               }]
                                           )
            expected = InferResponse(model_name="TestModel", response_id="123",
                                     parameters={
                                         "test-str": InferParameter(string_param="dummy"),
                                         "test-bool": InferParameter(bool_param=True),
                                         "test-int": InferParameter(int64_param=100)
                                     },
                                     infer_outputs=[
                                         InferOutput(name="output-0", datatype="INT32", shape=[1, 2], data=[1, 2],
                                                     parameters={
                                                         "test-str": InferParameter(string_param="dummy"),
                                                         "test-bool": InferParameter(bool_param=True),
                                                         "test-int": InferParameter(int64_param=100)
                                                     })],
                                     from_grpc=True
                                     )
            res = InferResponse.from_grpc(infer_res)
            assert InferResponseMatcher(expected).__eq__(res) is True


class InferInputMatcher:
    def __init__(self, infer_input: InferInput):
        self.input = infer_input

    def __eq__(self, other):
        if not isinstance(other, InferInput):
            return False
        if self.input.name != other.name:
            return False
        if self.input.shape != other.shape:
            return False
        if self.input.datatype != other.datatype:
            return False
        if self.input.parameters != other.parameters:
            return False
        if self.input.data != other.data:
            return False
        return True


class InferRequestMatcher:
    def __init__(self, req: InferRequest):
        self.req = req

    def __eq__(self, other):
        if not isinstance(other, InferRequest):
            return False
        if self.req.model_name != other.model_name:
            return False
        if self.req.id != other.id:
            return False
        if self.req.from_grpc != other.from_grpc:
            return False
        if self.req.parameters != other.parameters:
            return False
        if len(self.req.inputs) != len(other.inputs):
            return False
        for i in range(len(self.req.inputs)):
            if not InferInputMatcher(self.req.inputs[i]).__eq__(other.inputs[i]):
                return False
        return True


class InferOutputMatcher:
    def __init__(self, infer_input: InferOutput):
        self.output = infer_input

    def __eq__(self, other):
        if not isinstance(other, InferOutput):
            return False
        if self.output.name != other.name:
            return False
        if self.output.shape != other.shape:
            return False
        if self.output.datatype != other.datatype:
            return False
        if self.output.parameters != other.parameters:
            return False
        if self.output.data != other.data:
            return False
        return True


class InferResponseMatcher:
    def __init__(self, res: InferResponse):
        self.res = res

    def __eq__(self, other):
        if not isinstance(other, InferResponse):
            return False
        if self.res.model_name != other.model_name:
            return False
        if self.res.id != other.id:
            return False
        if self.res.from_grpc != other.from_grpc:
            return False
        if self.res.parameters != other.parameters:
            return False
        if len(self.res.outputs) != len(other.outputs):
            return False
        for i in range(len(self.res.outputs)):
            if not InferOutputMatcher(self.res.outputs[i]).__eq__(other.outputs[i]):
                return False
        return True
