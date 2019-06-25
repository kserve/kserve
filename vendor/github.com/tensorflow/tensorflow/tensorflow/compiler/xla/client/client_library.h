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

// The "client library" instantiates a local (in-process) XLA service for
// use by this process, and connects to it with a singleton XLA local
// client. ClientLibrary::GetOrCreateLocalClient will spawn a local service,
// and return a client that's connected to it and ready to run XLA
// computations.
#ifndef TENSORFLOW_COMPILER_XLA_CLIENT_CLIENT_LIBRARY_H_
#define TENSORFLOW_COMPILER_XLA_CLIENT_CLIENT_LIBRARY_H_

#include <functional>
#include <memory>
#include <string>
#include <vector>

#include "tensorflow/compiler/xla/client/compile_only_client.h"
#include "tensorflow/compiler/xla/client/local_client.h"
#include "tensorflow/compiler/xla/service/compile_only_service.h"
#include "tensorflow/compiler/xla/service/device_memory_allocator.h"
#include "tensorflow/compiler/xla/service/local_service.h"
#include "tensorflow/compiler/xla/statusor.h"
#include "tensorflow/compiler/xla/types.h"
#include "tensorflow/core/platform/macros.h"
#include "tensorflow/core/platform/mutex.h"
#include "tensorflow/core/platform/stream_executor_no_cuda.h"
#include "tensorflow/core/platform/thread_annotations.h"

namespace xla {

// Options to configure the local client when it is created.
class LocalClientOptions {
 public:
  LocalClientOptions(se::Platform* platform = nullptr,
                     int number_of_replicas = 1,
                     int intra_op_parallelism_threads = -1);

  // Set the platform backing the service, or nullptr for the default platform.
  LocalClientOptions& set_platform(se::Platform* platform);
  se::Platform* platform() const;

  // Set the number of replicas to use when compiling replicated
  // programs.
  LocalClientOptions& set_number_of_replicas(int number_of_replicas);
  int number_of_replicas() const;

  // Sets the thread pool size for parallel execution of an individual operator.
  LocalClientOptions& set_intra_op_parallelism_threads(int num_threads);
  int intra_op_parallelism_threads() const;

 private:
  se::Platform* platform_;
  int number_of_replicas_;
  int intra_op_parallelism_threads_;
};

class ClientLibrary {
 public:
  // Singleton constructor-or-accessor -- returns a client for the application
  // to issue XLA commands on. Arguments:
  //
  //   platform : The platform the underlying XLA service should target. If
  //     null then default platform is used.
  static StatusOr<LocalClient*> GetOrCreateLocalClient(
      se::Platform* platform = nullptr);
  static StatusOr<LocalClient*> GetOrCreateLocalClient(
      const LocalClientOptions& options);

  // Convenience "or-die" wrapper around the above which returns the existing
  // client library or creates one with default platform and allocator.
  static LocalClient* LocalClientOrDie();

  // Returns the service from the service thread. Only used in unit tests to
  // access user computations from client.
  static LocalService* GetXlaService(se::Platform* platform);

  // Singleton constructor-or-accessor for compile-only clients. Arguments:
  //
  //   platform : The platform the underlying XLA service should target. If
  //     null then default platform is used.
  static StatusOr<CompileOnlyClient*> GetOrCreateCompileOnlyClient(
      se::Platform* platform = nullptr);

  // Clears the local instance and compile only instance caches. The client
  // pointers returned by the previous GetOrCreateLocalClient() or
  // GetOrCreateCompileOnlyClient() invocations are not valid anymore.
  static void DestroyLocalInstances();

 private:
  // Returns the singleton instance of ClientLibrary.
  static ClientLibrary& Singleton();

  ClientLibrary();
  ~ClientLibrary();

  struct LocalInstance {
    // Service that is wrapped by the singleton client object.
    std::unique_ptr<LocalService> service;
    // Singleton client object.
    std::unique_ptr<LocalClient> client;
  };

  struct CompileOnlyInstance {
    // Service that is wrapped by the singleton client object.
    std::unique_ptr<CompileOnlyService> service;
    // Singleton client object.
    std::unique_ptr<CompileOnlyClient> client;
  };

  tensorflow::mutex service_mutex_;  // Guards the singleton creation state.
  std::unordered_map<se::Platform::Id, std::unique_ptr<LocalInstance>>
      local_instances_ GUARDED_BY(service_mutex_);

  std::unordered_map<se::Platform::Id, std::unique_ptr<CompileOnlyInstance>>
      compile_only_instances_ GUARDED_BY(service_mutex_);

  TF_DISALLOW_COPY_AND_ASSIGN(ClientLibrary);
};

}  // namespace xla

#endif  // TENSORFLOW_COMPILER_XLA_CLIENT_CLIENT_LIBRARY_H_
