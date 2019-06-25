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

// TODO(opensource): Use a more generic sounding preprocessor name than
// GOOGLE_CUDA
#if GOOGLE_CUDA

#define EIGEN_USE_GPU

#include "tensorflow/core/common_runtime/gpu/gpu_device.h"

#include <stdlib.h>
#include <string.h>
#include <algorithm>
#include <list>
#include <map>
#include <tuple>
#include <vector>

#include "third_party/eigen3/unsupported/Eigen/CXX11/Tensor"
#include "tensorflow/core/common_runtime/device_factory.h"
#include "tensorflow/core/common_runtime/gpu/gpu_event_mgr.h"
#include "tensorflow/core/common_runtime/gpu/gpu_id.h"
#include "tensorflow/core/common_runtime/gpu/gpu_id_manager.h"
#include "tensorflow/core/common_runtime/gpu/gpu_id_utils.h"
#include "tensorflow/core/common_runtime/gpu/gpu_init.h"
#include "tensorflow/core/common_runtime/gpu/gpu_process_state.h"
#include "tensorflow/core/common_runtime/gpu/gpu_stream_util.h"
#include "tensorflow/core/common_runtime/gpu/gpu_util.h"
#include "tensorflow/core/common_runtime/gpu_device_context.h"
#include "tensorflow/core/common_runtime/local_device.h"
#include "tensorflow/core/framework/allocator.h"
#include "tensorflow/core/framework/device_base.h"
#include "tensorflow/core/framework/op_kernel.h"
#include "tensorflow/core/framework/tensor.h"
#include "tensorflow/core/framework/tensor.pb.h"
#include "tensorflow/core/framework/types.h"
#include "tensorflow/core/framework/variant_op_registry.h"
#include "tensorflow/core/graph/types.h"
#include "tensorflow/core/lib/core/errors.h"
#include "tensorflow/core/lib/core/status.h"
#include "tensorflow/core/lib/gtl/stl_util.h"
#include "tensorflow/core/lib/strings/numbers.h"
#include "tensorflow/core/lib/strings/str_util.h"
#include "tensorflow/core/lib/strings/strcat.h"
#include "tensorflow/core/platform/cuda.h"
#include "tensorflow/core/platform/logging.h"
#include "tensorflow/core/platform/macros.h"
#include "tensorflow/core/platform/stream_executor.h"
#include "tensorflow/core/platform/tracing.h"
#include "tensorflow/core/platform/types.h"
#include "tensorflow/core/public/session_options.h"
#include "tensorflow/core/util/device_name_utils.h"
#include "tensorflow/core/util/env_var.h"
#include "tensorflow/core/util/stream_executor_util.h"

#if !defined(PLATFORM_GOOGLE)
#include "cuda/cuda_config.h"
#endif

namespace tensorflow {

// Eigen Ops directly allocate memory only for temporary buffers used
// during OpKernel::Compute().  The recommended way of allocating such
// memory is via OpKernelContext::allocate_temp().  However, Eigen Ops
// don't have access to OpKernelContext, instead they get access to
// memory directly through the device allocator.  As an Open Source
// project, Eigen assumes allocator semantics similar to those of the
// CUDA memory allocator, and may not work correctly due to race
// conditions if used with some other allocator.  For safety, we need
// to delay deallocation calls out of Eigen until all events on the
// corresponding stream have completed.  The following two classes
// serve this purpose in two different compilation environments.

class EigenGpuStreamDevice : public ::Eigen::StreamInterface {
 public:
  EigenGpuStreamDevice()
      : scratch_(nullptr), semaphore_(nullptr), context_(nullptr) {
    Eigen::initializeDeviceProp();
  }
  ~EigenGpuStreamDevice() override {}
  void Reinitialize(OpKernelContext* context, const cudaStream_t* cuda_stream,
                    TfGpuId tf_gpu_id, ::tensorflow::Allocator* alloc,
                    char* scratch) {
    if (LogMemory::IsEnabled()) {
      operation_ = context->op_kernel().name() + "/EigenAllocator";
      step_id_ = context->step_id();
    }
    context_ = context;
    scratch_ = scratch;
    semaphore_ =
        reinterpret_cast<unsigned int*>(scratch + Eigen::kGpuScratchSize);
    stream_ = cuda_stream;
    allocator_ = alloc;
    PlatformGpuId platform_gpu_id;
    TF_CHECK_OK(GpuIdManager::TfToPlatformGpuId(tf_gpu_id, &platform_gpu_id));
    device_prop_ = &Eigen::m_deviceProperties[platform_gpu_id.value()];
  }

  const cudaStream_t& stream() const override { return *stream_; }
  const cudaDeviceProp& deviceProperties() const override {
    return *device_prop_;
  }

  void* allocate(size_t num_bytes) const override {
    void* ret = allocator_->AllocateRaw(32 /* alignment */, num_bytes);
    if (ret == nullptr) {
      if (context_) {
        context_->SetStatus(errors::ResourceExhausted(
            strings::StrCat("Ran out of GPU memory when allocating ", num_bytes,
                            " bytes for ", operation_)));
      } else {
        LOG(FATAL)
            << "EigenAllocator for GPU ran out of memory when allocating "
            << num_bytes << ". See error logs for more detailed info.";
      }
    }
    if (LogMemory::IsEnabled() && ret != nullptr) {
      LogMemory::RecordRawAllocation(operation_, step_id_, num_bytes, ret,
                                     allocator_);
    }
    return ret;
  }
  void deallocate(void* buffer) const override {
    if (LogMemory::IsEnabled() && buffer != nullptr) {
      LogMemory::RecordRawDeallocation(operation_, step_id_, buffer, allocator_,
                                       true);
    }
    AsyncFreeData* afData =
        new AsyncFreeData(allocator_, buffer, operation_, step_id_);
    cudaError_t err = cudaStreamAddCallback(*stream_, asyncFree, afData, 0);
    CHECK_EQ(err, cudaSuccess);
  }

  // Return a pointer to a per stream scratchpad of 1024 bytes residing
  // in global memory.
  void* scratchpad() const override { return scratch_; }

  // Return a semaphore. The semaphore is initially initialized to 0, and
  // each kernel using it is responsible for resetting to 0 upon completion
  // to maintain the invariant that the semaphore is always equal to 0 upon
  // each kernel start.
  unsigned int* semaphore() const override { return semaphore_; }

 private:
  struct AsyncFreeData {
    AsyncFreeData(::tensorflow::Allocator* a, void* p, const string& o,
                  const int64 s)
        : allocator_(a), address_(p), operation_(o), step_id_(s) {}
    ::tensorflow::Allocator* allocator_;
    void* address_;
    const string operation_;
    const int64 step_id_;
  };

  static void CUDART_CB asyncFree(cudaStream_t stream, cudaError_t status,
                                  void* userData) {
    AsyncFreeData* data = static_cast<AsyncFreeData*>(userData);
    if (LogMemory::IsEnabled()) {
      LogMemory::RecordRawDeallocation(data->operation_, data->step_id_,
                                       data->address_, data->allocator_, false);
    }
    data->allocator_->DeallocateRaw(data->address_);
    delete data;
  }

  string operation_;
  int64 step_id_;
  const cudaStream_t* stream_;          // Not owned.
  const cudaDeviceProp* device_prop_;   // Not owned.
  ::tensorflow::Allocator* allocator_;  // Not owned.
  mutable char* scratch_;
  mutable unsigned int* semaphore_;
  OpKernelContext* context_;

  TF_DISALLOW_COPY_AND_ASSIGN(EigenGpuStreamDevice);
};

// This factory helps to ensure that different GPU device objects that refer to
// the same physical device and stream group id use the same stream group
// object (and therefore the same CUDA streams). This is necessary since there
// is a single memory allocator per device (see ProcessState::GetGPUAllocator)
// and allocators must not be shared across streams.
class BaseGPUDevice::StreamGroupFactory {
 public:
  // Returns the unique stream group for use with the stream defined by
  // {tf_gpu_id, stream_group_within_gpu}, creating it if it does not yet
  // exist.
  // This function is thread safe.
  BaseGPUDevice::StreamGroup* GetOrCreate(TfGpuId tf_gpu_id,
                                          int stream_group_within_gpu,
                                          se::StreamExecutor* executor,
                                          const GPUOptions& options) {
    mutex_lock guard(lock_);
    StreamGroup* group =
        &streams_[key_type(tf_gpu_id.value(), stream_group_within_gpu)];
    if (!group->compute) {
      group->compute = new se::Stream(executor);
      group->compute->Init();
      VLOG(2) << "Created stream[" << stream_group_within_gpu
              << "] = " << group->compute;

      group->host_to_device = new se::Stream(executor);
      group->host_to_device->Init();
      VLOG(2) << "Created host_to_device_stream[" << stream_group_within_gpu
              << "] = " << group->host_to_device;

      group->device_to_host = new se::Stream(executor);
      group->device_to_host->Init();
      VLOG(2) << "Created device_to_host_stream[" << stream_group_within_gpu
              << "] = " << group->device_to_host;

      int num_d2d_streams =
          options.experimental().num_dev_to_dev_copy_streams();
      if (num_d2d_streams == 0) num_d2d_streams = 1;
      if (num_d2d_streams < 1 || num_d2d_streams > 4) {
        LOG(ERROR)
            << "Illegal GPUOptions.experimental.num_dev_to_dev_copy_streams="
            << num_d2d_streams << " set to 1 instead.";
        num_d2d_streams = 1;
      }
      for (int i = 0; i < num_d2d_streams; ++i) {
        se::Stream* stream = new se::Stream(executor);
        stream->Init();
        group->device_to_device.push_back(stream);
        VLOG(2) << "Created device_to_device_stream[" << stream_group_within_gpu
                << "] = " << group->device_to_device.back();
      }
    }
    return group;
  }

  // Returns a reference to the StreamGroupFactory singleton. Note that this is
  // never destroyed, so the objects it owns are never deleted.
  static StreamGroupFactory& Global() {
    static StreamGroupFactory* instance = new StreamGroupFactory();
    return *instance;
  }

 private:
  mutex lock_;
  using key_type = std::tuple<int, int>;
  std::map<key_type, StreamGroup> streams_;

  // StreamGroupFactory cannot be created directly; Call
  // StreamGroupFactory::Global() to get the global instance.
  StreamGroupFactory() = default;
  TF_DISALLOW_COPY_AND_ASSIGN(StreamGroupFactory);
};

BaseGPUDevice::BaseGPUDevice(const SessionOptions& options, const string& name,
                             Bytes memory_limit, const DeviceLocality& locality,
                             TfGpuId tf_gpu_id,
                             const string& physical_device_desc,
                             Allocator* gpu_allocator, Allocator* cpu_allocator,
                             bool sync_every_op, int32 max_streams)
    : LocalDevice(options, Device::BuildDeviceAttributes(name, DEVICE_GPU,
                                                         memory_limit, locality,
                                                         physical_device_desc)),
      gpu_allocator_(gpu_allocator),
      cpu_allocator_(cpu_allocator),
      scoped_allocator_mgr_(new ScopedAllocatorMgr(name)),
      tf_gpu_id_(tf_gpu_id),
      sync_every_op_(sync_every_op),
      max_streams_(max_streams) {
  GPUProcessState::singleton()->EnableGPUDevice();
}

BaseGPUDevice::~BaseGPUDevice() {
  delete gpu_device_info_;
  for (auto sb : scratch_) gpu_allocator_->DeallocateRaw(sb);
  for (auto ctx : device_contexts_) ctx->Unref();
}

// This should be idempotent if already initialized.
Status BaseGPUDevice::InitScratchBuffers() {
  mutex_lock l(scratch_init_mutex_);
  if (scratch_.size() < max_streams_) {
    for (int i = 0; i < max_streams_; i++) {
      DCHECK(streams_[i]);
      if (scratch_.size() > i && scratch_[i]) continue;
      size_t scratch_buffer_size =
          Eigen::kGpuScratchSize + sizeof(unsigned int);
      void* scratch_buffer = gpu_allocator_->AllocateRaw(
          Allocator::kAllocatorAlignment, scratch_buffer_size);
      if (scratch_buffer == nullptr) {
        return errors::FailedPrecondition(
            "Failed to allocate scratch buffer for device ",
            tf_gpu_id_.value());
      }
      se::DeviceMemory<char> mem(
          se::DeviceMemoryBase(scratch_buffer, scratch_buffer_size));

      bool ok = executor_->SynchronousMemZero(
          &mem, Eigen::kGpuScratchSize + sizeof(unsigned int));
      if (!ok) {
        return errors::FailedPrecondition(
            "Failed to memcopy into scratch buffer for device ",
            tf_gpu_id_.value());
      }
      scratch_.push_back(static_cast<char*>(scratch_buffer));
    }
  }
  return Status::OK();
}

Status BaseGPUDevice::Init(const SessionOptions& options) {
  auto executor_status = GpuIdUtil::ExecutorForTfGpuId(tf_gpu_id_);
  if (!executor_status.status().ok()) {
    return errors::Internal("Failed to get StreamExecutor for device ",
                            tf_gpu_id_.value());
  }

  executor_ = executor_status.ValueOrDie();
  em_.reset(new EventMgr(executor_, options.config.gpu_options()));

  if (max_streams_ < 1) {
    return errors::InvalidArgument("Invalid value for max_streams.");
  }

  // Create the specified number of GPU streams
  for (int i = 0; i < max_streams_; i++) {
    streams_.push_back(StreamGroupFactory::Global().GetOrCreate(
        tf_gpu_id_, i, executor_, options.config.gpu_options()));
    device_contexts_.push_back(new GPUDeviceContext(
        i, streams_.back()->compute, streams_.back()->host_to_device,
        streams_.back()->device_to_host, streams_.back()->device_to_device));
  }
  gpu_device_info_ = new GpuDeviceInfo;
  gpu_device_info_->stream = streams_[0]->compute;
  gpu_device_info_->default_context = device_contexts_[0];
  gpu_device_info_->event_mgr = em_.get();
  PlatformGpuId platform_gpu_id;
  TF_RETURN_IF_ERROR(
      GpuIdManager::TfToPlatformGpuId(tf_gpu_id_, &platform_gpu_id));
  gpu_device_info_->gpu_id = platform_gpu_id.value();
  set_tensorflow_gpu_device_info(gpu_device_info_);

  // Whether and how the GPU device uses its own threadpool.
  // This option is experimental. Once we confirm the best setting, we
  // may change the default behavior and completely remove this flag.
  // Default values might change in future releases.
  // Possible values:
  //   * global: GPU uses threads shared with CPU in the main compute
  //          thread-pool. This is currently the default.
  //   * gpu_private: GPU uses threads dedicated to this device.
  //   * gpu_shared: All GPUs share a dedicated thread pool.
  string gpu_thread_mode;
  TF_RETURN_IF_ERROR(
      ReadStringFromEnvVar("TF_GPU_THREAD_MODE", "global", &gpu_thread_mode));
  gpu_thread_mode = str_util::Lowercase(gpu_thread_mode);
  if (gpu_thread_mode != "global") {
    int64 gpu_thread_count = -1;
    // Default to two threads. One for device compute and another for memory
    // copies.
    TF_RETURN_IF_ERROR(
        ReadInt64FromEnvVar("TF_GPU_THREAD_COUNT", 2, &gpu_thread_count));
    if (gpu_thread_mode == "gpu_private") {
      // TODO(zhengxq): since these threads only serve a single GPU device,
      //   we should set the device context once for each thread, and avoid
      //   setting them for each kernel.
      // TODO(zhengxq): pin the thread to the same socket of the target GPU.
      thread_pool_.reset(new thread::ThreadPool(
          options.env, strings::StrCat("gpu_private_", tf_gpu_id_.value()),
          static_cast<int32>(gpu_thread_count)));
      set_tensorflow_device_thread_pool(thread_pool_.get());
    } else if (gpu_thread_mode == "gpu_shared") {
      static thread::ThreadPool* thread_pool = new thread::ThreadPool(
          options.env, "gpu_shared", static_cast<int32>(gpu_thread_count));
      set_tensorflow_device_thread_pool(thread_pool);
    } else {
      string error_message =
          strings::StrCat("Invalid gpu_thread_mode: ", gpu_thread_mode);
      LOG(WARNING) << error_message;
      return errors::InvalidArgument(error_message);
    }
  }

  return Status::OK();
}

bool BaseGPUDevice::RequiresRecordingAccessedTensors() const {
  // When there is no more than one stream, we release the tensor reference
  // at the end of the kernel launch, instead of at the end of the kernel
  // execution.
  return streams_.size() > 1;
}

Status BaseGPUDevice::FillContextMap(const Graph* graph,
                                     DeviceContextMap* device_context_map) {
  VLOG(2) << "FillContextMap";

  const size_t num_streams = streams_.size();
  // Special case for single stream.
  if (num_streams == 1) {
    return Status::OK();
  }
  const int64 before = Env::Default()->NowMicros();
  gpu_stream_util::AssignStreamsOpts opts;
  opts.max_streams = static_cast<int32>(num_streams);
  std::unordered_map<int, int> node_to_stream_id;
  TF_RETURN_IF_ERROR(
      gpu_stream_util::AssignStreams(graph, opts, &node_to_stream_id));
  int64 elapsed = Env::Default()->NowMicros() - before;
  VLOG(3) << "AssignStreams took " << elapsed << "us";

  // Fill in the context map.  It is OK for this map to contain
  // duplicate DeviceContexts so long as we increment the refcount.
  device_context_map->resize(graph->num_node_ids());
  for (Node* n : graph->nodes()) {
    auto mapped_stream = node_to_stream_id[n->id()];
    CHECK_LE(mapped_stream, num_streams);
    auto ctx = device_contexts_[mapped_stream];
    VLOG(3) << "Assigned stream " << node_to_stream_id[n->id()]
            << " ==> stream[" << ctx->stream_id() << "] for node id " << n->id()
            << " " << n->type_string() << " " << n->name();
    ctx->Ref();
    (*device_context_map)[n->id()] = ctx;
  }

  return Status::OK();
}

void BaseGPUDevice::Compute(OpKernel* op_kernel, OpKernelContext* context) {
  // NOTE(tucker): We need to discriminate between Eigen GPU
  // operations and all others.  If an operation is Eigen
  // implemented (or otherwise tries to launch a cuda kernel
  // directly), we need to establish a stacked-scoped environment
  // that directs it to execute on the proper device.  Otherwise we
  // expect the Op to use StreamExecutor directly and correctly.  The
  // way we make this discrimination is quite hacky: At the moment
  // the only non-Eigen GPU Op is the recv-op, which is known to be
  // asynchronous.
  if (op_kernel->is_internal() && op_kernel->type_string() == "_Recv") {
    context->SetStatus(errors::Internal(
        "Invalid synchronous 'Compute' on GPU for '_Recv' op"));
  } else {
    ComputeHelper(op_kernel, context);
  }
}

string BaseGPUDevice::ComputeOpKernelDebugString(const OpKernel& op_kernel,
                                                 const int& stream_id) {
  return strings::StrCat(op_kernel.name(), " op ", op_kernel.type_string(),
                         " on GPU ", tf_gpu_id_.value(), " stream[", stream_id,
                         "]");
}

void BaseGPUDevice::ComputeHelper(OpKernel* op_kernel,
                                  OpKernelContext* context) {
  GPUDeviceContext* gpu_device_context = device_contexts_[0];
  if (context->op_device_context() != nullptr) {
    gpu_device_context =
        static_cast<GPUDeviceContext*>(context->op_device_context());
  }
  se::Stream* stream = gpu_device_context->stream();
  const auto stream_id = gpu_device_context->stream_id();

  const bool vlog_1 = VLOG_IS_ON(1);
  const bool vlog_2 = vlog_1 && VLOG_IS_ON(2);

  if (vlog_1) {
    VLOG(1) << "GpuDevice::ComputeHelper "
            << ComputeOpKernelDebugString(*op_kernel, stream_id);
  }

  const auto num_streams = streams_.size();
  if (num_streams > 1) {
    // If this op's device context is different from the other contexts,
    // we must wait on the stream.
    for (int i = 0; i < context->num_inputs(); ++i) {
      const GPUDeviceContext* idc =
          static_cast<GPUDeviceContext*>(context->input_device_context(i));
      OP_REQUIRES(context, idc != nullptr,
                  errors::Internal("Input device context ", i,
                                   " was not set properly."));
      if (vlog_2) {
        const void* base;
        size_t len;
        if (context->has_input(i)) {
          if (IsRefType(context->input_dtype(i))) {
            Tensor tensor = context->mutable_input(i, false);
            base = DMAHelper::base(&tensor);
            len = tensor.TotalBytes();
          } else {
            const Tensor& tensor = context->input(i);
            base = DMAHelper::base(&tensor);
            len = tensor.TotalBytes();
          }
          LOG(INFO) << "Input " << i << " " << base << "  " << len;
          LOG(INFO) << "  stream[" << stream_id << "].ThenWaitFor(stream["
                    << idc->stream_id() << "])"
                    << ((idc->stream() == stream) ? " not needed" : "");
        }
      }
      if (idc->stream() != stream) stream->ThenWaitFor(idc->stream());
    }
  }
  se::cuda::ScopedActivateExecutorContext scoped_activation{stream->parent()};
  op_kernel->Compute(context);
  if (context->status().ok()) {
    if (sync_every_op_) {
      // Note: GPUUtil::Sync() only syncs the default stream.
      // We need to either sync the stream used by this op, or
      // all streams.  Given that this flag is typically used for
      // debugging it makes more sense to sync all GPU activity.
      context->SetStatus(GPUUtil::SyncAll(this));
      if (vlog_1) {
        VLOG(1) << "GpuDevice::ComputeHelper finished "
                << ComputeOpKernelDebugString(*op_kernel, stream_id);
      }
    } else if (vlog_1) {
      VLOG(1) << "GpuDevice::ComputeHelper scheduled "
              << ComputeOpKernelDebugString(*op_kernel, stream_id);
    }
  } else {
    if (vlog_1) {
      VLOG(1) << "GpuDevice::ComputeHelper failed to schedule "
              << ComputeOpKernelDebugString(*op_kernel, stream_id);
    }
  }
}

void BaseGPUDevice::ConsumeListOfAccessedTensors(
    DeviceContext* device_context, const TensorReferenceVector& tensor_refs) {
  GPUDeviceContext* gpu_device_context = device_contexts_[0];
  if (device_context != nullptr) {
    gpu_device_context = static_cast<GPUDeviceContext*>(device_context);
  }
  se::Stream* stream = gpu_device_context->stream();
  em_->ThenDeleteTensors(stream, tensor_refs);
}

// Based on the semantics of Device::Sync this call should wait for
// all streams not just the current one.
Status BaseGPUDevice::Sync() { return GPUUtil::SyncAll(this); }

void BaseGPUDevice::ComputeAsync(AsyncOpKernel* op_kernel,
                                 OpKernelContext* context,
                                 AsyncOpKernel::DoneCallback done) {
  GPUDeviceContext* gpu_device_context = device_contexts_[0];
  if (context->op_device_context() != nullptr) {
    gpu_device_context =
        static_cast<GPUDeviceContext*>(context->op_device_context());
  }
  se::Stream* stream = gpu_device_context->stream();
  const auto stream_id = gpu_device_context->stream_id();

  VLOG(1) << "GpuDevice::ComputeAsync " << op_kernel->name() << " op "
          << op_kernel->type_string() << " on GPU" << tf_gpu_id_ << " stream["
          << stream_id << "]";

  // When Xprof profiling is off (which is the default), constructing the
  // activity is simple enough that its overhead is negligible.
  tracing::ScopedActivity activity(op_kernel->name(), op_kernel->type_string(),
                                   op_kernel->IsExpensive());
  se::cuda::ScopedActivateExecutorContext scoped_activation{stream->parent()};
  op_kernel->ComputeAsync(context, done);
}

Status BaseGPUDevice::MaybeCopyTensorToGPU(
    const AllocatorAttributes& alloc_attrs, const Tensor& from, Tensor* to,
    StatusCallback done) {
  if (alloc_attrs.on_host()) {
    *to = from;
    done(Status::OK());
    return Status::OK();
  } else {
    if (!DMAHelper::CanUseDMA(&from)) {
      Status err = errors::Internal("GPU copy from non-DMA ",
                                    DataTypeString(from.dtype()), " tensor");
      done(err);
      return err;
    }
    auto* copy =
        new Tensor(GetAllocator(alloc_attrs), from.dtype(), from.shape());

    // If the tensor is not initialized, we likely ran out of memory.
    if (!copy->IsInitialized()) {
      delete copy;
      Status err = errors::ResourceExhausted(
          "OOM when allocating tensor of shape ", from.shape().DebugString(),
          " and type ", DataTypeString(from.dtype()));
      done(err);
      return err;
    }

    StatusCallback wrapped_done = std::bind(
        [to, copy](StatusCallback done_,
                   // Begin unbound arguments.
                   const Status& s) {
          if (s.ok()) {
            *to = std::move(*copy);
          }
          delete copy;
          done_(s);
        },
        std::move(done), std::placeholders::_1);

    tracing::ScopedAnnotation annotation("MakeTensorFromProto");
    device_contexts_[0]->CopyCPUTensorToDevice(&from, this, copy,
                                               std::move(wrapped_done));
    return Status::OK();
  }
}

Status BaseGPUDevice::MakeTensorFromProto(const TensorProto& tensor_proto,
                                          const AllocatorAttributes alloc_attrs,
                                          Tensor* tensor) {
  AllocatorAttributes attr;
  attr.set_on_host(true);
  attr.set_gpu_compatible(true);
  Allocator* host_alloc = GetAllocator(attr);
  Tensor parsed(tensor_proto.dtype());
  if (!parsed.FromProto(host_alloc, tensor_proto)) {
    return errors::InvalidArgument("Cannot parse tensor from proto: ",
                                   tensor_proto.DebugString());
  }

  if (parsed.dtype() == DT_VARIANT) {
    const Variant* from = parsed.flat<Variant>().data();
    Tensor copy(cpu_allocator(), DT_VARIANT, parsed.shape());
    Variant* copy_variant = copy.flat<Variant>().data();

    std::list<Notification> notifications;
    Status copy_status;
    auto copier = [this, &alloc_attrs, &notifications, &copy_status](
                      const Tensor& from, Tensor* to) {
      // Copier isn't run in a multithreaded environment, so we don't
      // have to worry about the notifications list being modified in parallel.
      notifications.emplace_back();
      Notification& n = *notifications.rbegin();
      return MaybeCopyTensorToGPU(alloc_attrs, from, to,
                                  [&n, &copy_status](const Status& s) {
                                    if (copy_status.ok()) {
                                      copy_status.Update(s);
                                    }
                                    n.Notify();
                                  });
    };
    Status s;
    for (int64 ix = 0; ix < parsed.NumElements(); ++ix) {
      s = VariantDeviceCopy(VariantDeviceCopyDirection::HOST_TO_DEVICE,
                            from[ix], &copy_variant[ix], copier);
      if (!s.ok()) {
        break;
      }
    }
    for (auto& n : notifications) {
      n.WaitForNotification();
    }
    if (!s.ok()) {
      return s;
    }
    *tensor = std::move(copy);
    return copy_status;
  } else {
    Notification n;
    Status status;
    TF_RETURN_IF_ERROR(MaybeCopyTensorToGPU(alloc_attrs, parsed, tensor,
                                            [&n, &status](const Status& s) {
                                              status = s;
                                              n.Notify();
                                            }));
    n.WaitForNotification();
    return status;
  }
}

namespace {
class ConcretePerOpGpuDevice : public PerOpGpuDevice {
 public:
  ConcretePerOpGpuDevice() : device_(&stream_device_) {}

  void Reinitialize(OpKernelContext* context, const cudaStream_t* cuda_stream,
                    TfGpuId tf_gpu_id, Allocator* base_allocator,
                    char* scratch) {
    stream_device_.Reinitialize(context, cuda_stream, tf_gpu_id, base_allocator,
                                scratch);
  }

  const Eigen::GpuDevice& device() const override { return device_; }

 private:
  EigenGpuStreamDevice stream_device_;
  Eigen::GpuDevice device_;
};

// Parse 'visible_device_list' into a list of platform GPU ids.
Status ParseVisibleDeviceList(const string& visible_device_list,
                              std::vector<PlatformGpuId>* visible_gpu_order) {
  visible_gpu_order->clear();
  se::Platform* gpu_manager = GPUMachineManager();

  // If the user wants to remap the visible to virtual GPU mapping,
  // check for that here.
  if (visible_device_list.empty()) {
    visible_gpu_order->resize(gpu_manager->VisibleDeviceCount());
    // By default, visible to virtual mapping is unchanged.
    int deviceNo = 0;
    std::generate(visible_gpu_order->begin(), visible_gpu_order->end(),
                  [&deviceNo] { return deviceNo++; });
  } else {
    const std::vector<string> order_str =
        str_util::Split(visible_device_list, ',');
    for (const string& platform_gpu_id_str : order_str) {
      int32 platform_gpu_id;
      if (!strings::safe_strto32(platform_gpu_id_str, &platform_gpu_id)) {
        return errors::InvalidArgument(
            "Could not parse entry in 'visible_device_list': '",
            platform_gpu_id_str, "'. visible_device_list = ",
            visible_device_list);
      }
      if (platform_gpu_id < 0 ||
          platform_gpu_id >= gpu_manager->VisibleDeviceCount()) {
        return errors::InvalidArgument(
            "'visible_device_list' listed an invalid GPU id '", platform_gpu_id,
            "' but visible device count is ",
            gpu_manager->VisibleDeviceCount());
      }
      visible_gpu_order->push_back(PlatformGpuId(platform_gpu_id));
    }
  }

  // Validate no repeats.
  std::set<PlatformGpuId> visible_device_set(visible_gpu_order->begin(),
                                             visible_gpu_order->end());
  if (visible_device_set.size() != visible_gpu_order->size()) {
    return errors::InvalidArgument(
        "visible_device_list contained a duplicate entry: ",
        visible_device_list);
  }
  return Status::OK();
}

Status VerifyVirtualDeviceSettings(
    const size_t num_gpus_to_use, const GPUOptions& gpu_options,
    const std::vector<PlatformGpuId>& visible_gpu_order,
    const std::vector<PlatformGpuId>& valid_platform_gpu_ids) {
  const auto& virtual_devices = gpu_options.experimental().virtual_devices();
  CHECK(!virtual_devices.empty());
  if (gpu_options.per_process_gpu_memory_fraction() > 0) {
    return errors::InvalidArgument(
        "It's invalid to set per_process_gpu_memory_fraction when "
        "virtual_devices is set.");
  }
  if (num_gpus_to_use < virtual_devices.size()) {
    return errors::Unknown(
        "Not enough GPUs to create virtual devices."
        " num_gpus_to_use: ",
        num_gpus_to_use, " #virtual_devices: ", virtual_devices.size());
  }
  if (!gpu_options.visible_device_list().empty() &&
      visible_gpu_order.size() != virtual_devices.size()) {
    return errors::InvalidArgument(
        "The number of GPUs in visible_device_list doesn't match the number "
        "of elements in the virtual_devices list.",
        " #GPUs in visible_device_list: ", visible_gpu_order.size(),
        " virtual_devices.size(): ", virtual_devices.size());
  }
  if (valid_platform_gpu_ids.size() != virtual_devices.size()) {
    return errors::Unknown(
        "The number of valid GPUs doesn't match the number of elements in "
        "the virtual_devices list.",
        " #valid GPUs: ", valid_platform_gpu_ids.size(),
        " virtual_devices.size(): ", virtual_devices.size());
  }
  return Status::OK();
}

int64 MinSystemMemory(int64 available_memory) {
  // We use the following heuristic for now:
  //
  // If the available_memory is < 2GiB, we allocate 225MiB to system memory.
  // Otherwise, allocate max(300MiB, 0.05 * available_memory) to system memory.
  //
  // In the future we could be more sophisticated by using a table of devices.
  int64 min_system_memory;
  if (available_memory < (1LL << 31)) {
    // 225MiB
    min_system_memory = 225 * 1024 * 1024;
  } else {
    // max(300 MiB, 0.05 * available_memory)
    min_system_memory =
        std::max(int64{314572800}, static_cast<int64>(available_memory * 0.05));
  }
#if defined(__GNUC__) && defined(__OPTIMIZE__)
// Do nothing
#elif !defined(__GNUC__) && defined(NDEBUG)
// Do nothing
#else
  // Double the amount of available GPU memory in non-opt builds (debug
  // builds in windows); because in non-opt builds more system memory
  // is necessary.
  min_system_memory *= 2;
#endif

#if defined(ANDROID_TEGRA)
  // 1GB system mem for NVIDIA Tegra devices since they use the same mem for RAM
  // and Video RAM
  min_system_memory = 1 << 30;
#endif
  return min_system_memory;
}

// Get the memory limit for the virtual device being created on GPU with
// 'platform_gpu_id', when that virtual device is the only virtual device being
// created on that GPU.
Status SingleVirtualDeviceMemoryLimit(const GPUOptions& gpu_options,
                                      PlatformGpuId platform_gpu_id,
                                      int64* memory_limit) {
  int64 total_memory = 0;
  int64 available_memory = 0;
  se::StreamExecutor* se =
      GpuIdUtil::ExecutorForPlatformGpuId(platform_gpu_id).ValueOrDie();
  if (!se->DeviceMemoryUsage(&available_memory, &total_memory)) {
    return errors::Unknown("Failed to query available memory for GPU ",
                           platform_gpu_id.value());
  }

  int64 allocated_memory = 0;
  const double per_process_gpu_memory_fraction =
      gpu_options.per_process_gpu_memory_fraction();
  if (per_process_gpu_memory_fraction > 1.0 ||
      gpu_options.experimental().use_unified_memory()) {
    int cc_major = 0, cc_minor = 0;
    if (!se->GetDeviceDescription().cuda_compute_capability(&cc_major,
                                                            &cc_minor)) {
      return errors::Internal("Failed to get compute capability for device.");
    }
    if (cc_major < 6) {
      return errors::Internal(
          "Unified memory on GPUs with compute capability lower than 6.0 "
          "(pre-Pascal class GPUs) does not support oversubscription.");
    }
  }

  if (per_process_gpu_memory_fraction == 0) {
    allocated_memory = available_memory;
    const int64 min_system_memory = MinSystemMemory(available_memory);
    if (min_system_memory < allocated_memory) {
      allocated_memory -= min_system_memory;
    }
  } else {
    allocated_memory = total_memory * per_process_gpu_memory_fraction;
  }
  *memory_limit = allocated_memory;
  return Status::OK();
}
}  // namespace

void BaseGPUDevice::ReinitializeDevice(OpKernelContext* context,
                                       PerOpGpuDevice* device, int stream_id,
                                       Allocator* allocator) {
  ConcretePerOpGpuDevice* concrete_device =
      static_cast<ConcretePerOpGpuDevice*>(device);
  DCHECK(concrete_device);
  const cudaStream_t* cuda_stream = reinterpret_cast<const cudaStream_t*>(
      streams_[stream_id]->compute->implementation()->GpuStreamMemberHack());
  concrete_device->Reinitialize(context, cuda_stream, tf_gpu_id_, allocator,
                                scratch_[stream_id]);
}

PerOpGpuDevice* BaseGPUDevice::MakeGpuDevice() {
  return new ConcretePerOpGpuDevice();
}

Status BaseGPUDevice::ReinitializeGpuDevice(OpKernelContext* context,
                                            PerOpGpuDevice* device,
                                            DeviceContext* dc,
                                            Allocator* allocator) {
  TF_RETURN_IF_ERROR(InitScratchBuffers());
  if (dc) {
    const GPUDeviceContext* gpu_dc = static_cast<GPUDeviceContext*>(dc);
    const int stream_id = gpu_dc->stream_id();
    VLOG(1) << "  eigen_gpu_device(" << dc << ") => stream[" << stream_id
            << "]";
    CHECK_LT(stream_id, streams_.size());
    ReinitializeDevice(context, device, stream_id, allocator);
  } else {
    ReinitializeDevice(context, device, 0, allocator);
  }
  return Status::OK();
}

Allocator* BaseGPUDevice::GetScopedAllocator(AllocatorAttributes attr,
                                             int64 step_id) {
  if (attr.scope_id > 0) {
    return scoped_allocator_mgr_->GetContainer(step_id)->GetInstance(
        attr.scope_id);
  }
  LOG(FATAL) << "Unexpected call to BaseGPUDevice::GetScopedAllocator "
             << "attr.scope_id = " << attr.scope_id;
  return gpu_allocator_;
}

const int BaseGPUDeviceFactory::InterconnectMap::kSameDeviceStrength = 1000;
const int BaseGPUDeviceFactory::InterconnectMap::kStreamExecutorStrength = 1;

Status BaseGPUDeviceFactory::CreateDevices(
    const SessionOptions& options, const string& name_prefix,
    std::vector<std::unique_ptr<Device>>* devices) {
  TF_RETURN_IF_ERROR(ValidateGPUMachineManager());
  se::Platform* gpu_manager = GPUMachineManager();
  if (gpu_manager == nullptr) {
    return Status::OK();
  }
  // If there are no GPUs visible, do nothing.
  if (gpu_manager->VisibleDeviceCount() <= 0) {
    return Status::OK();
  }

  size_t num_gpus_to_use = INT_MAX;
  auto iter = options.config.device_count().find("GPU");
  if (iter != options.config.device_count().end()) {
    num_gpus_to_use = iter->second;
  }
  const auto& gpu_options = options.config.gpu_options();
  std::vector<PlatformGpuId> visible_gpu_order;
  std::vector<PlatformGpuId> valid_platform_gpu_ids;
  // If we aren't going to use any GPUs, don't initialize them.
  // We don't want to call ParseVisibleDeviceList if num_gpus_to_use is 0,
  // because it treats an empty gpu_options.visible_device_list as 'all GPUs are
  // visible'.
  if (num_gpus_to_use > 0) {
    TF_RETURN_IF_ERROR(ParseVisibleDeviceList(gpu_options.visible_device_list(),
                                              &visible_gpu_order));
    TF_RETURN_IF_ERROR(
        GetValidDeviceIds(visible_gpu_order, &valid_platform_gpu_ids));
  }
  if (num_gpus_to_use > valid_platform_gpu_ids.size()) {
    num_gpus_to_use = valid_platform_gpu_ids.size();
  }
  if (!valid_platform_gpu_ids.empty()) {
    // Save the original device.
    int original_device = 0;
    cudaError_t err = cudaGetDevice(&original_device);
    if (err != cudaSuccess) {
      return errors::Internal("cudaGetDevice() failed. Status: ",
                              cudaGetErrorString(err));
    }
    // Force to implicitly initialize CUDA runtime on each valid GPU before
    // CreateGPUDevice().
    for (PlatformGpuId platform_gpu_id : valid_platform_gpu_ids) {
      err = cudaSetDevice(platform_gpu_id.value());
      if (err != cudaSuccess) {
        return errors::Internal("cudaSetDevice() on GPU:",
                                platform_gpu_id.value(), " failed. Status: ",
                                cudaGetErrorString(err));
      }
      err = cudaFree(nullptr);
      if (err != cudaSuccess) {
        return errors::Internal("CUDA runtime implicit initialization on GPU:",
                                platform_gpu_id.value(), " failed. Status: ",
                                cudaGetErrorString(err));
      }
    }
    // Reset to the original device.
    err = cudaSetDevice(original_device);
    if (err != cudaSuccess) {
      return errors::Internal("cudaSetDevice() on GPU:", original_device,
                              " failed. Status: ", cudaGetErrorString(err));
    }
  }

  std::vector<InterconnectMap> interconnect_maps;
  TF_RETURN_IF_ERROR(
      GetInterconnectMaps(visible_gpu_order, gpu_manager, &interconnect_maps));

  // Print each interconnect map to the log.
  for (const InterconnectMap& im : interconnect_maps) {
    LOG(INFO) << "Device interconnect " << im.name << " with strength "
              << im.strength << " edge matrix:";
    string line_buf = "     ";
    for (int i = 0; i < visible_gpu_order.size(); ++i) {
      strings::StrAppend(&line_buf, visible_gpu_order[i].value(), " ");
    }
    LOG(INFO) << line_buf;
    for (int i = 0; i < visible_gpu_order.size(); ++i) {
      line_buf = strings::StrCat(visible_gpu_order[i].value(), ":   ");
      PlatformGpuId gpu_id_i = visible_gpu_order[i];
      for (int j = 0; j < visible_gpu_order.size(); ++j) {
        PlatformGpuId gpu_id_j = visible_gpu_order[j];
        if (im.directed_links.find({gpu_id_i, gpu_id_j}) !=
            im.directed_links.end()) {
          line_buf.append("Y ");
        } else {
          line_buf.append("N ");
        }
      }
      LOG(INFO) << line_buf;
    }
  }

  const auto& virtual_devices = gpu_options.experimental().virtual_devices();
  if (!virtual_devices.empty()) {
    TF_RETURN_IF_ERROR(VerifyVirtualDeviceSettings(num_gpus_to_use, gpu_options,
                                                   visible_gpu_order,
                                                   valid_platform_gpu_ids));
    // We've verified that num_gpus_to_use >= virtual_devices.size().
    num_gpus_to_use = virtual_devices.size();
    CHECK(gpu_options.visible_device_list().empty() ||
          valid_platform_gpu_ids == visible_gpu_order);
  }
  int next_tf_gpu_id = 0;
  std::vector<int64> memory_limit_bytes;
  for (int i = 0; i < num_gpus_to_use; ++i) {
    const PlatformGpuId platform_gpu_id = valid_platform_gpu_ids[i];
    if (virtual_devices.empty() ||
        virtual_devices.Get(i).memory_limit_mb_size() == 0) {
      int64 single_virtual_device_memory_limit = 0;
      TF_RETURN_IF_ERROR(SingleVirtualDeviceMemoryLimit(
          gpu_options, platform_gpu_id, &single_virtual_device_memory_limit));
      memory_limit_bytes.push_back(single_virtual_device_memory_limit);
    } else {
      const auto& memory_limit_mb = virtual_devices.Get(i).memory_limit_mb();
      std::transform(memory_limit_mb.begin(), memory_limit_mb.end(),
                     std::back_inserter(memory_limit_bytes), [](float mb) {
                       return static_cast<int64>(mb) * (1ll << 20);
                     });
    }
    while (next_tf_gpu_id < memory_limit_bytes.size()) {
      TfGpuId tf_gpu_id(next_tf_gpu_id);
      ++next_tf_gpu_id;
      TF_RETURN_IF_ERROR(
          GpuIdManager::InsertTfPlatformGpuIdPair(tf_gpu_id, platform_gpu_id));
    }
  }
  const int num_tf_gpus = next_tf_gpu_id;

  LocalityMap device_localities;
  TF_RETURN_IF_ERROR(
      GetDeviceLocalities(num_tf_gpus, interconnect_maps, &device_localities));

  // Build the GPUDevices
  CHECK_EQ(next_tf_gpu_id, memory_limit_bytes.size());
  for (int di = 0; di < num_tf_gpus; ++di) {
    TfGpuId tf_gpu_id(di);
    int64 bytes = memory_limit_bytes[di];
    auto it = device_localities.find(tf_gpu_id);
    if (it == device_localities.end()) {
      return errors::Internal("Failed to find DeviceLocality for GPU device ",
                              tf_gpu_id.value());
    }
    TF_RETURN_IF_ERROR(CreateGPUDevice(options, name_prefix, tf_gpu_id, bytes,
                                       it->second, devices));
  }
  return Status::OK();
}

static string GetShortDeviceDescription(PlatformGpuId platform_gpu_id,
                                        const se::DeviceDescription& desc) {
  int cc_major;
  int cc_minor;
  if (!desc.cuda_compute_capability(&cc_major, &cc_minor)) {
    cc_major = 0;
    cc_minor = 0;
  }
  // LINT.IfChange
  return strings::StrCat("device: ", platform_gpu_id.value(), ", name: ",
                         desc.name(), ", pci bus id: ", desc.pci_bus_id(),
                         ", compute capability: ", cc_major, ".", cc_minor);
  // LINT.ThenChange(//tensorflow/python/platform/test.py)
}

Status BaseGPUDeviceFactory::CreateGPUDevice(
    const SessionOptions& options, const string& name_prefix, TfGpuId tf_gpu_id,
    int64 memory_limit, const DeviceLocality& dev_locality,
    std::vector<std::unique_ptr<Device>>* devices) {
  CHECK_GE(tf_gpu_id.value(), 0);
  const string device_name =
      strings::StrCat(name_prefix, "/device:GPU:", tf_gpu_id.value());
  GpuIdUtil::CheckValidTfGpuId(tf_gpu_id);
  PlatformGpuId platform_gpu_id;
  TF_RETURN_IF_ERROR(
      GpuIdManager::TfToPlatformGpuId(tf_gpu_id, &platform_gpu_id));
  int numa_node = dev_locality.numa_node();

  se::StreamExecutor* se =
      GpuIdUtil::ExecutorForPlatformGpuId(platform_gpu_id).ValueOrDie();
  const se::DeviceDescription& desc = se->GetDeviceDescription();
  GPUProcessState* process_state = GPUProcessState::singleton();
  Allocator* gpu_allocator = process_state->GetGPUAllocator(
      options.config.gpu_options(), tf_gpu_id, memory_limit);
  if (gpu_allocator == nullptr) {
    return errors::Internal("Failed to get memory allocator for TF GPU ",
                            tf_gpu_id.value(), " with ", memory_limit,
                            " bytes of memory.");
  }
  AllocatorStats stats;
  gpu_allocator->GetStats(&stats);
  // 'memory_limit' is the required memory size, but if the allocator with given
  // tf_gpu_id was created before, we'll use it instead of creating a new one
  // (as TF gpu device is a shared resource), in which case the actual memory
  // limit represented by 'stats.bytes_limit' used by that allocator may be
  // different (which should be an error).
  //
  // TODO(laigd): report error if memory_limit doesn't match stats.bytes_limit.
  std::unique_ptr<BaseGPUDevice> gpu_device = CreateGPUDevice(
      options, device_name, static_cast<Bytes>(stats.bytes_limit), dev_locality,
      tf_gpu_id, GetShortDeviceDescription(platform_gpu_id, desc),
      gpu_allocator, ProcessState::singleton()->GetCPUAllocator(numa_node));
  LOG(INFO) << "Created TensorFlow device (" << device_name << " with "
            << (stats.bytes_limit >> 20) << " MB memory) -> physical GPU ("
            << GetShortDeviceDescription(platform_gpu_id, desc) << ")";
  TF_RETURN_IF_ERROR(gpu_device->Init(options));
  devices->push_back(std::move(gpu_device));

  return Status::OK();
}

namespace {
std::unique_ptr<std::map<std::pair<PlatformGpuId, PlatformGpuId>, bool>>
GetPeerAccessMap(se::Platform* platform,
                 const std::vector<PlatformGpuId>& visible_gpu_order) {
  std::unique_ptr<std::map<std::pair<PlatformGpuId, PlatformGpuId>, bool>> map(
      new std::map<std::pair<PlatformGpuId, PlatformGpuId>, bool>);
  for (PlatformGpuId platform_gpu_i : visible_gpu_order) {
    for (PlatformGpuId platform_gpu_j : visible_gpu_order) {
      se::StreamExecutor* from =
          GpuIdUtil::ExecutorForPlatformGpuId(platform, platform_gpu_i)
              .ValueOrDie();
      se::StreamExecutor* to =
          GpuIdUtil::ExecutorForPlatformGpuId(platform, platform_gpu_j)
              .ValueOrDie();
      (*map)[{platform_gpu_i, platform_gpu_j}] =
          from->CanEnablePeerAccessTo(to);
    }
  }

  return map;
}

}  // namespace

Status BaseGPUDeviceFactory::GetInterconnectMaps(
    const std::vector<PlatformGpuId>& visible_gpu_order,
    se::Platform* gpu_manager, std::vector<InterconnectMap>* maps) {
  // The default interconnect map is obtained from the StreamExecutor.
  auto access_map = GetPeerAccessMap(gpu_manager, visible_gpu_order);
  maps->resize(1);
  InterconnectMap& imap = maps->at(0);
  imap.name = "StreamExecutor";
  imap.strength = InterconnectMap::kStreamExecutorStrength;
  for (PlatformGpuId gpu_id_i : visible_gpu_order) {
    for (PlatformGpuId gpu_id_j : visible_gpu_order) {
      if (gpu_id_i == gpu_id_j) continue;
      if ((*access_map)[{gpu_id_i, gpu_id_j}]) {
        imap.directed_links.insert({gpu_id_i, gpu_id_j});
      }
    }
  }
  return Status::OK();
}

Status BaseGPUDeviceFactory::GetDeviceLocalities(
    int num_tf_gpus, const std::vector<InterconnectMap>& interconnects,
    LocalityMap* localities) {
  std::vector<TfGpuId> all_tf_gpu_ids;
  all_tf_gpu_ids.reserve(num_tf_gpus);
  for (int i = 0; i < num_tf_gpus; ++i) {
    all_tf_gpu_ids.push_back(TfGpuId(i));
  }
  for (TfGpuId tf_gpu_id : all_tf_gpu_ids) {
    PlatformGpuId platform_gpu_id;
    TF_RETURN_IF_ERROR(
        GpuIdManager::TfToPlatformGpuId(tf_gpu_id, &platform_gpu_id));
    // Get GPU bus_id from its reported NUMA affinity.  Because GPUs are
    // virtualized in some environments, we can't just use the GPU id.
    // NUMA locales are indexed from 0, buses are indexed from 1.
    se::StreamExecutor* se =
        GpuIdUtil::ExecutorForPlatformGpuId(platform_gpu_id).ValueOrDie();
    const se::DeviceDescription& desc = se->GetDeviceDescription();
    int numa_node = desc.numa_node();
    if (numa_node < 0) {
      // For some reason the StreamExecutor couldn't get the NUMA
      // affinity of the GPU.  If this is not a multi-socket mobo with
      // GPUs local to different buses, it doesn't matter.  If it is, we
      // may run into trouble later with data transfer operations.  The
      // trouble may manifest as slower than expected performance, or
      // outright failures.
      LOG(INFO) << "Could not identify NUMA node of platform GPU id "
                << platform_gpu_id
                << ", defaulting to 0.  Your kernel may not have been built "
                << "with NUMA support.";
      numa_node = 0;
    }
    DeviceLocality dev_locality;
    dev_locality.set_numa_node(numa_node);
    dev_locality.set_bus_id(numa_node + 1);

    // Set LocalLinks from InterconnectMaps.
    LocalLinks* links = dev_locality.mutable_links();
    for (const InterconnectMap& imap : interconnects) {
      for (TfGpuId tf_gpu_dst : all_tf_gpu_ids) {
        PlatformGpuId platform_gpu_dst;
        TF_RETURN_IF_ERROR(
            GpuIdManager::TfToPlatformGpuId(tf_gpu_dst, &platform_gpu_dst));
        if (imap.directed_links.find({platform_gpu_id, platform_gpu_dst}) !=
            imap.directed_links.end()) {
          InterconnectLink* ilink = links->add_link();
          ilink->set_device_id(tf_gpu_dst.value());
          ilink->set_type(imap.name);
          ilink->set_strength(imap.strength);
        }
      }
    }

    // If this is one of multiple virtual GPUs on the same physical GPU
    // add high strength links to the others.
    for (TfGpuId tf_gpu_dst : all_tf_gpu_ids) {
      if (tf_gpu_id == tf_gpu_dst) continue;
      PlatformGpuId platform_gpu_dst;
      TF_RETURN_IF_ERROR(
          GpuIdManager::TfToPlatformGpuId(tf_gpu_dst, &platform_gpu_dst));
      if (platform_gpu_id == platform_gpu_dst) {
        InterconnectLink* ilink = links->add_link();
        ilink->set_device_id(tf_gpu_dst.value());
        ilink->set_type("SAME_DEVICE");
        ilink->set_strength(InterconnectMap::kSameDeviceStrength);
      }
    }

    (*localities)[tf_gpu_id] = dev_locality;
    VLOG(1) << "GPUDevice PlatformGpuId " << platform_gpu_id << " TfGpuId "
            << tf_gpu_id << " on bus " << dev_locality.bus_id()
            << " numa: " << numa_node << " pci: " << desc.pci_bus_id()
            << " DeviceLocality: " << dev_locality.DebugString();
  }
  return Status::OK();
}

static int GetDefaultMinGPUMultiprocessorCount(
    se::Platform* gpu_manager,
    const std::vector<PlatformGpuId>& visible_gpu_order) {
  static const int kDefaultMinGPUMultiprocessorCount = 8;

  // Find the highest multi-processor count across all visible GPUs.
  int max_count = -1;
  for (int i = 0; i < visible_gpu_order.size(); ++i) {
    auto exec_status =
        GpuIdUtil::ExecutorForPlatformGpuId(gpu_manager, visible_gpu_order[i]);
    if (!exec_status.ok()) {
      continue;
    }

    se::StreamExecutor* se = exec_status.ValueOrDie();
    const se::DeviceDescription& desc = se->GetDeviceDescription();
    max_count = std::max(max_count, desc.core_count());
  }

  if (max_count < 0 || kDefaultMinGPUMultiprocessorCount < max_count) {
    return kDefaultMinGPUMultiprocessorCount;
  } else {
    return max_count;
  }
}

static int GetMinGPUMultiprocessorCount(
    se::Platform* gpu_manager,
    const std::vector<PlatformGpuId>& visible_gpu_order) {
  const char* tf_min_gpu_core_count = getenv("TF_MIN_GPU_MULTIPROCESSOR_COUNT");

  if (tf_min_gpu_core_count == nullptr ||
      strcmp(tf_min_gpu_core_count, "") == 0) {
    return GetDefaultMinGPUMultiprocessorCount(gpu_manager, visible_gpu_order);
  }

  int min_gpu_core_count = -1;
  if (strings::safe_strto32(tf_min_gpu_core_count, &min_gpu_core_count)) {
    if (min_gpu_core_count >= 0) {
      return min_gpu_core_count;
    }
  }

  int count =
      GetDefaultMinGPUMultiprocessorCount(gpu_manager, visible_gpu_order);
  LOG(ERROR) << "Invalid minimum GPU multiprocessor count: ["
             << tf_min_gpu_core_count << "]. "
             << "Using the default value: " << count;
  return count;
}

namespace {

struct CudaVersion {
  // Initialize from version_name in the form of "3.5"
  explicit CudaVersion(const std::string& version_name) {
    size_t dot_pos = version_name.find('.');
    CHECK(dot_pos != string::npos)
        << "Illegal version name: [" << version_name << "]";
    string major_str = version_name.substr(0, dot_pos);
    CHECK(strings::safe_strto32(major_str, &major_part))
        << "Illegal version name: [" << version_name << "]";
    string minor_str = version_name.substr(dot_pos + 1);
    CHECK(strings::safe_strto32(minor_str, &minor_part))
        << "Illegal version name: [" << version_name << "]";
  }
  CudaVersion() {}
  bool operator<(const CudaVersion& other) const {
    if (this->major_part != other.major_part) {
      return this->major_part < other.major_part;
    }
    return this->minor_part < other.minor_part;
  }
  friend std::ostream& operator<<(std::ostream& os,
                                  const CudaVersion& version) {
    os << version.major_part << "." << version.minor_part;
    return os;
  }
  int major_part = -1;
  int minor_part = -1;
};

std::vector<CudaVersion> supported_cuda_compute_capabilities = {
    TF_CUDA_CAPABILITIES,};

std::vector<CudaVersion> GetSupportedCudaComputeCapabilities() {
  auto cuda_caps = supported_cuda_compute_capabilities;
#ifdef TF_EXTRA_CUDA_CAPABILITIES
// TF_EXTRA_CUDA_CAPABILITIES should be defined a sequence separated by commas,
// for example:
//   TF_EXTRA_CUDA_CAPABILITIES=3.0,4.0,5.0
// Use two-level macro expansion for stringification.
#define TF_XSTRING(...) #__VA_ARGS__
#define TF_STRING(s) TF_XSTRING(s)
  string extra_cuda_caps = TF_STRING(TF_EXTRA_CUDA_CAPABILITIES);
#undef TF_STRING
#undef TF_XSTRING
  auto extra_capabilities = str_util::Split(extra_cuda_caps, ',');
  for (const auto& capability : extra_capabilities) {
    cuda_caps.push_back(CudaVersion(capability));
  }
#endif
  return cuda_caps;
}

Status EnablePeerAccess(se::Platform* platform,
                        const std::vector<PlatformGpuId>& visible_gpu_order) {
  int possible_peer_count = 0;
  int enabled_peer_count = 0;
  for (int i = 0; i < visible_gpu_order.size(); ++i) {
    const PlatformGpuId platform_gpu_i = visible_gpu_order[i];
    for (int j = 0; j < visible_gpu_order.size(); ++j) {
      const PlatformGpuId platform_gpu_j = visible_gpu_order[j];
      // We have already validated that ExecutorForDevice() calls return OK.
      se::StreamExecutor* from =
          GpuIdUtil::ExecutorForPlatformGpuId(platform, platform_gpu_i)
              .ValueOrDie();
      se::StreamExecutor* to =
          GpuIdUtil::ExecutorForPlatformGpuId(platform, platform_gpu_j)
              .ValueOrDie();

      if (from->CanEnablePeerAccessTo(to)) {
        ++possible_peer_count;
        auto status = from->EnablePeerAccessTo(to);
        if (!status.ok()) {
          LOG(WARNING)
              << "Unable to enable peer access between device ordinals "
              << platform_gpu_i << " and " << platform_gpu_j
              << ", status: " << status;
        } else {
          ++enabled_peer_count;
        }
      }
    }
  }

  // Return an error in the extreme failure case where the driver
  // reported that peering was possible but not a single peering was
  // successful.  This is to catch possible system misconfigurations
  // or more fundamental issues.
  if (possible_peer_count > 0 && enabled_peer_count == 0) {
    return errors::Internal(possible_peer_count,
                            " potential peer access pairs were reported by the "
                            "driver, but no peering could be enabled.");
  }
  return Status::OK();
}

}  // namespace

Status BaseGPUDeviceFactory::GetValidDeviceIds(
    const std::vector<PlatformGpuId>& visible_gpu_order,
    std::vector<PlatformGpuId>* ids) {
  se::Platform* gpu_manager = GPUMachineManager();
  bool new_gpu_found = false;
  for (int i = 0; i < visible_gpu_order.size(); ++i) {
    const PlatformGpuId visible_gpu_id = visible_gpu_order[i];

    // Only perform this once per visible platform gpu id.
    if (visible_gpu_initialized_[visible_gpu_id.value()]) {
      continue;
    }

    visible_gpu_initialized_[visible_gpu_id.value()] = true;
    new_gpu_found = true;

    auto executor =
        GpuIdUtil::ExecutorForPlatformGpuId(gpu_manager, visible_gpu_id);
    if (!executor.ok()) {
      return executor.status();
    }

    auto stream_exec = executor.ValueOrDie();
    int64 free_bytes;
    int64 total_bytes;
    if (!stream_exec->DeviceMemoryUsage(&free_bytes, &total_bytes)) {
      // Logs internally on failure.
      free_bytes = 0;
      total_bytes = 0;
    }
    const auto& description = stream_exec->GetDeviceDescription();
    int cc_major;
    int cc_minor;
    if (!description.cuda_compute_capability(&cc_major, &cc_minor)) {
      // Logs internally on failure.
      cc_major = 0;
      cc_minor = 0;
    }
    LOG(INFO) << "Found device " << i << " with properties: "
              << "\nname: " << description.name() << " major: " << cc_major
              << " minor: " << cc_minor
              << " memoryClockRate(GHz): " << description.clock_rate_ghz()
              << "\npciBusID: " << description.pci_bus_id() << "\ntotalMemory: "
              << strings::HumanReadableNumBytes(total_bytes)
              << " freeMemory: " << strings::HumanReadableNumBytes(free_bytes);
  }
  // Checking peering and shows matrix if more than one gpu found.
  if (new_gpu_found && visible_gpu_order.size() > 1) {
    // Enable peer access
    TF_RETURN_IF_ERROR(EnablePeerAccess(gpu_manager, visible_gpu_order));
  }

  auto cuda_supported_capabilities = GetSupportedCudaComputeCapabilities();
  if (cuda_supported_capabilities.empty()) {
    return errors::FailedPrecondition(
        "No supported cuda capabilities in binary.");
  }
  CudaVersion min_supported_capability = *std::min_element(
      cuda_supported_capabilities.begin(), cuda_supported_capabilities.end());

  int min_gpu_core_count =
      GetMinGPUMultiprocessorCount(gpu_manager, visible_gpu_order);

  // Filter out devices that don't have the right capability or power.
  for (int i = 0; i < visible_gpu_order.size(); ++i) {
    const PlatformGpuId visible_gpu_id = visible_gpu_order[i];
    auto exec_status =
        GpuIdUtil::ExecutorForPlatformGpuId(gpu_manager, visible_gpu_id);
    if (!exec_status.ok()) {
      LOG(INFO) << "Ignoring visible gpu device " << visible_gpu_id
                << " whose executor is in invalid state: "
                << exec_status.status().ToString();
      continue;
    }
    se::StreamExecutor* se = exec_status.ValueOrDie();
    const se::DeviceDescription& desc = se->GetDeviceDescription();
    CudaVersion device_capability;
    if (!desc.cuda_compute_capability(&device_capability.major_part,
                                      &device_capability.minor_part)) {
      LOG(INFO) << "Ignoring visible gpu device "
                << "(" << GetShortDeviceDescription(visible_gpu_id, desc)
                << ") "
                << "whose CUDA compute capability is not available.";
      continue;
    }
    // Only GPUs with no less than the minimum supported compute capability is
    // accepted.
    if (device_capability < min_supported_capability) {
      LOG(INFO) << "Ignoring visible gpu device "
                << "(" << GetShortDeviceDescription(visible_gpu_id, desc)
                << ") "
                << "with Cuda compute capability " << device_capability
                << ". The minimum required Cuda capability is "
                << min_supported_capability << ".";
      continue;
    }

    // Filter out slow GPUs. By default, GPUs with a lower multiprocessor
    // count than the fastest GPU are filtered out, unless they have 8 or more
    // multiprocessors. If the TF_MIN_GPU_MULTIPROCESSOR_COUNT environment
    // variable is set, its value will be used to filter out GPUs.
    if (desc.core_count() < min_gpu_core_count) {
      LOG(INFO) << "Ignoring visible gpu device "
                << "(" << GetShortDeviceDescription(visible_gpu_id, desc)
                << ") "
                << "with Cuda multiprocessor count: " << desc.core_count()
                << ". The minimum required count is " << min_gpu_core_count
                << ". You can adjust this requirement with the env var "
                   "TF_MIN_GPU_MULTIPROCESSOR_COUNT.";
      continue;
    }
    ids->push_back(visible_gpu_id);
  }
  if (!ids->empty()) {
    std::vector<int> raw_ids(ids->size());
    std::transform(ids->begin(), ids->end(), raw_ids.begin(),
                   [](PlatformGpuId id) -> int { return id.value(); });
    LOG(INFO) << "Adding visible gpu devices: "
              << str_util::Join(raw_ids, ", ");
  }

  return Status::OK();
}

}  // namespace tensorflow

#endif  // GOOGLE_CUDA
