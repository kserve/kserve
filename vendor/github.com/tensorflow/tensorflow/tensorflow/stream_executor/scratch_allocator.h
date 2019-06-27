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

#ifndef TENSORFLOW_STREAM_EXECUTOR_SCRATCH_ALLOCATOR_H_
#define TENSORFLOW_STREAM_EXECUTOR_SCRATCH_ALLOCATOR_H_

#include <memory>

#include "tensorflow/stream_executor/device_memory.h"
#include "tensorflow/stream_executor/lib/statusor.h"
#include "tensorflow/stream_executor/platform/port.h"
#include "tensorflow/stream_executor/temporary_device_memory.h"

namespace stream_executor {

class Stream;

// Interface that allows stream operations (e.g.
// Stream::ThenConvolveWithScratch) to optionally request scratch space be
// allocated in order to speed up the operation being enqueued.
//
// Note that the caller is responsible for deallocating the scratch space at a
// known-safe point, when all scratch-memory-consuming kernels are known for
// sure to have finished; e.g. at stream synchronization time. This is different
// from a traditional C++ object allocator, where the client is responsible for
// releasing. (Conceptually, scratch memory is a form of "temporary" device
// memory allocation.)
class ScratchAllocator {
 public:
  virtual ~ScratchAllocator();

  // Returns a limit of memory this scratch allocator wants to produce, in
  // bytes. This information may be used to help select an algorithm.
  //
  // Returns values < 0 to indicate that there is no recommended limit.
  virtual int64 GetMemoryLimitInBytes(Stream* stream) = 0;

  // Returns an allocation on byte_size bytes for use in an operation on stream.
  //
  // This is a temporary allocation, and the caller is responsible for
  // deallocating at some known-safe point. See the class comment above.
  virtual port::StatusOr<DeviceMemory<uint8>> AllocateBytes(
      Stream* stream, int64 byte_size) = 0;
};

// Allocates a single temporary memory allocation -- this memory is deallocated
// at the next stream synchronization point after this object has gone out of
// scope. This satisfies the lifetime and deallocation properties given in the
// class comment above.
//
// Thread-compatible, but not thread-safe (use in scenarios where only one
// thread will request the scratch allocation).
class OneTimeScratchAllocator : public ScratchAllocator {
 public:
  OneTimeScratchAllocator();
  ~OneTimeScratchAllocator() override;
  int64 GetMemoryLimitInBytes(Stream* stream) override;
  port::StatusOr<DeviceMemory<uint8>> AllocateBytes(Stream* stream,
                                                    int64 byte_size) override;

 private:
  std::unique_ptr<TemporaryDeviceMemory<uint8>> temporary_;

  SE_DISALLOW_COPY_AND_ASSIGN(OneTimeScratchAllocator);
};

}  // namespace stream_executor

#endif  // TENSORFLOW_STREAM_EXECUTOR_SCRATCH_ALLOCATOR_H_
