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

#include "tensorflow/compiler/xla/service/interpreter/compiler.h"

#include <string>
#include <utility>

#include "absl/memory/memory.h"
#include "tensorflow/compiler/xla/service/algebraic_simplifier.h"
#include "tensorflow/compiler/xla/service/computation_placer.h"
#include "tensorflow/compiler/xla/service/flatten_call_graph.h"
#include "tensorflow/compiler/xla/service/hlo_constant_folding.h"
#include "tensorflow/compiler/xla/service/hlo_cse.h"
#include "tensorflow/compiler/xla/service/hlo_dce.h"
#include "tensorflow/compiler/xla/service/hlo_pass_fix.h"
#include "tensorflow/compiler/xla/service/hlo_pass_pipeline.h"
#include "tensorflow/compiler/xla/service/hlo_subcomputation_unification.h"
#include "tensorflow/compiler/xla/service/interpreter/executable.h"
#include "tensorflow/compiler/xla/service/layout_assignment.h"
#include "tensorflow/compiler/xla/service/map_inliner.h"
#include "tensorflow/compiler/xla/service/reshape_mover.h"
#include "tensorflow/compiler/xla/service/while_loop_simplifier.h"
#include "tensorflow/compiler/xla/status_macros.h"
#include "tensorflow/core/lib/core/errors.h"
#include "tensorflow/core/platform/types.h"

namespace xla {
namespace interpreter {

Status InterpreterCompiler::RunHloOptimization(HloModule* hlo_module) {
  HloPassPipeline pipeline("Interpreter");

  pipeline.AddPass<LayoutAssignment>(
      hlo_module->mutable_entry_computation_layout(),
      LayoutAssignment::InstructionCanChangeLayout);
  return pipeline.Run(hlo_module).status();
}

StatusOr<std::unique_ptr<HloModule>> InterpreterCompiler::RunHloPasses(
    std::unique_ptr<HloModule> hlo_module, se::StreamExecutor* /*stream_exec*/,
    DeviceMemoryAllocator* /*device_allocator*/) {
  VLOG(1) << "Run hlo passes on graph " << hlo_module->name();
  TF_RETURN_IF_ERROR(RunHloOptimization(hlo_module.get()));
  return std::move(hlo_module);
}

Status InterpreterCompiler::RunHloPassesOnModuleGroup(
    HloModuleGroup* module_group,
    absl::Span<se::StreamExecutor* const> executors,
    DeviceMemoryAllocator* device_allocator) {
  return Unimplemented("Module group compilation not supported on Interpreter");
}

StatusOr<std::unique_ptr<Executable>> InterpreterCompiler::RunBackend(
    std::unique_ptr<HloModule> hlo_module, se::StreamExecutor* stream_exec,
    DeviceMemoryAllocator* /*device_allocator*/) {
  TF_RET_CHECK(stream_exec != nullptr);

  VLOG(1) << "Run backend " << hlo_module->name();

  // Typically you would visit the HLO graph, building up a compiled equivalent
  // In this case we are using an HloEvaluator at execution time, so we don't
  // need to compile anything

  // Create executable from only the Hlo module.
  auto evaluator = absl::make_unique<HloEvaluator>();
  evaluator->set_use_fast_path(
      hlo_module->config().debug_options().xla_hlo_evaluator_use_fast_path());
  std::unique_ptr<Executable> executable =
      absl::make_unique<InterpreterExecutable>(std::move(hlo_module),
                                               std::move(evaluator));

  return std::move(executable);
}

StatusOr<std::vector<std::unique_ptr<Executable>>>
InterpreterCompiler::RunBackendOnModuleGroup(
    std::unique_ptr<HloModuleGroup> module_group,
    std::vector<std::vector<se::StreamExecutor*>> stream_exec,
    DeviceMemoryAllocator* device_allocator) {
  return Unimplemented(
      "Module group compilation is not supported on Interpreter.");
}

StatusOr<std::vector<std::unique_ptr<Executable>>> InterpreterCompiler::Compile(
    std::unique_ptr<HloModuleGroup> module_group,
    std::vector<std::vector<se::StreamExecutor*>> stream_exec,
    DeviceMemoryAllocator* device_allocator) {
  if (module_group->empty()) {
    return std::vector<std::unique_ptr<Executable>>();
  }
  if (module_group->size() > 1) {
    return tensorflow::errors::Unimplemented(
        "Compilation of multiple HLO modules is not supported on Interpreter.");
  }
  if (stream_exec.size() != 1 || stream_exec[0].size() != 1) {
    return tensorflow::errors::Unimplemented(
        "Unexpected number of StreamExecutor's.");
  }
  auto hlo_modules = module_group->ConsumeModules();
  TF_ASSIGN_OR_RETURN(auto module,
                      RunHloPasses(std::move(hlo_modules[0]), stream_exec[0][0],
                                   device_allocator));
  TF_ASSIGN_OR_RETURN(
      auto executable,
      RunBackend(std::move(module), stream_exec[0][0], device_allocator));
  std::vector<std::unique_ptr<Executable>> ret;
  ret.push_back(std::move(executable));
  return std::move(ret);
}

StatusOr<std::vector<std::unique_ptr<AotCompilationResult>>>
InterpreterCompiler::CompileAheadOfTime(
    std::unique_ptr<HloModuleGroup> module_group,
    const AotCompilationOptions& aot_options) {
  return tensorflow::errors::InvalidArgument(
      "AOT compilation not supported on Interpreter");
}

se::Platform::Id InterpreterCompiler::PlatformId() const {
  return se::interpreter::kXlaInterpreterPlatformId;
}

HloCostAnalysis::ShapeSizeFunction InterpreterCompiler::ShapeSizeBytesFunction()
    const {
  return InterpreterExecutable::ShapeSizeBytes;
}

static bool InitModule() {
  xla::Compiler::RegisterCompilerFactory(
      se::interpreter::kXlaInterpreterPlatformId, []() {
        return absl::make_unique<xla::interpreter::InterpreterCompiler>();
      });
  xla::ComputationPlacer::RegisterComputationPlacer(
      se::interpreter::kXlaInterpreterPlatformId,
      []() { return absl::make_unique<xla::ComputationPlacer>(); });
  return true;
}

static bool module_initialized = InitModule();

}  // namespace interpreter
}  // namespace xla
