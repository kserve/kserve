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

#include "tensorflow/compiler/xla/service/compilation_cache.h"

#include <utility>

#include "tensorflow/compiler/xla/types.h"
#include "tensorflow/compiler/xla/util.h"
#include "tensorflow/compiler/xla/xla_data.pb.h"
#include "tensorflow/core/lib/strings/strcat.h"
#include "tensorflow/core/platform/logging.h"

namespace xla {

namespace {

int64 GetUniqueId() {
  static tensorflow::mutex mu(tensorflow::LINKER_INITIALIZED);
  static int64 counter = 0;
  tensorflow::mutex_lock loc(mu);
  const int64 id = counter++;
  return id;
}

}  // namespace

ExecutionHandle CompilationCache::Insert(
    std::unique_ptr<Executable> executable) {
  tensorflow::mutex_lock lock(mutex_);

  CacheKey key = GetUniqueId();
  VLOG(2) << "inserting cache key: " << key;
  CHECK_EQ(cache_.count(key), 0);
  cache_.emplace(key, std::move(executable));

  ExecutionHandle handle;
  handle.set_handle(key);
  return handle;
}

StatusOr<std::shared_ptr<Executable>> CompilationCache::LookUp(
    const ExecutionHandle& handle) const {
  tensorflow::mutex_lock lock(mutex_);

  CacheKey key = handle.handle();
  VLOG(2) << "looking up cache key: " << key;
  if (cache_.count(key) == 0) {
    VLOG(2) << "cache key not found: " << key;
    return InvalidArgumentStrCat("can not find executable with handle ", key);
  } else {
    auto& result = cache_.at(key);
    VLOG(2) << "hit executable: " << result->module().name();
    return result;
  }
}

}  // namespace xla
