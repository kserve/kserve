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

#ifndef TENSORFLOW_COMPILER_XLA_SERVICE_HLO_DCE_H_
#define TENSORFLOW_COMPILER_XLA_SERVICE_HLO_DCE_H_

#include "tensorflow/compiler/xla/service/hlo_computation.h"
#include "tensorflow/compiler/xla/service/hlo_instruction.h"
#include "tensorflow/compiler/xla/service/hlo_module.h"
#include "tensorflow/compiler/xla/service/hlo_pass_interface.h"
#include "tensorflow/compiler/xla/statusor.h"

namespace xla {

// HLO pass which removes dead instructions from each computation in the module
// and removes dead computations from the module.
//
// An instruction is dead if it is not reachable from the root. A computation is
// dead if it is not the entry computation of the module and it is not reachable
// from the entry computation.
//
// This pass does not remove dead parameter instructions, as parameter
// instructions cannot be deleted.
class HloDCE : public HloModulePass {
 public:
  ~HloDCE() override {}
  absl::string_view name() const override { return "dce"; }

  // Run the pass on the given module. Returns whether the module was changed
  // (instructions were removed).
  StatusOr<bool> Run(HloModule* module) override;
};

}  // namespace xla

#endif  // TENSORFLOW_COMPILER_XLA_SERVICE_HLO_DCE_H_
