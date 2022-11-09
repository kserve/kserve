# Copyright 2022 The KServe Authors.
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
from typing import Optional, List, Union, Dict

import orjson
from pydantic import BaseModel

# TODO: in the future, this file can be auto generated
# https://pydantic-docs.helpmanual.io/datamodel_code_generator/

# Reference: https://github.com/kserve/kserve/blob/master/docs/predict-api/v2/rest_predict_v2.yaml


InferParameter = Union[str, int, float, bool]
Parameters = Dict[str, InferParameter]


class ServerLiveResponse(BaseModel):
    live: bool

    class Config:
        schema_extra = {
            "example": {
                "live": True
            }
        }


class ServerReadyResponse(BaseModel):
    ready: bool

    class Config:
        schema_extra = {
            "example": {
                "ready": True
            }
        }


class ServerMetadataResponse(BaseModel):
    """ServerMetadataResponse

        $server_metadata_response =
        {
          "name" : $string,
          "version" : $string,
          "extensions" : [ $string, ... ]
        }
    """
    name: str
    version: str
    extensions: List[str]


class MetadataTensor(BaseModel):
    """MetadataTensor

        $metadata_tensor =
        {
          "name" : $string,
          "datatype" : $string,
          "shape" : [ $number, ... ]
        }
    """
    name: str
    datatype: str
    shape: List[int]


class ModelMetadataResponse(BaseModel):
    """ModelMetadataResponse

        $metadata_model_response =
        {
          "name" : $string,
          "versions" : [ $string, ... ] #optional,
          "platform" : $string,
          "inputs" : [ $metadata_tensor, ... ],
          "outputs" : [ $metadata_tensor, ... ]
        }
    """
    name: str
    versions: Optional[List[str]]
    platform: str
    inputs: List[MetadataTensor]
    outputs: List[MetadataTensor]


class RequestInput(BaseModel):
    """RequestInput Model

        $request_input =
        {
          "name" : $string,
          "shape" : [ $number, ... ],
          "datatype"  : $string,
          "parameters" : $parameters #optional,
          "data" : $tensor_data
        }
    """
    name: str
    shape: List[int]
    datatype: str
    parameters: Optional[Parameters]
    data: List


class RequestOutput(BaseModel):
    """RequestOutput Model

        $request_output =
        {
          "name" : $string,
          "parameters" : $parameters #optional,
        }
    """
    name: str
    parameters: Optional[Parameters]


class ResponseOutput(BaseModel):
    """ResponseOutput Model

        $response_output =
        {
          "name" : $string,
          "shape" : [ $number, ... ],
          "datatype"  : $string,
          "parameters" : $parameters #optional,
          "data" : $tensor_data
        }
    """
    name: str
    shape: List[int]
    datatype: str
    parameters: Optional[Parameters]
    data: List


class InferenceRequest(BaseModel):
    """InferenceRequest Model

        $inference_request =
        {
          "id" : $string #optional,
          "parameters" : $parameters #optional,
          "inputs" : [ $request_input, ... ],
          "outputs" : [ $request_output, ... ] #optional
        }
    """
    id: Optional[str]
    parameters: Optional[Parameters] = None
    inputs: List[RequestInput]
    outputs: Optional[List[RequestOutput]] = None

    class Config:
        json_loads = orjson.loads
        schema_extra = {
            "example": {
                "id": "42",
                "inputs": [
                    {
                        "name": "input0",
                        "shape": [2, 2],
                        "datatype": "UINT32",
                        "data": [1, 2, 3, 4]
                    },
                    {
                        "name": "input1",
                        "shape": [3],
                        "datatype": "BOOL",
                        "data": ["true"]
                    }
                ],
                "outputs": [
                    {
                        "name": "output0"
                    }
                ]
            }
        }


class InferenceResponse(BaseModel):
    """InferenceResponse

        $inference_response =
        {
          "model_name" : $string,
          "model_version" : $string #optional,
          "id" : $string,
          "parameters" : $parameters #optional,
          "outputs" : [ $response_output, ... ]
        }
    """
    model_name: str
    model_version: Optional[str]
    id: str
    parameters: Optional[Parameters]
    outputs: List[ResponseOutput]

    class Config:
        schema_extra = {
            "example": {
                "id": "42",
                "outputs": [
                    {
                        "name": "output0",
                        "shape": [3, 2],
                        "datatype": "FP32",
                        "data": [1.0, 1.1, 2.0, 2.1, 3.0, 3.1]
                    }
                ]
            }
        }
