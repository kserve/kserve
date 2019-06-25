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

#include "tensorflow/compiler/aot/codegen.h"

#include <string>
#include <vector>

#include "absl/strings/match.h"
#include "absl/strings/string_view.h"
#include "llvm/Support/TargetSelect.h"
#include "tensorflow/compiler/xla/shape_util.h"
#include "tensorflow/core/lib/core/status.h"
#include "tensorflow/core/lib/core/status_test_util.h"
#include "tensorflow/core/lib/io/path.h"
#include "tensorflow/core/platform/env.h"
#include "tensorflow/core/platform/test.h"

namespace tensorflow {
namespace tfcompile {
namespace {

using ::tensorflow::cpu_function_runtime::BufferInfo;

void ExpectErrorContains(const Status& status, absl::string_view str) {
  EXPECT_NE(Status::OK(), status);
  EXPECT_TRUE(absl::StrContains(status.error_message(), str))
      << "expected error: " << status.error_message() << " to contain: " << str;
}

TEST(ValidateCppIdent, Simple) {
  TF_EXPECT_OK(ValidateCppIdent("a", ""));
  TF_EXPECT_OK(ValidateCppIdent("abc", ""));
  TF_EXPECT_OK(ValidateCppIdent("_abc", ""));
  TF_EXPECT_OK(ValidateCppIdent("_abc123", ""));
  // Make sure we didn't skip a valid letter or digit
  string ident;
  for (char c = 'a'; c <= 'z'; c++) {
    ident.append(1, c);
  }
  for (char c = 'A'; c <= 'Z'; c++) {
    ident.append(1, c);
  }
  for (char c = '0'; c <= '9'; c++) {
    ident.append(1, c);
  }
  ident += "_";
  TF_EXPECT_OK(ValidateCppIdent(ident, ""));

  ExpectErrorContains(ValidateCppIdent("", ""), "empty identifier");
  ExpectErrorContains(ValidateCppIdent(" ", ""), "illegal leading char");
  ExpectErrorContains(ValidateCppIdent("0", ""), "illegal leading char");
  ExpectErrorContains(ValidateCppIdent(".", ""), "illegal leading char");
  ExpectErrorContains(ValidateCppIdent(":", ""), "illegal leading char");
  ExpectErrorContains(ValidateCppIdent("a.", ""), "illegal char");
  ExpectErrorContains(ValidateCppIdent("a:", ""), "illegal char");
  ExpectErrorContains(ValidateCppIdent("a:", ""), "illegal char");
}

class ParseCppClassTest : public ::testing::Test {
 protected:
  void ExpectOK(const string& cpp_class, const string& want_class_name,
                const std::vector<string>& want_namespaces) {
    string class_name;
    std::vector<string> namespaces;
    TF_EXPECT_OK(ParseCppClass(cpp_class, &class_name, &namespaces));
    EXPECT_EQ(class_name, want_class_name);
    EXPECT_EQ(namespaces, want_namespaces);
  }

  void ExpectFail(const string& cpp_class) {
    string class_name;
    std::vector<string> namespaces;
    EXPECT_NE(ParseCppClass(cpp_class, &class_name, &namespaces), Status::OK());
  }
};

TEST_F(ParseCppClassTest, ParseOK) {
  ExpectOK("MyClass", "MyClass", {});
  ExpectOK("_MyClass", "_MyClass", {});
  ExpectOK("a::MyClass", "MyClass", {"a"});
  ExpectOK("a::foo::MyClass", "MyClass", {"a", "foo"});
  ExpectOK("a::foo::b::MyClass", "MyClass", {"a", "foo", "b"});
  ExpectOK("a::foo::b::bar::MyClass", "MyClass", {"a", "foo", "b", "bar"});
  ExpectOK("foo::MyClass", "MyClass", {"foo"});
  ExpectOK("_foo::MyClass", "MyClass", {"_foo"});
  ExpectOK("_foo::_MyClass", "_MyClass", {"_foo"});
  // Make sure we didn't skip a valid letter or digit
  string ident;
  for (char c = 'a'; c <= 'z'; c++) {
    ident.append(1, c);
  }
  for (char c = 'A'; c <= 'Z'; c++) {
    ident.append(1, c);
  }
  for (char c = '0'; c <= '9'; c++) {
    ident.append(1, c);
  }
  ident += "_";
  ExpectOK(ident, ident, {});
  ExpectOK(ident + "::" + ident, ident, {ident});
  ExpectOK(ident + "::" + ident + "::" + ident, ident, {ident, ident});
}

TEST_F(ParseCppClassTest, ParseFail) {
  ExpectFail("");
  ExpectFail("::");
  ExpectFail("::MyClass");  // valid C++, but disallowed for simpler code.
  ExpectFail("0");
  ExpectFail("a.b");
  ExpectFail("a:b");
  ExpectFail("good::.bad");
  ExpectFail("good:::bad");
  ExpectFail("good:: bad");
  ExpectFail("good::0bad");
}

static void CompareWithGoldenFile(
    const string& tensorflow_relative_golden_file_name,
    const string& expected_contents) {
  // To update the golden file, flip update_golden to true and run the
  // following:
  // bazel test --test_strategy=local \
  //   third_party/tensorflow/compiler/aot:codegen_test
  const bool update_golden = false;
  const string golden_file_name = io::JoinPath(
      testing::TensorFlowSrcRoot(), tensorflow_relative_golden_file_name);

  if (update_golden) {
    TF_EXPECT_OK(
        WriteStringToFile(Env::Default(), golden_file_name, expected_contents));
  }

  string golden_file_contents;
  TF_ASSERT_OK(ReadFileToString(Env::Default(), golden_file_name,
                                &golden_file_contents));
  EXPECT_EQ(golden_file_contents, expected_contents);
}

TEST(CodegenTest, Golden) {
  // Normally CpuCompiler::CpuCompiler does this, but in this test we've
  // bypassed the Cpu compiler so we have to do this manually.
  llvm::InitializeNativeTarget();
  llvm::InitializeNativeTargetAsmPrinter();
  LLVMInitializeX86Target();
  LLVMInitializeX86TargetMC();

  CodegenOpts opts;
  opts.class_name = "MyClass";
  opts.target_triple = "x86_64-pc-linux";
  opts.namespaces = {"foo", "bar"};
  opts.gen_name_to_index = true;
  opts.gen_program_shape = true;
  tf2xla::Config config;
  tf2xla::Feed* feed = config.add_feed();
  feed->mutable_id()->set_node_name("feed0");
  feed->set_name("myfeed");
  feed = config.add_feed();
  feed->mutable_id()->set_node_name("feed1");
  tf2xla::Fetch* fetch = config.add_fetch();
  fetch->mutable_id()->set_node_name("fetch0");
  fetch->set_name("myfetch");
  CompileResult compile_result;
  compile_result.aot.reset(new xla::cpu::CpuAotCompilationResult(
      {},
      {BufferInfo::MakeTempBuffer(1),
       BufferInfo::MakeEntryParameter(/*size=*/8, /*param_number=*/0),
       BufferInfo::MakeTempBuffer(2),
       BufferInfo::MakeEntryParameter(/*size=*/96, /*param_number=*/1),
       BufferInfo::MakeTempBuffer(3), BufferInfo::MakeTempBuffer(120)},
      5, {}));
  compile_result.program_shape =
      xla::ShapeUtil::MakeProgramShape(
          {
              xla::ShapeUtil::MakeShape(xla::F32, {1, 2}),
              xla::ShapeUtil::MakeShape(xla::S64, {3, 4}),
          },
          xla::ShapeUtil::MakeTupleShape(
              {xla::ShapeUtil::MakeShape(xla::U32, {5, 6})}))
          .ToProto();
  compile_result.entry_point = "entry_point";
  compile_result.pointer_size = 8;

  MetadataResult metadata_result;
  TF_ASSERT_OK(GenerateMetadata(opts, compile_result, &metadata_result));

  // The other fields in metadata_result are tested as part of the generated
  // header test.

  CompareWithGoldenFile("compiler/aot/codegen_test_o.golden",
                        metadata_result.object_file_data);

  string header;
  TF_ASSERT_OK(
      GenerateHeader(opts, config, compile_result, metadata_result, &header));

  CompareWithGoldenFile("compiler/aot/codegen_test_h.golden", header);
}
}  // namespace
}  // namespace tfcompile
}  // namespace tensorflow
