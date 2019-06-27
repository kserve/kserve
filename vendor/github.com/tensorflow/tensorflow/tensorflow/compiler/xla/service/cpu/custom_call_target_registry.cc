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

#include "tensorflow/compiler/xla/service/cpu/custom_call_target_registry.h"

namespace xla {
namespace cpu {

CustomCallTargetRegistry* CustomCallTargetRegistry::Global() {
  static auto* registry = new CustomCallTargetRegistry;
  return registry;
}

void CustomCallTargetRegistry::Register(const std::string& symbol,
                                        void* address) {
  std::lock_guard<std::mutex> lock(mu_);
  registered_symbols_[symbol] = address;
}

void* CustomCallTargetRegistry::Lookup(const std::string& symbol) const {
  std::lock_guard<std::mutex> lock(mu_);
  auto it = registered_symbols_.find(symbol);
  return it == registered_symbols_.end() ? nullptr : it->second;
}

}  // namespace cpu
}  // namespace xla
