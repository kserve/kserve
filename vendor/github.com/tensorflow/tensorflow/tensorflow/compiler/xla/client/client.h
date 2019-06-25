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

#ifndef TENSORFLOW_COMPILER_XLA_CLIENT_CLIENT_H_
#define TENSORFLOW_COMPILER_XLA_CLIENT_CLIENT_H_

#include <memory>
#include <vector>

#include "absl/types/span.h"
#include "tensorflow/compiler/xla/client/global_data.h"
#include "tensorflow/compiler/xla/client/xla_computation.h"
#include "tensorflow/compiler/xla/literal.h"
#include "tensorflow/compiler/xla/service/hlo.pb.h"
#include "tensorflow/compiler/xla/service_interface.h"
#include "tensorflow/compiler/xla/statusor.h"
#include "tensorflow/compiler/xla/types.h"
#include "tensorflow/compiler/xla/xla.pb.h"
#include "tensorflow/compiler/xla/xla_data.pb.h"
#include "tensorflow/core/platform/macros.h"

namespace xla {

// XLA service's client object -- wraps the service with convenience and
// lifetime-oriented methods.
class Client {
 public:
  explicit Client(ServiceInterface* stub);
  virtual ~Client();

  // Compile the computation with the given argument shapes and returns the
  // handle to the compiled executable. The compiled executable is cached on the
  // service, and the returned handle can be used for exection without
  // re-compile.
  // * The shape and layout of the arguments being executed with will affect how
  //   the computation is compiled. If argument_shapes is empty, the parameters'
  //   shape and layout will be used in the compilation.
  // * If execution_options is not nullptr, these options are passed to the
  //   service to affect how it compiles our computation.  (The pointer does not
  //   need to live beyond this call.)
  // * If execution_options.device_handles should be empty. If you need
  //   non-empty device handles, call 'Execute' instead.
  StatusOr<ExecutionHandle> Compile(
      const XlaComputation& computation,
      absl::Span<const Shape> argument_shapes,
      const ExecutionOptions* execution_options = nullptr);

  // Executes the compiled executable for the given handle with the given
  // arguments and returns the global data that was produced from the execution.
  // * If execution_profile is not nullptr then the pointed-to ExecutionProfile
  //   will be filled with profile data from the execution.
  StatusOr<std::unique_ptr<GlobalData>> Execute(
      const ExecutionHandle& handle, absl::Span<GlobalData* const> arguments,
      ExecutionProfile* execution_profile = nullptr);

  // Executes the computation with the given arguments and returns the global
  // data that was produced from the execution.
  // * If execution_options is not nullptr, these options are passed to the
  //   service to affect how it compiles our computation.  (The pointer does not
  //   need to live beyond this call.)
  // * If execution_options.device_handles is not empty, the computation is
  //   executed on the devices associated with the handles by partitioning the
  //   computation based on the attached sharding attributes. Otherwise, a
  //   device is chosen by the service.
  // * If execution_profile is not nullptr then the pointed-to ExecutionProfile
  //   will be filled with profile data from the execution.
  StatusOr<std::unique_ptr<GlobalData>> Execute(
      const XlaComputation& computation,
      absl::Span<GlobalData* const> arguments,
      const ExecutionOptions* execution_options = nullptr,
      ExecutionProfile* execution_profile = nullptr);

  // A struct to represent a computation instance to be executed.
  // * If execution_options.device_handles is not empty, the computation is
  //   executed on the devices associated with the handles by partitioning the
  //   computation based on the attached sharding attributes. Otherwise, a
  //   device is chosen by the service.
  struct XlaComputationInstance {
    const XlaComputation& computation;
    std::vector<GlobalData*> arguments;
    ExecutionOptions execution_options;
    ExecutionProfile* execution_profile;

    XlaComputationInstance(const XlaComputation& computation,
                           std::vector<GlobalData*> arguments,
                           ExecutionOptions execution_options,
                           ExecutionProfile* execution_profile)
        : computation(computation),
          arguments(std::move(arguments)),
          execution_options(execution_options),
          execution_profile(execution_profile) {}
  };

  // Executes a list XlaComputationInstances and returns global data produced
  // from each computation.
  //
  StatusOr<std::vector<std::unique_ptr<GlobalData>>> ExecuteParallel(
      absl::Span<const XlaComputationInstance> computations);

  // Requests device_count device handles available on the target. The returned
  // device handles are used to specify the devices to execute the computations
  // (see ExecuteParallel) or to transfer data (see TransferToServer or
  // TransferToInfeed).
  StatusOr<std::vector<DeviceHandle>> GetDeviceHandles(int64 device_count);

  // Transfer the global data provided to this client process, which is
  // returned in the provided literal. Use sparingly to avoid transfer
  // overheads.
  //
  // If shape_with_layout is not nullptr, it points to a shape whose layout will
  // be the layout of the returned literal.
  StatusOr<Literal> Transfer(const GlobalData& data,
                             const Shape* shape_with_layout = nullptr);

  // Transfer the given literal to the server. This allocates memory on the
  // device and copies the literal's contents over. Returns a global data handle
  // that can be used to refer to this value from the client.
  //
  // If device_handle is not nullptr, data is transferred to the associated
  // device (and its replicas if replication is enabled). Otherwise, data is
  // transferred to the default device (and its replicas).
  StatusOr<std::unique_ptr<GlobalData>> TransferToServer(
      const LiteralSlice& literal, const DeviceHandle* device_handle = nullptr);

  // Transfer the given literal to the Infeed interface of the device.
  //
  // device_handle and replica_id together specify a particular device; a device
  // assigned for the given replica_id among the replicas that the given device
  // handle belongs to.
  Status TransferToInfeed(const LiteralSlice& literal, int64 replica_id = 0,
                          const DeviceHandle* device_handle = nullptr);

  // Transfers from the Outfeed of the device.
  //
  // device_handle and replica_id together specify a particular device; a device
  // assigned for the given replica_id among the replicas that the given device
  // handle belongs to.
  StatusOr<Literal> TransferFromOutfeed(
      const Shape* shape_with_layout, int64 replica_id = 0,
      const DeviceHandle* device_handle = nullptr);

  // Resets the device, clearing all existing state on the device.
  Status ResetDevice();

  // Executes the computation with the given arguments and transfers the result
  // to the client as a literal. Parameters are defined the same as for
  // Execute() and Transfer().
  StatusOr<Literal> ExecuteAndTransfer(
      const XlaComputation& computation,
      absl::Span<GlobalData* const> arguments,
      const ExecutionOptions* execution_options = nullptr,
      ExecutionProfile* execution_profile = nullptr);

  // Computes the value of the given computation using a non-optimized
  // interpreter on the host.
  //
  // The computation must not depend on any parameters, or on stateful operators
  // such as `RngNormal` or `Infeed`.
  //
  // This functionality can be useful when translating a computation into XLA
  // where something that looked dynamic is required by XLA to be specified as a
  // constant. E.g. the source computation (outside of XLA) may include a
  // dynamic computation of the shape of something and ComputeConstant lets you
  // determine what the value of that computation is in the case where the value
  // can be determined at compile time.
  //
  // If output_layout is non-null, then the output of the computation will be
  // stored using that layout.
  StatusOr<Literal> ComputeConstant(
      const XlaComputation& computation,
      const Layout* output_layout = nullptr) const;

  // Unregister the memory for the given GlobalData on the device.
  Status Unregister(const GlobalData& data);

  // Returns a vector of global data handles that point to the tuple elements.
  StatusOr<std::vector<std::unique_ptr<GlobalData>>> DeconstructTuple(
      const GlobalData& data);

  // Retrieves the statistics of the given computation.
  StatusOr<ComputationStats> GetComputationStats(
      const XlaComputation& computation,
      const DebugOptions& debug_options) const;

  // Returns the Shape of the given array specified by 'data'. The shape
  // includes the Layout of the array as it is stored on the service.
  StatusOr<Shape> GetShape(const GlobalData& data);

  // As above, but returns the shape of the provided computation (parameter
  // types/names and return type).
  StatusOr<std::unique_ptr<ProgramShape>> GetComputationShape(
      const XlaComputation& computation);

  // Creates a channel handle that can be used to transfer data between two
  // computations on different devices via a pair of Send and Recv instructions.
  StatusOr<ChannelHandle> CreateChannelHandle();

  // Create a channel for communicating with the host via a SendtoHost or
  // RecvFromHost operation.
  StatusOr<ChannelHandle> CreateHostToDeviceChannelHandle();
  StatusOr<ChannelHandle> CreateDeviceToHostChannelHandle();

  StatusOr<XlaComputation> LoadSnapshot(const HloSnapshot& module);

  ServiceInterface* stub() { return stub_; }

 private:
  // Returns the execution statistics (e.g., gflop/s) as a string from the
  // ExecutionProfile returned from an execution of the computation.
  StatusOr<string> ExecutionStatsAsString(const XlaComputation& computation,
                                          const ExecutionProfile& profile);

  StatusOr<ChannelHandle> CreateChannelHandleByType(
      ChannelHandle::ChannelType type);

  ServiceInterface* stub_;  // Stub that this client is connected on.

  TF_DISALLOW_COPY_AND_ASSIGN(Client);
};

}  // namespace xla

#endif  // TENSORFLOW_COMPILER_XLA_CLIENT_CLIENT_H_
