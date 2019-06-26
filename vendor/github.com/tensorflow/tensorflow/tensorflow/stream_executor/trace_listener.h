/* Copyright 2015 The TensorFlow Authors. All Rights Reserved.

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

// This file defines the StreamExecutor trace listener, used for inserting
// non-device-specific instrumentation into the StreamExecutor.
#ifndef TENSORFLOW_STREAM_EXECUTOR_TRACE_LISTENER_H_
#define TENSORFLOW_STREAM_EXECUTOR_TRACE_LISTENER_H_

#include "tensorflow/stream_executor/device_memory.h"
#include "tensorflow/stream_executor/kernel.h"
#include "tensorflow/stream_executor/launch_dim.h"
#include "tensorflow/stream_executor/lib/status.h"

namespace stream_executor {

class Stream;

// Traces StreamExecutor PIMPL-level events.
// The few StreamExecutor interfaces that are synchronous have both Begin and
// Complete versions of their trace calls. Asynchronous operations only have
// Submit calls, as execution of the underlying operations is device-specific.
// As all tracing calls mirror StreamExecutor routines, documentation here is
// minimal.
//
// All calls have default implementations that perform no work; subclasses
// should override functionality of interest. Keep in mind that these routines
// are not called on a dedicated thread, so callbacks should execute quickly.
//
// Note: This API is constructed on an as-needed basis. Users should add
// support for further StreamExecutor operations as required. By enforced
// convention (see SCOPED_TRACE in stream_executor_pimpl.cc), synchronous
// tracepoints should be named NameBegin and NameComplete.
class TraceListener {
 public:
  virtual ~TraceListener() {}

  virtual void LaunchSubmit(Stream* stream, const ThreadDim& thread_dims,
                            const BlockDim& block_dims,
                            const KernelBase& kernel,
                            const KernelArgsArrayBase& args) {}

  virtual void SynchronousMemcpyH2DBegin(int64 correlation_id,
                                         const void* host_src, int64 size,
                                         DeviceMemoryBase* gpu_dst) {}
  virtual void SynchronousMemcpyH2DComplete(int64 correlation_id,
                                            const port::Status* result) {}

  virtual void SynchronousMemcpyD2HBegin(int64 correlation_id,
                                         const DeviceMemoryBase& gpu_src,
                                         int64 size, void* host_dst) {}
  virtual void SynchronousMemcpyD2HComplete(int64 correlation_id,
                                            const port::Status* result) {}

  virtual void BlockHostUntilDoneBegin(int64 correlation_id, Stream* stream) {}
  virtual void BlockHostUntilDoneComplete(int64 correlation_id,
                                          const port::Status* result) {}
};

}  // namespace stream_executor

#endif  // TENSORFLOW_STREAM_EXECUTOR_TRACE_LISTENER_H_
