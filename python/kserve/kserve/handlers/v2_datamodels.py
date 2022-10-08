from typing import Optional, List, Union, Dict

from pydantic import BaseModel

# Reference: https://github.com/kserve/kserve/blob/master/docs/predict-api/v2/rest_predict_v2.yaml

InferParameter = Union[str, int, float, bool]
Parameters = Dict[str, InferParameter]


class ServerLiveResponse(BaseModel):
    live: bool


class ServerReadyResponse(BaseModel):
    ready: bool


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
