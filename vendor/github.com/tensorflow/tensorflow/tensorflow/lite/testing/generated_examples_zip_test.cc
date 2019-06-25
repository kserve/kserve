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
#include <cstdarg>
#include <cstdio>
#include <cstdlib>
#include <fstream>
#include <map>
#include <sstream>
#include <gtest/gtest.h>
#include "re2/re2.h"
#include "tensorflow/lite/testing/parse_testdata.h"
#include "tensorflow/lite/testing/tflite_driver.h"
#include "tensorflow/lite/testing/util.h"
#include "tensorflow/core/lib/core/status_test_util.h"
#include "tensorflow/core/platform/env.h"
#include "tensorflow/core/platform/subprocess.h"
#include "tensorflow/core/util/command_line_flags.h"

namespace tflite {
namespace testing {

namespace {
bool FLAGS_ignore_known_bugs = true;
// As archive file names are test-specific, no default is possible.
//
// This test supports input as both zip and tar, as a stock android image does
// not have unzip but does have tar.
string* FLAGS_zip_file_path = new string;
string* FLAGS_tar_file_path = new string;
#ifndef __ANDROID__
string* FLAGS_unzip_binary_path = new string("/usr/bin/unzip");
string* FLAGS_tar_binary_path = new string("/bin/tar");
#else
string* FLAGS_unzip_binary_path = new string("/system/bin/unzip");
string* FLAGS_tar_binary_path = new string("/system/bin/tar");
#endif
bool FLAGS_use_nnapi = false;
bool FLAGS_ignore_unsupported_nnapi = false;
}  // namespace

// TensorFlow system environment for file system called.
tensorflow::Env* env = tensorflow::Env::Default();

// List of tests that are expected to fail when
//   --test_arg=--ignore_known_bugs=false
// Key is a substring of the test name and value is a bug number.
// TODO(ahentz): make sure we clean this list up frequently.
std::map<string, string> kBrokenTests = {
    // L2Norm only supports tensors with 4D or fewer.
    {R"(^\/l2norm_dim=.*,epsilon=.*,input_shape=\[.,.,.,.,.*\])", "67963684"},

    // SpaceToBatchND only supports 4D tensors.
    {R"(^\/space_to_batch_nd.*input_shape=\[1,4,4,4,1,1\])", "70848787"},

    // L2Norm only works for dim=-1.
    {R"(^\/l2norm_dim=-2,epsilon=.*,input_shape=\[.,.\])", "67963812"},
    {R"(^\/l2norm_dim=0,epsilon=.*,input_shape=\[.,.\])", "67963812"},
    {R"(^\/l2norm_dim=-2,epsilon=.*,input_shape=\[3,15,14,3\])", "67963812"},
    {R"(^\/l2norm_dim=-2,epsilon=.*,input_shape=\[1,3,4,3\])", "67963812"},
    {R"(^\/l2norm_dim=2,epsilon=.*,input_shape=\[3,15,14,3\])", "67963812"},
    {R"(^\/l2norm_dim=2,epsilon=.*,input_shape=\[1,3,4,3\])", "67963812"},
    {R"(^\/l2norm_dim=0,epsilon=.*,input_shape=\[3,15,14,3\])", "67963812"},
    {R"(^\/l2norm_dim=0,epsilon=.*,input_shape=\[1,3,4,3\])", "67963812"},
    {R"(^\/l2norm_dim=1,epsilon=.*,input_shape=\[3,15,14,3\])", "67963812"},
    {R"(^\/l2norm_dim=1,epsilon=.*,input_shape=\[1,3,4,3\])", "67963812"},
    {R"(^\/l2norm_dim=\[2,3\],epsilon=.*,input_shape=\[3,15,14,3\])",
     "67963812"},
    {R"(^\/l2norm_dim=\[2,3\],epsilon=.*,input_shape=\[1,3,4,3\])", "67963812"},

    // ResizeBilinear looks completely incompatible with Tensorflow
    {R"(^\/resize_bilinear.*dtype=tf.int32)", "72401107"},

    // Transpose only supports 1D-4D input tensors.
    {R"(^\/transpose.*input_shape=\[.,.,.,.,.\])", "71545879"},

    // No Support for float.
    {R"(^\/floor_div.*dtype=tf\.float32)", "112859002"},

    // Relu does not support int32.
    // These test cases appends a Relu after the tested ops when
    // activation=True. The tests are failing since Relu doesn't support int32.
    {R"(^\/div.*activation=True.*dtype=tf\.int32)", "112968789"},
    {R"(^\/floor_div.*activation=True.*dtype=tf\.int32)", "112968789"},
    {R"(^\/floor_mod.*activation=True.*dtype=tf\.int32)", "112968789"},
    {R"(^\/floor_mod.*activation=True.*dtype=tf\.int64)", "112968789"},

    {R"(^\/sub.*dtype=tf\.int64)", "119126484"},
    {R"(^\/div.*dtype=tf\.int64)", "119126484"},
    {R"(^\/mul.*dtype=tf\.int64)", "119126484"},
    {R"(^\/add.*dtype=tf\.int64)", "119126484"},
    {R"(^\/floor_div.*dtype=tf\.int64)", "119126484"},
    {R"(^\/squared_difference.*dtype=tf\.int64)", "119126484"},

    // Strided Slice chooses the wrong dimension.
    {R"(^\/strided_slice_buggy)", "119786029"},
};

// Allows test data to be unarchived into a temporary directory and makes
// sure those temporary directories are removed later.
class ArchiveEnvironment : public ::testing::Environment {
 public:
  ~ArchiveEnvironment() override {}

  // Delete all temporary directories on teardown.
  void TearDown() override {
    for (const auto& dir : temporary_directories_) {
      tensorflow::int64 undeleted_dirs, undeleted_files;
      TF_CHECK_OK(
          env->DeleteRecursively(dir, &undeleted_dirs, &undeleted_files));
    }
    temporary_directories_.clear();
  }

  // Unarchive `archive` file into a new temporary directory  `out_dir`.
  tensorflow::Status UnArchive(const string& zip, const string& tar,
                               string* out_dir) {
    string dir;
    TF_CHECK_OK(MakeTemporaryDirectory(&dir));
    tensorflow::SubProcess proc;
    if (!zip.empty()) {
      string unzip_binary = *FLAGS_unzip_binary_path;
      TF_CHECK_OK(env->FileExists(unzip_binary));
      TF_CHECK_OK(env->FileExists(zip));
      proc.SetProgram(unzip_binary, {"unzip", "-d", dir, zip});
    } else {
      string tar_binary = *FLAGS_tar_binary_path;
      TF_CHECK_OK(env->FileExists(tar_binary));
      TF_CHECK_OK(env->FileExists(tar));
      // 'o' needs to be explicitly set on Android so that
      // untarring works as non-root (otherwise tries to chown
      // files, which fails)
      proc.SetProgram(tar_binary, {"tar", "xfo", tar, "-C", dir});
    }
    proc.SetChannelAction(tensorflow::CHAN_STDOUT, tensorflow::ACTION_PIPE);
    proc.SetChannelAction(tensorflow::CHAN_STDERR, tensorflow::ACTION_PIPE);
    if (!proc.Start())
      return tensorflow::Status(tensorflow::error::UNKNOWN,
                                "unzip couldn't start");
    string out, err;
    int status = proc.Communicate(nullptr, &out, &err);
    if (WEXITSTATUS(status) == 0) {
      *out_dir = dir;
      return tensorflow::Status::OK();
    } else {
      return tensorflow::Status(tensorflow::error::UNKNOWN,
                                "unzip failed. "
                                "stdout:\n" +
                                    out + "\nstderr:\n" + err);
    }
  }

 private:
  // Make a temporary directory and return its name in `temporary`.
  tensorflow::Status MakeTemporaryDirectory(string* temporary) {
    if (env->LocalTempFilename(temporary)) {
      TF_CHECK_OK(env->CreateDir(*temporary));
      temporary_directories_.push_back(*temporary);
      return tensorflow::Status::OK();
    }
    return tensorflow::Status(tensorflow::error::UNKNOWN,
                              "make temporary directory failed");
  }

  std::vector<string> temporary_directories_;
};

// Return the singleton archive_environment.
ArchiveEnvironment* archive_environment() {
  static ArchiveEnvironment* env = new ArchiveEnvironment;
  return env;
}

// Read the manifest.txt out of the unarchived archive file. Specifically
// `original_file` is the original zip file for error messages. `dir` is
// the temporary directory where the archive file has been unarchived and
// `test_paths` is the list of test prefixes that were in the manifest.
// Note, it is an error for a manifest to contain no tests.
tensorflow::Status ReadManifest(const string& original_file, const string& dir,
                                std::vector<string>* test_paths) {
  // Read the newline delimited list of entries in the manifest.
  std::ifstream manifest_fp(dir + "/manifest.txt");
  string manifest((std::istreambuf_iterator<char>(manifest_fp)),
                  std::istreambuf_iterator<char>());
  size_t pos = 0;
  int added = 0;
  while (true) {
    size_t end_pos = manifest.find("\n", pos);
    if (end_pos == string::npos) break;
    string filename = manifest.substr(pos, end_pos - pos);
    test_paths->push_back(dir + "/" + filename);
    pos = end_pos + 1;
    added += 1;
  }
  if (!added) {
    string message = "Test had no examples: " + original_file;
    return tensorflow::Status(tensorflow::error::UNKNOWN, message);
  }
  return tensorflow::Status::OK();
}

// Get a list of tests from either zip or tar file
std::vector<string> UnarchiveAndFindTestNames(const string& zip_file,
                                              const string& tar_file) {
  if (zip_file.empty() && tar_file.empty()) {
    TF_CHECK_OK(tensorflow::Status(tensorflow::error::UNKNOWN,
                                   "Neither zip_file nor tar_file was given"));
  }
  string decompress_tmp_dir;
  TF_CHECK_OK(archive_environment()->UnArchive(zip_file, tar_file,
                                               &decompress_tmp_dir));
  std::vector<string> stuff;
  if (!zip_file.empty()) {
    TF_CHECK_OK(ReadManifest(zip_file, decompress_tmp_dir, &stuff));
  } else {
    TF_CHECK_OK(ReadManifest(tar_file, decompress_tmp_dir, &stuff));
  }
  return stuff;
}

class OpsTest : public ::testing::TestWithParam<string> {};

TEST_P(OpsTest, RunZipTests) {
  string test_path = GetParam();
  string tflite_test_case = test_path + "_tests.txt";
  string tflite_dir = test_path.substr(0, test_path.find_last_of("/"));
  string test_name = test_path.substr(test_path.find_last_of('/'));

  std::ifstream tflite_stream(tflite_test_case);
  ASSERT_TRUE(tflite_stream.is_open()) << tflite_test_case;
  tflite::testing::TfLiteDriver test_driver(FLAGS_use_nnapi);
  test_driver.SetModelBaseDir(tflite_dir);

  string bug_number;
  for (const auto& p : kBrokenTests) {
    if (RE2::PartialMatch(test_name, p.first)) {
      bug_number = p.second;
    }
  }

  bool result = tflite::testing::ParseAndRunTests(&tflite_stream, &test_driver);
  string message = test_driver.GetErrorMessage();
  if (bug_number.empty()) {
    if (FLAGS_use_nnapi && FLAGS_ignore_unsupported_nnapi && !result) {
      EXPECT_EQ(message, string("Failed to invoke interpreter")) << message;
    } else {
      EXPECT_TRUE(result) << message;
    }
  } else {
    if (FLAGS_ignore_known_bugs) {
      EXPECT_FALSE(result) << "Test was expected to fail but is now passing; "
                              "you can mark http://b/"
                           << bug_number << " as fixed! Yay!";
    } else {
      EXPECT_TRUE(result) << message << ": Possibly due to http://b/"
                          << bug_number;
    }
  }
}

struct ZipPathParamName {
  template <class ParamType>
  string operator()(const ::testing::TestParamInfo<ParamType>& info) const {
    string param_name = info.param;
    size_t last_slash = param_name.find_last_of("\\/");
    if (last_slash != string::npos) {
      param_name = param_name.substr(last_slash);
    }
    for (size_t index = 0; index < param_name.size(); ++index) {
      if (!isalnum(param_name[index]) && param_name[index] != '_')
        param_name[index] = '_';
    }
    return param_name;
  }
};

INSTANTIATE_TEST_CASE_P(tests, OpsTest,
                        ::testing::ValuesIn(UnarchiveAndFindTestNames(
                            *FLAGS_zip_file_path, *FLAGS_tar_file_path)),
                        ZipPathParamName());

}  // namespace testing
}  // namespace tflite

int main(int argc, char** argv) {
  ::testing::AddGlobalTestEnvironment(tflite::testing::archive_environment());

  std::vector<tensorflow::Flag> flags = {
      tensorflow::Flag(
          "ignore_known_bugs", &tflite::testing::FLAGS_ignore_known_bugs,
          "If a particular model is affected by a known bug, the "
          "corresponding test should expect the outputs to not match."),
      tensorflow::Flag(
          "tar_file_path", tflite::testing::FLAGS_tar_file_path,
          "Required (or zip_file_path): Location of the test tar file."),
      tensorflow::Flag(
          "zip_file_path", tflite::testing::FLAGS_zip_file_path,
          "Required (or tar_file_path): Location of the test zip file."),
      tensorflow::Flag("unzip_binary_path",
                       tflite::testing::FLAGS_unzip_binary_path,
                       "Location of a suitable unzip binary."),
      tensorflow::Flag("tar_binary_path",
                       tflite::testing::FLAGS_tar_binary_path,
                       "Location of a suitable tar binary."),
      tensorflow::Flag("use_nnapi", &tflite::testing::FLAGS_use_nnapi,
                       "Whether to enable the NNAPI delegate"),
      tensorflow::Flag("ignore_unsupported_nnapi",
                       &tflite::testing::FLAGS_ignore_unsupported_nnapi,
                       "Don't fail tests just because delegation to NNAPI "
                       "is not possible")};
  bool success = tensorflow::Flags::Parse(&argc, argv, flags);
  if (!success || (argc == 2 && !strcmp(argv[1], "--helpfull"))) {
    fprintf(stderr, "%s", tensorflow::Flags::Usage(argv[0], flags).c_str());
    return 1;
  }

  ::tflite::LogToStderr();
  // TODO(mikie): googletest arguments do not work - maybe the tensorflow flags
  // parser removes them?
  ::testing::InitGoogleTest(&argc, argv);
  return RUN_ALL_TESTS();
}
