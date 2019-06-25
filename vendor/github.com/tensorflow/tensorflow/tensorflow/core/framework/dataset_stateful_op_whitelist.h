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

#ifndef TENSORFLOW_CORE_FRAMEWORK_DATASET_STATEFUL_OP_WHITELIST_H_
#define TENSORFLOW_CORE_FRAMEWORK_DATASET_STATEFUL_OP_WHITELIST_H_

#include <unordered_set>
#include "tensorflow/core/lib/core/status.h"

namespace tensorflow {
namespace data {
// Registry for stateful ops that need to be used in dataset functions.
// See below macro for usage details.
class WhitelistedStatefulOpRegistry {
 public:
  Status Add(string op_name) {
    op_names_.insert(std::move(op_name));
    return Status::OK();
  }

  bool Contains(const string& op_name) { return op_names_.count(op_name); }

  static WhitelistedStatefulOpRegistry* Global() {
    static auto* reg = new WhitelistedStatefulOpRegistry;
    return reg;
  }

 private:
  WhitelistedStatefulOpRegistry() = default;
  WhitelistedStatefulOpRegistry(WhitelistedStatefulOpRegistry const& copy) =
      delete;
  WhitelistedStatefulOpRegistry operator=(
      WhitelistedStatefulOpRegistry const& copy) = delete;

  std::unordered_set<string> op_names_;
};

}  // namespace data

// Use this macro to whitelist an op that is marked stateful but needs to be
// used inside a map_fn in an input pipeline. This is only needed if you wish
// to be able to checkpoint the state of the input pipeline. We currently
// do not allow stateful ops to be defined inside of map_fns since it is not
// possible to save their state.
// Note that the state of the whitelisted ops inside functions will not be
// saved during checkpointing, hence this should only be used if the op is
// marked stateful for reasons like to avoid constant folding during graph
// optimiztion but is not stateful.
// If possible, try to remove the stateful flag on the op first.
// Example usage:
//
//   WHITELIST_STATEFUL_OP_FOR_DATASET_FUNCTIONS("LegacyStatefulReader");
//
#define WHITELIST_STATEFUL_OP_FOR_DATASET_FUNCTIONS(name) \
  WHITELIST_STATEFUL_OP_FOR_DATASET_FUNCTIONS_UNIQ_HELPER(__COUNTER__, name)
#define WHITELIST_STATEFUL_OP_FOR_DATASET_FUNCTIONS_UNIQ_HELPER(ctr, name) \
  WHITELIST_STATEFUL_OP_FOR_DATASET_FUNCTIONS_UNIQ(ctr, name)
#define WHITELIST_STATEFUL_OP_FOR_DATASET_FUNCTIONS_UNIQ(ctr, name)   \
  static ::tensorflow::Status whitelist_op##ctr TF_ATTRIBUTE_UNUSED = \
      ::tensorflow::data::WhitelistedStatefulOpRegistry::Global()->Add(name)

}  // namespace tensorflow

#endif  // TENSORFLOW_CORE_FRAMEWORK_DATASET_STATEFUL_OP_WHITELIST_H_
