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

from typing import Optional, List, Dict

import numpy
import numpy as np
from tritonclient.utils import raise_error, serialize_byte_tensor

from ..constants.constants import GRPC_CONTENT_DATATYPE_MAPPINGS
from ..errors import InvalidInput
from ..protocol.grpc.grpc_predict_v2_pb2 import ModelInferRequest, InferTensorContents, ModelInferResponse
from ..utils.numpy_codec import to_np_dtype, from_np_dtype


class InferInput:
    _name: str
    _shape: List[int]
    _datatype: str
    _parameters: Dict

    def __init__(self, name, shape, datatype, data=None, parameters={}):
        """An object of InferInput class is used to describe
        input tensor for an inference request.
        Parameters
        ----------
        name : str
            The name of input whose data will be described by this object
        shape : list
            The shape of the associated input.
        datatype : str
            The datatype of the associated input.
        data: Union[List, InferTensorContents]
            The data of the REST/gRPC input. When data is not set, raw_data is used for gRPC for numpy array bytes.
        """
        self._name = name
        self._shape = shape
        self._datatype = datatype
        self._parameters = parameters
        self._data = data
        self._raw_data = None

    @property
    def name(self):
        """Get the name of input associated with this object.
        Returns
        -------
        str
            The name of input
        """
        return self._name

    @property
    def datatype(self):
        """Get the datatype of input associated with this object.
        Returns
        -------
        str
            The datatype of input
        """
        return self._datatype

    @property
    def data(self):
        """Get the data of InferInput

        """
        return self._data

    @property
    def shape(self):
        """Get the shape of input associated with this object.
        Returns
        -------
        list
            The shape of input
        """
        return self._shape

    @property
    def parameters(self):
        """Get the parameters of input associated with this object.
        Returns
        -------
        dict
            The key, value pair of string and InferParameter
        """
        return self._parameters

    def set_shape(self, shape):
        """Set the shape of input.
        Parameters
        ----------
        shape : list
            The shape of the associated input.
        """
        self._shape = shape

    def as_numpy(self) -> np.ndarray:
        dtype = to_np_dtype(self.datatype)
        if dtype is None:
            raise InvalidInput("invalid datatype in the input")
        if self._raw_data is not None:
            np_array = np.frombuffer(self._raw_data, dtype=dtype)
            return np_array.reshape(self._shape)
        else:
            np_array = np.array(self._data, dtype=dtype)
            return np_array.reshape(self._shape)

    def set_data_from_numpy(self, input_tensor, binary_data=True):
        """Set the tensor data from the specified numpy array for
        input associated with this object.
        Parameters
        ----------
        input_tensor : numpy array
            The tensor data in numpy array format
        binary_data : bool
            Indicates whether to set data for the input in binary format
            or explicit tensor within JSON. The default value is True,
            which means the data will be delivered as binary data in the
            HTTP body after the JSON object.
        Raises
        ------
        InferenceServerException
            If failed to set data for the tensor.
        """
        if not isinstance(input_tensor, (np.ndarray,)):
            raise_error("input_tensor must be a numpy array")

        dtype = from_np_dtype(input_tensor.dtype)
        if self._datatype != dtype:
            raise_error(
                "got unexpected datatype {} from numpy array, expected {}".format(dtype, self._datatype))
        valid_shape = True
        if len(self._shape) != len(input_tensor.shape):
            valid_shape = False
        else:
            for i in range(len(self._shape)):
                if self._shape[i] != input_tensor.shape[i]:
                    valid_shape = False
        if not valid_shape:
            raise_error(
                "got unexpected numpy array shape [{}], expected [{}]".format(
                    str(input_tensor.shape)[1:-1],
                    str(self._shape)[1:-1]))

        if not binary_data:
            self._parameters.pop('binary_data_size', None)
            self._raw_data = None
            if self._datatype == "BYTES":
                self._data = []
                try:
                    if input_tensor.size > 0:
                        for obj in np.nditer(input_tensor,
                                             flags=["refs_ok"],
                                             order='C'):
                            # We need to convert the object to string using utf-8,
                            # if we want to use the binary_data=False. JSON requires
                            # the input to be a UTF-8 string.
                            if input_tensor.dtype == np.object_:
                                if type(obj.item()) == bytes:
                                    self._data.append(
                                        str(obj.item(), encoding='utf-8'))
                                else:
                                    self._data.append(str(obj.item()))
                            else:
                                self._data.append(
                                    str(obj.item(), encoding='utf-8'))
                except UnicodeDecodeError:
                    raise_error(
                        f'Failed to encode "{obj.item()}" using UTF-8. Please use binary_data=True, if'
                        ' you want to pass a byte array.')
            else:
                self._data = [val.item() for val in input_tensor.flatten()]
        else:
            self._data = None
            if self._datatype == "BYTES":
                serialized_output = serialize_byte_tensor(input_tensor)
                if serialized_output.size > 0:
                    self._raw_data = serialized_output.item()
                else:
                    self._raw_data = b''
            else:
                self._raw_data = input_tensor.tobytes()
            self._parameters['binary_data_size'] = len(self._raw_data)


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
    elif datatype == "FP32":
        return list(data.fp32_contents)
    elif datatype == "FP64":
        return list(data.fp64_contents)
    elif datatype == "BYTES":
        return list(data.bytes_contents)
    else:
        raise InvalidInput("invalid content type")


class InferRequest:
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
    model_name: str
    parameters: Optional[Dict]
    inputs: List[InferInput]
    from_grpc: bool

    def __init__(self, model_name: str, infer_inputs: List[InferInput],
                 request_id=None, raw_inputs=None, from_grpc=False, parameters={}):
        self.id = request_id
        self.model_name = model_name
        self.inputs = infer_inputs
        self.parameters = parameters
        self.from_grpc = from_grpc
        if raw_inputs:
            for i, raw_input in enumerate(raw_inputs):
                self.inputs[i]._raw_data = raw_input

    @classmethod
    def from_grpc(cls, request: ModelInferRequest):
        infer_inputs = [InferInput(name=input_tensor.name, shape=list(input_tensor.shape),
                                   datatype=input_tensor.datatype,
                                   data=get_content(input_tensor.datatype, input_tensor.contents),
                                   parameters=input_tensor.parameters)
                        for input_tensor in request.inputs]
        return cls(request_id=request.id, model_name=request.model_name, infer_inputs=infer_inputs,
                   raw_inputs=request.raw_input_contents, from_grpc=True, parameters=request.parameters)

    def to_rest(self) -> Dict:
        """ Converts the InferRequest object to v2 REST InferenceRequest message

                """
        infer_inputs = []
        for infer_input in self.inputs:
            infer_input_dict = {
                "name": infer_input.name,
                "shape": infer_input.shape,
                "datatype": infer_input.datatype
            }
            if isinstance(infer_input.data, numpy.ndarray):
                infer_input.set_data_from_numpy(infer_input.data, binary_data=False)
                infer_input_dict["data"] = infer_input.data
            else:
                infer_input_dict["data"] = infer_input.data
            infer_inputs.append(infer_input_dict)
        return {
            'id': self.id,
            'inputs': infer_inputs
        }

    def to_grpc(self) -> ModelInferRequest:
        """ Converts the InferRequest object to gRPC ModelInferRequest message

        """
        infer_inputs = []
        raw_input_contents = []
        for infer_input in self.inputs:
            if isinstance(infer_input.data, numpy.ndarray):
                infer_input.set_data_from_numpy(infer_input.data, binary_data=True)
            infer_input_dict = {
                "name": infer_input.name,
                "shape": infer_input.shape,
                "datatype": infer_input.datatype,
            }
            if infer_input._raw_data is not None:
                raw_input_contents.append(infer_input._raw_data)
            else:
                if not isinstance(infer_input.data, List):
                    raise InvalidInput("input data is not a List")
                infer_input_dict["contents"] = {}
                data_key = GRPC_CONTENT_DATATYPE_MAPPINGS.get(infer_input.datatype, None)
                if data_key is not None:
                    infer_input_dict["contents"][data_key] = infer_input.data
                else:
                    raise InvalidInput("invalid input datatype")
            infer_inputs.append(infer_input_dict)

        return ModelInferRequest(model_name=self.model_name, inputs=infer_inputs,
                                 raw_input_contents=raw_input_contents)


class InferOutput:
    def __init__(self, name, shape, datatype, data=None, parameters={}):
        """An object of InferOutput class is used to describe
        input tensor for an inference request.
        Parameters
        ----------
        name : str
            The name of input whose data will be described by this object
        shape : list
            The shape of the associated input.
        datatype : str
            The datatype of the associated input.
        data: Union[List, InferTensorContents]
            The data of the REST/gRPC input. When data is not set, raw_data is used for gRPC for numpy array bytes.
        """
        self._name = name
        self._shape = shape
        self._datatype = datatype
        self._parameters = parameters
        self._data = data
        self._raw_data = None

    @property
    def name(self):
        """Get the name of input associated with this object.
        Returns
        -------
        str
            The name of input
        """
        return self._name

    @property
    def datatype(self):
        """Get the datatype of input associated with this object.
        Returns
        -------
        str
            The datatype of input
        """
        return self._datatype

    @property
    def data(self):
        """Get the data of InferOutput

        """
        return self._data

    @property
    def shape(self):
        """Get the shape of input associated with this object.
        Returns
        -------
        list
            The shape of input
        """
        return self._shape

    @property
    def parameters(self):
        """Get the parameters of input associated with this object.
        Returns
        -------
        dict
            The key, value pair of string and InferParameter
        """
        return self._parameters

    def set_shape(self, shape):
        """Set the shape of input.
        Parameters
        ----------
        shape : list
            The shape of the associated input.
        """
        self._shape = shape

    def as_numpy(self) -> numpy.ndarray:
        dtype = to_np_dtype(self.datatype)
        if dtype is None:
            raise InvalidInput("invalid datatype in the input")
        if self._raw_data is not None:
            np_array = np.frombuffer(self._raw_data, dtype=dtype)
            return np_array.reshape(self._shape)
        else:
            np_array = np.array(self._data, dtype=dtype)
            return np_array.reshape(self._shape)

    def set_data_from_numpy(self, input_tensor, binary_data=True):
        """Set the tensor data from the specified numpy array for
        input associated with this object.
        Parameters
        ----------
        input_tensor : numpy array
            The tensor data in numpy array format
        binary_data : bool
            Indicates whether to set data for the input in binary format
            or explicit tensor within JSON. The default value is True,
            which means the data will be delivered as binary data in the
            HTTP body after the JSON object.
        Raises
        ------
        InferenceServerException
            If failed to set data for the tensor.
        """
        if not isinstance(input_tensor, (np.ndarray,)):
            raise_error("input_tensor must be a numpy array")

        dtype = from_np_dtype(input_tensor.dtype)
        if self._datatype != dtype:
            raise_error(
                "got unexpected datatype {} from numpy array, expected {}".format(dtype, self._datatype))
        valid_shape = True
        if len(self._shape) != len(input_tensor.shape):
            valid_shape = False
        else:
            for i in range(len(self._shape)):
                if self._shape[i] != input_tensor.shape[i]:
                    valid_shape = False
        if not valid_shape:
            raise_error(
                "got unexpected numpy array shape [{}], expected [{}]".format(
                    str(input_tensor.shape)[1:-1],
                    str(self._shape)[1:-1]))

        if not binary_data:
            self._parameters.pop('binary_data_size', None)
            self._raw_data = None
            if self._datatype == "BYTES":
                self._data = []
                try:
                    if input_tensor.size > 0:
                        for obj in np.nditer(input_tensor,
                                             flags=["refs_ok"],
                                             order='C'):
                            # We need to convert the object to string using utf-8,
                            # if we want to use the binary_data=False. JSON requires
                            # the input to be a UTF-8 string.
                            if input_tensor.dtype == np.object_:
                                if type(obj.item()) == bytes:
                                    self._data.append(
                                        str(obj.item(), encoding='utf-8'))
                                else:
                                    self._data.append(str(obj.item()))
                            else:
                                self._data.append(
                                    str(obj.item(), encoding='utf-8'))
                except UnicodeDecodeError:
                    raise_error(
                        f'Failed to encode "{obj.item()}" using UTF-8. Please use binary_data=True, if'
                        ' you want to pass a byte array.')
            else:
                self._data = [val.item() for val in input_tensor.flatten()]
        else:
            self._data = None
            if self._datatype == "BYTES":
                serialized_output = serialize_byte_tensor(input_tensor)
                if serialized_output.size > 0:
                    self._raw_data = serialized_output.item()
                else:
                    self._raw_data = b''
            else:
                self._raw_data = input_tensor.tobytes()
            self._parameters['binary_data_size'] = len(self._raw_data)


class InferResponse:
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
    id: str
    model_name: str
    parameters: Optional[Dict]
    outputs: List[InferOutput]
    from_grpc: bool

    def __init__(self, response_id: str, model_name: str, infer_outputs: List[InferOutput],
                 raw_outputs=None, from_grpc=False, parameters={}):
        self.id = response_id
        self.model_name = model_name
        self.outputs = infer_outputs
        self.parameters = parameters
        self.from_grpc = from_grpc
        if raw_outputs:
            for i, raw_output in enumerate(raw_outputs):
                self.outputs[i]._raw_data = raw_output

    @classmethod
    def from_grpc(cls, response: ModelInferResponse):
        infer_outputs = [InferOutput(name=output.name, shape=list(output.shape),
                                     datatype=output.datatype,
                                     data=get_content(output.datatype, output.contents),
                                     parameters=output.parameters)
                         for output in response.outputs]
        return cls(model_name=response.model_name, response_id=response.id, parameters=response.parameters,
                   infer_outputs=infer_outputs, raw_outputs=response.raw_output_contents, from_grpc=True)

    def to_rest(self) -> Dict:
        """ Converts the InferResponse object to v2 REST InferenceRequest message

        """
        infer_outputs = []
        for i, infer_output in enumerate(self.outputs):
            infer_output_dict = {
                "name": infer_output.name,
                "shape": infer_output.shape,
                "datatype": infer_output.datatype
            }
            if isinstance(infer_output.data, numpy.ndarray):
                infer_output.set_data_from_numpy(infer_output.data, binary_data=False)
                infer_output_dict["data"] = infer_output.data
            elif isinstance(infer_output._raw_data, bytes):
                infer_output_dict["data"] = infer_output.as_numpy().tolist()
            else:
                infer_output_dict["data"] = infer_output.data
            infer_outputs.append(infer_output_dict)
        res = {
            'id': self.id,
            'model_name': self.model_name,
            'outputs': infer_outputs
        }
        return res

    def to_grpc(self) -> ModelInferResponse:
        """ Converts the InferResponse object to gRPC ModelInferRequest message

        """
        infer_outputs = []
        raw_output_contents = []
        for infer_output in self.outputs:
            if isinstance(infer_output.data, numpy.ndarray):
                infer_output.set_data_from_numpy(infer_output.data, binary_data=True)
            infer_output_dict = {
                "name": infer_output.name,
                "shape": infer_output.shape,
                "datatype": infer_output.datatype,
            }
            if infer_output._raw_data is not None:
                raw_output_contents.append(infer_output._raw_data)
            else:
                if not isinstance(infer_output.data, List):
                    raise InvalidInput("output data is not a List")
                infer_output_dict["contents"] = {}
                data_key = GRPC_CONTENT_DATATYPE_MAPPINGS.get(infer_output.datatype, None)
                if data_key is not None:
                    infer_output_dict["contents"][data_key] = infer_output.data
                else:
                    raise InvalidInput("to_grpc: invalid output datatype")
            infer_outputs.append(infer_output_dict)

        return ModelInferResponse(model_name=self.model_name, outputs=infer_outputs,
                                  raw_output_contents=raw_output_contents)
