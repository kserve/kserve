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

#ifndef TENSORFLOW_CORE_DISTRIBUTED_RUNTIME_RPC_EAGER_GRPC_EAGER_SERVICE_IMPL_H_
#define TENSORFLOW_CORE_DISTRIBUTED_RUNTIME_RPC_EAGER_GRPC_EAGER_SERVICE_IMPL_H_

#include "grpcpp/alarm.h"
#include "grpcpp/completion_queue.h"
#include "grpcpp/server_builder.h"
#include "tensorflow/core/distributed_runtime/eager/eager_service_impl.h"
#include "tensorflow/core/distributed_runtime/rpc/async_service_interface.h"
#include "tensorflow/core/distributed_runtime/rpc/eager/grpc_eager_service.h"
#include "tensorflow/core/distributed_runtime/rpc/grpc_call.h"
#include "tensorflow/core/distributed_runtime/rpc/grpc_util.h"

namespace tensorflow {
namespace eager {

// This class is a wrapper that handles communication for gRPC.
class GrpcEagerServiceImpl : public AsyncServiceInterface {
 public:
  template <class RequestMessage, class ResponseMessage>
  using EagerCall = Call<GrpcEagerServiceImpl, grpc::EagerService::AsyncService,
                         RequestMessage, ResponseMessage>;

  GrpcEagerServiceImpl(const WorkerEnv* env,
                       ::grpc::ServerBuilder* server_builder);
  virtual ~GrpcEagerServiceImpl() {}

  void HandleRPCsLoop() override;
  void Shutdown() override;

 private:
#define HANDLER(method)                                                        \
  void method##Handler(EagerCall<method##Request, method##Response>* call) {   \
    env_->compute_pool->Schedule([this, call]() {                              \
      call->SendResponse(                                                      \
          ToGrpcStatus(local_impl_.method(&call->request, &call->response)));  \
    });                                                                        \
    Call<GrpcEagerServiceImpl,                                                 \
         tensorflow::eager::grpc::EagerService::AsyncService, method##Request, \
         method##Response>::                                                   \
        EnqueueRequest(&service_, cq_.get(),                                   \
                       &grpc::EagerService::AsyncService::Request##method,     \
                       &GrpcEagerServiceImpl::method##Handler, false);         \
  }
  HANDLER(CreateContext);
  HANDLER(Enqueue);
  HANDLER(WaitQueueDone);
  HANDLER(KeepAlive);
  HANDLER(CloseContext);
  HANDLER(RegisterFunction);
  HANDLER(SendTensor);
#undef HANDLER

  const WorkerEnv* const env_;  // Not owned.
  EagerServiceImpl local_impl_;

  std::unique_ptr<::grpc::Alarm> shutdown_alarm_;

  std::unique_ptr<::grpc::ServerCompletionQueue> cq_;
  tensorflow::eager::grpc::EagerService::AsyncService service_;

  TF_DISALLOW_COPY_AND_ASSIGN(GrpcEagerServiceImpl);
};

}  // namespace eager
}  // namespace tensorflow

#endif  // TENSORFLOW_CORE_DISTRIBUTED_RUNTIME_RPC_EAGER_GRPC_EAGER_SERVICE_IMPL_H_
