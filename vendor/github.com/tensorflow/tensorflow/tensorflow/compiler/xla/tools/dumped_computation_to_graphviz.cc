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

// Usage: dumped_computation_to_graphviz some_binary_snapshot_proto*
//
// Dumps a graphviz URL for a snapshot computation to the command line.
//
// some_binary_snapshot_proto is obtained by serializing the HloSnapshot from
// ServiceInterface::SnapshotComputation to disk.
//
// The GraphViz URL is placed into the log stderr, whereas computation
// statistics are printed on stdout (implementation note: getting computation
// statistics is how we trigger compilation to split out a GraphViz URL).

#include <stdio.h>
#include <memory>
#include <string>

#include "absl/types/span.h"
#include "tensorflow/compiler/xla/client/client.h"
#include "tensorflow/compiler/xla/client/client_library.h"
#include "tensorflow/compiler/xla/client/local_client.h"
#include "tensorflow/compiler/xla/client/xla_computation.h"
#include "tensorflow/compiler/xla/debug_options_flags.h"
#include "tensorflow/compiler/xla/service/hlo.pb.h"
#include "tensorflow/compiler/xla/service/service.h"
#include "tensorflow/compiler/xla/statusor.h"
#include "tensorflow/compiler/xla/types.h"
#include "tensorflow/compiler/xla/xla_data.pb.h"
#include "tensorflow/core/platform/env.h"
#include "tensorflow/core/platform/init_main.h"
#include "tensorflow/core/platform/logging.h"

namespace xla {
namespace tools {

void RealMain(absl::Span<char* const> args) {
  Client* client = ClientLibrary::LocalClientOrDie();
  for (char* arg : args) {
    HloSnapshot module;
    TF_CHECK_OK(
        tensorflow::ReadBinaryProto(tensorflow::Env::Default(), arg, &module));
    XlaComputation computation =
        client->LoadSnapshot(module).ConsumeValueOrDie();
    DebugOptions debug_options = GetDebugOptionsFromFlags();
    debug_options.set_xla_generate_hlo_graph(".*");
    ComputationStats stats =
        client->GetComputationStats(computation, debug_options)
            .ConsumeValueOrDie();
    fprintf(stdout, ">>> %s :: %s\n", arg, stats.DebugString().c_str());
  }
}

}  // namespace tools
}  // namespace xla

int main(int argc, char** argv) {
  std::vector<tensorflow::Flag> flag_list;
  xla::AppendDebugOptionsFlags(&flag_list);
  xla::string usage = tensorflow::Flags::Usage(argv[0], flag_list);
  const bool parse_result = tensorflow::Flags::Parse(&argc, argv, flag_list);
  if (!parse_result) {
    LOG(ERROR) << "\n" << usage;
    return 2;
  }
  tensorflow::port::InitMain(argv[0], &argc, &argv);

  absl::Span<char* const> args(argv, argc);
  args.remove_prefix(1);  // Pop off the binary name, argv[0]
  xla::tools::RealMain(args);
  return 0;
}
