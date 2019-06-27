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
#include <deque>

#include "tensorflow/core/common_runtime/process_function_library_runtime.h"
#include "tensorflow/core/framework/dataset.h"
#include "tensorflow/core/framework/function.h"
#include "tensorflow/core/framework/function_handle_cache.h"
#include "tensorflow/core/framework/op_kernel.h"
#include "tensorflow/core/framework/resource_op_kernel.h"
#include "tensorflow/core/kernels/data/dataset_utils.h"
#include "tensorflow/core/kernels/ops_util.h"
#include "tensorflow/core/lib/core/threadpool.h"
#include "tensorflow/core/lib/random/random.h"
#include "tensorflow/core/util/device_name_utils.h"

namespace tensorflow {
namespace data {
namespace {

struct HostBufferElement {
  Status status;
  bool end_of_sequence;
  std::vector<Tensor> value;
};

using MultiDeviceIteratorCallback =
    std::function<void(const HostBufferElement&)>;

class MultiDeviceIterator : public ResourceBase {
 public:
  MultiDeviceIterator(
      const DataTypeVector& output_types,
      const std::vector<PartialTensorShape>& output_shapes,
      const std::vector<string>& devices,
      std::unique_ptr<FunctionLibraryDefinition> flib_def,
      std::unique_ptr<ProcessFunctionLibraryRuntime> pflr,
      FunctionLibraryRuntime* lib,
      std::unique_ptr<FunctionHandleCache> function_handle_cache)
      : output_types_(output_types),
        output_shapes_(output_shapes),
        devices_(devices),
        flib_def_(std::move(flib_def)),
        pflr_(std::move(pflr)),
        lib_(lib),
        function_handle_cache_(std::move(function_handle_cache)) {
    DCHECK(lib_ != nullptr);
  }

  string DebugString() override {
    return strings::StrCat("MultiDeviceIterator for ", devices_.size(),
                           " devices");
  }

  Status Init(std::unique_ptr<IteratorBase> iterator, int64 max_buffer_size,
              int64* incarnation_id) {
    if (iterator) {
      TF_RETURN_IF_ERROR(
          VerifyTypesMatch(output_types_, iterator->output_dtypes()));
      TF_RETURN_IF_ERROR(
          VerifyShapesCompatible(output_shapes_, iterator->output_shapes()));
    }

    mutex_lock l(mu_);
    if (multi_device_buffer_) {
      multi_device_buffer_->Reset();
    }

    ++incarnation_id_;
    *incarnation_id = incarnation_id_;

    multi_device_buffer_.reset(
        new MultiDeviceBuffer(devices_.size(), max_buffer_size, incarnation_id_,
                              std::move(iterator)));
    return Status::OK();
  }

  void GetNextFromShard(IteratorContext* ctx, int shard_num,
                        int64 incarnation_id,
                        MultiDeviceIteratorCallback callback) {
    if (ctx->lib() == lib_) {
      tf_shared_lock l(mu_);
      multi_device_buffer_->GetNextFromShard(ctx, shard_num, incarnation_id,
                                             std::move(callback));
    } else {
      IteratorContext::Params params(ctx);
      params.lib = lib_;
      params.function_handle_cache = function_handle_cache_.get();
      params.resource_mgr = &resource_mgr_;
      IteratorContext iter_ctx(std::move(params));
      tf_shared_lock l(mu_);
      multi_device_buffer_->GetNextFromShard(
          &iter_ctx, shard_num, incarnation_id, std::move(callback));
    }
  }

  const DataTypeVector& output_types() const { return output_types_; }

  const std::vector<PartialTensorShape>& output_shapes() const {
    return output_shapes_;
  }

  std::shared_ptr<const FunctionLibraryDefinition> function_library() {
    tf_shared_lock l(mu_);
    return lib_def_;
  }

  FunctionLibraryRuntime* const lib() {
    tf_shared_lock l(mu_);
    return lib_;
  }

  FunctionHandleCache* function_handle_cache() {
    return function_handle_cache_.get();
  }

  ResourceMgr* resource_mgr() { return &resource_mgr_; }

 private:
  // A private class that uses a background thread to keep a per device buffer
  // full.
  class MultiDeviceBuffer {
   public:
    MultiDeviceBuffer(size_t size, int64 max_buffer_size, int64 incarnation_id,
                      std::unique_ptr<IteratorBase> host_iterator)
        : buffer_(size),
          size_(size),
          max_buffer_size_(max_buffer_size),
          incarnation_id_(incarnation_id),
          host_iterator_(std::move(host_iterator)) {}

    ~MultiDeviceBuffer() {
      {
        mutex_lock l(mu_);
        if (!background_thread_started_) return;
      }
      Reset();
    }

    void Reset() LOCKS_EXCLUDED(mu_) {
      {
        mutex_lock l(mu_);
        if (!background_thread_finished_) {
          cancelled_ = true;
          // Wake up the background thread.
          for (int i = 0; i < size_; ++i) {
            buffer_[i].cond_var.notify_all();
          }

          // Make sure background thread has finished first.
          while (!background_thread_finished_) {
            shutdown_cond_var_.wait(l);
          }
        }
      }
      RunPendingCallbacks();
    }

    void GetNextFromShard(IteratorContext* ctx, int shard_num,
                          int64 incarnation_id,
                          MultiDeviceIteratorCallback callback) {
      HostBufferElement elem;
      if (incarnation_id_ != incarnation_id) {
        elem.status = errors::InvalidArgument("Invalid incarnation id");
        callback(elem);
        return;
      }

      bool produced_output = false;
      {
        mutex_lock l(mu_);
        if (cancelled_) {
          elem.status = errors::Cancelled("Cancelled Multidevice iterator");
          callback(elem);
          return;
        }

        EnsureBackgroundThreadStarted(ctx);

        if (!buffer_[shard_num].data.empty()) {
          produced_output = true;
          std::swap(elem, buffer_[shard_num].data.front());
          buffer_[shard_num].data.pop_front();
          // Wake up background thread if it is blocked on this element.
          if (buffer_[shard_num].data.size() == max_buffer_size_ - 1) {
            buffer_[shard_num].cond_var.notify_all();
          }
        } else {
          if (end_of_iterator_) {
            produced_output = true;
            elem.end_of_sequence = true;
          } else {
            buffer_[shard_num].callbacks.push_back(std::move(callback));
            callback = nullptr;
          }
        }
      }

      if (produced_output) {
        callback(elem);
      }
    }

   private:
    void EnsureBackgroundThreadStarted(IteratorContext* ctx)
        EXCLUSIVE_LOCKS_REQUIRED(mu_) {
      if (!background_thread_) {
        background_thread_.reset(ctx->env()->StartThread(
            {}, "tf_data_multi_device_iterator",
            std::bind(&MultiDeviceIterator::MultiDeviceBuffer::BackgroundThread,
                      this, new IteratorContext(*ctx))));
      }
    }

    void RunPendingCallbacks() LOCKS_EXCLUDED(mu_) {
      // Run all remaining callbacks.
      std::vector<MultiDeviceIteratorCallback> cancellation_callbacks;
      std::vector<HostBufferElement> cancellation_elements;
      {
        mutex_lock l(mu_);

        for (int i = 0; i < size_; ++i) {
          while (!buffer_[i].callbacks.empty()) {
            if (buffer_[i].data.empty()) {
              HostBufferElement elem;
              if (end_of_iterator_) {
                elem.end_of_sequence = true;
              } else {
                elem.status =
                    errors::Cancelled("Cancelled and buffer not filled.");
              }
              cancellation_elements.push_back(std::move(elem));
            } else {
              cancellation_elements.push_back(
                  std::move(buffer_[i].data.front()));
              buffer_[i].data.pop_front();
            }
            cancellation_callbacks.push_back(
                std::move(buffer_[i].callbacks.front()));
            buffer_[i].callbacks.pop_front();
          }
        }
      }
      for (int i = 0; i < cancellation_callbacks.size(); ++i) {
        cancellation_callbacks[i](cancellation_elements[i]);
      }
    }

    void BackgroundThread(IteratorContext* ctx) {
      {
        mutex_lock l(mu_);
        background_thread_started_ = true;
      }
      std::unique_ptr<IteratorContext> cleanup(ctx);
      int shard_to_fetch = 0;
      while (true) {
        HostBufferElement elem;
        MultiDeviceIteratorCallback callback = nullptr;
        bool end_of_iterator = false;

        {
          mutex_lock l(mu_);
          while (!cancelled_ &&
                 buffer_[shard_to_fetch].data.size() >= max_buffer_size_) {
            buffer_[shard_to_fetch].cond_var.wait(l);
          }

          if (cancelled_) {
            background_thread_finished_ = true;
            shutdown_cond_var_.notify_all();
            return;
          }
        }

        elem.status =
            host_iterator_->GetNext(ctx, &elem.value, &elem.end_of_sequence);

        if (elem.status.ok() && elem.end_of_sequence) {
          end_of_iterator = true;
        }

        {
          mutex_lock l(mu_);
          // Try to find a callback, else just push stuff into buffer.
          if (!buffer_[shard_to_fetch].callbacks.empty()) {
            callback = buffer_[shard_to_fetch].callbacks.front();
            buffer_[shard_to_fetch].callbacks.pop_front();
          } else {
            buffer_[shard_to_fetch].data.push_back(std::move(elem));
            elem = HostBufferElement();
          }
        }

        if (callback) {
          (*ctx->runner())(std::bind(std::move(callback), std::move(elem)));
        }

        // Finish off the thread if we reach the end of the iterator. Runs
        // pending callbacks.
        if (end_of_iterator) {
          {
            mutex_lock l(mu_);
            background_thread_finished_ = true;
            end_of_iterator_ = true;
            shutdown_cond_var_.notify_all();
          }
          RunPendingCallbacks();
          return;
        }
        shard_to_fetch = (shard_to_fetch + 1) % size_;
      }
    }

    struct HostBuffer {
      condition_variable cond_var;
      std::deque<HostBufferElement> data;
      std::deque<MultiDeviceIteratorCallback> callbacks;
    };

    mutex mu_;
    std::unique_ptr<Thread> background_thread_ GUARDED_BY(mu_);
    bool background_thread_finished_ GUARDED_BY(mu_) = false;
    bool background_thread_started_ GUARDED_BY(mu_) = false;
    bool end_of_iterator_ GUARDED_BY(mu_) = false;
    bool cancelled_ GUARDED_BY(mu_) = false;
    condition_variable shutdown_cond_var_ GUARDED_BY(mu_);

    std::vector<HostBuffer> buffer_;

    const size_t size_;
    const int64 max_buffer_size_;
    const int64 incarnation_id_;
    const std::unique_ptr<IteratorBase> host_iterator_;
  };

  mutex mu_;
  const DataTypeVector output_types_;
  const std::vector<PartialTensorShape> output_shapes_;
  const std::vector<string> devices_;
  const std::unique_ptr<FunctionLibraryDefinition> flib_def_;
  const std::unique_ptr<ProcessFunctionLibraryRuntime> pflr_;
  FunctionLibraryRuntime* const lib_ = nullptr;  // not owned.
  const std::unique_ptr<FunctionHandleCache> function_handle_cache_;
  ResourceMgr resource_mgr_;
  std::shared_ptr<const FunctionLibraryDefinition> lib_def_ GUARDED_BY(mu_);

  int64 incarnation_id_ GUARDED_BY(mu_) = 0;
  std::unique_ptr<MultiDeviceBuffer> multi_device_buffer_ GUARDED_BY(mu_);
};

// Just creates a MultiDeviceIterator and returns it.
class MultiDeviceIteratorHandleOp : public OpKernel {
 public:
  explicit MultiDeviceIteratorHandleOp(OpKernelConstruction* ctx)
      : OpKernel(ctx), graph_def_version_(ctx->graph_def_version()) {
    OP_REQUIRES_OK(ctx, ctx->GetAttr("output_types", &output_types_));
    OP_REQUIRES_OK(ctx, ctx->GetAttr("output_shapes", &output_shapes_));
    OP_REQUIRES_OK(ctx, ctx->GetAttr("shared_name", &name_));
    OP_REQUIRES_OK(ctx, ctx->GetAttr("container", &container_));
    OP_REQUIRES_OK(ctx, ctx->GetAttr("devices", &devices_));
  }

  // The resource is deleted from the resource manager only when it is private
  // to kernel.
  ~MultiDeviceIteratorHandleOp() override {
    if (resource_ != nullptr) {
      resource_->Unref();
      if (cinfo_.resource_is_private_to_kernel()) {
        if (!cinfo_.resource_manager()
                 ->template Delete<MultiDeviceIterator>(cinfo_.container(),
                                                        cinfo_.name())
                 .ok()) {
          // Do nothing; the resource can have been deleted by session resets.
        }
      }
    }
  }

  void Compute(OpKernelContext* context) override LOCKS_EXCLUDED(mu_) {
    {
      mutex_lock l(mu_);
      if (resource_ == nullptr) {
        FunctionLibraryRuntime* lib;
        std::unique_ptr<FunctionLibraryDefinition> flib_def(nullptr);
        std::unique_ptr<ProcessFunctionLibraryRuntime> pflr(nullptr);
        OP_REQUIRES_OK(context, context->function_library()->Clone(
                                    &flib_def, &pflr, &lib));
        std::unique_ptr<FunctionHandleCache> function_handle_cache(
            new FunctionHandleCache(lib));
        ResourceMgr* mgr = context->resource_manager();
        OP_REQUIRES_OK(context, cinfo_.Init(mgr, def()));

        MultiDeviceIterator* resource;
        OP_REQUIRES_OK(context,
                       mgr->LookupOrCreate<MultiDeviceIterator>(
                           cinfo_.container(), cinfo_.name(), &resource,
                           [this, lib, &flib_def, &pflr,
                            &function_handle_cache](MultiDeviceIterator** ret)
                               EXCLUSIVE_LOCKS_REQUIRED(mu_) {
                                 *ret = new MultiDeviceIterator(
                                     output_types_, output_shapes_, devices_,
                                     std::move(flib_def), std::move(pflr), lib,
                                     std::move(function_handle_cache));
                                 return Status::OK();
                               }));

        Status s = VerifyResource(resource);
        if (TF_PREDICT_FALSE(!s.ok())) {
          resource->Unref();
          context->SetStatus(s);
          return;
        }

        resource_ = resource;
      }
    }
    OP_REQUIRES_OK(context, MakeResourceHandleToOutput(
                                context, 0, cinfo_.container(), cinfo_.name(),
                                MakeTypeIndex<MultiDeviceIterator>()));
  }

 private:
  // During the first Compute(), resource is either created or looked up using
  // shared_name. In the latter case, the resource found should be verified if
  // it is compatible with this op's configuration. The verification may fail in
  // cases such as two graphs asking queues of the same shared name to have
  // inconsistent capacities.
  Status VerifyResource(MultiDeviceIterator* resource) {
    TF_RETURN_IF_ERROR(
        VerifyTypesMatch(output_types_, resource->output_types()));
    TF_RETURN_IF_ERROR(
        VerifyShapesCompatible(output_shapes_, resource->output_shapes()));
    return Status::OK();
  }

  mutex mu_;
  ContainerInfo cinfo_;  // Written once under mu_ then constant afterwards.
  MultiDeviceIterator* resource_ GUARDED_BY(mu_) = nullptr;
  DataTypeVector output_types_;
  std::vector<PartialTensorShape> output_shapes_;
  const int graph_def_version_;
  string name_;
  string container_;
  std::vector<string> devices_;
};

REGISTER_KERNEL_BUILDER(Name("MultiDeviceIterator").Device(DEVICE_CPU),
                        MultiDeviceIteratorHandleOp);

// Calls init on the MultiDeviceIterator.
class MultiDeviceIteratorInitOp : public OpKernel {
 public:
  explicit MultiDeviceIteratorInitOp(OpKernelConstruction* ctx)
      : OpKernel(ctx) {}

  void Compute(OpKernelContext* ctx) override {
    const Tensor* tensor_max_buffer_size;
    OP_REQUIRES_OK(ctx, ctx->input("max_buffer_size", &tensor_max_buffer_size));
    int64 max_buffer_size = tensor_max_buffer_size->scalar<int64>()();

    DatasetBase* dataset;
    OP_REQUIRES_OK(ctx, GetDatasetFromVariantTensor(ctx->input(0), &dataset));
    MultiDeviceIterator* resource;
    OP_REQUIRES_OK(ctx,
                   LookupResource(ctx, HandleFromInput(ctx, 1), &resource));
    core::ScopedUnref unref(resource);

    std::unique_ptr<IteratorBase> iterator;
    IteratorContext::Params params(ctx);
    params.lib = resource->lib();
    params.function_handle_cache = resource->function_handle_cache();
    params.resource_mgr = resource->resource_mgr();
    IteratorContext iter_ctx(std::move(params));
    OP_REQUIRES_OK(
        ctx, dataset->MakeIterator(std::move(iter_ctx), "Iterator", &iterator));
    int64 incarnation_id;
    OP_REQUIRES_OK(ctx, resource->Init(std::move(iterator), max_buffer_size,
                                       &incarnation_id));
    Tensor tensor_incarnation_id(DT_INT64, TensorShape({}));
    tensor_incarnation_id.scalar<int64>()() = incarnation_id;
    OP_REQUIRES_OK(ctx,
                   ctx->set_output("incarnation_id", tensor_incarnation_id));
  }
};

REGISTER_KERNEL_BUILDER(Name("MultiDeviceIteratorInit").Device(DEVICE_CPU),
                        MultiDeviceIteratorInitOp);

// Calls GetNextFromShard(shard) and returns a vector of Tensors as output.
// TODO(rohanj): Implement using BackgroundWorker that Derek built?
class MultiDeviceIteratorGetNextFromShardOp : public AsyncOpKernel {
 public:
  explicit MultiDeviceIteratorGetNextFromShardOp(OpKernelConstruction* ctx)
      : AsyncOpKernel(ctx),
        background_worker_(ctx->env(),
                           "tf_data_multi_device_iterator_get_next") {}

  void ComputeAsync(OpKernelContext* ctx, DoneCallback done) override {
    const Tensor* tensor_shard_num;
    OP_REQUIRES_OK_ASYNC(ctx, ctx->input("shard_num", &tensor_shard_num), done);
    int32 shard_num = tensor_shard_num->scalar<int32>()();

    const Tensor* tensor_incarnation_id;
    OP_REQUIRES_OK_ASYNC(
        ctx, ctx->input("incarnation_id", &tensor_incarnation_id), done);
    int64 incarnation_id = tensor_incarnation_id->scalar<int64>()();

    MultiDeviceIterator* iterator;
    OP_REQUIRES_OK_ASYNC(
        ctx, LookupResource(ctx, HandleFromInput(ctx, 0), &iterator), done);
    background_worker_.Schedule(std::bind(
        [ctx, iterator, shard_num, incarnation_id](DoneCallback done) {
          MultiDeviceIteratorCallback callback = std::bind(
              [ctx](const HostBufferElement& elem, DoneCallback done) {
                // iterator->Unref();
                Status s = elem.status;
                if (!s.ok()) {
                  ctx->SetStatus(s);
                } else if (elem.end_of_sequence) {
                  ctx->SetStatus(errors::OutOfRange("End of sequence"));
                } else {
                  for (int i = 0; i < elem.value.size(); ++i) {
                    ctx->set_output(i, elem.value[i]);
                  }
                }
                done();
              },
              std::placeholders::_1, std::move(done));

          IteratorContext::Params params(ctx);
          params.function_library = iterator->function_library();
          IteratorContext iter_ctx(std::move(params));
          iterator->GetNextFromShard(&iter_ctx, shard_num, incarnation_id,
                                     callback);
          iterator->Unref();
        },
        std::move(done)));
  }

 private:
  BackgroundWorker background_worker_;
};

REGISTER_KERNEL_BUILDER(
    Name("MultiDeviceIteratorGetNextFromShard").Device(DEVICE_CPU),
    MultiDeviceIteratorGetNextFromShardOp);

class MultiDeviceIteratorToStringHandleOp : public OpKernel {
 public:
  explicit MultiDeviceIteratorToStringHandleOp(OpKernelConstruction* ctx)
      : OpKernel(ctx) {}

  void Compute(OpKernelContext* ctx) override {
    const Tensor& resource_handle_t = ctx->input(0);
    OP_REQUIRES(ctx, TensorShapeUtils::IsScalar(resource_handle_t.shape()),
                errors::InvalidArgument("resource_handle must be a scalar"));

    // Validate that the handle corresponds to a real resource, and
    // that it is an MultiDeviceIterator.
    MultiDeviceIterator* resource;
    OP_REQUIRES_OK(ctx,
                   LookupResource(ctx, HandleFromInput(ctx, 0), &resource));
    resource->Unref();

    Tensor* string_handle_t;
    OP_REQUIRES_OK(ctx,
                   ctx->allocate_output(0, TensorShape({}), &string_handle_t));
    string_handle_t->scalar<string>()() =
        resource_handle_t.scalar<ResourceHandle>()().SerializeAsString();
  }
};

REGISTER_KERNEL_BUILDER(
    Name("MultiDeviceIteratorToStringHandle").Device(DEVICE_CPU),
    MultiDeviceIteratorToStringHandleOp);

class MultiDeviceIteratorFromStringHandleOp : public OpKernel {
 public:
  explicit MultiDeviceIteratorFromStringHandleOp(OpKernelConstruction* ctx)
      : OpKernel(ctx) {
    OP_REQUIRES_OK(ctx, ctx->GetAttr("output_types", &output_types_));
    OP_REQUIRES_OK(ctx, ctx->GetAttr("output_shapes", &output_shapes_));
    OP_REQUIRES(
        ctx,
        output_types_.empty() || output_shapes_.empty() ||
            output_types_.size() == output_shapes_.size(),
        errors::InvalidArgument("If both 'output_types' and 'output_shapes' "
                                "are set, they must have the same length."));
  }

  void Compute(OpKernelContext* ctx) override {
    const Tensor& string_handle_t = ctx->input(0);
    OP_REQUIRES(ctx, TensorShapeUtils::IsScalar(string_handle_t.shape()),
                errors::InvalidArgument("string_handle must be a scalar"));

    ResourceHandle resource_handle;
    OP_REQUIRES(
        ctx,
        resource_handle.ParseFromString(string_handle_t.scalar<string>()()),
        errors::InvalidArgument(
            "Could not parse string_handle as a valid ResourceHandle"));

    OP_REQUIRES(
        ctx, resource_handle.device() == ctx->device()->attributes().name(),
        errors::InvalidArgument("Attempted create an iterator on device \"",
                                ctx->device()->attributes().name(),
                                "\" from handle defined on device \"",
                                resource_handle.device(), "\""));

    // Validate that the handle corresponds to a real resource, and
    // that it is an MultiDeviceIterator.
    MultiDeviceIterator* resource;
    OP_REQUIRES_OK(ctx, LookupResource(ctx, resource_handle, &resource));
    core::ScopedUnref unref_iterator(resource);
    if (!output_types_.empty()) {
      OP_REQUIRES_OK(ctx,
                     VerifyTypesMatch(output_types_, resource->output_types()));
    }
    if (!output_shapes_.empty()) {
      OP_REQUIRES_OK(ctx, VerifyShapesCompatible(output_shapes_,
                                                 resource->output_shapes()));
    }

    Tensor* resource_handle_t;
    OP_REQUIRES_OK(
        ctx, ctx->allocate_output(0, TensorShape({}), &resource_handle_t));
    resource_handle_t->scalar<ResourceHandle>()() = resource_handle;
  }

 private:
  DataTypeVector output_types_;
  std::vector<PartialTensorShape> output_shapes_;
};

REGISTER_KERNEL_BUILDER(
    Name("MultiDeviceIteratorFromStringHandle").Device(DEVICE_CPU),
    MultiDeviceIteratorFromStringHandleOp);

}  // namespace
}  // namespace data
}  // namespace tensorflow
