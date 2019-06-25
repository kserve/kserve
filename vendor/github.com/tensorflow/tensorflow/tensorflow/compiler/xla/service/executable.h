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

#ifndef TENSORFLOW_COMPILER_XLA_SERVICE_EXECUTABLE_H_
#define TENSORFLOW_COMPILER_XLA_SERVICE_EXECUTABLE_H_

#include <memory>
#include <utility>
#include <vector>

#include "absl/types/span.h"
#include "absl/types/variant.h"
#include "tensorflow/compiler/xla/debug_options_flags.h"
#include "tensorflow/compiler/xla/service/computation_layout.h"
#include "tensorflow/compiler/xla/service/device_memory_allocator.h"
#include "tensorflow/compiler/xla/service/hlo.pb.h"
#include "tensorflow/compiler/xla/service/hlo_execution_profile.h"
#include "tensorflow/compiler/xla/service/hlo_graph_dumper.h"
#include "tensorflow/compiler/xla/service/hlo_module.h"
#include "tensorflow/compiler/xla/service/maybe_owning_device_memory.h"
#include "tensorflow/compiler/xla/service/owning_device_memory.h"
#include "tensorflow/compiler/xla/service/service_executable_run_options.h"
#include "tensorflow/compiler/xla/service/shaped_buffer.h"
#include "tensorflow/compiler/xla/shape_tree.h"
#include "tensorflow/compiler/xla/statusor.h"
#include "tensorflow/compiler/xla/util.h"
#include "tensorflow/compiler/xla/xla_data.pb.h"
#include "tensorflow/core/platform/mutex.h"
#include "tensorflow/core/platform/stream_executor_no_cuda.h"
#include "tensorflow/core/platform/thread_annotations.h"

namespace xla {

// ExecutionOutput encapsulates the output buffers of a execution and the
// leftover buffers to be released by the caller.
struct ExecutionOutput {
  ExecutionOutput(ScopedShapedBuffer result,
                  std::vector<OwningDeviceMemory> to_be_released)
      : result(std::move(result)), to_be_released(std::move(to_be_released)) {}
  ScopedShapedBuffer result;

  // Leftover buffers for the caller to release. Elements in this list are
  // donated input memory buffers that are not reused by XLA as outputs.
  std::vector<OwningDeviceMemory> to_be_released;
};

// A given platform's compiler will produce an Executable -- this is a uniform
// interface that is used for launching compiled programs across platforms.
class Executable {
 public:
  explicit Executable(
      std::unique_ptr<HloModule> hlo_module,
      std::unique_ptr<HloProfilePrinterData> hlo_profile_printer_data,
      std::unique_ptr<HloProfileIndexMap> hlo_profile_index_map)
      : hlo_module_(std::move(hlo_module)),
        hlo_profile_printer_data_(std::move(hlo_profile_printer_data)),
        hlo_profile_index_map_(std::move(hlo_profile_index_map)) {
    CHECK_EQ(hlo_profile_printer_data_.get() == nullptr,
             hlo_profile_index_map_.get() == nullptr);
  }
  virtual ~Executable() {}

  // Enqueues the compilation result on the provided stream, passing the given
  // arguments. This call is blocking and returns after the execution is done.
  //
  // If the hlo_execution_profile is provided as non-nullptr, profiling will be
  // enabled.
  //
  // Returns a shaped buffer containing the result of the computation.
  virtual StatusOr<ScopedShapedBuffer> ExecuteOnStream(
      const ServiceExecutableRunOptions* run_options,
      absl::Span<const ShapedBuffer* const> arguments,
      HloExecutionProfile* hlo_execution_profile) = 0;

  // Same as ExecuteOnStream(), but this call is non-blocking and returns as
  // soon as all of the operations are enqueued for launch on the stream.
  virtual StatusOr<ScopedShapedBuffer> ExecuteAsyncOnStream(
      const ServiceExecutableRunOptions* run_options,
      absl::Span<const ShapedBuffer* const> arguments) = 0;

  // Starts the given program executing on the given stream/executor.
  //
  // `arguments` are ShapeTree containing the input parameters. For each element
  // in the shape tree, if the element holds the ownership of the memory, it is
  // considered donated and XLA will potentially reuse it as output buffers. For
  // all donated inputs, XLA is also responsible for freeing them.
  //
  // If an input is donated to XLA but is not reused as output, it is returned
  // as an leftover buffer for the caller to release.
  virtual StatusOr<ExecutionOutput> ExecuteOnStream(
      const ServiceExecutableRunOptions* run_options,
      std::vector<ShapeTree<xla::MaybeOwningDeviceMemory>> arguments,
      HloExecutionProfile* hlo_execution_profile) {
    return Unimplemented(
        "MaybeOwningDeviceMemory version of overload is not implemented ");
  }

  virtual StatusOr<ExecutionOutput> ExecuteAsyncOnStream(
      const ServiceExecutableRunOptions* run_options,
      std::vector<ShapeTree<xla::MaybeOwningDeviceMemory>> arguments) {
    return Unimplemented(
        "MaybeOwningDeviceMemory version of overload is not implemented ");
  }

  // Same as ExecuteOnStream(), but runs this executable on multiple
  // streams. arguments[i] contains the arguments to the execution on
  // run_options[i]->stream() and the returned value is at index i of the
  // returned vector.
  virtual StatusOr<std::vector<ScopedShapedBuffer>> ExecuteOnStreams(
      absl::Span<const ServiceExecutableRunOptions> run_options,
      absl::Span<const absl::Span<const ShapedBuffer* const>> arguments);

  // Populates `hlo_execution_profile` from `executor`. This is implicit in any
  // Execute* API call that takes a hlo_execution_profile argument, but must be
  // called explicitly for other (async, for example) variants after the stream
  // has completed.
  virtual Status PopulateExecutionProfile(
      HloExecutionProfile* hlo_execution_profile, se::Stream* stream) {
    return Status::OK();
  }

  // Convenience wrapper for calling Executable::ExecuteOnStream. Sets up a
  // timer for the execution, sets up HLO profiling if enabled, and fills in the
  // given ExecutionProfile if non-null.
  StatusOr<ScopedShapedBuffer> ExecuteOnStreamWrapper(
      const ServiceExecutableRunOptions* run_options, ExecutionProfile* profile,
      absl::Span<const ShapedBuffer* const> arguments);

  // Returns the ExecutionProfile from executing on the device. This includes
  // the number of cycles taken for the computation or the compilation time.
  ExecutionProfile execution_profile() const {
    tensorflow::mutex_lock lock(mutex_);
    return execution_profile_;
  }

  const HloProfilePrinterData& hlo_profile_printer_data() const {
    CHECK(hlo_profiling_enabled());
    return *hlo_profile_printer_data_;
  }

  const HloProfileIndexMap& hlo_profile_index_map() const {
    CHECK(hlo_profiling_enabled());
    return *hlo_profile_index_map_;
  }

  // Returns whether this executable was compiled with HLO profilings support
  // enabled. If not, the caller should not expect an hlo_execution_profile
  // passed to ExecuteOnStream above to be populated during execution.
  bool hlo_profiling_enabled() const {
    return hlo_profile_printer_data_ != nullptr;
  }

  HloModule& module() const { return *hlo_module_; }

  const bool has_module() const { return hlo_module_ != nullptr; }

  const HloModuleConfig& module_config() const { return hlo_module_->config(); }

  // The shape (including layout) that results from this execution. This is the
  // shape of the DeviceMemoryBase result value in ExecuteOnStream above.
  const Shape& result_shape() const {
    return hlo_module_->config().entry_computation_layout().result_shape();
  }

  // Returns the size of the executable in bytes. Returns -1 by default if the
  // method is not overridden to support this kind of query.
  virtual int64 SizeInBytes();

  // Dumping helpers.
  void set_hlo_snapshot(std::unique_ptr<xla::HloSnapshot> hlo_snapshot) {
    hlo_snapshot_ = std::move(hlo_snapshot);
  }
  bool dumping_snapshot() const { return hlo_snapshot_ != nullptr; }
  HloSnapshot* hlo_snapshot() const { return hlo_snapshot_.get(); }
  Status DumpHloSnapshot();

  // Dump hlo snapshot to directory_path/filename.
  static Status DumpToDirectory(const string& directory_path, string filename,
                                const HloSnapshot& hlo_session);

 protected:
  mutable tensorflow::mutex mutex_;

  // Execution profile data on the device.
  ExecutionProfile execution_profile_ GUARDED_BY(mutex_);

  // HloModule this was compiled from. BufferAssignment keeps pointers to
  // HloInstructions owned by the HloModule so we need to keep the HloModule
  // around.
  const std::unique_ptr<HloModule> hlo_module_;

  // HloSnapshot this was compiled from. Null if not dumping executions.
  std::unique_ptr<HloSnapshot> hlo_snapshot_;

  // Execution count, used to generate a unique filename for each dumped
  // execution.
  int64 execution_count_ = 0;

  std::unique_ptr<HloProfilePrinterData> hlo_profile_printer_data_;
  std::unique_ptr<HloProfileIndexMap> hlo_profile_index_map_;
};

}  // namespace xla

#endif  // TENSORFLOW_COMPILER_XLA_SERVICE_EXECUTABLE_H_
