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

#ifndef TENSORFLOW_CORE_FRAMEWORK_RESOURCE_VAR_H_
#define TENSORFLOW_CORE_FRAMEWORK_RESOURCE_VAR_H_

#include "tensorflow/core/framework/resource_mgr.h"

namespace tensorflow {

// Resource stored by variables in the resource manager (new, resource-style
// version).
//
// These variables have a mixed access mode: they can operate on copy-on-write
// mode (the default) or copy-on-read mode (used only for sparse access).
//
// When copy-on-write mode is enabled reading the value of the variable involves
// grabbing its mutex in shared mode and aliasing the internal tensor as the
// output of the read operation, increasing its reference count. Writing,
// conversely, works by, under an exclusive lock, detecting whether there are
// outstanding aliases of the tensor, using the reference count, copying the
// tensor if they exist, and writing to either the original or a copy with no
// outstanding aliases. Sparse operations are not supported in copy-on-write
// mode.
//
// When a variable is accessed sparsely it switches to copy-on-read mode. To
// switch we need to grab an exclusive lock and might (if there are aliases)
// need to copy the entire tensor. Once copy-on-read mode is enabled, no tensor
// is allowed to alias the variable's internal tensor. This means dense reads
// must return a copy of the variable, done while holding a shared lock. Dense
// writes do not need to check whether aliases exist, and can always write
// directly to the buffer without making a copy, while holding an exclusive
// lock. Sparse reads and sparse writes, on the other hand, can be done under a
// shared or exclusive mutex (the damage from writes under a shared mutex is
// limited since no other buffer is allowed to alias the variable's
// buffer). Using an exclusive mutex disallows concurrent writes and concurrent
// sparse reads, providing some extra safety at the expense of performance,
// while shared mutex allow for "hogwild" behavior. Doing sparse writes under a
// shared mutex prevents them from overlapping with dense writes, which is
// necessary as dense writes can change the shape the of the tensor.
//
// Transitioning a variable from copy-on-read mode to copy-on-write mode is
// currently not supported. To upgrade a variable from copy-on-write to
// copy-on-read use `EnsureSparseVariableAccess()`, and then grab the variable's
// mutex as desired. To access the variable in dense mode grab the mutex either
// directly or via `MaybeLockVariableInputMutexesInOrder` on all variables being
// modified and then call `PrepareToUpdateVariable` on them in any order.
class Var : public ResourceBase {
 public:
  explicit Var(DataType dtype) : tensor_(dtype) {}

  // When locking multiple variables, the locks must be acquired in order of
  // increasing mu() address.
  // TODO(ebrevdo): Use LockSet instead of exposing mu.
  mutex* mu() { return &mu_; }
  Tensor* tensor() { return &tensor_; }

  string DebugString() override {
    return strings::StrCat(DataTypeString(tensor_.dtype()), "/",
                           tensor_.shape().DebugString());
  }

  // Only used in the resource variable path. In resource variables,
  // tensor.IsInitialized() can be true (i.e. have memory allocated to it) while
  // there is not a good value there due to a race condition, and it's possible
  // to stumble upon this during variable.initialized_value(). So it's best to
  // just store directly whether the variable is initialized.
  bool is_initialized = false;  // GUARDED_BY(mu_) but annotalysis doesn't like
                                // it.

  // Also fake-guarded by mu_. Should be set to True whenever any sparse
  // operation uses the variable. Once this is true no tensor is allowed to
  // alias the memory of the variable, and we always copy the variable on
  // reads. This allows sparse operations to happen with only a shared lock if
  // so desired.
  std::atomic<bool> copy_on_read_mode{false};

 private:
  mutex mu_;
  Tensor tensor_;

  ~Var() override {}
  TF_DISALLOW_COPY_AND_ASSIGN(Var);
};

}  //  end namespace tensorflow

#endif  // TENSORFLOW_CORE_FRAMEWORK_RESOURCE_VAR_H_
