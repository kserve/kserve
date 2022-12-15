# -*- coding: utf-8 -*-
# Generated by the protocol buffer compiler.  DO NOT EDIT!
# source: grpc_predict_v2.proto
"""Generated protocol buffer code."""
from google.protobuf import descriptor as _descriptor
from google.protobuf import descriptor_pool as _descriptor_pool
from google.protobuf import message as _message
from google.protobuf import reflection as _reflection
from google.protobuf import symbol_database as _symbol_database
# @@protoc_insertion_point(imports)

_sym_db = _symbol_database.Default()




DESCRIPTOR = _descriptor_pool.Default().AddSerializedFile(b'\n\x15grpc_predict_v2.proto\x12\tinference\"\x13\n\x11ServerLiveRequest\"\"\n\x12ServerLiveResponse\x12\x0c\n\x04live\x18\x01 \x01(\x08\"\x14\n\x12ServerReadyRequest\"$\n\x13ServerReadyResponse\x12\r\n\x05ready\x18\x01 \x01(\x08\"2\n\x11ModelReadyRequest\x12\x0c\n\x04name\x18\x01 \x01(\t\x12\x0f\n\x07version\x18\x02 \x01(\t\"#\n\x12ModelReadyResponse\x12\r\n\x05ready\x18\x01 \x01(\x08\"\x17\n\x15ServerMetadataRequest\"K\n\x16ServerMetadataResponse\x12\x0c\n\x04name\x18\x01 \x01(\t\x12\x0f\n\x07version\x18\x02 \x01(\t\x12\x12\n\nextensions\x18\x03 \x03(\t\"5\n\x14ModelMetadataRequest\x12\x0c\n\x04name\x18\x01 \x01(\t\x12\x0f\n\x07version\x18\x02 \x01(\t\"\x8d\x02\n\x15ModelMetadataResponse\x12\x0c\n\x04name\x18\x01 \x01(\t\x12\x10\n\x08versions\x18\x02 \x03(\t\x12\x10\n\x08platform\x18\x03 \x01(\t\x12?\n\x06inputs\x18\x04 \x03(\x0b\x32/.inference.ModelMetadataResponse.TensorMetadata\x12@\n\x07outputs\x18\x05 \x03(\x0b\x32/.inference.ModelMetadataResponse.TensorMetadata\x1a?\n\x0eTensorMetadata\x12\x0c\n\x04name\x18\x01 \x01(\t\x12\x10\n\x08\x64\x61tatype\x18\x02 \x01(\t\x12\r\n\x05shape\x18\x03 \x03(\x03\"\xee\x06\n\x11ModelInferRequest\x12\x12\n\nmodel_name\x18\x01 \x01(\t\x12\x15\n\rmodel_version\x18\x02 \x01(\t\x12\n\n\x02id\x18\x03 \x01(\t\x12@\n\nparameters\x18\x04 \x03(\x0b\x32,.inference.ModelInferRequest.ParametersEntry\x12=\n\x06inputs\x18\x05 \x03(\x0b\x32-.inference.ModelInferRequest.InferInputTensor\x12H\n\x07outputs\x18\x06 \x03(\x0b\x32\x37.inference.ModelInferRequest.InferRequestedOutputTensor\x12\x1a\n\x12raw_input_contents\x18\x07 \x03(\x0c\x1a\x94\x02\n\x10InferInputTensor\x12\x0c\n\x04name\x18\x01 \x01(\t\x12\x10\n\x08\x64\x61tatype\x18\x02 \x01(\t\x12\r\n\x05shape\x18\x03 \x03(\x03\x12Q\n\nparameters\x18\x04 \x03(\x0b\x32=.inference.ModelInferRequest.InferInputTensor.ParametersEntry\x12\x30\n\x08\x63ontents\x18\x05 \x01(\x0b\x32\x1e.inference.InferTensorContents\x1aL\n\x0fParametersEntry\x12\x0b\n\x03key\x18\x01 \x01(\t\x12(\n\x05value\x18\x02 \x01(\x0b\x32\x19.inference.InferParameter:\x02\x38\x01\x1a\xd5\x01\n\x1aInferRequestedOutputTensor\x12\x0c\n\x04name\x18\x01 \x01(\t\x12[\n\nparameters\x18\x02 \x03(\x0b\x32G.inference.ModelInferRequest.InferRequestedOutputTensor.ParametersEntry\x1aL\n\x0fParametersEntry\x12\x0b\n\x03key\x18\x01 \x01(\t\x12(\n\x05value\x18\x02 \x01(\x0b\x32\x19.inference.InferParameter:\x02\x38\x01\x1aL\n\x0fParametersEntry\x12\x0b\n\x03key\x18\x01 \x01(\t\x12(\n\x05value\x18\x02 \x01(\x0b\x32\x19.inference.InferParameter:\x02\x38\x01\"\xd5\x04\n\x12ModelInferResponse\x12\x12\n\nmodel_name\x18\x01 \x01(\t\x12\x15\n\rmodel_version\x18\x02 \x01(\t\x12\n\n\x02id\x18\x03 \x01(\t\x12\x41\n\nparameters\x18\x04 \x03(\x0b\x32-.inference.ModelInferResponse.ParametersEntry\x12@\n\x07outputs\x18\x05 \x03(\x0b\x32/.inference.ModelInferResponse.InferOutputTensor\x12\x1b\n\x13raw_output_contents\x18\x06 \x03(\x0c\x1a\x97\x02\n\x11InferOutputTensor\x12\x0c\n\x04name\x18\x01 \x01(\t\x12\x10\n\x08\x64\x61tatype\x18\x02 \x01(\t\x12\r\n\x05shape\x18\x03 \x03(\x03\x12S\n\nparameters\x18\x04 \x03(\x0b\x32?.inference.ModelInferResponse.InferOutputTensor.ParametersEntry\x12\x30\n\x08\x63ontents\x18\x05 \x01(\x0b\x32\x1e.inference.InferTensorContents\x1aL\n\x0fParametersEntry\x12\x0b\n\x03key\x18\x01 \x01(\t\x12(\n\x05value\x18\x02 \x01(\x0b\x32\x19.inference.InferParameter:\x02\x38\x01\x1aL\n\x0fParametersEntry\x12\x0b\n\x03key\x18\x01 \x01(\t\x12(\n\x05value\x18\x02 \x01(\x0b\x32\x19.inference.InferParameter:\x02\x38\x01\"i\n\x0eInferParameter\x12\x14\n\nbool_param\x18\x01 \x01(\x08H\x00\x12\x15\n\x0bint64_param\x18\x02 \x01(\x03H\x00\x12\x16\n\x0cstring_param\x18\x03 \x01(\tH\x00\x42\x12\n\x10parameter_choice\"\xd0\x01\n\x13InferTensorContents\x12\x15\n\rbool_contents\x18\x01 \x03(\x08\x12\x14\n\x0cint_contents\x18\x02 \x03(\x05\x12\x16\n\x0eint64_contents\x18\x03 \x03(\x03\x12\x15\n\ruint_contents\x18\x04 \x03(\r\x12\x17\n\x0fuint64_contents\x18\x05 \x03(\x04\x12\x15\n\rfp32_contents\x18\x06 \x03(\x02\x12\x15\n\rfp64_contents\x18\x07 \x03(\x01\x12\x16\n\x0e\x62ytes_contents\x18\x08 \x03(\x0c\"0\n\x1aRepositoryModelLoadRequest\x12\x12\n\nmodel_name\x18\x01 \x01(\t\"C\n\x1bRepositoryModelLoadResponse\x12\x12\n\nmodel_name\x18\x01 \x01(\t\x12\x10\n\x08isLoaded\x18\x02 \x01(\x08\"2\n\x1cRepositoryModelUnloadRequest\x12\x12\n\nmodel_name\x18\x01 \x01(\t\"G\n\x1dRepositoryModelUnloadResponse\x12\x12\n\nmodel_name\x18\x01 \x01(\t\x12\x12\n\nisUnloaded\x18\x02 \x01(\x08\x32\xd2\x05\n\x14GRPCInferenceService\x12K\n\nServerLive\x12\x1c.inference.ServerLiveRequest\x1a\x1d.inference.ServerLiveResponse\"\x00\x12N\n\x0bServerReady\x12\x1d.inference.ServerReadyRequest\x1a\x1e.inference.ServerReadyResponse\"\x00\x12K\n\nModelReady\x12\x1c.inference.ModelReadyRequest\x1a\x1d.inference.ModelReadyResponse\"\x00\x12W\n\x0eServerMetadata\x12 .inference.ServerMetadataRequest\x1a!.inference.ServerMetadataResponse\"\x00\x12T\n\rModelMetadata\x12\x1f.inference.ModelMetadataRequest\x1a .inference.ModelMetadataResponse\"\x00\x12K\n\nModelInfer\x12\x1c.inference.ModelInferRequest\x1a\x1d.inference.ModelInferResponse\"\x00\x12\x66\n\x13RepositoryModelLoad\x12%.inference.RepositoryModelLoadRequest\x1a&.inference.RepositoryModelLoadResponse\"\x00\x12l\n\x15RepositoryModelUnload\x12\'.inference.RepositoryModelUnloadRequest\x1a(.inference.RepositoryModelUnloadResponse\"\x00\x62\x06proto3')



_SERVERLIVEREQUEST = DESCRIPTOR.message_types_by_name['ServerLiveRequest']
_SERVERLIVERESPONSE = DESCRIPTOR.message_types_by_name['ServerLiveResponse']
_SERVERREADYREQUEST = DESCRIPTOR.message_types_by_name['ServerReadyRequest']
_SERVERREADYRESPONSE = DESCRIPTOR.message_types_by_name['ServerReadyResponse']
_MODELREADYREQUEST = DESCRIPTOR.message_types_by_name['ModelReadyRequest']
_MODELREADYRESPONSE = DESCRIPTOR.message_types_by_name['ModelReadyResponse']
_SERVERMETADATAREQUEST = DESCRIPTOR.message_types_by_name['ServerMetadataRequest']
_SERVERMETADATARESPONSE = DESCRIPTOR.message_types_by_name['ServerMetadataResponse']
_MODELMETADATAREQUEST = DESCRIPTOR.message_types_by_name['ModelMetadataRequest']
_MODELMETADATARESPONSE = DESCRIPTOR.message_types_by_name['ModelMetadataResponse']
_MODELMETADATARESPONSE_TENSORMETADATA = _MODELMETADATARESPONSE.nested_types_by_name['TensorMetadata']
_MODELINFERREQUEST = DESCRIPTOR.message_types_by_name['ModelInferRequest']
_MODELINFERREQUEST_INFERINPUTTENSOR = _MODELINFERREQUEST.nested_types_by_name['InferInputTensor']
_MODELINFERREQUEST_INFERINPUTTENSOR_PARAMETERSENTRY = _MODELINFERREQUEST_INFERINPUTTENSOR.nested_types_by_name['ParametersEntry']
_MODELINFERREQUEST_INFERREQUESTEDOUTPUTTENSOR = _MODELINFERREQUEST.nested_types_by_name['InferRequestedOutputTensor']
_MODELINFERREQUEST_INFERREQUESTEDOUTPUTTENSOR_PARAMETERSENTRY = _MODELINFERREQUEST_INFERREQUESTEDOUTPUTTENSOR.nested_types_by_name['ParametersEntry']
_MODELINFERREQUEST_PARAMETERSENTRY = _MODELINFERREQUEST.nested_types_by_name['ParametersEntry']
_MODELINFERRESPONSE = DESCRIPTOR.message_types_by_name['ModelInferResponse']
_MODELINFERRESPONSE_INFEROUTPUTTENSOR = _MODELINFERRESPONSE.nested_types_by_name['InferOutputTensor']
_MODELINFERRESPONSE_INFEROUTPUTTENSOR_PARAMETERSENTRY = _MODELINFERRESPONSE_INFEROUTPUTTENSOR.nested_types_by_name['ParametersEntry']
_MODELINFERRESPONSE_PARAMETERSENTRY = _MODELINFERRESPONSE.nested_types_by_name['ParametersEntry']
_INFERPARAMETER = DESCRIPTOR.message_types_by_name['InferParameter']
_INFERTENSORCONTENTS = DESCRIPTOR.message_types_by_name['InferTensorContents']
_REPOSITORYMODELLOADREQUEST = DESCRIPTOR.message_types_by_name['RepositoryModelLoadRequest']
_REPOSITORYMODELLOADRESPONSE = DESCRIPTOR.message_types_by_name['RepositoryModelLoadResponse']
_REPOSITORYMODELUNLOADREQUEST = DESCRIPTOR.message_types_by_name['RepositoryModelUnloadRequest']
_REPOSITORYMODELUNLOADRESPONSE = DESCRIPTOR.message_types_by_name['RepositoryModelUnloadResponse']
ServerLiveRequest = _reflection.GeneratedProtocolMessageType('ServerLiveRequest', (_message.Message,), {
  'DESCRIPTOR' : _SERVERLIVEREQUEST,
  '__module__' : 'grpc_predict_v2_pb2'
  # @@protoc_insertion_point(class_scope:inference.ServerLiveRequest)
  })
_sym_db.RegisterMessage(ServerLiveRequest)

ServerLiveResponse = _reflection.GeneratedProtocolMessageType('ServerLiveResponse', (_message.Message,), {
  'DESCRIPTOR' : _SERVERLIVERESPONSE,
  '__module__' : 'grpc_predict_v2_pb2'
  # @@protoc_insertion_point(class_scope:inference.ServerLiveResponse)
  })
_sym_db.RegisterMessage(ServerLiveResponse)

ServerReadyRequest = _reflection.GeneratedProtocolMessageType('ServerReadyRequest', (_message.Message,), {
  'DESCRIPTOR' : _SERVERREADYREQUEST,
  '__module__' : 'grpc_predict_v2_pb2'
  # @@protoc_insertion_point(class_scope:inference.ServerReadyRequest)
  })
_sym_db.RegisterMessage(ServerReadyRequest)

ServerReadyResponse = _reflection.GeneratedProtocolMessageType('ServerReadyResponse', (_message.Message,), {
  'DESCRIPTOR' : _SERVERREADYRESPONSE,
  '__module__' : 'grpc_predict_v2_pb2'
  # @@protoc_insertion_point(class_scope:inference.ServerReadyResponse)
  })
_sym_db.RegisterMessage(ServerReadyResponse)

ModelReadyRequest = _reflection.GeneratedProtocolMessageType('ModelReadyRequest', (_message.Message,), {
  'DESCRIPTOR' : _MODELREADYREQUEST,
  '__module__' : 'grpc_predict_v2_pb2'
  # @@protoc_insertion_point(class_scope:inference.ModelReadyRequest)
  })
_sym_db.RegisterMessage(ModelReadyRequest)

ModelReadyResponse = _reflection.GeneratedProtocolMessageType('ModelReadyResponse', (_message.Message,), {
  'DESCRIPTOR' : _MODELREADYRESPONSE,
  '__module__' : 'grpc_predict_v2_pb2'
  # @@protoc_insertion_point(class_scope:inference.ModelReadyResponse)
  })
_sym_db.RegisterMessage(ModelReadyResponse)

ServerMetadataRequest = _reflection.GeneratedProtocolMessageType('ServerMetadataRequest', (_message.Message,), {
  'DESCRIPTOR' : _SERVERMETADATAREQUEST,
  '__module__' : 'grpc_predict_v2_pb2'
  # @@protoc_insertion_point(class_scope:inference.ServerMetadataRequest)
  })
_sym_db.RegisterMessage(ServerMetadataRequest)

ServerMetadataResponse = _reflection.GeneratedProtocolMessageType('ServerMetadataResponse', (_message.Message,), {
  'DESCRIPTOR' : _SERVERMETADATARESPONSE,
  '__module__' : 'grpc_predict_v2_pb2'
  # @@protoc_insertion_point(class_scope:inference.ServerMetadataResponse)
  })
_sym_db.RegisterMessage(ServerMetadataResponse)

ModelMetadataRequest = _reflection.GeneratedProtocolMessageType('ModelMetadataRequest', (_message.Message,), {
  'DESCRIPTOR' : _MODELMETADATAREQUEST,
  '__module__' : 'grpc_predict_v2_pb2'
  # @@protoc_insertion_point(class_scope:inference.ModelMetadataRequest)
  })
_sym_db.RegisterMessage(ModelMetadataRequest)

ModelMetadataResponse = _reflection.GeneratedProtocolMessageType('ModelMetadataResponse', (_message.Message,), {

  'TensorMetadata' : _reflection.GeneratedProtocolMessageType('TensorMetadata', (_message.Message,), {
    'DESCRIPTOR' : _MODELMETADATARESPONSE_TENSORMETADATA,
    '__module__' : 'grpc_predict_v2_pb2'
    # @@protoc_insertion_point(class_scope:inference.ModelMetadataResponse.TensorMetadata)
    })
  ,
  'DESCRIPTOR' : _MODELMETADATARESPONSE,
  '__module__' : 'grpc_predict_v2_pb2'
  # @@protoc_insertion_point(class_scope:inference.ModelMetadataResponse)
  })
_sym_db.RegisterMessage(ModelMetadataResponse)
_sym_db.RegisterMessage(ModelMetadataResponse.TensorMetadata)

ModelInferRequest = _reflection.GeneratedProtocolMessageType('ModelInferRequest', (_message.Message,), {

  'InferInputTensor' : _reflection.GeneratedProtocolMessageType('InferInputTensor', (_message.Message,), {

    'ParametersEntry' : _reflection.GeneratedProtocolMessageType('ParametersEntry', (_message.Message,), {
      'DESCRIPTOR' : _MODELINFERREQUEST_INFERINPUTTENSOR_PARAMETERSENTRY,
      '__module__' : 'grpc_predict_v2_pb2'
      # @@protoc_insertion_point(class_scope:inference.ModelInferRequest.InferInputTensor.ParametersEntry)
      })
    ,
    'DESCRIPTOR' : _MODELINFERREQUEST_INFERINPUTTENSOR,
    '__module__' : 'grpc_predict_v2_pb2'
    # @@protoc_insertion_point(class_scope:inference.ModelInferRequest.InferInputTensor)
    })
  ,

  'InferRequestedOutputTensor' : _reflection.GeneratedProtocolMessageType('InferRequestedOutputTensor', (_message.Message,), {

    'ParametersEntry' : _reflection.GeneratedProtocolMessageType('ParametersEntry', (_message.Message,), {
      'DESCRIPTOR' : _MODELINFERREQUEST_INFERREQUESTEDOUTPUTTENSOR_PARAMETERSENTRY,
      '__module__' : 'grpc_predict_v2_pb2'
      # @@protoc_insertion_point(class_scope:inference.ModelInferRequest.InferRequestedOutputTensor.ParametersEntry)
      })
    ,
    'DESCRIPTOR' : _MODELINFERREQUEST_INFERREQUESTEDOUTPUTTENSOR,
    '__module__' : 'grpc_predict_v2_pb2'
    # @@protoc_insertion_point(class_scope:inference.ModelInferRequest.InferRequestedOutputTensor)
    })
  ,

  'ParametersEntry' : _reflection.GeneratedProtocolMessageType('ParametersEntry', (_message.Message,), {
    'DESCRIPTOR' : _MODELINFERREQUEST_PARAMETERSENTRY,
    '__module__' : 'grpc_predict_v2_pb2'
    # @@protoc_insertion_point(class_scope:inference.ModelInferRequest.ParametersEntry)
    })
  ,
  'DESCRIPTOR' : _MODELINFERREQUEST,
  '__module__' : 'grpc_predict_v2_pb2'
  # @@protoc_insertion_point(class_scope:inference.ModelInferRequest)
  })
_sym_db.RegisterMessage(ModelInferRequest)
_sym_db.RegisterMessage(ModelInferRequest.InferInputTensor)
_sym_db.RegisterMessage(ModelInferRequest.InferInputTensor.ParametersEntry)
_sym_db.RegisterMessage(ModelInferRequest.InferRequestedOutputTensor)
_sym_db.RegisterMessage(ModelInferRequest.InferRequestedOutputTensor.ParametersEntry)
_sym_db.RegisterMessage(ModelInferRequest.ParametersEntry)

ModelInferResponse = _reflection.GeneratedProtocolMessageType('ModelInferResponse', (_message.Message,), {

  'InferOutputTensor' : _reflection.GeneratedProtocolMessageType('InferOutputTensor', (_message.Message,), {

    'ParametersEntry' : _reflection.GeneratedProtocolMessageType('ParametersEntry', (_message.Message,), {
      'DESCRIPTOR' : _MODELINFERRESPONSE_INFEROUTPUTTENSOR_PARAMETERSENTRY,
      '__module__' : 'grpc_predict_v2_pb2'
      # @@protoc_insertion_point(class_scope:inference.ModelInferResponse.InferOutputTensor.ParametersEntry)
      })
    ,
    'DESCRIPTOR' : _MODELINFERRESPONSE_INFEROUTPUTTENSOR,
    '__module__' : 'grpc_predict_v2_pb2'
    # @@protoc_insertion_point(class_scope:inference.ModelInferResponse.InferOutputTensor)
    })
  ,

  'ParametersEntry' : _reflection.GeneratedProtocolMessageType('ParametersEntry', (_message.Message,), {
    'DESCRIPTOR' : _MODELINFERRESPONSE_PARAMETERSENTRY,
    '__module__' : 'grpc_predict_v2_pb2'
    # @@protoc_insertion_point(class_scope:inference.ModelInferResponse.ParametersEntry)
    })
  ,
  'DESCRIPTOR' : _MODELINFERRESPONSE,
  '__module__' : 'grpc_predict_v2_pb2'
  # @@protoc_insertion_point(class_scope:inference.ModelInferResponse)
  })
_sym_db.RegisterMessage(ModelInferResponse)
_sym_db.RegisterMessage(ModelInferResponse.InferOutputTensor)
_sym_db.RegisterMessage(ModelInferResponse.InferOutputTensor.ParametersEntry)
_sym_db.RegisterMessage(ModelInferResponse.ParametersEntry)

InferParameter = _reflection.GeneratedProtocolMessageType('InferParameter', (_message.Message,), {
  'DESCRIPTOR' : _INFERPARAMETER,
  '__module__' : 'grpc_predict_v2_pb2'
  # @@protoc_insertion_point(class_scope:inference.InferParameter)
  })
_sym_db.RegisterMessage(InferParameter)

InferTensorContents = _reflection.GeneratedProtocolMessageType('InferTensorContents', (_message.Message,), {
  'DESCRIPTOR' : _INFERTENSORCONTENTS,
  '__module__' : 'grpc_predict_v2_pb2'
  # @@protoc_insertion_point(class_scope:inference.InferTensorContents)
  })
_sym_db.RegisterMessage(InferTensorContents)

RepositoryModelLoadRequest = _reflection.GeneratedProtocolMessageType('RepositoryModelLoadRequest', (_message.Message,), {
  'DESCRIPTOR' : _REPOSITORYMODELLOADREQUEST,
  '__module__' : 'grpc_predict_v2_pb2'
  # @@protoc_insertion_point(class_scope:inference.RepositoryModelLoadRequest)
  })
_sym_db.RegisterMessage(RepositoryModelLoadRequest)

RepositoryModelLoadResponse = _reflection.GeneratedProtocolMessageType('RepositoryModelLoadResponse', (_message.Message,), {
  'DESCRIPTOR' : _REPOSITORYMODELLOADRESPONSE,
  '__module__' : 'grpc_predict_v2_pb2'
  # @@protoc_insertion_point(class_scope:inference.RepositoryModelLoadResponse)
  })
_sym_db.RegisterMessage(RepositoryModelLoadResponse)

RepositoryModelUnloadRequest = _reflection.GeneratedProtocolMessageType('RepositoryModelUnloadRequest', (_message.Message,), {
  'DESCRIPTOR' : _REPOSITORYMODELUNLOADREQUEST,
  '__module__' : 'grpc_predict_v2_pb2'
  # @@protoc_insertion_point(class_scope:inference.RepositoryModelUnloadRequest)
  })
_sym_db.RegisterMessage(RepositoryModelUnloadRequest)

RepositoryModelUnloadResponse = _reflection.GeneratedProtocolMessageType('RepositoryModelUnloadResponse', (_message.Message,), {
  'DESCRIPTOR' : _REPOSITORYMODELUNLOADRESPONSE,
  '__module__' : 'grpc_predict_v2_pb2'
  # @@protoc_insertion_point(class_scope:inference.RepositoryModelUnloadResponse)
  })
_sym_db.RegisterMessage(RepositoryModelUnloadResponse)

_GRPCINFERENCESERVICE = DESCRIPTOR.services_by_name['GRPCInferenceService']
if _descriptor._USE_C_DESCRIPTORS == False:

  DESCRIPTOR._options = None
  _MODELINFERREQUEST_INFERINPUTTENSOR_PARAMETERSENTRY._options = None
  _MODELINFERREQUEST_INFERINPUTTENSOR_PARAMETERSENTRY._serialized_options = b'8\001'
  _MODELINFERREQUEST_INFERREQUESTEDOUTPUTTENSOR_PARAMETERSENTRY._options = None
  _MODELINFERREQUEST_INFERREQUESTEDOUTPUTTENSOR_PARAMETERSENTRY._serialized_options = b'8\001'
  _MODELINFERREQUEST_PARAMETERSENTRY._options = None
  _MODELINFERREQUEST_PARAMETERSENTRY._serialized_options = b'8\001'
  _MODELINFERRESPONSE_INFEROUTPUTTENSOR_PARAMETERSENTRY._options = None
  _MODELINFERRESPONSE_INFEROUTPUTTENSOR_PARAMETERSENTRY._serialized_options = b'8\001'
  _MODELINFERRESPONSE_PARAMETERSENTRY._options = None
  _MODELINFERRESPONSE_PARAMETERSENTRY._serialized_options = b'8\001'
  _SERVERLIVEREQUEST._serialized_start=36
  _SERVERLIVEREQUEST._serialized_end=55
  _SERVERLIVERESPONSE._serialized_start=57
  _SERVERLIVERESPONSE._serialized_end=91
  _SERVERREADYREQUEST._serialized_start=93
  _SERVERREADYREQUEST._serialized_end=113
  _SERVERREADYRESPONSE._serialized_start=115
  _SERVERREADYRESPONSE._serialized_end=151
  _MODELREADYREQUEST._serialized_start=153
  _MODELREADYREQUEST._serialized_end=203
  _MODELREADYRESPONSE._serialized_start=205
  _MODELREADYRESPONSE._serialized_end=240
  _SERVERMETADATAREQUEST._serialized_start=242
  _SERVERMETADATAREQUEST._serialized_end=265
  _SERVERMETADATARESPONSE._serialized_start=267
  _SERVERMETADATARESPONSE._serialized_end=342
  _MODELMETADATAREQUEST._serialized_start=344
  _MODELMETADATAREQUEST._serialized_end=397
  _MODELMETADATARESPONSE._serialized_start=400
  _MODELMETADATARESPONSE._serialized_end=669
  _MODELMETADATARESPONSE_TENSORMETADATA._serialized_start=606
  _MODELMETADATARESPONSE_TENSORMETADATA._serialized_end=669
  _MODELINFERREQUEST._serialized_start=672
  _MODELINFERREQUEST._serialized_end=1550
  _MODELINFERREQUEST_INFERINPUTTENSOR._serialized_start=980
  _MODELINFERREQUEST_INFERINPUTTENSOR._serialized_end=1256
  _MODELINFERREQUEST_INFERINPUTTENSOR_PARAMETERSENTRY._serialized_start=1180
  _MODELINFERREQUEST_INFERINPUTTENSOR_PARAMETERSENTRY._serialized_end=1256
  _MODELINFERREQUEST_INFERREQUESTEDOUTPUTTENSOR._serialized_start=1259
  _MODELINFERREQUEST_INFERREQUESTEDOUTPUTTENSOR._serialized_end=1472
  _MODELINFERREQUEST_INFERREQUESTEDOUTPUTTENSOR_PARAMETERSENTRY._serialized_start=1180
  _MODELINFERREQUEST_INFERREQUESTEDOUTPUTTENSOR_PARAMETERSENTRY._serialized_end=1256
  _MODELINFERREQUEST_PARAMETERSENTRY._serialized_start=1180
  _MODELINFERREQUEST_PARAMETERSENTRY._serialized_end=1256
  _MODELINFERRESPONSE._serialized_start=1553
  _MODELINFERRESPONSE._serialized_end=2150
  _MODELINFERRESPONSE_INFEROUTPUTTENSOR._serialized_start=1793
  _MODELINFERRESPONSE_INFEROUTPUTTENSOR._serialized_end=2072
  _MODELINFERRESPONSE_INFEROUTPUTTENSOR_PARAMETERSENTRY._serialized_start=1180
  _MODELINFERRESPONSE_INFEROUTPUTTENSOR_PARAMETERSENTRY._serialized_end=1256
  _MODELINFERRESPONSE_PARAMETERSENTRY._serialized_start=1180
  _MODELINFERRESPONSE_PARAMETERSENTRY._serialized_end=1256
  _INFERPARAMETER._serialized_start=2152
  _INFERPARAMETER._serialized_end=2257
  _INFERTENSORCONTENTS._serialized_start=2260
  _INFERTENSORCONTENTS._serialized_end=2468
  _REPOSITORYMODELLOADREQUEST._serialized_start=2470
  _REPOSITORYMODELLOADREQUEST._serialized_end=2518
  _REPOSITORYMODELLOADRESPONSE._serialized_start=2520
  _REPOSITORYMODELLOADRESPONSE._serialized_end=2587
  _REPOSITORYMODELUNLOADREQUEST._serialized_start=2589
  _REPOSITORYMODELUNLOADREQUEST._serialized_end=2639
  _REPOSITORYMODELUNLOADRESPONSE._serialized_start=2641
  _REPOSITORYMODELUNLOADRESPONSE._serialized_end=2712
  _GRPCINFERENCESERVICE._serialized_start=2715
  _GRPCINFERENCESERVICE._serialized_end=3437
# @@protoc_insertion_point(module_scope)
