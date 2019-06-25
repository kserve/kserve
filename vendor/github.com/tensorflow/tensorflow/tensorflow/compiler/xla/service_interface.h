/* Copyright 2017 The TensorFlow Authors. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
==============================================================================*/

#ifndef TENSORFLOW_COMPILER_XLA_SERVICE_INTERFACE_H_
#define TENSORFLOW_COMPILER_XLA_SERVICE_INTERFACE_H_

#include "tensorflow/compiler/xla/status.h"
#include "tensorflow/compiler/xla/xla.pb.h"
#include "tensorflow/compiler/xla/xla_data.pb.h"

namespace xla {

// Defines the interface for an XLA service on the client side. This service
// helps abstract around the actual implementation of a service - the service
// can be local (running in the same process), or remote - in which case an RPC
// stub is used as the implementation.
class ServiceInterface {
 public:
  ServiceInterface() {}
  virtual ~ServiceInterface() = default;

  // TODO(b/31824348): Convert to use StatusOr.
  virtual Status TransferToClient(const TransferToClientRequest* arg,
                                  TransferToClientResponse* result) = 0;

  virtual Status TransferToServer(const TransferToServerRequest* arg,
                                  TransferToServerResponse* result) = 0;

  virtual Status TransferToInfeed(const TransferToInfeedRequest* arg,
                                  TransferToInfeedResponse* result) = 0;

  virtual Status TransferFromOutfeed(const TransferFromOutfeedRequest* arg,
                                     TransferFromOutfeedResponse* result) = 0;

  virtual Status ResetDevice(const ResetDeviceRequest* arg,
                             ResetDeviceResponse* result) = 0;

  virtual Status Compile(const CompileRequest* arg,
                         CompileResponse* result) = 0;

  virtual Status Execute(const ExecuteRequest* arg,
                         ExecuteResponse* result) = 0;

  virtual Status ExecuteGraphParallel(const ExecuteGraphParallelRequest* arg,
                                      ExecuteParallelResponse* result) = 0;

  virtual Status WaitForExecution(const WaitForExecutionRequest* arg,
                                  WaitForExecutionResponse* result) = 0;

  virtual Status DeconstructTuple(const DeconstructTupleRequest* arg,
                                  DeconstructTupleResponse* result) = 0;

  virtual Status GetComputationGraphStats(
      const ComputationGraphStatsRequest* arg,
      ComputationStatsResponse* result) = 0;

  virtual Status GetShape(const GetShapeRequest* arg,
                          GetShapeResponse* result) = 0;

  virtual Status CreateChannelHandle(const CreateChannelHandleRequest* arg,
                                     CreateChannelHandleResponse* result) = 0;

  virtual Status GetDeviceHandles(const GetDeviceHandlesRequest* arg,
                                  GetDeviceHandlesResponse* result) = 0;

  virtual Status ComputeConstantGraph(const ComputeConstantGraphRequest* arg,
                                      ComputeConstantResponse* result) = 0;

  // Methods used by GlobalData.
  virtual Status Unregister(const UnregisterRequest* arg,
                            UnregisterResponse* result) = 0;
};

}  // namespace xla

#endif  // TENSORFLOW_COMPILER_XLA_SERVICE_INTERFACE_H_
