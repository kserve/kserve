/* Copyright 2018 The TensorFlow Authors. All Rights Reserved.

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

#include "tensorflow/core/grappler/optimizers/evaluation_utils.h"

#include "tensorflow/core/framework/tensor.pb.h"
#include "tensorflow/core/lib/core/threadpool.h"
#include "tensorflow/core/platform/cpu_info.h"
#include "tensorflow/core/platform/denormal.h"
#include "tensorflow/core/platform/setround.h"
#include "tensorflow/core/public/version.h"

namespace tensorflow {
namespace grappler {
using TensorVector = gtl::InlinedVector<TensorValue, 4>;

namespace {
class EigenThreadPoolWrapper : public Eigen::ThreadPoolInterface {
 public:
  explicit EigenThreadPoolWrapper(thread::ThreadPool* pool) : pool_(pool) {}
  ~EigenThreadPoolWrapper() override {}
  void Schedule(std::function<void()> fn) override {
    auto wrapped = [=]() {
      // TensorFlow flushes denormals to zero and rounds to nearest, so we do
      // the same here.
      port::ScopedFlushDenormal flush;
      port::ScopedSetRound round(FE_TONEAREST);
      fn();
    };
    pool_->Schedule(std::move(wrapped));
  }
  int NumThreads() const override { return pool_->NumThreads(); }
  int CurrentThreadId() const override { return pool_->CurrentThreadId(); }

 private:
  thread::ThreadPool* pool_ = nullptr;
};

}  // namespace

DeviceSimple::DeviceSimple() : DeviceBase(Env::Default()) {
  eigen_worker_threads_.num_threads = port::NumSchedulableCPUs();
  eigen_worker_threads_.workers = new thread::ThreadPool(
      Env::Default(), "evaluation_utils", eigen_worker_threads_.num_threads);
  eigen_threadpool_wrapper_.reset(
      new EigenThreadPoolWrapper(eigen_worker_threads_.workers));
  eigen_device_.reset(new Eigen::ThreadPoolDevice(
      eigen_threadpool_wrapper_.get(), eigen_worker_threads_.num_threads));
  set_tensorflow_cpu_worker_threads(&eigen_worker_threads_);
  set_eigen_cpu_device(eigen_device_.get());
}

DeviceSimple::~DeviceSimple() {
  eigen_threadpool_wrapper_.reset();
  eigen_device_.reset();
  delete eigen_worker_threads_.workers;
}

Status DeviceSimple::MakeTensorFromProto(const TensorProto& tensor_proto,
                                         const AllocatorAttributes alloc_attrs,
                                         Tensor* tensor) {
  Tensor parsed(tensor_proto.dtype());
  if (!parsed.FromProto(cpu_allocator(), tensor_proto)) {
    return errors::InvalidArgument("Cannot parse tensor from tensor_proto.");
  }
  *tensor = parsed;
  return Status::OK();
}

Status EvaluateNode(const NodeDef& node, const TensorVector& inputs,
                    DeviceBase* cpu_device, ResourceMgr* resource_mgr,
                    TensorVector* output) {
  Status status;
  std::unique_ptr<DeviceBase> device;
  if (cpu_device == nullptr) {
    device.reset(new DeviceSimple());
    cpu_device = device.get();
  }

  std::unique_ptr<OpKernel> op_kernel(
      CreateOpKernel("CPU", cpu_device, cpu_device->GetAllocator({}), node,
                     TF_GRAPH_DEF_VERSION, &status));
  TF_RETURN_IF_ERROR(status);
  OpKernelContext::Params params;
  params.device = cpu_device;
  params.frame_iter = FrameAndIter(0, 0);
  params.inputs = &inputs;
  params.op_kernel = op_kernel.get();
  params.resource_manager = resource_mgr;

  gtl::InlinedVector<AllocatorAttributes, 4> output_attrs;
  const int num_outputs = op_kernel->num_outputs();
  for (int i = 0; i < num_outputs; i++) {
    AllocatorAttributes attr;
    attr.set_on_host(true);
    output_attrs.push_back(attr);
  }
  params.output_attr_array = output_attrs.data();

  OpKernelContext op_context(&params);
  op_kernel->Compute(&op_context);
  for (int i = 0; i < num_outputs; i++) {
    output->push_back(op_context.release_output(i));
  }
  return op_context.status();
}

}  // end namespace grappler
}  // end namespace tensorflow
