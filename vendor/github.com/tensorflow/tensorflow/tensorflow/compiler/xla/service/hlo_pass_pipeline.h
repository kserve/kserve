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

#ifndef TENSORFLOW_COMPILER_XLA_SERVICE_HLO_PASS_PIPELINE_H_
#define TENSORFLOW_COMPILER_XLA_SERVICE_HLO_PASS_PIPELINE_H_

#include <algorithm>
#include <memory>
#include <string>
#include <vector>

#include "absl/memory/memory.h"
#include "absl/strings/str_cat.h"
#include "tensorflow/compiler/xla/service/hlo_module.h"
#include "tensorflow/compiler/xla/service/hlo_pass_interface.h"
#include "tensorflow/compiler/xla/statusor.h"
#include "tensorflow/compiler/xla/types.h"
#include "tensorflow/core/platform/macros.h"

namespace xla {

// Pipeline of HLO passes.
class HloPassPipeline : public HloPassInterface {
 public:
  explicit HloPassPipeline(const string& name) : name_(name) {}
  absl::string_view name() const override { return name_; }

  // Add a pass to the pipeline. It should be called with the arguments for the
  // pass constructor:
  //
  //   pipeline.AddPass<FooPass>(constructor_arg1, constructor_arg2);
  //
  // Returns a reference to the added pass.
  template <typename T, typename... Args>
  T& AddPass(Args&&... args) {
    CHECK(!run_called_) << "AddPass cannot be called after Run";
    auto pass = new T(std::forward<Args>(args)...);
    passes_.push_back(std::unique_ptr<T>(pass));
    return *pass;
  }

  // Add an invariant-checking pass to the pipeline. It will be run before and
  // after each HLO pass. The invariant checking pass must not mutate the graph
  // (it is required to always return "false" from its Run() method).
  template <typename T, typename... Args>
  T& AddInvariantChecker(Args&&... args) {
    CHECK(!run_called_) << "AddInvariantChecker cannot be called after Run";
    auto pass = new T(std::forward<Args>(args)...);
    invariant_checkers_.push_back(std::unique_ptr<T>(pass));
    return *pass;
  }

  StatusOr<bool> Run(HloModule* module) override;
  StatusOr<bool> RunOnModuleGroup(HloModuleGroup* module_group) override;

 private:
  // Returns the set of passes which are enabled. DebugOptions can selectively
  // disable passes via --xla_disable_hlo_passes flag.
  std::vector<HloPassInterface*> GetEnabledPasses(
      const DebugOptions& debug_options);

  // Maybe dumps the given module or module group depending on flag values
  // contained in DebugOptions of module config.
  void MaybeDumpHlo(const HloModuleGroup& module_group,
                    absl::string_view after_pass_name,
                    absl::string_view before_pass_name);
  void MaybeDumpHlo(const HloModule& module, absl::string_view after_pass_name,
                    absl::string_view before_pass_name);

  // Runs the invariant checker on the given HLO. HloT can be either HloModule
  // or HloModuleGroup.
  template <typename HloT>
  Status RunInvariantCheckers(HloT* hlo, absl::string_view after_pass_name);

  // Helper which runs the given pass on the given HLO. HloT can be either
  // HloModule or HloModuleGroup.
  template <typename HloT>
  StatusOr<bool> RunPassesInternal(HloT* hlo,
                                   absl::Span<HloPassInterface* const> passes);

  // Helpers which run the given passes on the given HLO construct. These
  // helpers enable templating of the core of the pipeline logic by providing
  // HloModule and HloModuleGroup specific methods with the same name.
  static StatusOr<bool> RunHelper(HloPassInterface* pass, HloModule* module) {
    return pass->Run(module);
  }
  static StatusOr<bool> RunHelper(HloPassInterface* pass,
                                  HloModuleGroup* module_group) {
    return pass->RunOnModuleGroup(module_group);
  }

  const string name_;
  std::vector<std::unique_ptr<HloPassInterface>> passes_;
  std::vector<std::unique_ptr<HloPassInterface>> invariant_checkers_;
  bool run_called_ = false;
};

}  // namespace xla

#endif  // TENSORFLOW_COMPILER_XLA_SERVICE_HLO_PASS_PIPELINE_H_
