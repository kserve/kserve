from google.protobuf.internal import containers as _containers
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from typing import ClassVar as _ClassVar, Iterable as _Iterable, Mapping as _Mapping, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class InferParameter(_message.Message):
    __slots__ = ["bool_param", "int64_param", "string_param"]
    BOOL_PARAM_FIELD_NUMBER: _ClassVar[int]
    INT64_PARAM_FIELD_NUMBER: _ClassVar[int]
    STRING_PARAM_FIELD_NUMBER: _ClassVar[int]
    bool_param: bool
    int64_param: int
    string_param: str
    def __init__(self, bool_param: bool = ..., int64_param: _Optional[int] = ..., string_param: _Optional[str] = ...) -> None: ...

class InferTensorContents(_message.Message):
    __slots__ = ["bool_contents", "bytes_contents", "fp32_contents", "fp64_contents", "int64_contents", "int_contents", "uint64_contents", "uint_contents"]
    BOOL_CONTENTS_FIELD_NUMBER: _ClassVar[int]
    BYTES_CONTENTS_FIELD_NUMBER: _ClassVar[int]
    FP32_CONTENTS_FIELD_NUMBER: _ClassVar[int]
    FP64_CONTENTS_FIELD_NUMBER: _ClassVar[int]
    INT64_CONTENTS_FIELD_NUMBER: _ClassVar[int]
    INT_CONTENTS_FIELD_NUMBER: _ClassVar[int]
    UINT64_CONTENTS_FIELD_NUMBER: _ClassVar[int]
    UINT_CONTENTS_FIELD_NUMBER: _ClassVar[int]
    bool_contents: _containers.RepeatedScalarFieldContainer[bool]
    bytes_contents: _containers.RepeatedScalarFieldContainer[bytes]
    fp32_contents: _containers.RepeatedScalarFieldContainer[float]
    fp64_contents: _containers.RepeatedScalarFieldContainer[float]
    int64_contents: _containers.RepeatedScalarFieldContainer[int]
    int_contents: _containers.RepeatedScalarFieldContainer[int]
    uint64_contents: _containers.RepeatedScalarFieldContainer[int]
    uint_contents: _containers.RepeatedScalarFieldContainer[int]
    def __init__(self, bool_contents: _Optional[_Iterable[bool]] = ..., int_contents: _Optional[_Iterable[int]] = ..., int64_contents: _Optional[_Iterable[int]] = ..., uint_contents: _Optional[_Iterable[int]] = ..., uint64_contents: _Optional[_Iterable[int]] = ..., fp32_contents: _Optional[_Iterable[float]] = ..., fp64_contents: _Optional[_Iterable[float]] = ..., bytes_contents: _Optional[_Iterable[bytes]] = ...) -> None: ...

class ModelInferRequest(_message.Message):
    __slots__ = ["id", "inputs", "model_name", "model_version", "outputs", "parameters", "raw_input_contents"]
    class InferInputTensor(_message.Message):
        __slots__ = ["contents", "datatype", "name", "parameters", "shape"]
        class ParametersEntry(_message.Message):
            __slots__ = ["key", "value"]
            KEY_FIELD_NUMBER: _ClassVar[int]
            VALUE_FIELD_NUMBER: _ClassVar[int]
            key: str
            value: InferParameter
            def __init__(self, key: _Optional[str] = ..., value: _Optional[_Union[InferParameter, _Mapping]] = ...) -> None: ...
        CONTENTS_FIELD_NUMBER: _ClassVar[int]
        DATATYPE_FIELD_NUMBER: _ClassVar[int]
        NAME_FIELD_NUMBER: _ClassVar[int]
        PARAMETERS_FIELD_NUMBER: _ClassVar[int]
        SHAPE_FIELD_NUMBER: _ClassVar[int]
        contents: InferTensorContents
        datatype: str
        name: str
        parameters: _containers.MessageMap[str, InferParameter]
        shape: _containers.RepeatedScalarFieldContainer[int]
        def __init__(self, name: _Optional[str] = ..., datatype: _Optional[str] = ..., shape: _Optional[_Iterable[int]] = ..., parameters: _Optional[_Mapping[str, InferParameter]] = ..., contents: _Optional[_Union[InferTensorContents, _Mapping]] = ...) -> None: ...
    class InferRequestedOutputTensor(_message.Message):
        __slots__ = ["name", "parameters"]
        class ParametersEntry(_message.Message):
            __slots__ = ["key", "value"]
            KEY_FIELD_NUMBER: _ClassVar[int]
            VALUE_FIELD_NUMBER: _ClassVar[int]
            key: str
            value: InferParameter
            def __init__(self, key: _Optional[str] = ..., value: _Optional[_Union[InferParameter, _Mapping]] = ...) -> None: ...
        NAME_FIELD_NUMBER: _ClassVar[int]
        PARAMETERS_FIELD_NUMBER: _ClassVar[int]
        name: str
        parameters: _containers.MessageMap[str, InferParameter]
        def __init__(self, name: _Optional[str] = ..., parameters: _Optional[_Mapping[str, InferParameter]] = ...) -> None: ...
    class ParametersEntry(_message.Message):
        __slots__ = ["key", "value"]
        KEY_FIELD_NUMBER: _ClassVar[int]
        VALUE_FIELD_NUMBER: _ClassVar[int]
        key: str
        value: InferParameter
        def __init__(self, key: _Optional[str] = ..., value: _Optional[_Union[InferParameter, _Mapping]] = ...) -> None: ...
    ID_FIELD_NUMBER: _ClassVar[int]
    INPUTS_FIELD_NUMBER: _ClassVar[int]
    MODEL_NAME_FIELD_NUMBER: _ClassVar[int]
    MODEL_VERSION_FIELD_NUMBER: _ClassVar[int]
    OUTPUTS_FIELD_NUMBER: _ClassVar[int]
    PARAMETERS_FIELD_NUMBER: _ClassVar[int]
    RAW_INPUT_CONTENTS_FIELD_NUMBER: _ClassVar[int]
    id: str
    inputs: _containers.RepeatedCompositeFieldContainer[ModelInferRequest.InferInputTensor]
    model_name: str
    model_version: str
    outputs: _containers.RepeatedCompositeFieldContainer[ModelInferRequest.InferRequestedOutputTensor]
    parameters: _containers.MessageMap[str, InferParameter]
    raw_input_contents: _containers.RepeatedScalarFieldContainer[bytes]
    def __init__(self, model_name: _Optional[str] = ..., model_version: _Optional[str] = ..., id: _Optional[str] = ..., parameters: _Optional[_Mapping[str, InferParameter]] = ..., inputs: _Optional[_Iterable[_Union[ModelInferRequest.InferInputTensor, _Mapping]]] = ..., outputs: _Optional[_Iterable[_Union[ModelInferRequest.InferRequestedOutputTensor, _Mapping]]] = ..., raw_input_contents: _Optional[_Iterable[bytes]] = ...) -> None: ...

class ModelInferResponse(_message.Message):
    __slots__ = ["id", "model_name", "model_version", "outputs", "parameters", "raw_output_contents"]
    class InferOutputTensor(_message.Message):
        __slots__ = ["contents", "datatype", "name", "parameters", "shape"]
        class ParametersEntry(_message.Message):
            __slots__ = ["key", "value"]
            KEY_FIELD_NUMBER: _ClassVar[int]
            VALUE_FIELD_NUMBER: _ClassVar[int]
            key: str
            value: InferParameter
            def __init__(self, key: _Optional[str] = ..., value: _Optional[_Union[InferParameter, _Mapping]] = ...) -> None: ...
        CONTENTS_FIELD_NUMBER: _ClassVar[int]
        DATATYPE_FIELD_NUMBER: _ClassVar[int]
        NAME_FIELD_NUMBER: _ClassVar[int]
        PARAMETERS_FIELD_NUMBER: _ClassVar[int]
        SHAPE_FIELD_NUMBER: _ClassVar[int]
        contents: InferTensorContents
        datatype: str
        name: str
        parameters: _containers.MessageMap[str, InferParameter]
        shape: _containers.RepeatedScalarFieldContainer[int]
        def __init__(self, name: _Optional[str] = ..., datatype: _Optional[str] = ..., shape: _Optional[_Iterable[int]] = ..., parameters: _Optional[_Mapping[str, InferParameter]] = ..., contents: _Optional[_Union[InferTensorContents, _Mapping]] = ...) -> None: ...
    class ParametersEntry(_message.Message):
        __slots__ = ["key", "value"]
        KEY_FIELD_NUMBER: _ClassVar[int]
        VALUE_FIELD_NUMBER: _ClassVar[int]
        key: str
        value: InferParameter
        def __init__(self, key: _Optional[str] = ..., value: _Optional[_Union[InferParameter, _Mapping]] = ...) -> None: ...
    ID_FIELD_NUMBER: _ClassVar[int]
    MODEL_NAME_FIELD_NUMBER: _ClassVar[int]
    MODEL_VERSION_FIELD_NUMBER: _ClassVar[int]
    OUTPUTS_FIELD_NUMBER: _ClassVar[int]
    PARAMETERS_FIELD_NUMBER: _ClassVar[int]
    RAW_OUTPUT_CONTENTS_FIELD_NUMBER: _ClassVar[int]
    id: str
    model_name: str
    model_version: str
    outputs: _containers.RepeatedCompositeFieldContainer[ModelInferResponse.InferOutputTensor]
    parameters: _containers.MessageMap[str, InferParameter]
    raw_output_contents: _containers.RepeatedScalarFieldContainer[bytes]
    def __init__(self, model_name: _Optional[str] = ..., model_version: _Optional[str] = ..., id: _Optional[str] = ..., parameters: _Optional[_Mapping[str, InferParameter]] = ..., outputs: _Optional[_Iterable[_Union[ModelInferResponse.InferOutputTensor, _Mapping]]] = ..., raw_output_contents: _Optional[_Iterable[bytes]] = ...) -> None: ...

class ModelMetadataRequest(_message.Message):
    __slots__ = ["name", "version"]
    NAME_FIELD_NUMBER: _ClassVar[int]
    VERSION_FIELD_NUMBER: _ClassVar[int]
    name: str
    version: str
    def __init__(self, name: _Optional[str] = ..., version: _Optional[str] = ...) -> None: ...

class ModelMetadataResponse(_message.Message):
    __slots__ = ["inputs", "name", "outputs", "platform", "versions"]
    class TensorMetadata(_message.Message):
        __slots__ = ["datatype", "name", "shape"]
        DATATYPE_FIELD_NUMBER: _ClassVar[int]
        NAME_FIELD_NUMBER: _ClassVar[int]
        SHAPE_FIELD_NUMBER: _ClassVar[int]
        datatype: str
        name: str
        shape: _containers.RepeatedScalarFieldContainer[int]
        def __init__(self, name: _Optional[str] = ..., datatype: _Optional[str] = ..., shape: _Optional[_Iterable[int]] = ...) -> None: ...
    INPUTS_FIELD_NUMBER: _ClassVar[int]
    NAME_FIELD_NUMBER: _ClassVar[int]
    OUTPUTS_FIELD_NUMBER: _ClassVar[int]
    PLATFORM_FIELD_NUMBER: _ClassVar[int]
    VERSIONS_FIELD_NUMBER: _ClassVar[int]
    inputs: _containers.RepeatedCompositeFieldContainer[ModelMetadataResponse.TensorMetadata]
    name: str
    outputs: _containers.RepeatedCompositeFieldContainer[ModelMetadataResponse.TensorMetadata]
    platform: str
    versions: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, name: _Optional[str] = ..., versions: _Optional[_Iterable[str]] = ..., platform: _Optional[str] = ..., inputs: _Optional[_Iterable[_Union[ModelMetadataResponse.TensorMetadata, _Mapping]]] = ..., outputs: _Optional[_Iterable[_Union[ModelMetadataResponse.TensorMetadata, _Mapping]]] = ...) -> None: ...

class ModelReadyRequest(_message.Message):
    __slots__ = ["name", "version"]
    NAME_FIELD_NUMBER: _ClassVar[int]
    VERSION_FIELD_NUMBER: _ClassVar[int]
    name: str
    version: str
    def __init__(self, name: _Optional[str] = ..., version: _Optional[str] = ...) -> None: ...

class ModelReadyResponse(_message.Message):
    __slots__ = ["ready"]
    READY_FIELD_NUMBER: _ClassVar[int]
    ready: bool
    def __init__(self, ready: bool = ...) -> None: ...

class RepositoryModelLoadRequest(_message.Message):
    __slots__ = ["model_name"]
    MODEL_NAME_FIELD_NUMBER: _ClassVar[int]
    model_name: str
    def __init__(self, model_name: _Optional[str] = ...) -> None: ...

class RepositoryModelLoadResponse(_message.Message):
    __slots__ = ["isLoaded", "model_name"]
    ISLOADED_FIELD_NUMBER: _ClassVar[int]
    MODEL_NAME_FIELD_NUMBER: _ClassVar[int]
    isLoaded: bool
    model_name: str
    def __init__(self, model_name: _Optional[str] = ..., isLoaded: bool = ...) -> None: ...

class RepositoryModelUnloadRequest(_message.Message):
    __slots__ = ["model_name"]
    MODEL_NAME_FIELD_NUMBER: _ClassVar[int]
    model_name: str
    def __init__(self, model_name: _Optional[str] = ...) -> None: ...

class RepositoryModelUnloadResponse(_message.Message):
    __slots__ = ["isUnloaded", "model_name"]
    ISUNLOADED_FIELD_NUMBER: _ClassVar[int]
    MODEL_NAME_FIELD_NUMBER: _ClassVar[int]
    isUnloaded: bool
    model_name: str
    def __init__(self, model_name: _Optional[str] = ..., isUnloaded: bool = ...) -> None: ...

class ServerLiveRequest(_message.Message):
    __slots__ = []
    def __init__(self) -> None: ...

class ServerLiveResponse(_message.Message):
    __slots__ = ["live"]
    LIVE_FIELD_NUMBER: _ClassVar[int]
    live: bool
    def __init__(self, live: bool = ...) -> None: ...

class ServerMetadataRequest(_message.Message):
    __slots__ = []
    def __init__(self) -> None: ...

class ServerMetadataResponse(_message.Message):
    __slots__ = ["extensions", "name", "version"]
    EXTENSIONS_FIELD_NUMBER: _ClassVar[int]
    NAME_FIELD_NUMBER: _ClassVar[int]
    VERSION_FIELD_NUMBER: _ClassVar[int]
    extensions: _containers.RepeatedScalarFieldContainer[str]
    name: str
    version: str
    def __init__(self, name: _Optional[str] = ..., version: _Optional[str] = ..., extensions: _Optional[_Iterable[str]] = ...) -> None: ...

class ServerReadyRequest(_message.Message):
    __slots__ = []
    def __init__(self) -> None: ...

class ServerReadyResponse(_message.Message):
    __slots__ = ["ready"]
    READY_FIELD_NUMBER: _ClassVar[int]
    ready: bool
    def __init__(self, ready: bool = ...) -> None: ...
