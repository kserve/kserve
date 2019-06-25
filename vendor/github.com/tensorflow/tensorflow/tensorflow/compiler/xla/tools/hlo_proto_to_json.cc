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

// Usage:
//   hlo_proto_to_json --input_file=some_binary_proto
//   --output_file=path_to_dump_output
//
// Reads one serilized Hlo module, convert it into JSON format and dump into
// some output directory. some_binaray_proto is obtained by serializing Hlo
// module to disk using --xla_dump_optimized_hlo_proto_to debug option.

#include <stdio.h>
#include <string>
#include <vector>

#include "tensorflow/compiler/xla/service/hlo.pb.h"
#include "tensorflow/compiler/xla/statusor.h"
#include "tensorflow/compiler/xla/util.h"
#include "tensorflow/core/lib/core/status.h"
#include "tensorflow/core/platform/env.h"
#include "tensorflow/core/platform/init_main.h"
#include "tensorflow/core/platform/logging.h"
#include "tensorflow/core/util/command_line_flags.h"

using tensorflow::Env;
using xla::string;

namespace xla {
namespace tools {

StatusOr<string> ToJson(const tensorflow::protobuf::Message& message) {
  string json_output;
  tensorflow::protobuf::util::JsonPrintOptions json_options;
  json_options.add_whitespace = true;
  json_options.always_print_primitive_fields = true;
  auto status = tensorflow::protobuf::util::MessageToJsonString(
      message, &json_output, json_options);
  if (!status.ok()) {
    return InternalError("MessageToJsonString failed: %s",
                         status.error_message().data());
  }
  return json_output;
}

void RealMain(const string& input, const string& output) {
  HloProto hlo_proto;
  TF_CHECK_OK(tensorflow::ReadBinaryProto(tensorflow::Env::Default(), input,
                                          &hlo_proto))
      << "Can't open, read, or parse input file " << input;

  auto statusor = ToJson(hlo_proto);
  QCHECK(statusor.ok()) << "Error converting " << input << " to JSON."
                        << statusor.status();

  TF_CHECK_OK(tensorflow::WriteStringToFile(tensorflow::Env::Default(), output,
                                            statusor.ValueOrDie()));
}

}  // namespace tools
}  // namespace xla

int main(int argc, char** argv) {
  string input_file, output_file;
  const std::vector<tensorflow::Flag> flag_list = {
      tensorflow::Flag("input_file", &input_file, "file to convert."),
      tensorflow::Flag("output_file", &output_file, "converted file"),
  };
  const string usage = tensorflow::Flags::Usage(argv[0], flag_list);
  bool parse_ok = tensorflow::Flags::Parse(&argc, argv, flag_list);
  tensorflow::port::InitMain(usage.c_str(), &argc, &argv);
  QCHECK(parse_ok && argc == 1) << "\n" << usage;

  QCHECK(!input_file.empty()) << "--input_file is required";
  QCHECK(!output_file.empty()) << "--output_file is required";

  xla::tools::RealMain(input_file, output_file);

  return 0;
}
