# Copyright 2023 The KServe Authors.
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

import struct
from typing import Optional, List, Dict, Union, Tuple

import numpy as np
import orjson
import pandas as pd
import uuid
from google.protobuf.internal.containers import MessageMap

from .rest.v2_datamodels import InferenceRequest
from ..constants.constants import GRPC_CONTENT_DATATYPE_MAPPINGS
from ..errors import InvalidInput, InferenceError
from ..protocol.grpc.grpc_predict_v2_pb2 import (
    ModelInferRequest,
    InferTensorContents,
    ModelInferResponse,
    InferParameter,
)
from ..utils.numpy_codec import to_np_dtype, from_np_dtype


def serialize_byte_tensor(input_tensor: np.ndarray) -> np.ndarray:
    """
    Serializes a bytes tensor into a flat numpy array of length prepended
    bytes. The numpy array should use dtype of np.object. For np.bytes,
    numpy will remove trailing zeros at the end of byte sequence and because
    of this it should be avoided.

    Args:
        input_tensor : np.array
            The bytes tensor to serialize.
    Returns:
        serialized_bytes_tensor : np.array
            The 1-D numpy array of type uint8 containing the serialized bytes in row-major form.
    Raises:
        InferenceError If unable to serialize the given tensor.
    """

    if input_tensor.size == 0:
        return np.empty([0], dtype=np.object_)

    # If the input is a tensor of string/bytes objects, then must flatten those into
    # a 1-dimensional array containing the 4-byte byte size followed by the
    # actual element bytes. All elements are concatenated together in row-major
    # order.

    if (input_tensor.dtype != np.object_) and (input_tensor.dtype.type != np.bytes_):
        raise InferenceError("cannot serialize bytes tensor: invalid datatype")

    flattened_ls = []
    # 'C' order is row-major.
    for obj in np.nditer(input_tensor, flags=["refs_ok"], order="C"):
        # If directly passing bytes to BYTES type,
        # don't convert it to str as Python will encode the
        # bytes which may distort the meaning
        if input_tensor.dtype == np.object_:
            if type(obj.item()) == bytes:
                s = obj.item()
            else:
                s = str(obj.item()).encode("utf-8")
        else:
            s = obj.item()
        flattened_ls.append(struct.pack("<I", len(s)))
        flattened_ls.append(s)
    flattened = b"".join(flattened_ls)
    flattened_array = np.asarray(flattened, dtype=np.object_)
    if not flattened_array.flags["C_CONTIGUOUS"]:
        flattened_array = np.ascontiguousarray(flattened_array, dtype=np.object_)
    return flattened_array


def deserialize_bytes_tensor(encoded_tensor: bytes) -> np.ndarray:
    """
    Deserializes an encoded bytes tensor into a
    numpy array of dtype of python objects

    Args:
        encoded_tensor : bytes
            The encoded bytes tensor where each element
            has its length in first 4 bytes followed by
            the content
    Returns:
        string_tensor : np.array
            The 1-D numpy array of type object containing the
            deserialized bytes in row-major form.
    """
    strs = list()
    offset = 0
    val_buf = encoded_tensor
    while offset < len(val_buf):
        length = struct.unpack_from("<I", val_buf, offset)[0]
        offset += 4
        sb = struct.unpack_from("<{}s".format(length), val_buf, offset)[0]
        offset += length
        strs.append(sb)
    return np.array(strs, dtype=np.object_)


class InferInput:
    _name: str
    _shape: List[int]
    _datatype: str
    _parameters: Dict

    def __init__(
        self,
        name: str,
        shape: List[int],
        datatype: str,
        data: Union[List, np.ndarray, InferTensorContents] = None,
        parameters: Optional[Union[Dict, MessageMap[str, InferParameter]]] = None,
    ):
        """An object of InferInput class is used to describe the input tensor of an inference request.

        Args:
            name: The name of the inference input whose data will be described by this object.
            shape : The shape of the associated inference input.
            datatype : The data type of the associated inference input.
            data : The data of the inference input.
                   When data is not set, raw_data is used for gRPC to transmit with numpy array bytes
                   by using `set_data_from_numpy`.
            parameters : The additional inference parameters.
        """

        self._name = name
        self._shape = shape
        self._datatype = datatype.upper()
        self._parameters = parameters
        self._data = data
        self._raw_data = None

    @property
    def name(self) -> str:
        """Get the name of inference input associated with this object.

        Returns:
            The name of the inference input
        """
        return self._name

    @property
    def datatype(self) -> str:
        """Get the datatype of inference input associated with this object.

        Returns:
            The datatype of the inference input.
        """
        return self._datatype

    @property
    def data(self) -> Union[List, np.ndarray, InferTensorContents]:
        """Get the data of the inference input associated with this object.

        Returns:
            The data of the inference input.
        """
        return self._data

    @data.setter
    def data(self, data: List):
        """Set the data of the inference input associated with this object.

        Args:
             data: data of the inference input.
        """
        self._data = data

    @property
    def shape(self) -> List[int]:
        """Get the shape of inference input associated with this object.

        Returns:
            The shape of the inference input.
        """
        return self._shape

    @property
    def parameters(self) -> Union[Dict, MessageMap[str, InferParameter], None]:
        """Get the parameters of the inference input associated with this object.

        Returns:
            The additional inference parameters
        """
        return self._parameters

    @parameters.setter
    def parameters(
        self, params: Optional[Union[Dict, MessageMap[str, InferParameter]]]
    ):
        """Set the parameters of the inference input associated with this object.

        Args:
             params: parameters of the inference input
        """
        self._parameters = params

    @shape.setter
    def shape(self, shape: List[int]):
        """Set the shape of inference input.

        Args:
            shape : The shape of the associated inference input.
        """
        self._shape = shape

    def as_string(self) -> List[List[str]]:
        """
        Decodes the inference input data as a list of strings.

        Returns:
            List[List[str]]: The decoded data as a list of strings.

        Raises:
            InvalidInput: If the datatype of the inference input is not 'BYTES'.
        """
        if self.datatype == "BYTES":
            return [s.decode("utf-8") for li in self._data for s in li]
        else:
            raise InvalidInput(f"invalid datatype {self.datatype} in the input")

    def as_numpy(self) -> np.ndarray:
        """Decode the inference input data as numpy array.

        Returns:
            A numpy array of the inference input data
        Raises:
            InvalidInput: If the datatype of the inference input is not recognized.
        """
        dtype = to_np_dtype(self.datatype)
        if dtype is None:
            raise InvalidInput(f"invalid datatype {dtype} in the input")
        if self._raw_data is not None:
            if self.datatype == "BYTES":
                # String results contain a 4-byte string length
                # followed by the actual string characters. Hence,
                # need to decode the raw bytes to convert into
                # array elements.
                np_array = deserialize_bytes_tensor(self._raw_data)
            else:
                np_array = np.frombuffer(self._raw_data, dtype=dtype)
            return np_array.reshape(self._shape)
        else:
            np_array = np.array(self._data, dtype=dtype)
            return np_array.reshape(self._shape)

    def set_data_from_numpy(self, input_tensor: np.ndarray, binary_data: bool = True):
        """Set the tensor data from the specified numpy array for input associated with this object.

        Args:
            input_tensor : The tensor data in numpy array format.
            binary_data : Indicates whether to set data for the input in binary format
                          or explicit tensor within JSON. The default value is True,
                          which means the data will be delivered as binary data with gRPC or in the
                          HTTP body after the JSON object for REST.

        Raises:
            InferenceError if failed to set data for the tensor.
        """
        if not isinstance(input_tensor, (np.ndarray,)):
            raise InferenceError("input_tensor must be a numpy array")

        dtype = from_np_dtype(input_tensor.dtype)
        if self._datatype != dtype:
            raise InferenceError(
                "got unexpected datatype {} from numpy array, expected {}".format(
                    dtype, self._datatype
                )
            )
        valid_shape = True
        if len(self._shape) != len(input_tensor.shape):
            valid_shape = False
        else:
            for i in range(len(self._shape)):
                if self._shape[i] != input_tensor.shape[i]:
                    valid_shape = False
        if not valid_shape:
            raise InferenceError(
                "got unexpected numpy array shape [{}], expected [{}]".format(
                    str(input_tensor.shape)[1:-1], str(self._shape)[1:-1]
                )
            )

        if not binary_data:
            if self._parameters:
                self._parameters.pop("binary_data_size", None)
            self._raw_data = None
            if self._datatype == "BYTES":
                self._data = []
                try:
                    if input_tensor.size > 0:
                        for obj in np.nditer(
                            input_tensor, flags=["refs_ok"], order="C"
                        ):
                            # We need to convert the object to string using utf-8,
                            # if we want to use the binary_data=False. JSON requires
                            # the input to be a UTF-8 string.
                            if input_tensor.dtype == np.object_:
                                if type(obj.item()) == bytes:
                                    self._data.append(str(obj.item(), encoding="utf-8"))
                                else:
                                    self._data.append(str(obj.item()))
                            else:
                                self._data.append(str(obj.item(), encoding="utf-8"))
                except UnicodeDecodeError:
                    raise InferenceError(
                        f'Failed to encode "{obj.item()}" using UTF-8. Please use binary_data=True, if'
                        " you want to pass a byte array."
                    )
            else:
                self._data = [val.item() for val in input_tensor.flatten()]
        else:
            self._data = None
            if self._datatype == "BYTES":
                serialized_output = serialize_byte_tensor(input_tensor)
                if serialized_output.size > 0:
                    self._raw_data = serialized_output.item()
                else:
                    self._raw_data = b""
            else:
                self._raw_data = input_tensor.tobytes()
            if self._parameters is None:
                self._parameters = {"binary_data_size": len(self._raw_data)}
            else:
                self._parameters["binary_data_size"] = len(self._raw_data)

    def __eq__(self, other):
        if not isinstance(other, InferInput):
            return False
        if self.name != other.name:
            return False
        if self.shape != other.shape:
            return False
        if self.datatype != other.datatype:
            return False
        if self.parameters != other.parameters:
            return False
        if self.data != other.data:
            return False
        if self._raw_data != other._raw_data:
            return False
        return True

    @parameters.setter
    def parameters(self, value):
        self._parameters = value

    def to_dict(self) -> dict:
        return {
            "name": self.name,
            "shape": self.shape,
            "datatype": self.datatype,
            "data": self.data,
            "parameters": self.parameters,
        }

    def __repr__(self) -> str:
        return (
            f'"name": "{self.name}",'
            f'"shape": {self.shape},'
            f'"datatype": "{self.datatype}",'
            f'"data": {self.data},'
            f'"parameters": {self.parameters}'
        )

    def __str__(self) -> str:
        return self.__repr__()


def get_content(datatype: str, data: InferTensorContents):
    if datatype == "BOOL":
        return list(data.bool_contents)
    elif datatype in ["UINT8", "UINT16", "UINT32"]:
        return list(data.uint_contents)
    elif datatype == "UINT64":
        return list(data.uint64_contents)
    elif datatype in ["INT8", "INT16", "INT32"]:
        return list(data.int_contents)
    elif datatype == "INT64":
        return list(data.int64_contents)
    elif datatype == "FP16":
        # FP16 data should be present in raw_input_content, so return an empty list.
        return list()
    elif datatype == "FP32":
        return list(data.fp32_contents)
    elif datatype == "FP64":
        return list(data.fp64_contents)
    elif datatype == "BYTES":
        return list(data.bytes_contents)
    else:
        raise InvalidInput("invalid content type")


class RequestedOutput:
    def __init__(self, name: str, parameters: Optional[Dict] = None):
        """
        The RequestedOutput class represents an output that is requested as part of an inference request.

        Args:
            name (str): The name of the output.
            parameters (Optional[Dict]): Additional parameters for the output.
        """
        self._name = name
        self._parameters = parameters

    @property
    def name(self) -> str:
        """
        Get the name of the output.

        Returns:
            str: The name of the output.
        """
        return self._name

    @property
    def parameters(self) -> Optional[Dict]:
        """
        Get the additional parameters for the output.

        Returns:
            Optional[Dict]: The additional parameters for the output.
        """
        return self._parameters

    @parameters.setter
    def parameters(
        self, params: Optional[Union[Dict, MessageMap[str, InferParameter]]]
    ):
        """Set the parameters of the inference input associated with this object.

        Args:
             params: parameters of the inference input
        """
        self._parameters = params

    @property
    def binary_data(self) -> Optional[bool]:
        """Get the binary_data attribute from the parameters.
        This attribute indicates whether the data for the input should be in binary format.

        Returns:
            bool or None: True if the data should be in binary format, False otherwise.
                              If the binary_data attribute is not set, returns None.
        """
        if self.parameters and "binary_data" in self.parameters:
            return self.parameters["binary_data"]
        else:
            return None

    def __eq__(self, other):
        if not isinstance(other, RequestedOutput):
            return False
        if self.name != other.name:
            return False
        if self.parameters != other.parameters:
            return False
        return True

    def __repr__(self):
        return f"RequestedOutput(name={self.name}, parameters={self.parameters})"

    def __str__(self):
        return self.__repr__()


class InferRequest:
    id: Optional[str]
    model_name: str
    parameters: Optional[Dict]
    inputs: List[InferInput]
    from_grpc: bool
    model_version: Optional[str]

    def __init__(
        self,
        model_name: str,
        infer_inputs: List[InferInput],
        request_id: Optional[str] = None,
        raw_inputs=None,
        from_grpc: Optional[bool] = False,
        parameters: Optional[Union[Dict, MessageMap[str, InferParameter]]] = None,
        request_outputs: Optional[List[RequestedOutput]] = None,
        model_version: Optional[str] = None,
    ):
        """InferRequest Data Model.

        Args:
            model_name: The model name.
            infer_inputs: The inference inputs for the model.
            request_id: The id for the inference request.
            raw_inputs: The binary data for the inference inputs.
            from_grpc: Indicate if the data model is constructed from gRPC request.
            parameters: The additional inference parameters.
            request_outputs: The output tensors requested for this inference.
            model_version: The version of the model.
        """

        self.id = request_id
        self.model_name = model_name
        self.inputs = infer_inputs
        self.parameters = parameters
        self.from_grpc = from_grpc
        self._use_raw_outputs = False
        if raw_inputs:
            self._use_raw_outputs = True
            for i, raw_input in enumerate(raw_inputs):
                self.inputs[i]._raw_data = raw_input
        self.request_outputs = request_outputs
        self.model_version = model_version

    @property
    def use_binary_outputs(self) -> bool:
        """This attribute is used to determine if all the outputs should be returned as raw binary format.
        For REST,
            Get the binary_data_output attribute from the parameters. This will be ovverided by the individual output's 'binary_data' parameter.
        For GRPC,
            It is True, if the received inputs are raw_inputs, otherwise False. For GRPC, if the inputs are raw_inputs,
            then the outputs should be returned as raw_outputs.

        Returns:
            a boolean indicating whether to use binary raw outputs
        """
        # If the request is from gRPC and we receive the inputs as raw inputs, then the outputs should be returned as raw binary format.
        if self._use_raw_outputs and self.from_grpc:
            return True
        # If the request is from REST and the 'use_binary_outputs' parameter is set to True, then the outputs should be returned as raw binary format.
        elif self.parameters and not self.from_grpc:
            # If it is a grpc request, then this configuration has no effect on the output.
            return self.parameters.get("binary_data_output", False)
        else:
            return False

    @classmethod
    def from_grpc(cls, request: ModelInferRequest) -> "InferRequest":
        """
        Class method to construct an InferRequest object from a ModelInferRequest object.

        Args:
            request (ModelInferRequest): The gRPC ModelInferRequest object to be converted.

        Returns:
            InferRequest: The resulting InferRequest object.
        """
        infer_inputs = [
            InferInput(
                name=input_tensor.name,
                shape=list(input_tensor.shape),
                datatype=input_tensor.datatype,
                data=get_content(input_tensor.datatype, input_tensor.contents),
                parameters=(
                    to_http_parameters(input_tensor.parameters)
                    if input_tensor.parameters
                    else None
                ),
            )
            for input_tensor in request.inputs
        ]
        request_outputs = [
            RequestedOutput(
                name=output.name,
                parameters=(
                    to_http_parameters(output.parameters) if output.parameters else None
                ),
            )
            for output in request.outputs
        ]
        return cls(
            request_id=request.id,
            model_name=request.model_name,
            infer_inputs=infer_inputs,
            raw_inputs=request.raw_input_contents,
            from_grpc=True,
            parameters=request.parameters,
            request_outputs=request_outputs if request_outputs else None,
            model_version=request.model_version,
        )

    @classmethod
    def from_bytes(
        cls, req_bytes: bytes, json_length: int, model_name: str
    ) -> "InferRequest":
        """The class method to construct the InferRequest object from REST raw request bytes.

        Args:
            req_bytes (bytes): The raw InferRequest in bytes.
            json_length (int): The length of the json bytes.
            model_name (str): The name of the model.

        Returns:
            InferRequest: The resulting InferRequest object.

        Raises:
            InvalidInput: If the request format is unrecognized or if necessary fields are missing.
        """
        json_bytes = req_bytes[:json_length]
        try:
            infer_req_dict = orjson.loads(json_bytes)
        except orjson.JSONDecodeError as e:
            raise InvalidInput(f"Unrecognized request format: {e}")
        infer_inputs = []
        # Read the raw binary inputs appended after json
        start_index = json_length
        for input_ in infer_req_dict["inputs"]:
            parameters = input_.get("parameters", None)
            infer_input = InferInput(
                name=input_["name"],
                shape=input_["shape"],
                datatype=input_["datatype"],
                parameters=parameters,
            )
            infer_input_data = input_.get("data", None)
            if infer_input_data is not None:
                if infer_input.datatype == "FP16":
                    raise InvalidInput(
                        f"Receiving FP16 data via JSON is not supported. Please use the binary data format "
                        f"for input {infer_input.name}"
                    )
                infer_input.data = infer_input_data
            elif parameters and "binary_data_size" in parameters:
                binary_data_size = parameters.get("binary_data_size", None)
                if binary_data_size is None:
                    raise InvalidInput(
                        f"'binary_data_size' is not specified for input '{infer_input.name}' for model '{model_name}'"
                    )
                end_index = start_index + binary_data_size
                infer_input._raw_data = req_bytes[start_index:end_index]
                infer_input.set_data_from_numpy(
                    infer_input.as_numpy(), binary_data=False
                )
                start_index = end_index
            else:
                raise InvalidInput(
                    f"'data' field is missing for input '{infer_input.name}' for model '{model_name}'"
                )
            infer_inputs.append(infer_input)
        requested_outputs = None
        if infer_req_dict.get("outputs", None) is not None:
            requested_outputs = [
                RequestedOutput(
                    name=output["name"],
                    parameters=output.get("parameters", None),
                )
                for output in infer_req_dict["outputs"]
            ]
        return cls(
            model_name=model_name,
            request_id=infer_req_dict.get("id", None),
            parameters=infer_req_dict.get("parameters", None),
            infer_inputs=infer_inputs,
            request_outputs=requested_outputs,
        )

    @classmethod
    def from_inference_request(
        cls, request: InferenceRequest, model_name: str
    ) -> "InferRequest":
        """The class method to construct the InferRequest object from InferenceRequest object.

        Args:
            request (InferenceRequest): The InferenceRequest object.
            model_name (str): The name of the model.
        Returns:
            InferRequest: The resulting InferRequest object.
        Raises:
            InvalidInput: If the request format is unrecognized.
        """
        infer_inputs = []
        for infer_input in request.inputs:
            if infer_input.datatype == "FP16" and len(infer_input.data) != 0:
                raise InvalidInput(
                    f"Sending FP16 data via JSON is not supported. "
                    f"Please use the binary data format for input {infer_input.name}"
                )
            infer_inputs.append(
                InferInput(
                    name=infer_input.name,
                    shape=infer_input.shape,
                    datatype=infer_input.datatype,
                    data=infer_input.data,
                    parameters=(
                        {} if infer_input.parameters is None else infer_input.parameters
                    ),
                )
            )

        requested_outputs = None
        if request.outputs:
            requested_outputs = [
                RequestedOutput(
                    name=output.name,
                    parameters=({} if output.parameters is None else output.parameters),
                )
                for output in request.outputs
            ]
        return cls(
            request_id=request.id,
            model_name=model_name,
            infer_inputs=infer_inputs,
            parameters=request.parameters,
            request_outputs=requested_outputs,
        )

    def to_rest(self) -> Tuple[Union[bytes, Dict], Optional[int]]:
        """
        Converts the InferRequest object to v2 REST InferRequest Dict or bytes.
        This method is used to convert the InferRequest object into a format that can be sent over a REST API.

        Returns:
            Tuple[Union[bytes, Dict], Optional[int]]: A tuple containing the InferRequest in bytes or Dict and the length of the JSON part of the request.
                                                      If it is dict, then the json length will be None.

        Raises:
            InvalidInput: If the data is missing for an input or if both 'data' and 'raw_data' fields are set for an input.
        """
        infer_inputs = []
        raw_inputs = []
        for infer_input in self.inputs:
            if infer_input.data is None and infer_input._raw_data is None:
                raise InvalidInput(
                    f"'data' field is missing for output '{infer_input.name}' for model '{self.model_name}'"
                )
            if isinstance(infer_input.data, np.ndarray):
                infer_input.set_data_from_numpy(infer_input.data, binary_data=False)
            if infer_input.data and infer_input._raw_data:
                raise InvalidInput(
                    f"Both 'data' and 'raw_data' fields are set for input '{infer_input.name}' for model '{self.model_name}'"
                )

            if infer_input.datatype == "FP16" and infer_input.data:
                raise InvalidInput(
                    f"Sending FP16 data via JSON is not supported. Please use the binary data format for input {infer_input.name}"
                )

            infer_input_dict = {
                "name": infer_input.name,
                "shape": infer_input.shape,
                "datatype": infer_input.datatype,
            }
            if infer_input.parameters:
                infer_input_dict["parameters"] = to_http_parameters(
                    infer_input.parameters
                )
            if infer_input._raw_data:
                raw_inputs.append(infer_input._raw_data)
            else:
                infer_input_dict["data"] = infer_input.data
            infer_inputs.append(infer_input_dict)
        requested_outputs = []
        if self.request_outputs:
            for requested_output in self.request_outputs:
                requested_output_dict = {
                    "name": requested_output.name,
                }
                if requested_output.parameters:
                    requested_output_dict["parameters"] = to_http_parameters(
                        requested_output.parameters
                    )
                requested_outputs.append(requested_output_dict)
        res = {
            "id": self.id if self.id else str(uuid.uuid4()),
            "model_name": self.model_name,
            "inputs": infer_inputs,
        }
        if requested_outputs:
            res["outputs"] = requested_outputs
        if self.parameters:
            res["parameters"] = to_http_parameters(self.parameters)

        if len(raw_inputs) != 0:
            infer_response_bytes = orjson.dumps(res)
            json_length = len(infer_response_bytes)
            infer_response_bytes = b"".join([infer_response_bytes] + raw_inputs)
            return infer_response_bytes, json_length

        return res, None

    def to_grpc(self) -> ModelInferRequest:
        """Converts the InferRequest object to gRPC ModelInferRequest type.

        Returns:
            ModelInferRequest gRPC type converted from InferRequest object.
        """
        infer_inputs = []
        raw_input_contents = []
        for infer_input in self.inputs:
            if isinstance(infer_input.data, np.ndarray):
                infer_input.set_data_from_numpy(infer_input.data, binary_data=True)
            infer_input_dict = {
                "name": infer_input.name,
                "shape": infer_input.shape,
                "datatype": infer_input.datatype,
            }
            if infer_input.parameters:
                infer_input_dict["parameters"] = to_grpc_parameters(
                    infer_input.parameters
                )
            if infer_input._raw_data is not None:
                raw_input_contents.append(infer_input._raw_data)
            else:
                if not isinstance(infer_input.data, List):
                    raise InvalidInput("input data is not a List")
                infer_input_dict["contents"] = {}
                data_key = GRPC_CONTENT_DATATYPE_MAPPINGS.get(
                    infer_input.datatype, None
                )
                if data_key is not None:
                    infer_input._data = [
                        bytes(val, "utf-8") if isinstance(val, str) else val
                        for val in infer_input.data
                    ]  # str to byte conversion for grpc proto
                    infer_input_dict["contents"][data_key] = infer_input.data
                else:
                    raise InvalidInput("invalid input datatype")
            infer_inputs.append(infer_input_dict)
        request_outputs = []
        if self.request_outputs:
            for request_output in self.request_outputs:
                request_output_dict = {"name": request_output.name}
                if request_output.parameters:
                    request_output_dict["parameters"] = to_grpc_parameters(
                        request_output.parameters
                    )
                request_outputs.append(request_output_dict)

        return ModelInferRequest(
            id=self.id,
            model_name=self.model_name,
            model_version=self.model_version,
            inputs=infer_inputs,
            raw_input_contents=raw_input_contents,
            parameters=to_grpc_parameters(self.parameters) if self.parameters else None,
            outputs=request_outputs if request_outputs else None,
        )

    def as_dataframe(self) -> pd.DataFrame:
        """Decode the tensor inputs as pandas dataframe.

        Returns:
            The inference input data as pandas dataframe
        """
        dfs = []
        for input in self.inputs:
            input_data = input.data
            if input.datatype == "BYTES":
                input_data = [
                    str(val, "utf-8") if isinstance(val, bytes) else val
                    for val in input.data
                ]
            dfs.append(pd.DataFrame(input_data, columns=[input.name]))
        return pd.concat(dfs, axis=1)

    def get_input_by_name(self, name: str) -> Optional[InferInput]:
        """Find an input Tensor in the InferenceRequest that has the given name
        Args:
            name : str
                name of the input Tensor object
        Returns:
            InferInput
                The InferInput with the specified name, or None if no
                input with this name exists
        """
        for infer_input in self.inputs:
            if name == infer_input.name:
                return infer_input
        return None

    def __eq__(self, other):
        if not isinstance(other, InferRequest):
            return False
        if self.model_name != other.model_name:
            return False
        if self.id != other.id:
            return False
        if self.from_grpc != other.from_grpc:
            return False
        if self.parameters != other.parameters:
            return False
        if self.inputs != other.inputs:
            return False
        if self.request_outputs != other.request_outputs:
            return False
        return True

    def to_dict(self) -> dict:
        return {
            "id": self.id,
            "model_name": self.model_name,
            "inputs": [infer_input.to_dict() for infer_input in self.inputs],
            "parameters": self.parameters,
            "from_grpc": self.from_grpc,
        }

    def __repr__(self) -> str:
        return (
            f'"id": "{self.id}",'
            f'"model_name": "{self.model_name}",'
            f'"inputs": {self.inputs.__repr__()},'
            f'"parameters": {self.parameters},'
            f'"from_grpc": {self.from_grpc}'
        )

    def __str__(self) -> str:
        return self.__repr__()


class InferOutput:
    def __init__(
        self,
        name: str,
        shape: List[int],
        datatype: str,
        data: Union[List, np.ndarray, InferTensorContents] = None,
        parameters: Optional[Union[Dict, MessageMap[str, InferParameter]]] = None,
    ):
        """An object of InferOutput class is used to describe the output tensor for an inference response.

        Args:
            name : The name of inference output whose data will be described by this object.
            shape : The shape of the associated inference output.
            datatype : The data type of the associated inference output.
            data : The data of the inference output. When data is not set,
                   raw_data is used for gRPC with numpy array bytes by calling set_data_from_numpy.
            parameters : The additional inference parameters.
        """

        self._name = name
        self._shape = shape
        self._datatype = datatype.upper()
        self._parameters = parameters
        self._data = data
        self._raw_data = None

    @property
    def name(self) -> str:
        """Get the name of inference output associated with this object.

        Returns:
            The name of inference output.
        """
        return self._name

    @property
    def datatype(self) -> str:
        """Get the data type of inference output associated with this object.

        Returns:
            The data type of inference output.
        """
        return self._datatype

    @property
    def data(self) -> Union[List, np.ndarray, InferTensorContents]:
        """Get the data of inference output associated with this object.

        Returns:
            The data of inference output.
        """
        return self._data

    @data.setter
    def data(self, data: Union[List, np.ndarray, InferTensorContents]):
        """Set the data of inference output associated with this object.

        Args:
            data: inference output data.
        """
        self._data = data

    @property
    def shape(self) -> List[int]:
        """Get the shape of inference output associated with this object.

        Returns:
            The shape of inference output
        """
        return self._shape

    @shape.setter
    def shape(self, shape: List[int]):
        """Set the shape of inference output.

        Args:
            shape: The shape of the associated inference output.
        """
        self._shape = shape

    @property
    def parameters(self) -> Union[Dict, MessageMap[str, InferParameter], None]:
        """Get the parameters of inference output associated with this object.

        Returns:
            The additional inference parameters associated with the inference output.
        """
        return self._parameters

    @parameters.setter
    def parameters(self, params: Union[Dict, MessageMap[str, InferParameter]]):
        """Set the parameters of inference output associated with this object.

        :param params: The parameters of inference output
        """
        self._parameters = params

    def as_numpy(self) -> np.ndarray:
        """Decode the tensor output data as numpy array.

        Returns:
            The numpy array of the associated inference output data.
        """
        dtype = to_np_dtype(self.datatype)
        if dtype is None:
            raise InvalidInput("invalid datatype in the input")
        if self._raw_data is not None:
            if self.datatype == "BYTES":
                # String results contain a 4-byte string length
                # followed by the actual string characters. Hence,
                # need to decode the raw bytes to convert into
                # array elements.
                np_array = deserialize_bytes_tensor(self._raw_data)
            else:
                np_array = np.frombuffer(self._raw_data, dtype=dtype)
            return np_array.reshape(self._shape)
        else:
            np_array = np.array(self._data, dtype=dtype)
            return np_array.reshape(self._shape)

    def set_data_from_numpy(self, output_tensor: np.ndarray, binary_data: bool = True):
        """Set the tensor data from the specified numpy array for the inference output associated with this object.

        Args:
            output_tensor : The tensor data in numpy array format.
            binary_data : Indicates whether to set data for the input in binary format
                          or explicit tensor within JSON. The default value is True,
                          which means the data will be delivered as binary data with gRPC or in the
                          HTTP body after the JSON object for REST.

        Raises:
            InferenceError if failed to set data for the output tensor.
        """
        if not isinstance(output_tensor, (np.ndarray,)):
            raise InferenceError("input_tensor must be a numpy array")

        dtype = from_np_dtype(output_tensor.dtype)
        if np.issubdtype(output_tensor.dtype, np.datetime64):
            output_tensor = output_tensor.astype(np.object_)
        if self._datatype != dtype:
            raise InferenceError(
                "got unexpected datatype {} from numpy array, expected {}".format(
                    dtype, self._datatype
                )
            )
        valid_shape = True
        if len(self._shape) != len(output_tensor.shape):
            valid_shape = False
        else:
            for i in range(len(self._shape)):
                if self._shape[i] != output_tensor.shape[i]:
                    valid_shape = False
        if not valid_shape:
            raise InferenceError(
                "got unexpected numpy array shape [{}], expected [{}]".format(
                    str(output_tensor.shape)[1:-1], str(self._shape)[1:-1]
                )
            )

        if not binary_data:
            if self._parameters:
                self._parameters.pop("binary_data_size", None)
            self._raw_data = None
            if self._datatype == "BYTES":
                self._data = []
                try:
                    if output_tensor.size > 0:
                        for obj in np.nditer(
                            output_tensor, flags=["refs_ok"], order="C"
                        ):
                            # We need to convert the object to string using utf-8,
                            # if we want to use the binary_data=False. JSON requires
                            # the input to be a UTF-8 string.
                            if output_tensor.dtype == np.object_:
                                if type(obj.item()) == bytes:
                                    self._data.append(str(obj.item(), encoding="utf-8"))
                                else:
                                    self._data.append(str(obj.item()))
                            else:
                                self._data.append(str(obj.item(), encoding="utf-8"))
                except UnicodeDecodeError:
                    raise InferenceError(
                        f'Failed to encode "{obj.item()}" using UTF-8. Please use binary_data=True, if'
                        " you want to pass a byte array."
                    )
            else:
                self._data = [val.item() for val in output_tensor.flatten()]
        else:
            self._data = None
            if self._datatype == "BYTES":
                serialized_output = serialize_byte_tensor(output_tensor)
                if serialized_output.size > 0:
                    self._raw_data = serialized_output.item()
                else:
                    self._raw_data = b""
            else:
                self._raw_data = output_tensor.tobytes()
            if self._parameters is None:
                self._parameters = {"binary_data_size": len(self._raw_data)}
            else:
                self._parameters["binary_data_size"] = len(self._raw_data)

    def __eq__(self, other):
        if not isinstance(other, InferOutput):
            return False
        if self.name != other.name:
            return False
        if self.shape != other.shape:
            return False
        if self.datatype != other.datatype:
            return False
        if self.parameters != other.parameters:
            return False
        if self.data != other.data:
            return False
        if self._raw_data != other._raw_data:
            return False
        return True

    def to_dict(self) -> dict:
        return {
            "name": self.name,
            "shape": self.shape,
            "datatype": self.datatype,
            "data": self.data,
            "parameters": self.parameters,
        }

    def __repr__(self) -> str:
        return (
            f'"name": "{self.name}",'
            f'"shape": {self.shape},'
            f'"datatype": "{self.datatype}",'
            f'"data": {self.data},'
            f'"parameters": {self.parameters}'
        )

    def __str__(self) -> str:
        return self.__repr__()


class InferResponse:
    id: str
    model_name: str
    model_version: Optional[str]
    parameters: Optional[Dict]
    outputs: List[InferOutput]
    from_grpc: bool

    def __init__(
        self,
        response_id: str,
        model_name: str,
        infer_outputs: List[InferOutput],
        model_version: Optional[str] = None,
        raw_outputs=None,
        from_grpc: Optional[bool] = False,
        parameters: Optional[Union[Dict, MessageMap[str, InferParameter]]] = None,
        use_binary_outputs: Optional[bool] = False,
        requested_outputs: Optional[List[RequestedOutput]] = None,
    ):
        """The InferResponse Data Model

        Args:
            response_id: The id of the inference response.
            model_name: The name of the model.
            infer_outputs: The inference outputs of the inference response.
            model_version: The version of the model.
            raw_outputs: The raw binary data of the inference outputs.
            from_grpc: Indicate if the InferResponse is constructed from a gRPC response.
            parameters: The additional inference parameters.
            use_binary_outputs: A boolean indicating whether the data for the outputs should be in binary format when sent over REST API.
                                This will be overridden by the individual output's binary_data attribute.
            requested_outputs: The output tensors requested for this inference.
        """

        self.id = response_id
        self.model_name = model_name
        self.model_version = model_version
        self.outputs = infer_outputs
        self.parameters = parameters
        self.from_grpc = from_grpc
        self._requested_outputs = requested_outputs
        self._use_binary_outputs: bool = use_binary_outputs
        if raw_outputs:
            for i, raw_output in enumerate(raw_outputs):
                self.outputs[i]._raw_data = raw_output

    @classmethod
    def from_grpc(cls, response: ModelInferResponse) -> "InferResponse":
        """The class method to construct the InferResponse object from gRPC message type.

        Args:
            response: The GRPC response as ModelInferResponse object.
        Returns:
            InferResponse object.
        """
        infer_outputs = [
            InferOutput(
                name=output.name,
                shape=list(output.shape),
                datatype=output.datatype,
                data=get_content(output.datatype, output.contents),
                parameters=output.parameters,
            )
            for output in response.outputs
        ]
        return cls(
            model_name=response.model_name,
            model_version=response.model_version,
            response_id=response.id,
            parameters=response.parameters,
            infer_outputs=infer_outputs,
            raw_outputs=response.raw_output_contents,
            from_grpc=True,
        )

    @classmethod
    def from_rest(cls, response: Dict) -> "InferResponse":
        """The class method to construct the InferResponse object from REST message type.

        Args:
            response: The response as a dict.
        Returns:
            InferResponse object.
        """
        infer_outputs = [
            InferOutput(
                name=output["name"],
                shape=list(output["shape"]),
                datatype=output["datatype"],
                data=output["data"],
                parameters=output.get("parameters", None),
            )
            for output in response["outputs"]
        ]
        return cls(
            model_name=response.get("model_name"),
            model_version=response.get("model_version", None),
            response_id=response.get("id", None),
            parameters=response.get("parameters", None),
            infer_outputs=infer_outputs,
        )

    @classmethod
    def from_bytes(
        cls,
        res_bytes: bytes,
        json_length: int,
    ) -> "InferResponse":
        """
        Class method to construct an InferResponse object from raw response bytes.
        This method is used to convert the raw response bytes received from a REST API into an InferResponse object.

        Args:
            res_bytes (bytes): The raw response bytes received from the REST API.
            json_length (int): The length of the JSON part of the response.

        Returns:
            InferResponse: The constructed InferResponse object.

        Raises:
            InvalidInput: If the response format is unrecognized or if necessary fields are missing in the response.
            InferenceError: if failed to set data for the output tensor.
        """
        # If json_length is equal to the length of the response bytes, then the response does not have
        # any appended binary data after the json.
        json_bytes = res_bytes[:json_length]
        try:
            infer_res_dict = orjson.loads(json_bytes)
        except orjson.JSONDecodeError as e:
            raise InvalidInput(f"Unrecognized request format: {e}")
        model_name = infer_res_dict["model_name"]
        infer_outputs = []
        # Read the raw binary outputs appended after json
        start_index = json_length
        for output in infer_res_dict["outputs"]:
            parameters = output.get("parameters", None)
            infer_output = InferOutput(
                name=output["name"],
                shape=output["shape"],
                datatype=output["datatype"],
                parameters=parameters,
            )
            if parameters and "binary_data_size" in parameters:
                binary_data_size = parameters.get("binary_data_size")
                end_index = start_index + binary_data_size
                infer_output._raw_data = res_bytes[start_index:end_index]
                infer_output.set_data_from_numpy(
                    infer_output.as_numpy(), binary_data=False
                )
                start_index = end_index
            else:
                infer_output_data = output.get("data", None)
                if infer_output_data is None:
                    raise InvalidInput(
                        f"'data' field is missing for output '{infer_output.name}' for model '{model_name}'"
                    )
                infer_output.data = infer_output_data
            infer_outputs.append(infer_output)
        return cls(
            model_name=model_name,
            response_id=infer_res_dict.get("id", None),
            parameters=infer_res_dict.get("parameters", None),
            infer_outputs=infer_outputs,
        )

    def to_rest(self) -> Tuple[Union[bytes, Dict], Optional[int]]:
        """
        Converts the InferResponse object to v2 REST InferResponse Dict or bytes.
        This method is used to convert the InferResponse object into a format that can be sent over a REST API.

        Returns:
            Tuple[Union[bytes, Dict], Optional[int]]: A tuple containing the InferResponse in bytes or Dict and the length of the JSON part of the response.
                                                      If it is dict, then the json length will be None.

        Raises:
            InvalidInput: If the output data is not a numpy array, bytes, or list.
        """
        infer_outputs = []
        raw_outputs = []
        use_binary_data = self._use_binary_outputs
        outputs = self._requested_outputs if self._requested_outputs else self.outputs

        for output in outputs:
            infer_output = (
                self.get_output_by_name(output.name)
                if self._requested_outputs
                else output
            )
            if self._requested_outputs:
                use_binary_data = output.binary_data
            if infer_output is None:
                raise InvalidInput(
                    f"Unexpected inference output '{output.name}' for model '{self.model_name}'"
                )
            if infer_output.data is None and infer_output._raw_data is None:
                raise InvalidInput(
                    f"'data' field is missing for output '{infer_output.name}' for model '{self.model_name}'"
                )
            if isinstance(infer_output.data, np.ndarray):
                infer_output.set_data_from_numpy(
                    infer_output.data, binary_data=use_binary_data
                )
            elif infer_output.data or infer_output._raw_data:
                infer_output.set_data_from_numpy(
                    infer_output.as_numpy(), binary_data=use_binary_data
                )
            if infer_output.datatype == "FP16" and infer_output.data:
                raise InvalidInput(
                    f"Sending FP16 data via JSON is not supported. Please use the binary data format for output {infer_output.name}"
                )

            infer_output_dict = {
                "name": infer_output.name,
                "shape": infer_output.shape,
                "datatype": infer_output.datatype,
            }
            if infer_output.parameters:
                infer_output_dict["parameters"] = to_http_parameters(
                    infer_output.parameters
                )
            if use_binary_data:
                raw_outputs.append(infer_output._raw_data)
            else:
                infer_output_dict["data"] = infer_output.data
            infer_outputs.append(infer_output_dict)

        res = {
            "id": self.id,
            "model_name": self.model_name,
            "model_version": self.model_version,
            "outputs": infer_outputs,
        }
        if self.parameters:
            res["parameters"] = to_http_parameters(self.parameters)

        if len(raw_outputs) != 0:
            infer_response_bytes = orjson.dumps(res)
            json_length = len(infer_response_bytes)
            infer_response_bytes = b"".join([infer_response_bytes] + raw_outputs)
            return infer_response_bytes, json_length

        return res, None

    def to_grpc(self) -> ModelInferResponse:
        """Converts the InferResponse object to gRPC ModelInferResponse type.

        Returns:
            The ModelInferResponse gRPC message.
        Raises:
            InvalidInput: If the output data is not a List or if the datatype is invalid.
        """
        infer_outputs = []
        raw_output_contents = []
        use_raw_outputs = self._use_binary_outputs
        if not self._use_binary_outputs:
            # If FP16 datatype is present in the outputs use raw outputs.
            if _contains_fp16_datatype(self):
                use_raw_outputs = True
        for infer_output in self.outputs:
            if (
                use_raw_outputs
                and infer_output.data
                and isinstance(infer_output.data, list)
            ):
                infer_output.data = infer_output.as_numpy()
            if isinstance(infer_output.data, np.ndarray):
                infer_output.set_data_from_numpy(infer_output.data, binary_data=True)
            infer_output_dict = {
                "name": infer_output.name,
                "shape": infer_output.shape,
                "datatype": infer_output.datatype,
            }
            if infer_output.parameters:
                infer_output_dict["parameters"] = to_grpc_parameters(
                    infer_output.parameters
                )
            if infer_output._raw_data is not None:
                raw_output_contents.append(infer_output._raw_data)
            else:
                if not isinstance(infer_output.data, List):
                    raise InvalidInput("output data is not a List")
                infer_output_dict["contents"] = {}
                data_key = GRPC_CONTENT_DATATYPE_MAPPINGS.get(
                    infer_output.datatype, None
                )
                if data_key is not None:
                    infer_output._data = [
                        bytes(val, "utf-8") if isinstance(val, str) else val
                        for val in infer_output.data
                    ]  # str to byte conversion for grpc proto
                    infer_output_dict["contents"][data_key] = infer_output.data
                else:
                    raise InvalidInput("to_grpc: invalid output datatype")
            infer_outputs.append(infer_output_dict)

        return ModelInferResponse(
            id=self.id,
            model_name=self.model_name,
            model_version=self.model_version,
            outputs=infer_outputs,
            raw_output_contents=raw_output_contents,
            parameters=to_grpc_parameters(self.parameters) if self.parameters else None,
        )

    def get_output_by_name(self, name: str) -> Optional[InferOutput]:
        """Find an output Tensor in the InferResponse that has the given name

        Args:
            name : str
                name of the output Tensor object
        Returns:
            InferOutput
                The InferOutput with the specified name, or None if no
                output with this name exists
        """
        for infer_output in self.outputs:
            if name == infer_output.name:
                return infer_output
        return None

    def __eq__(self, other):
        if not isinstance(other, InferResponse):
            return False
        if self.model_name != other.model_name:
            return False
        if self.model_version != other.model_version:
            return False
        if self.id != other.id:
            return False
        if self.from_grpc != other.from_grpc:
            return False
        if self.parameters != other.parameters:
            return False
        if self.outputs != other.outputs:
            return False
        return True

    def to_dict(self) -> dict:
        return {
            "id": self.id,
            "model_name": self.model_name,
            "outputs": [infer_output.to_dict() for infer_output in self.outputs],
            "parameters": self.parameters,
            "from_grpc": self.from_grpc,
        }

    def __repr__(self) -> str:
        return (
            f'"id": "{self.id}",'
            f'"model_name": "{self.model_name}",'
            f'"outputs": {self.outputs.__repr__()},'
            f'"parameters": {self.parameters},'
            f'"from_grpc": {self.from_grpc}'
        )

    def __str__(self) -> str:
        return self.__repr__()


def to_grpc_parameters(
    parameters: Union[
        Dict[str, Union[str, bool, int]], MessageMap[str, InferParameter]
    ],
) -> Dict[str, InferParameter]:
    """
    Converts REST parameters to GRPC InferParameter objects

    :param parameters: parameters to be converted.
    :return: converted parameters as Dict[str, InferParameter]
    :raises InvalidInput: if the parameter type is not supported.
    """
    grpc_params: Dict[str, InferParameter] = {}
    for key, val in parameters.items():
        if isinstance(val, str):
            grpc_params[key] = InferParameter(string_param=val)
        elif isinstance(val, bool):
            grpc_params[key] = InferParameter(bool_param=val)
        elif isinstance(val, int):
            grpc_params[key] = InferParameter(int64_param=val)
        elif isinstance(val, InferParameter):
            grpc_params[key] = val
        else:
            raise InvalidInput(f"to_grpc: invalid parameter value: {val}")
    return grpc_params


def to_http_parameters(
    parameters: Union[dict, MessageMap[str, InferParameter]],
) -> Dict[str, Union[str, bool, int]]:
    """
    Converts GRPC InferParameter parameters to REST parameters

    :param parameters: parameters to be converted.
    :return: converted parameters as Dict[str, Union[str, bool, int]]
    """
    http_params: Dict[str, Union[str, bool, int]] = {}
    for key, val in parameters.items():
        if isinstance(val, InferParameter):
            if val.HasField("bool_param"):
                http_params[key] = val.bool_param
            elif val.HasField("int64_param"):
                http_params[key] = val.int64_param
            elif val.HasField("string_param"):
                http_params[key] = val.string_param
        else:
            http_params[key] = val
    return http_params


def _contains_fp16_datatype(infer_response: InferResponse) -> bool:
    """
    Checks whether the InferResponse outputs contains FP16 datatype.

    :param infer_response: An InferResponse object containing model inference results.
    :return: A boolean indicating whether any output in the InferResponse uses the FP16 datatype.
    """
    for infer_output in infer_response.outputs:
        if infer_output.datatype == "FP16":
            return True
    return False
