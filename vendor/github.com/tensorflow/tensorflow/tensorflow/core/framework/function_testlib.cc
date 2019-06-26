/* Copyright 2015 The TensorFlow Authors. All Rights Reserved.

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

#include "tensorflow/core/framework/function_testlib.h"

#include "tensorflow/core/framework/function.h"
#include "tensorflow/core/framework/node_def.pb.h"
#include "tensorflow/core/framework/tensor_testutil.h"
#include "tensorflow/core/framework/versions.pb.h"
#include "tensorflow/core/lib/core/threadpool.h"
#include "tensorflow/core/public/version.h"

namespace tensorflow {
namespace test {
namespace function {

typedef FunctionDefHelper FDH;

GraphDef GDef(gtl::ArraySlice<NodeDef> nodes,
              gtl::ArraySlice<FunctionDef> funcs) {
  GraphDef g;
  VersionDef* versions = g.mutable_versions();
  versions->set_producer(TF_GRAPH_DEF_VERSION);
  versions->set_min_consumer(TF_GRAPH_DEF_VERSION_MIN_CONSUMER);
  for (const auto& n : nodes) {
    *(g.add_node()) = n;
  }
  auto lib = g.mutable_library();
  for (const auto& f : funcs) {
    *(lib->add_function()) = f;
  }
  return g;
}

// Helper to construct a NodeDef.
NodeDef NDef(StringPiece name, StringPiece op, gtl::ArraySlice<string> inputs,
             gtl::ArraySlice<std::pair<string, FDH::AttrValueWrapper>> attrs,
             const string& device) {
  NodeDef n;
  n.set_name(string(name));
  n.set_op(string(op));
  for (const auto& in : inputs) n.add_input(in);
  n.set_device(device);
  for (auto na : attrs) n.mutable_attr()->insert({na.first, na.second.proto});
  return n;
}

FunctionDef NonZero() {
  return FDH::Define(
      // Name
      "NonZero",
      // Args
      {"x:T"},
      // Return values
      {"y:T"},
      // Attr def
      {"T:{float, double, int32, int64, string}"},
      // Nodes
      {
          {{"y"}, "Identity", {"x"}, {{"T", "$T"}}},
      });
}

FunctionDef IsZero() {
  const Tensor kZero = test::AsScalar<int64>(0);
  return FDH::Define(
      // Name
      "IsZero",
      // Args
      {"x: T"},
      // Return values
      {"equal: T"},
      // Attr def
      {"T:{float, double, int32, int64, string}"},
      {
          {{"zero"}, "Const", {}, {{"value", kZero}, {"dtype", DT_INT64}}},
          {{"cast"}, "Cast", {"zero"}, {{"SrcT", DT_INT64}, {"DstT", "$T"}}},
          {{"equal"}, "Equal", {"x", "cast"}, {{"T", "$T"}}},
      });
}

FunctionDef RandomUniform() {
  const Tensor kZero = test::AsScalar<int64>(0);

  return FDH::Define(
      // Name
      "RandomUniform",
      // Args
      {"x: T"},
      // Return values
      {"random_uniform: int64"},
      // Attr def
      {"T:{float, double, int32, int64, string}"},
      {{{"random_uniform/shape"},
        "Const",
        {},
        {{"value", kZero}, {"dtype", DT_INT64}}},
       {{"random_uniform"},
        "RandomUniform",
        {"random_uniform/shape"},
        {{"T", DT_INT32},
         {"Tout", DT_FLOAT},
         {"seed", 87654321},
         {"seed2", 42}}}});
}

FunctionDef XTimesTwo() {
  const Tensor kTwo = test::AsScalar<int64>(2);
  return FDH::Define(
      // Name
      "XTimesTwo",
      // Args
      {"x: T"},
      // Return values
      {"y: T"},
      // Attr def
      {"T: {float, double, int32, int64}"},
      // Nodes
      {
          {{"two"}, "Const", {}, {{"value", kTwo}, {"dtype", DT_INT64}}},
          {{"scale"}, "Cast", {"two"}, {{"SrcT", DT_INT64}, {"DstT", "$T"}}},
          {{"y"}, "Mul", {"x", "scale"}, {{"T", "$T"}}},
      });
}

FunctionDef XAddX() {
  return FDH::Define(
      // Name
      "XAddX",
      // Args
      {"x: T"},
      // Return values
      {"y: T"},
      // Attr def
      {"T: {float, double, int32, int64}"},
      // Nodes
      {
          {{"y"}, "Add", {"x", "x"}, {{"T", "$T"}}},
      });
}

FunctionDef XTimesTwoInt32() {
  const Tensor kTwo = test::AsScalar<int64>(2);
  return FDH::Define(
      // Name
      "XTimesTwoInt32",
      // Args
      {"x: int32"},
      // Return values
      {"y: int32"}, {},
      // Nodes
      {
          {{"two"}, "Const", {}, {{"value", kTwo}, {"dtype", DT_INT64}}},
          {{"scale"},
           "Cast",
           {"two"},
           {{"SrcT", DT_INT64}, {"DstT", DT_INT32}}},
          {{"y"}, "Mul", {"x", "scale"}, {{"T", DT_INT32}}},
      });
}

FunctionDef XTimesFour() {
  return FDH::Create(
      // Name
      "XTimesFour",
      // Args
      {"x: T"},
      // Return values
      {"y: T"},
      // Attr def
      {"T: {float, double, int32, int64}"},
      // Nodes
      {
          {{"x2"}, "XTimesTwo", {"x"}, {{"T", "$T"}}},
          {{"y"}, "XTimesTwo", {"x2:y:0"}, {{"T", "$T"}}},
      },
      {{"y", "y:y:0"}});
}

FunctionDef XTimes16() {
  return FDH::Create(
      // Name
      "XTimes16",
      // Args
      {"x: T"},
      // Return values
      {"y: T"},
      // Attr def
      {"T: {float, double, int32, int64}"},
      // Nodes
      {
          {{"x4"}, "XTimesFour", {"x"}, {{"T", "$T"}}},
          {{"y"}, "XTimesFour", {"x4:y:0"}, {{"T", "$T"}}},
      },
      {{"y", "y:y:0"}});
}

FunctionDef WXPlusB() {
  return FDH::Define(
      // Name
      "WXPlusB",
      // Args
      {"w: T", "x: T", "b: T"},
      // Return values
      {"y: T"},
      // Attr def
      {"T: {float, double}"},
      // Nodes
      {{{"mm"},
        "MatMul",
        {"w", "x"},
        {{"T", "$T"},
         {"transpose_a", false},
         {"transpose_b", false},
         {"_kernel", "eigen"}}},
       {{"y"}, "Add", {"mm", "b"}, {{"T", "$T"}}}});
}

FunctionDef Swap() {
  return FDH::Define(
      // Name
      "Swap",
      // Args
      {"i0: T", "i1: T"},
      // Return values
      {"o0: T", "o1: T"},
      // Attr def
      {"T: {float, double}"},
      // Nodes
      {{{"o0"}, "Identity", {"i1"}, {{"T", "$T"}}},
       {{"o1"}, "Identity", {"i0"}, {{"T", "$T"}}}});
}

FunctionDef InvalidControlFlow() {
  return FDH::Create(
      // Name
      "InvalidControlFlow",
      // Args
      {"i: int32"},
      // Return values
      {"o: int32"},
      // Attr def
      {},
      // Nodes
      {{{"enter"}, "Enter", {"i"}, {{"T", DT_INT32}, {"frame_name", "while"}}},
       {{"add"}, "Add", {"enter:output", "i"}, {{"T", DT_INT32}}}},
      // Output mapping
      {{"o", "add:z"}});
}

FunctionDef LessThanOrEqualToN(int64 N) {
  const Tensor kN = test::AsScalar<int64>(N);
  return FDH::Define(
      // Name
      "LessThanOrEqualToN",
      // Args
      {"x: T"},
      // Return values
      {"z: bool"},
      // Attr def
      {"T: {float, double, int32, int64}"},
      // Nodes
      {
          {{"N"}, "Const", {}, {{"value", kN}, {"dtype", DT_INT64}}},
          {{"y"}, "Cast", {"N"}, {{"SrcT", DT_INT64}, {"DstT", "$T"}}},
          {{"z"}, "LessEqual", {"x", "y"}, {{"T", "$T"}}},
      });
}

FunctionDef XPlusOneXTimesY() {
  const Tensor kOne = test::AsScalar<int64>(1);
  return FDH::Define(
      // Name
      "XPlusOneXTimesY",
      // Args
      {"x: T", "y: T"},
      // Return values
      {"s: T", "t: T"},
      // Attr def
      {"T: {float, double, int32, int64}"},
      // Nodes
      {{{"one"}, "Const", {}, {{"value", kOne}, {"dtype", DT_INT64}}},
       {{"increment"}, "Cast", {"one"}, {{"SrcT", DT_INT64}, {"DstT", "$T"}}},
       {{"s"}, "Add", {"x", "increment"}, {{"T", "$T"}}},
       {{"t"}, "Mul", {"x", "y"}, {{"T", "$T"}}}});
}

FunctionDef XYXLessThanOrEqualToN(int64 N) {
  const Tensor kN = test::AsScalar<int64>(N);
  return FDH::Define(
      // Name
      "XYXLessThanOrEqualToN",
      // Args
      {"x: T", "y: T"},
      // Return values
      {"z: bool"},
      // Attr def
      {"T: {float, double, int32, int64}"},
      // Nodes
      {
          {{"N"}, "Const", {}, {{"value", kN}, {"dtype", DT_INT64}}},
          {{"N1"}, "Cast", {"N"}, {{"SrcT", DT_INT64}, {"DstT", "$T"}}},
          {{"z"}, "LessEqual", {"x", "N1"}, {{"T", "$T"}}},
      });
}

void FunctionTestSchedClosure(std::function<void()> fn) {
  static thread::ThreadPool* w =
      new thread::ThreadPool(Env::Default(), "Test", 8);
  w->Schedule(std::move(fn));
}

}  // end namespace function
}  // end namespace test
}  // end namespace tensorflow
