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

#include "tensorflow/compiler/xla/debug_options_flags.h"

#include <mutex>  // NOLINT(build/c++11): only using std::call_once, not mutex.
#include <vector>
#include "absl/strings/str_split.h"
#include "tensorflow/compiler/xla/debug_options_parsers.h"
#include "tensorflow/compiler/xla/parse_flags_from_env.h"

namespace xla {

DebugOptions DefaultDebugOptionsIgnoringFlags() {
  DebugOptions opts;
  opts.set_xla_llvm_enable_alias_scope_metadata(true);
  opts.set_xla_llvm_enable_noalias_metadata(true);
  opts.set_xla_llvm_enable_invariant_load_metadata(true);
  opts.set_xla_llvm_disable_expensive_passes(false);
  opts.set_xla_backend_optimization_level(3);
  opts.set_xla_cpu_multi_thread_eigen(true);
  opts.set_xla_gpu_cuda_data_dir("./cuda_sdk_lib");
  opts.set_xla_eliminate_hlo_implicit_broadcast(true);
  opts.set_xla_hlo_dump_as_html(false);
#ifdef INTEL_MKL
  opts.set_xla_cpu_use_mkl_dnn(true);
#endif  // INTEL_MKL
  opts.set_xla_gpu_max_kernel_unroll_factor(4);
  // Set cudnn batchnorm off by default; it does not provide a performance win
  // on average.
  opts.set_xla_gpu_use_cudnn_batchnorm(false);

  // Run all GPU work on one stream by default.  Using multiple streams
  // increases memory usage and we lack strong motivating benchmarks for tuning
  // the heuristics needed to decide when to run on multiple streams.  See
  // b/77879207.
  opts.set_xla_gpu_disable_multi_streaming(true);

  // TODO(jlebar): Disable fastmath once doing so is not a performance
  // regression.
  opts.set_xla_cpu_enable_fast_math(true);
  opts.set_xla_gpu_enable_fast_min_max(true);

  opts.set_xla_force_host_platform_device_count(1);
  return opts;
}

static DebugOptions* flag_values;
static std::vector<tensorflow::Flag>* flag_objects;
static std::once_flag flags_init;

// Allocates flag_values and flag_objects; this function must not be called more
// than once - its call done via call_once.
static void AllocateFlags() {
  flag_values = new DebugOptions(DefaultDebugOptionsIgnoringFlags());

  // Returns a lambda that calls "member_setter" on "flag_values" with the
  // argument passed in to the lambda.
  auto bool_setter_for = [](void (DebugOptions::*member_setter)(bool)) {
    return [member_setter](bool value) {
      (flag_values->*member_setter)(value);
      return true;
    };
  };

  // Returns a lambda that calls "member_setter" on "flag_values" with the
  // argument passed in to the lambda.
  auto int32_setter_for = [](void (DebugOptions::*member_setter)(int32)) {
    return [member_setter](int32 value) {
      (flag_values->*member_setter)(value);
      return true;
    };
  };

  // Custom "sub-parser" lambda for xla_disable_hlo_passes.
  auto setter_for_xla_disable_hlo_passes = [](string comma_separated_values) {
    std::vector<string> disabled_passes =
        absl::StrSplit(comma_separated_values, ',');
    for (const auto& passname : disabled_passes) {
      flag_values->add_xla_disable_hlo_passes(passname);
    }
    return true;
  };

  // Custom "sub-parser" lambda for xla_backend_extra_options.
  auto setter_for_xla_backend_extra_options =
      [](string comma_separated_values) {
        auto* extra_options_map =
            flag_values->mutable_xla_backend_extra_options();
        parse_xla_backend_extra_options(extra_options_map,
                                        comma_separated_values);
        return true;
      };

  // Custom "sub-parser" lambda for xla_reduce_precision.
  auto setter_for_xla_reduce_precision =
      [](string reduce_precision_option_value) {
        HloReducePrecisionOptions* option_proto =
            flag_values->add_hlo_reduce_precision_options();
        return parse_xla_reduce_precision_option(option_proto,
                                                 reduce_precision_option_value);
      };

  flag_objects = new std::vector<tensorflow::Flag>({
      tensorflow::Flag(
          "xla_generate_hlo_graph",
          flag_values->mutable_xla_generate_hlo_graph(),
          "HLO modules matching this regex will be dumped to a .dot file "
          "throughout various stages in compilation."),
      tensorflow::Flag(
          "xla_hlo_graph_addresses",
          bool_setter_for(&DebugOptions::set_xla_hlo_graph_addresses),
          flag_values->xla_hlo_graph_addresses(),
          "With xla_generate_hlo_graph, show addresses of HLO ops in "
          "graph dump."),
      tensorflow::Flag(
          "xla_hlo_graph_path", flag_values->mutable_xla_hlo_graph_path(),
          "With xla_generate_hlo_graph, dump the graphs into this path."),
      tensorflow::Flag(
          "xla_hlo_dump_as_graphdef",
          bool_setter_for(&DebugOptions::set_xla_hlo_dump_as_graphdef),
          flag_values->xla_hlo_dump_as_graphdef(),
          "Dump HLO graphs as TensorFlow GraphDefs."),
      tensorflow::Flag("xla_hlo_dump_as_html",
                       bool_setter_for(&DebugOptions::set_xla_hlo_dump_as_html),
                       flag_values->xla_hlo_dump_as_html(),
                       "Dump HLO graphs as an HTML (DOT rendered into SVG "
                       "inlined in HTML)."),
      tensorflow::Flag(
          "xla_hlo_graph_sharding_color",
          bool_setter_for(&DebugOptions::set_xla_hlo_graph_sharding_color),
          flag_values->xla_hlo_graph_sharding_color(),
          "Assign colors based on sharding assignments when generating the "
          "HLO graphs."),
      tensorflow::Flag(
          "xla_hlo_tfgraph_device_scopes",
          bool_setter_for(&DebugOptions::set_xla_hlo_tfgraph_device_scopes),
          flag_values->xla_hlo_tfgraph_device_scopes(),
          "When generating TensorFlow HLO graphs, if the HLO instructions "
          "are assigned to a specific device, prefix the name scope with "
          "\"devX\" with X being the device ordinal."),
      tensorflow::Flag(
          "xla_log_hlo_text", flag_values->mutable_xla_log_hlo_text(),
          "HLO modules matching this regex will be dumped to LOG(INFO)."),
      tensorflow::Flag(
          "xla_generate_hlo_text_to",
          flag_values->mutable_xla_generate_hlo_text_to(),
          "Dump all HLO modules as text into the provided directory path."),
      tensorflow::Flag(
          "xla_cpu_enable_fast_math",
          bool_setter_for(&DebugOptions::set_xla_cpu_enable_fast_math),
          flag_values->xla_cpu_enable_fast_math(),
          "Enable unsafe fast-math optimizations in the CPU compiler; "
          "this may produce faster code at the expense of some accuracy."),
      tensorflow::Flag(
          "xla_gpu_enable_fast_min_max",
          bool_setter_for(&DebugOptions::set_xla_gpu_enable_fast_min_max),
          flag_values->xla_gpu_enable_fast_min_max(),
          "Enable fast floating point min/max lowering that does not propagate "
          "NaNs."),
      tensorflow::Flag(
          "xla_llvm_enable_alias_scope_metadata",
          bool_setter_for(
              &DebugOptions::set_xla_llvm_enable_alias_scope_metadata),
          flag_values->xla_llvm_enable_alias_scope_metadata(),
          "In LLVM-based backends, enable the emission of "
          "!alias.scope metadata in the generated IR."),
      tensorflow::Flag(
          "xla_llvm_enable_noalias_metadata",
          bool_setter_for(&DebugOptions::set_xla_llvm_enable_noalias_metadata),
          flag_values->xla_llvm_enable_noalias_metadata(),
          "In LLVM-based backends, enable the emission of "
          "!noalias metadata in the generated IR."),
      tensorflow::Flag(
          "xla_llvm_enable_invariant_load_metadata",
          bool_setter_for(
              &DebugOptions::set_xla_llvm_enable_invariant_load_metadata),
          flag_values->xla_llvm_enable_invariant_load_metadata(),
          "In LLVM-based backends, enable the emission of "
          "!invariant.load metadata in "
          "the generated IR."),
      tensorflow::Flag(
          "xla_llvm_disable_expensive_passes",
          bool_setter_for(&DebugOptions::set_xla_llvm_disable_expensive_passes),
          flag_values->xla_llvm_disable_expensive_passes(),
          "In LLVM-based backends, disable a custom set of "
          "expensive optimization passes."),
      tensorflow::Flag(
          "xla_backend_optimization_level",
          int32_setter_for(&DebugOptions::set_xla_backend_optimization_level),
          flag_values->xla_backend_optimization_level(),
          "Numerical optimization level for the XLA compiler backend."),
      tensorflow::Flag(
          "xla_disable_hlo_passes", setter_for_xla_disable_hlo_passes, "",
          "Comma-separated list of hlo passes to be disabled. These names "
          "must exactly match the passes' names; no whitespace around "
          "commas."),
      tensorflow::Flag(
          "xla_disable_all_hlo_passes",
          bool_setter_for(&DebugOptions::set_xla_disable_all_hlo_passes), false,
          "Disables all HLO passes.  Notes that some passes are necessary for "
          "correctness and the invariants that must be satisfied by 'fully "
          "optimized' HLO are different for different devices and may change "
          "over time.  The only 'guarantee', such as it is, is that if you "
          "compile XLA and dump the optimized HLO for some graph, you should "
          "be able to run it again on the same device with the same build of "
          "XLA."),
      tensorflow::Flag(
          "xla_embed_ir_in_executable",
          bool_setter_for(&DebugOptions::set_xla_embed_ir_in_executable),
          flag_values->xla_embed_ir_in_executable(),
          "Embed the compiler IR as a string in the executable."),
      tensorflow::Flag(
          "xla_dump_ir_to", flag_values->mutable_xla_dump_ir_to(),
          "Dump the compiler IR into this directory as individual files."),
      tensorflow::Flag(
          "xla_eliminate_hlo_implicit_broadcast",
          bool_setter_for(
              &DebugOptions::set_xla_eliminate_hlo_implicit_broadcast),
          flag_values->xla_eliminate_hlo_implicit_broadcast(),
          "Eliminate implicit broadcasts when lowering user "
          "computations to HLO instructions; use explicit "
          "broadcast instead."),
      tensorflow::Flag(
          "xla_cpu_multi_thread_eigen",
          bool_setter_for(&DebugOptions::set_xla_cpu_multi_thread_eigen),
          flag_values->xla_cpu_multi_thread_eigen(),
          "When generating calls to Eigen in the CPU backend, "
          "use multi-threaded Eigen mode."),
      tensorflow::Flag("xla_gpu_cuda_data_dir",
                       flag_values->mutable_xla_gpu_cuda_data_dir(),
                       "If non-empty, speficies a local directory containing "
                       "ptxas and nvvm libdevice files; otherwise we use "
                       "those from runfile directories."),
      tensorflow::Flag("xla_gpu_ftz",
                       bool_setter_for(&DebugOptions::set_xla_gpu_ftz),
                       flag_values->xla_gpu_ftz(),
                       "If true, flush-to-zero semantics are enabled in the "
                       "code generated for GPUs."),
      tensorflow::Flag(
          "xla_gpu_disable_multi_streaming",
          bool_setter_for(&DebugOptions::set_xla_gpu_disable_multi_streaming),
          flag_values->xla_gpu_disable_multi_streaming(),
          "If true, multi-streaming in the GPU backend is disabled."),
      tensorflow::Flag(
          "xla_gpu_max_kernel_unroll_factor",
          int32_setter_for(&DebugOptions::set_xla_gpu_max_kernel_unroll_factor),
          flag_values->xla_gpu_max_kernel_unroll_factor(),
          "Specify the maximum kernel unroll factor for the GPU backend."),
      tensorflow::Flag(
          "xla_dump_optimized_hlo_proto_to",
          flag_values->mutable_xla_dump_optimized_hlo_proto_to(),
          "Dump Hlo after all hlo passes are executed as proto binary into "
          "this directory."),
      tensorflow::Flag(
          "xla_dump_unoptimized_hlo_proto_to",
          flag_values->mutable_xla_dump_unoptimized_hlo_proto_to(),
          "Dump HLO before any hlo passes are executed as proto binary into "
          "this directory."),
      tensorflow::Flag("xla_dump_per_pass_hlo_proto_to",
                       flag_values->mutable_xla_dump_per_pass_hlo_proto_to(),
                       "Dump HLO after each pass as an HloProto in binary file "
                       "format into this directory."),
      tensorflow::Flag(
          "xla_test_all_output_layouts",
          bool_setter_for(&DebugOptions::set_xla_test_all_output_layouts),
          flag_values->xla_test_all_output_layouts(),
          "Let ClientLibraryTestBase::ComputeAndCompare* test "
          "all permutations of output layouts. For example, with "
          "a 3D shape, all permutations of the set {0, 1, 2} are "
          "tried."),
      tensorflow::Flag(
          "xla_test_all_input_layouts",
          bool_setter_for(&DebugOptions::set_xla_test_all_input_layouts),
          flag_values->xla_test_all_input_layouts(),
          "Let ClientLibraryTestBase::ComputeAndCompare* test "
          "all permutations of *input* layouts. For example, for "
          "2 input arguments with 2D shape and 4D shape, the "
          "computation will run 2! * 4! times for every possible "
          "layouts"),
      tensorflow::Flag(
          "xla_hlo_profile",
          bool_setter_for(&DebugOptions::set_xla_hlo_profile),
          flag_values->xla_hlo_profile(),
          "Instrument the computation to collect per-HLO cycle counts"),
      tensorflow::Flag("xla_dump_computations_to",
                       flag_values->mutable_xla_dump_computations_to(),
                       "Dump computations that XLA executes into the provided "
                       "directory path"),
      tensorflow::Flag("xla_dump_executions_to",
                       flag_values->mutable_xla_dump_executions_to(),
                       "Dump parameters and results of computations that XLA "
                       "executes into the provided directory path"),
      tensorflow::Flag("xla_backend_extra_options",
                       setter_for_xla_backend_extra_options, "",
                       "Extra options to pass to a backend; "
                       "comma-separated list of 'key=val' strings (=val "
                       "may be omitted); no whitespace around commas."),
      tensorflow::Flag("xla_reduce_precision", setter_for_xla_reduce_precision,
                       "",
                       "Directions for adding reduce-precision operations. "
                       "Format is 'LOCATION=E,M:OPS;NAMES' where LOCATION is "
                       "the class of locations in which to insert the "
                       "operations (e.g., 'OP_OUTPUTS'), E and M are the "
                       "exponent and matissa bit counts respectively, and "
                       "OPS and NAMES are comma-separated (no spaces) lists "
                       "of the operation types and names to which to attach "
                       "the reduce-precision operations.  The NAMES string "
                       "and its preceding ';' may be omitted.  This option "
                       "may be repeated to define multiple sets of added "
                       "reduce-precision operations."),
      tensorflow::Flag(
          "xla_gpu_use_cudnn_batchnorm",
          bool_setter_for(&DebugOptions::set_xla_gpu_use_cudnn_batchnorm),
          flag_values->xla_gpu_use_cudnn_batchnorm(),
          "Allows the GPU backend to implement batchnorm HLOs using cudnn, "
          "rather than expanding them to a soup of HLOs."),
      tensorflow::Flag("xla_cpu_use_mkl_dnn",
                       bool_setter_for(&DebugOptions::set_xla_cpu_use_mkl_dnn),
                       flag_values->xla_cpu_use_mkl_dnn(),
                       "Generate calls to MKL-DNN in the CPU backend."),
      tensorflow::Flag(
          "xla_gpu_crash_on_verification_failures",
          bool_setter_for(
              &DebugOptions::set_xla_gpu_crash_on_verification_failures),
          flag_values->xla_gpu_crash_on_verification_failures(),
          "Crashes the program on extra verification failures, e.g. cuDNN "
          "cross checking failures"),
      tensorflow::Flag(
          "xla_force_host_platform_device_count",
          int32_setter_for(
              &DebugOptions::set_xla_force_host_platform_device_count),
          flag_values->xla_force_host_platform_device_count(),
          "Force the host platform to pretend that there are these many "
          "host \"devices\". All of these host devices are backed by the same"
          "threadpool.  Setting this to anything other than 1 can increase "
          "overhead from context switching but we let the user override this "
          "behavior to help run tests on the host that run models in parallel "
          "across multiple devices."),
      tensorflow::Flag(
          "xla_gpu_disable_ptxas_optimizations",
          bool_setter_for(
              &DebugOptions::set_xla_gpu_disable_ptxas_optimizations),
          flag_values->xla_gpu_disable_ptxas_optimizations(),
          "In XLA:GPU run ptxas in -O0 (default is -O3)."),
  });
  ParseFlagsFromEnvAndDieIfUnknown("XLA_FLAGS", *flag_objects);
}

void AppendDebugOptionsFlags(std::vector<tensorflow::Flag>* flag_list) {
  std::call_once(flags_init, &AllocateFlags);
  flag_list->insert(flag_list->end(), flag_objects->begin(),
                    flag_objects->end());
}

xla::DebugOptions GetDebugOptionsFromFlags() {
  std::call_once(flags_init, &AllocateFlags);
  return *flag_values;
}

}  // namespace xla
