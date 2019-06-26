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

#ifndef TENSORFLOW_CORE_DISTRIBUTED_RUNTIME_EAGER_EAGER_SERVICE_IMPL_H_
#define TENSORFLOW_CORE_DISTRIBUTED_RUNTIME_EAGER_EAGER_SERVICE_IMPL_H_

#include <unordered_map>

#include "tensorflow/core/common_runtime/eager/context.h"
#include "tensorflow/core/common_runtime/eager/tensor_handle.h"
#include "tensorflow/core/distributed_runtime/eager/remote_tensor_handle.h"
#include "tensorflow/core/distributed_runtime/worker_env.h"
#include "tensorflow/core/lib/core/refcount.h"
#include "tensorflow/core/lib/gtl/array_slice.h"
#include "tensorflow/core/lib/strings/stringprintf.h"
#include "tensorflow/core/protobuf/eager_service.pb.h"

namespace tensorflow {
namespace eager {

// A TensorFlow Eager Worker runs ops and supports worker to worker
// Tensor transfer.
//
// See eager_service.proto for more details about each method.
// This class can be wrapped by specific classes that implement rpc transports
// over this (e.g. gRPC).
class EagerServiceImpl {
 public:
  explicit EagerServiceImpl(const WorkerEnv* env) : env_(env) {
    gc_thread_.reset(
        env_->env->StartThread({}, "EagerServiceContextGC", [this]() {
          while (true) {
            {
              mutex_lock l(gc_thread_shutdown_mu_);
              gc_thread_cv_.wait_for(l, std::chrono::seconds(1));

              if (shutting_down_) {
                return;
              }
            }
            {
              mutex_lock l(contexts_mu_);
              for (auto it = contexts_.begin(); it != contexts_.end();) {
                if (it->second->IsStale()) {
                  it->second->Unref();
                  it = contexts_.erase(it);
                } else {
                  it++;
                }
              }
            }
          }
        }));
  }
  virtual ~EagerServiceImpl() {
    {
      mutex_lock l(gc_thread_shutdown_mu_);
      shutting_down_ = true;
      gc_thread_cv_.notify_all();
    }
    gc_thread_.reset();

    mutex_lock l(contexts_mu_);
    for (auto& entry : contexts_) {
      entry.second->Unref();
    }
  }

  Status CreateContext(const CreateContextRequest* request,
                       CreateContextResponse* response);

  Status Enqueue(const EnqueueRequest* request, EnqueueResponse* response);

  Status WaitQueueDone(const WaitQueueDoneRequest* request,
                       WaitQueueDoneResponse* response);

  Status KeepAlive(const KeepAliveRequest* request,
                   KeepAliveResponse* response);

  Status CloseContext(const CloseContextRequest* request,
                      CloseContextResponse* response);

  Status RegisterFunction(const RegisterFunctionRequest* request,
                          RegisterFunctionResponse* response);

  Status SendTensor(const SendTensorRequest* request,
                    SendTensorResponse* response);

 protected:
  // This is the server-side execution context. All state regarding execution of
  // a client's ops is held in this server-side context (all generated tensors,
  // and the EagerContext).
  class ServerContext : public core::RefCounted {
   public:
    explicit ServerContext(std::unique_ptr<tensorflow::EagerContext> ctx,
                           int64 destroy_after_secs, const WorkerEnv* env)
        : ctx_(std::move(ctx)), env_(env) {
      destroy_after_micros_ =
          destroy_after_secs * tensorflow::EnvTime::kSecondsToMicros;
      RecordAccess();
    }
    ~ServerContext() {
      for (const auto& entry : tensors_) {
        entry.second->Unref();
      }
    }

    tensorflow::EagerContext* Context() const { return ctx_.get(); }

    void AddOperationOutputs(
        const gtl::ArraySlice<tensorflow::TensorHandle*>& handles,
        int64 operation_id) {
      mutex_lock l(tensors_mu_);
      for (int i = 0; i < handles.size(); i++) {
        // TODO(nareshmodi): Correctly handle operation_id not being unique.
        tensors_.emplace(RemoteTensorHandleInternal(operation_id, i),
                         handles[i]);
      }
    }

    Status GetTensorHandle(const RemoteTensorHandleInternal& remote_handle,
                           tensorflow::TensorHandle** handle) {
      mutex_lock l(tensors_mu_);
      auto iter = tensors_.find(remote_handle);
      if (iter == tensors_.end()) {
        return errors::InvalidArgument(
            "Unable to find the relevant tensor remote_handle: Op ID: ",
            remote_handle.op_id, ", Output num: ", remote_handle.output_num);
      }

      *handle = iter->second;

      return Status::OK();
    }

    Status DeleteTensorHandle(const RemoteTensorHandleInternal& remote_handle) {
      mutex_lock l(tensors_mu_);
      auto iter = tensors_.find(remote_handle);
      if (iter == tensors_.end()) {
        return errors::InvalidArgument(
            "Unable to find the relevant tensor remote_handle: Op ID: ",
            remote_handle.op_id, ", Output num: ", remote_handle.output_num);
      }

      iter->second->Unref();
      tensors_.erase(iter);

      return Status::OK();
    }

    void RecordAccess() {
      mutex_lock l(last_accessed_mu_);
      last_accessed_micros_ = env_->env->NowMicros();
    }

    bool IsStale() {
      mutex_lock l(last_accessed_mu_);
      return (destroy_after_micros_ > 0 &&
              (env_->env->NowMicros() - last_accessed_micros_) >
                  destroy_after_micros_);
    }

   private:
    using RemoteTensorHandleMap =
        gtl::FlatMap<RemoteTensorHandleInternal, tensorflow::TensorHandle*,
                     RemoteTensorHandleInternalHash,
                     RemoteTensorHandleInternalEquals>;

    // The context for this execution.
    std::unique_ptr<tensorflow::EagerContext> ctx_;

    // The state related to the context for this execution.
    mutex tensors_mu_;
    RemoteTensorHandleMap tensors_ GUARDED_BY(tensors_mu_);

    const WorkerEnv* const env_;  // Not owned.

    mutex last_accessed_mu_;
    int64 last_accessed_micros_ GUARDED_BY(last_accessed_mu_);
    int64 destroy_after_micros_;
  };
  // The returned ServerContext will need to be Unrefed.
  tensorflow::Status GetServerContext(uint64, ServerContext**);

 private:
  Status ExecuteOp(const Operation& operation, ServerContext* server_context,
                   QueueResponse* queue_response);
  const WorkerEnv* const env_;  // Not owned.

  mutex contexts_mu_;
  std::unordered_map<uint64, ServerContext*> contexts_ GUARDED_BY(contexts_mu_);

  std::unique_ptr<Thread> gc_thread_;
  mutex gc_thread_shutdown_mu_;
  condition_variable gc_thread_cv_;
  bool shutting_down_ GUARDED_BY(gc_thread_shutdown_mu_) = false;

  TF_DISALLOW_COPY_AND_ASSIGN(EagerServiceImpl);
};

}  // namespace eager
}  // namespace tensorflow

#endif  // TENSORFLOW_CORE_DISTRIBUTED_RUNTIME_EAGER_EAGER_SERVICE_IMPL_H_
