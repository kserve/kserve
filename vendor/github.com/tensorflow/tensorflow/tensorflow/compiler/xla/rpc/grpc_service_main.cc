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

// Basic server binary that exposes a xla::Service through a GRPC interface
// on a configurable port.
#include "grpcpp/security/server_credentials.h"
#include "grpcpp/server.h"
#include "grpcpp/server_builder.h"
#include "absl/strings/str_format.h"
#include "tensorflow/compiler/xla/rpc/grpc_service.h"
#include "tensorflow/compiler/xla/service/platform_util.h"
#include "tensorflow/core/platform/init_main.h"
#include "tensorflow/core/platform/logging.h"
#include "tensorflow/core/util/command_line_flags.h"

namespace xla {
namespace {

int RealMain(int argc, char** argv) {
  int32 port = 1685;
  bool any_address = false;
  string platform_str;
  std::vector<tensorflow::Flag> flag_list = {
      tensorflow::Flag("platform", &platform_str,
                       "The XLA platform this service should be bound to"),
      tensorflow::Flag("port", &port, "The TCP port to listen on"),
      tensorflow::Flag(
          "any", &any_address,
          "Whether to listen to any host address or simply localhost"),
  };
  string usage = tensorflow::Flags::Usage(argv[0], flag_list);
  bool parsed_values_ok = tensorflow::Flags::Parse(&argc, argv, flag_list);
  if (!parsed_values_ok) {
    LOG(ERROR) << usage;
    return 2;
  }
  tensorflow::port::InitMain(argv[0], &argc, &argv);

  se::Platform* platform = nullptr;
  if (!platform_str.empty()) {
    platform = PlatformUtil::GetPlatform(platform_str).ValueOrDie();
  }
  std::unique_ptr<xla::GRPCService> service =
      xla::GRPCService::NewService(platform).ConsumeValueOrDie();

  ::grpc::ServerBuilder builder;
  string server_address(
      absl::StrFormat("%s:%d", any_address ? "[::]" : "localhost", port));

  builder.SetMaxReceiveMessageSize(INT_MAX);
  builder.AddListeningPort(server_address, ::grpc::InsecureServerCredentials());
  builder.RegisterService(service.get());
  std::unique_ptr<::grpc::Server> server(builder.BuildAndStart());

  LOG(INFO) << "Server listening on " << server_address;
  server->Wait();
  return 0;
}

}  // namespace
}  // namespace xla

int main(int argc, char** argv) { return xla::RealMain(argc, argv); }
