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

#include "tensorflow/compiler/xla/client/compile_only_client.h"

#include "absl/memory/memory.h"
#include "llvm/ADT/Triple.h"
#include "tensorflow/compiler/xla/status_macros.h"

namespace xla {

StatusOr<std::vector<std::unique_ptr<AotCompilationResult>>>
CompileOnlyClient::CompileAheadOfTime(
    const absl::Span<const AotXlaComputationInstance> computations,
    const AotCompilationOptions& options,
    std::unique_ptr<AotCompilationMetadata>* metadata) {
  std::vector<CompileOnlyService::AotXlaComputationInstance> service_instances;
  service_instances.reserve(computations.size());
  for (const AotXlaComputationInstance& instance : computations) {
    service_instances.emplace_back();
    CompileOnlyService::AotXlaComputationInstance& service_instance =
        service_instances.back();
    TF_RET_CHECK(instance.computation != nullptr);
    service_instance.computation = instance.computation->proto();
    service_instance.argument_layouts = instance.argument_layouts;
    service_instance.result_layout = instance.result_layout;
  }
  return compiler_service_->CompileAheadOfTime(service_instances, options,
                                               metadata);
}

int64 CompileOnlyClient::PointerSizeForTriple(absl::string_view triple) {
  llvm::Triple llvm_triple(
      llvm::Triple::normalize(llvm::StringRef(triple.data(), triple.size())));
  if (llvm_triple.isArch64Bit()) {
    return 8;
  } else if (llvm_triple.isArch32Bit()) {
    return 4;
  } else {
    CHECK(llvm_triple.isArch16Bit());
    return 2;
  }
}

}  // namespace xla
