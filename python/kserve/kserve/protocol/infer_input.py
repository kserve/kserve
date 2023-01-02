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

from ..errors import InvalidInput
from ..protocol.grpc.grpc_predict_v2_pb2 import ModelInferRequest, InferTensorContents
from ..utils.numpy_codec import to_np_dtype, from_np_dtype


class InferInput:
    _name: str
    _shape: List[int]
    _datatype: str
    _parameters: Dict

    def __init__(self, name, shape, datatype, data=None):
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
        self._parameters = {}
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
        if isinstance(self._data, InferTensorContents):
            if self._datatype == "BOOL":
                return self._data.bool_contents
            elif self._datatype == "UINT8" or self._datatype == "UINT16" or self._datatype == "UINT32":
                return self._data.uint_contents
            elif self._datatype == "UINT64":
                return self._data.uint64_contents
            elif self._datatype == "INT8" or self._datatype == "INT16" or self._datatype == "INT32":
                return self._data.int_contents
            elif self._datatype == "INT64":
                return self._data.int64_contents
            elif self._datatype == "FLOAT32":
                return self._data.fp32_contents
            elif self._datatype == "FLOAT64":
                return self._data.fp64_contents
            elif self._datatype == "BYTES":
                return self._data.bytes_contents
            else:
                raise InvalidInput("invalid input datatype")
        else:
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

    def set_shape(self, shape):
        """Set the shape of input.
        Parameters
        ----------
        shape : list
            The shape of the associated input.
        """
        self._shape = shape

    def as_numpy(self):
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
    inputs: List[InferInput]
    raw_inputs: List

    def __init__(self, infer_inputs: List[InferInput], raw_inputs=None):
        self.inputs = infer_inputs
        if raw_inputs:
            for i, raw_input in enumerate(raw_inputs):
                self.inputs[i]._raw_data = raw_input

    def to_rest(self) -> Dict:
        """ Converts the InferInput object to v2 REST InferenceRequest message

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
            infer_inputs.append(infer_input_dict)
        return {
            'inputs': infer_inputs
        }

    def to_grpc(self, model_name) -> ModelInferRequest:
        """ Converts the InferInput object to gRPC ModelInferRequest message

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
                if infer_input.datatype == "BOOL":
                    infer_input_dict["contents"]["bool_contents"] = infer_input.data
                elif infer_input.datatype == "UINT8" or infer_input.datatype == "UINT16" or \
                        infer_input.datatype == "UINT32":
                    infer_input_dict["contents"]["uint_contents"] = infer_input.data
                elif infer_input.datatype == "UINT64":
                    infer_input_dict["contents"]["uint64_contents"] = infer_input.data
                elif infer_input.datatype == "INT8" or infer_input.datatype == "INT16" or \
                        infer_input.datatype == "INT32":
                    infer_input_dict["contents"]["int_contents"] = infer_input.data
                elif infer_input.datatype == "INT64":
                    infer_input_dict["contents"]["uint64_contents"] = infer_input.data
                elif infer_input.datatype == "FLOAT32":
                    infer_input_dict["contents"]["fp32_contents"] = infer_input.data
                elif infer_input.datatype == "FLOAT64":
                    infer_input_dict["contents"]["fp64_contents"] = infer_input.data
                elif infer_input.datatype == "BYTES":
                    infer_input_dict["contents"]["bytes_contents"] = infer_input.data
                else:
                    raise InvalidInput("invalid input datatype")
            infer_inputs.append(infer_input_dict)

        return ModelInferRequest(model_name=model_name, inputs=infer_inputs,
                                 raw_input_contents=raw_input_contents)
