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

#include "tensorflow/compiler/tf2xla/tf2xla_util.h"

#include "absl/strings/match.h"
#include "absl/strings/str_cat.h"
#include "absl/strings/string_view.h"
#include "tensorflow/cc/framework/ops.h"
#include "tensorflow/cc/ops/data_flow_ops.h"
#include "tensorflow/cc/ops/function_ops.h"
#include "tensorflow/cc/ops/standard_ops.h"
#include "tensorflow/compiler/tf2xla/sharding_util.h"
#include "tensorflow/core/common_runtime/graph_optimizer.h"
#include "tensorflow/core/common_runtime/process_function_library_runtime.h"
#include "tensorflow/core/framework/function.h"
#include "tensorflow/core/framework/node_def.pb.h"
#include "tensorflow/core/graph/graph.h"
#include "tensorflow/core/lib/core/status.h"
#include "tensorflow/core/lib/core/status_test_util.h"
#include "tensorflow/core/platform/test.h"
#include "tensorflow/core/public/version.h"

namespace tensorflow {
namespace {

void ExpectErrorContains(const Status& status, absl::string_view str) {
  EXPECT_NE(Status::OK(), status);
  EXPECT_TRUE(absl::StrContains(status.error_message(), str))
      << "expected error: " << status.error_message() << " to contain: " << str;
}

TEST(ValidateConfig, Good) {
  tf2xla::Config config;
  tf2xla::Feed* feed = config.add_feed();
  feed->mutable_id()->set_node_name("foo");
  feed->mutable_id()->set_output_index(123);
  feed->set_name("foo_debug");
  feed = config.add_feed();
  feed->mutable_id()->set_node_name("bar");
  feed->mutable_id()->set_output_index(0);
  tf2xla::Fetch* fetch = config.add_fetch();
  fetch->mutable_id()->set_node_name("baz");
  fetch->mutable_id()->set_output_index(456);
  fetch->set_name("baz_debug");
  fetch = config.add_fetch();
  fetch->mutable_id()->set_node_name("banana");
  fetch->mutable_id()->set_output_index(0);
  TF_EXPECT_OK(ValidateConfig(config));
}

TEST(ValidateConfig, BadEmpty) {
  tf2xla::Config config;
  ExpectErrorContains(ValidateConfig(config), "fetches must be specified");
}

TEST(ValidateConfig, BadNoFetch) {
  tf2xla::Config config;
  tf2xla::Feed* feed = config.add_feed();
  feed->mutable_id()->set_node_name("foo");
  ExpectErrorContains(ValidateConfig(config), "fetches must be specified");
}

TEST(ValidateConfig, BadFeedNodeName) {
  tf2xla::Config config;
  config.add_feed();
  ExpectErrorContains(ValidateConfig(config), "node_name must be non-empty");
}

TEST(ValidateConfig, BadFeedOutputIndex) {
  tf2xla::Config config;
  tf2xla::Feed* feed = config.add_feed();
  feed->mutable_id()->set_node_name("foo");
  feed->mutable_id()->set_output_index(-1);
  ExpectErrorContains(ValidateConfig(config), "output_index must be positive");
}

TEST(ValidateConfig, BadFetchNodeName) {
  tf2xla::Config config;
  tf2xla::Feed* feed = config.add_feed();
  feed->mutable_id()->set_node_name("foo");
  config.add_fetch();
  ExpectErrorContains(ValidateConfig(config), "node_name must be non-empty");
}

TEST(ValidateConfig, BadFetchOutputIndex) {
  tf2xla::Config config;
  tf2xla::Feed* feed = config.add_feed();
  feed->mutable_id()->set_node_name("foo");
  tf2xla::Fetch* fetch = config.add_fetch();
  fetch->mutable_id()->set_node_name("bar");
  fetch->mutable_id()->set_output_index(-1);
  ExpectErrorContains(ValidateConfig(config), "output_index must be positive");
}

TEST(ValidateConfig, DuplicateFeedName) {
  tf2xla::Config config;
  tf2xla::Feed* feed = config.add_feed();
  feed->mutable_id()->set_node_name("foo");
  feed->set_name("dup");
  feed = config.add_feed();
  feed->mutable_id()->set_node_name("bar");
  feed->set_name("dup");
  ExpectErrorContains(ValidateConfig(config), "duplicate feed name");
}

TEST(ValidateConfig, DuplicateFetchName) {
  tf2xla::Config config;
  tf2xla::Feed* feed = config.add_feed();
  feed->mutable_id()->set_node_name("foo");
  tf2xla::Fetch* fetch = config.add_fetch();
  fetch->mutable_id()->set_node_name("bar");
  fetch->set_name("dup");
  fetch = config.add_fetch();
  fetch->mutable_id()->set_node_name("baz");
  fetch->set_name("dup");
  ExpectErrorContains(ValidateConfig(config), "duplicate fetch name");
}

TEST(ValidateConfig, ConflictingFeedName) {
  tf2xla::Config config;
  tf2xla::Feed* feed = config.add_feed();
  feed->mutable_id()->set_node_name("foo");
  feed->set_name("conflict");
  feed = config.add_feed();
  feed->mutable_id()->set_node_name("bar");
  feed->set_name("conflict_data");
  ExpectErrorContains(ValidateConfig(config), "conflicting feed name");
}

TEST(ValidateConfig, ConflictingFetchName) {
  tf2xla::Config config;
  tf2xla::Feed* feed = config.add_feed();
  feed->mutable_id()->set_node_name("foo");
  tf2xla::Fetch* fetch = config.add_fetch();
  fetch->mutable_id()->set_node_name("bar");
  fetch->set_name("conflict");
  fetch = config.add_fetch();
  fetch->mutable_id()->set_node_name("baz");
  fetch->set_name("conflict_data");
  ExpectErrorContains(ValidateConfig(config), "conflicting fetch name");
}

static tf2xla::Config FetchesConfig(std::vector<string> fetches) {
  tf2xla::Config config;
  for (const auto& fetch_node_name : fetches) {
    auto* fetch = config.add_fetch();
    fetch->set_name(absl::StrCat("fetch_", fetch_node_name));
    fetch->mutable_id()->set_node_name(fetch_node_name);
  }
  return config;
}

TEST(PruneGraphDefInto, Basic) {
  GraphDef def;
  auto* n = def.add_node();
  n->set_name("a");
  n->add_input("b:0");
  n->add_input("^c");

  GraphDef copy;
  ExpectErrorContains(PruneGraphDefInto(FetchesConfig({"missing"}), def, &copy),
                      "node missing needed");
  ExpectErrorContains(PruneGraphDefInto(FetchesConfig({"a"}), def, &copy),
                      "node b needed");

  n = def.add_node();
  n->set_name("b");
  ExpectErrorContains(PruneGraphDefInto(FetchesConfig({"a"}), def, &copy),
                      "node c needed");
  n->add_input("d:1");

  n = def.add_node();
  n->set_name("c");
  n->add_input("d:1");

  n = def.add_node();
  n->set_name("d");

  // Graph is full, no pruning done.
  // Graph right now has diamond from d:
  //   d --> b --> a
  //   d --> c --> a
  TF_EXPECT_OK(PruneGraphDefInto(FetchesConfig({"a"}), def, &copy));
  EXPECT_EQ(def.DebugString(), copy.DebugString());
  GraphDef pruned_a = copy;

  // Add some unrelated fields that use b and c, but are not needed for a.
  n = def.add_node();
  n->set_name("e");
  n->add_input("^d");
  n->add_input("b:2");
  copy.Clear();
  TF_EXPECT_OK(PruneGraphDefInto(FetchesConfig({"a"}), def, &copy));
  EXPECT_EQ(pruned_a.DebugString(), copy.DebugString());

  // Fetch "a" and "e" to get the original graph.
  copy.Clear();
  TF_EXPECT_OK(PruneGraphDefInto(FetchesConfig({"a", "e"}), def, &copy));
  EXPECT_EQ(def.DebugString(), copy.DebugString());
}

TEST(SetNodeShardingFromNeighbors, Basic) {
  // Builds a graph that adds two Tensors.
  Scope scope = Scope::NewRootScope().ExitOnError();
  auto a = ops::_Arg(scope.WithOpName("A"), DT_INT32, 0);
  auto b = ops::_Arg(scope.WithOpName("B"), DT_INT32, 1);
  auto c = ops::Add(scope.WithOpName("C"), a, b);
  std::unique_ptr<Graph> graph(new Graph(OpRegistry::Global()));
  TF_ASSERT_OK(scope.ToGraph(graph.get()));

  Node* a_node = nullptr;
  Node* b_node = nullptr;
  Node* c_node = nullptr;
  for (Node* n : graph->nodes()) {
    if (n->name() == "A") a_node = n;
    if (n->name() == "B") b_node = n;
    if (n->name() == "C") c_node = n;
  }

  const int num_cores_per_replica = 4;

  a_node->set_assigned_device_name("foo");
  EXPECT_FALSE(SetNodeShardingFromNeighbors(c_node, /*out_edges=*/false).ok());

  // Test where one input to c_node has a device.
  a_node->set_assigned_device_name("/device:TPU_REPLICATED_CORE:2");
  TF_ASSERT_OK(SetNodeShardingFromNeighbors(c_node, /*out_edges=*/false));
  auto parse_status = ParseShardingFromDevice(*c_node, num_cores_per_replica);
  TF_ASSERT_OK(parse_status.status());
  ASSERT_TRUE(parse_status.ValueOrDie().has_value());
  EXPECT_EQ(2, parse_status.ValueOrDie().value().tile_assignment_devices(0));

  // Test where two inputs to c_node have a device.
  b_node->set_assigned_device_name("/device:TPU_REPLICATED_CORE:1");
  TF_ASSERT_OK(SetNodeShardingFromNeighbors(c_node, /*out_edges=*/false));
  parse_status = ParseShardingFromDevice(*c_node, num_cores_per_replica);
  TF_ASSERT_OK(parse_status.status());
  ASSERT_TRUE(parse_status.ValueOrDie().has_value());
  EXPECT_EQ(1, parse_status.ValueOrDie().value().tile_assignment_devices(0));

  // Test setting based on out edges.
  TF_ASSERT_OK(SetNodeShardingFromNeighbors(a_node, /*out_edges=*/true));
  parse_status = ParseShardingFromDevice(*a_node, num_cores_per_replica);
  TF_ASSERT_OK(parse_status.status());
  ASSERT_TRUE(parse_status.ValueOrDie().has_value());
  EXPECT_EQ(1, parse_status.ValueOrDie().value().tile_assignment_devices(0));
}

REGISTER_OP("One")
    .Output("y: T")
    .Attr("T: {float, double, int32, int64}")
    .Doc(R"doc(
Returns a tensor with a single element (1) of type T.

y: A scalar in type T.

)doc");

// Tests that CachedFunctionHandles class works.
TEST(CachedFunctionHandles, Basic) {
  FunctionDef func = FunctionDefHelper::Define(
      // Name
      "TestFunc",
      // Args
      {},
      // Return values
      {"y:T"},
      // Attr def
      {"T:{float, double, int32, int64}"},
      // Nodes
      {
          {{"y"}, "One", {}, {{"T", "$T"}}},
      });
  FunctionDefLibrary proto;
  *proto.add_function() = func;
  FunctionLibraryDefinition fld(OpRegistry::Global(), proto);
  std::unique_ptr<ProcessFunctionLibraryRuntime> pflr(
      new ProcessFunctionLibraryRuntime(
          /*device_mgr=*/nullptr, Env::Default(), TF_GRAPH_DEF_VERSION, &fld,
          OptimizerOptions()));
  FunctionLibraryRuntime* flr =
      pflr->GetFLR(ProcessFunctionLibraryRuntime::kDefaultFLRDevice);

  CachedFunctionHandles cached_function_handles(flr);

  // Tests that GetOrInstantiate() works.
  FunctionLibraryRuntime::Handle first_handle;
  AttrValue attr;
  attr.set_type(DT_FLOAT);
  AttrValueMap attrs;
  attrs["T"] = attr;
  TF_ASSERT_OK(cached_function_handles.GetOrInstantiate(
      "TestFunc", AttrSlice(&attrs), &first_handle));

  // Tests that we can get FunctionBody.
  const FunctionBody* body = flr->GetFunctionBody(first_handle);
  EXPECT_NE(body, nullptr);

  // Tests that GetOrInstantiate() returns cached handle when called with same
  // function name and attributes.
  FunctionLibraryRuntime::Handle second_handle;
  TF_ASSERT_OK(cached_function_handles.GetOrInstantiate(
      "TestFunc", AttrSlice(&attrs), &second_handle));
  EXPECT_EQ(first_handle, second_handle);

  // Tests that GetOrInstantiate() returns new handle when called with same
  // function name but different attributes.
  attr.set_type(DT_INT32);
  attrs["T"] = attr;
  FunctionLibraryRuntime::Handle third_handle;
  TF_ASSERT_OK(cached_function_handles.GetOrInstantiate(
      "TestFunc", AttrSlice(&attrs), &third_handle));
  EXPECT_NE(first_handle, third_handle);

  // Tests that ReleaseAllHandles() works.
  TF_EXPECT_OK(cached_function_handles.ReleaseAllHandles());
}

}  // namespace
}  // namespace tensorflow
