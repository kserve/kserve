/* Copyright 2016 The TensorFlow Authors. All Rights Reserved.

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

#ifndef TENSORFLOW_CORE_COMMON_RUNTIME_RENAMED_DEVICE_H_
#define TENSORFLOW_CORE_COMMON_RUNTIME_RENAMED_DEVICE_H_

#include "tensorflow/core/common_runtime/device.h"
#include "tensorflow/core/util/device_name_utils.h"

namespace tensorflow {

// Wraps a device with a new name, delegating work to the wrapped device.
//
// This class is used to wrap local devices when using clusterspec propagation
// where the name of a particular device may change in the context of a given
// session.
class RenamedDevice : public Device {
 public:
  static std::unique_ptr<Device> NewRenamedDevice(const string& new_base,
                                                  Device* underlying,
                                                  bool owns_underlying,
                                                  bool isolate_session_state);

  ~RenamedDevice() override;

  // Below are virtual methods defined on DeviceBase
  bool RequiresRecordingAccessedTensors() const override {
    return underlying_->RequiresRecordingAccessedTensors();
  }

  const DeviceBase* UnderlyingDevice() const override {
    return underlying_->UnderlyingDevice();
  }
  DeviceBase* UnderlyingDevice() override {
    return underlying_->UnderlyingDevice();
  }

  const CpuWorkerThreads* tensorflow_cpu_worker_threads() const override {
    return underlying_->tensorflow_cpu_worker_threads();
  }

  const GpuDeviceInfo* tensorflow_gpu_device_info() const override {
    return underlying_->tensorflow_gpu_device_info();
  }

  Allocator* GetAllocator(AllocatorAttributes attr) override {
    return underlying_->GetAllocator(attr);
  }

  Allocator* GetScopedAllocator(AllocatorAttributes attr,
                                int64 step_id) override {
    return underlying_->GetScopedAllocator(attr, step_id);
  }

  ScopedAllocatorMgr* GetScopedAllocatorMgr() const override {
    return underlying_->GetScopedAllocatorMgr();
  }

  const Eigen::ThreadPoolDevice* eigen_cpu_device() override {
    return underlying_->eigen_cpu_device();
  }

#ifdef TENSORFLOW_USE_SYCL
  const Eigen::SyclDevice* eigen_sycl_device() const override {
    return underlying_->eigen_sycl_device();
  }
#endif

  PerOpGpuDevice* MakeGpuDevice() override {
    return underlying_->MakeGpuDevice();
  }

  Status ReinitializeGpuDevice(OpKernelContext* context, PerOpGpuDevice* device,
                               DeviceContext* dc,
                               Allocator* allocator) override {
    return underlying_->ReinitializeGpuDevice(context, device, dc, allocator);
  }

  Status MakeTensorFromProto(const TensorProto& tensor_proto,
                             const AllocatorAttributes alloc_attrs,
                             Tensor* tensor) override {
    return underlying_->MakeTensorFromProto(tensor_proto, alloc_attrs, tensor);
  }

  // Below are virtual methods defined on Device

  void Compute(OpKernel* op_kernel, OpKernelContext* context) override {
    underlying_->Compute(op_kernel, context);
  }

  void ComputeAsync(AsyncOpKernel* op_kernel, OpKernelContext* context,
                    AsyncOpKernel::DoneCallback done) override {
    underlying_->ComputeAsync(op_kernel, context, std::move(done));
  }

  void ConsumeListOfAccessedTensors(
      DeviceContext* context, const TensorReferenceVector& tensors) override {
    underlying_->ConsumeListOfAccessedTensors(context, tensors);
  }

  Status Sync() override { return underlying_->Sync(); }

  Status MaybeRewriteGraph(std::unique_ptr<Graph>* graph) override {
    return underlying_->MaybeRewriteGraph(graph);
  }

  Status FillContextMap(const Graph* graph,
                        DeviceContextMap* device_context_map) override {
    return underlying_->FillContextMap(graph, device_context_map);
  }

  // Returns the resource manager associated w/ this device.
  ResourceMgr* resource_manager() override {
    if (isolate_session_state_) {
      return Device::resource_manager();
    } else {
      return underlying_->resource_manager();
    }
  }

 private:
  RenamedDevice(Device* underlying, const DeviceAttributes& attributes,
                bool owns_underlying, bool isolate_session_state);
  Device* const underlying_;
  const bool owns_underlying_;
  const bool isolate_session_state_;
};

}  // namespace tensorflow

#endif  // TENSORFLOW_CORE_COMMON_RUNTIME_RENAMED_DEVICE_H_
