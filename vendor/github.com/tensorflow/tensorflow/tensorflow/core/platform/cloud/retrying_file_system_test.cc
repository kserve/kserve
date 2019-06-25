/* Copyright 2016 The TensorFlow Authors. All Rights Reserved.

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

#include "tensorflow/core/platform/cloud/retrying_file_system.h"
#include <fstream>
#include "tensorflow/core/lib/core/status_test_util.h"
#include "tensorflow/core/lib/strings/str_util.h"
#include "tensorflow/core/platform/test.h"

namespace tensorflow {
namespace {

typedef std::vector<std::tuple<string, Status>> ExpectedCalls;

ExpectedCalls CreateRetriableErrors(const string& method, int n) {
  ExpectedCalls expected_calls;
  expected_calls.reserve(n);
  for (int i = 0; i < n; i++) {
    expected_calls.emplace_back(std::make_tuple(
        method, errors::Unavailable(strings::StrCat("Retriable error #", i))));
  }
  return expected_calls;
}

// A class to manage call expectations on mock implementations.
class MockCallSequence {
 public:
  explicit MockCallSequence(const ExpectedCalls& calls) : calls_(calls) {}

  ~MockCallSequence() {
    EXPECT_TRUE(calls_.empty())
        << "Not all expected calls have been made, "
        << "the next expected call: " << std::get<0>(calls_.front());
  }

  Status ConsumeNextCall(const string& method) {
    EXPECT_FALSE(calls_.empty()) << "No more calls were expected.";
    auto call = calls_.front();
    calls_.erase(calls_.begin());
    EXPECT_EQ(std::get<0>(call), method) << "Unexpected method called.";
    return std::get<1>(call);
  }

 private:
  ExpectedCalls calls_;
};

class MockRandomAccessFile : public RandomAccessFile {
 public:
  explicit MockRandomAccessFile(const ExpectedCalls& calls) : calls_(calls) {}
  Status Read(uint64 offset, size_t n, StringPiece* result,
              char* scratch) const override {
    return calls_.ConsumeNextCall("Read");
  }

 private:
  mutable MockCallSequence calls_;
};

class MockWritableFile : public WritableFile {
 public:
  explicit MockWritableFile(const ExpectedCalls& calls) : calls_(calls) {}
  Status Append(StringPiece data) override {
    return calls_.ConsumeNextCall("Append");
  }
  Status Close() override { return calls_.ConsumeNextCall("Close"); }
  Status Flush() override { return calls_.ConsumeNextCall("Flush"); }
  Status Sync() override { return calls_.ConsumeNextCall("Sync"); }

 private:
  mutable MockCallSequence calls_;
};

class MockFileSystem : public FileSystem {
 public:
  explicit MockFileSystem(const ExpectedCalls& calls, bool* flushed = nullptr)
      : calls_(calls), flushed_(flushed) {}

  Status NewRandomAccessFile(
      const string& fname, std::unique_ptr<RandomAccessFile>* result) override {
    *result = std::move(random_access_file_to_return);
    return calls_.ConsumeNextCall("NewRandomAccessFile");
  }

  Status NewWritableFile(const string& fname,
                         std::unique_ptr<WritableFile>* result) override {
    *result = std::move(writable_file_to_return);
    return calls_.ConsumeNextCall("NewWritableFile");
  }

  Status NewAppendableFile(const string& fname,
                           std::unique_ptr<WritableFile>* result) override {
    *result = std::move(writable_file_to_return);
    return calls_.ConsumeNextCall("NewAppendableFile");
  }

  Status NewReadOnlyMemoryRegionFromFile(
      const string& fname,
      std::unique_ptr<ReadOnlyMemoryRegion>* result) override {
    return calls_.ConsumeNextCall("NewReadOnlyMemoryRegionFromFile");
  }

  Status FileExists(const string& fname) override {
    return calls_.ConsumeNextCall("FileExists");
  }

  Status GetChildren(const string& dir, std::vector<string>* result) override {
    return calls_.ConsumeNextCall("GetChildren");
  }

  Status GetMatchingPaths(const string& dir,
                          std::vector<string>* result) override {
    return calls_.ConsumeNextCall("GetMatchingPaths");
  }

  Status Stat(const string& fname, FileStatistics* stat) override {
    return calls_.ConsumeNextCall("Stat");
  }

  Status DeleteFile(const string& fname) override {
    return calls_.ConsumeNextCall("DeleteFile");
  }

  Status CreateDir(const string& dirname) override {
    return calls_.ConsumeNextCall("CreateDir");
  }

  Status DeleteDir(const string& dirname) override {
    return calls_.ConsumeNextCall("DeleteDir");
  }

  Status GetFileSize(const string& fname, uint64* file_size) override {
    return calls_.ConsumeNextCall("GetFileSize");
  }

  Status RenameFile(const string& src, const string& target) override {
    return calls_.ConsumeNextCall("RenameFile");
  }

  Status IsDirectory(const string& dirname) override {
    return calls_.ConsumeNextCall("IsDirectory");
  }

  Status DeleteRecursively(const string& dirname, int64* undeleted_files,
                           int64* undeleted_dirs) override {
    return calls_.ConsumeNextCall("DeleteRecursively");
  }

  void FlushCaches() override {
    if (flushed_) {
      *flushed_ = true;
    }
  }

  std::unique_ptr<WritableFile> writable_file_to_return;
  std::unique_ptr<RandomAccessFile> random_access_file_to_return;

 private:
  MockCallSequence calls_;
  bool* flushed_ = nullptr;
};

TEST(RetryingFileSystemTest, NewRandomAccessFile_ImmediateSuccess) {
  // Configure the mock base random access file.
  ExpectedCalls expected_file_calls({std::make_tuple("Read", Status::OK())});
  std::unique_ptr<RandomAccessFile> base_file(
      new MockRandomAccessFile(expected_file_calls));

  // Configure the mock base file system.
  ExpectedCalls expected_fs_calls(
      {std::make_tuple("NewRandomAccessFile", Status::OK())});
  std::unique_ptr<MockFileSystem> base_fs(
      new MockFileSystem(expected_fs_calls));
  base_fs->random_access_file_to_return = std::move(base_file);
  RetryingFileSystem<MockFileSystem> fs(
      std::move(base_fs), RetryConfig(0 /* init_delay_time_us */));

  // Retrieve the wrapped random access file.
  std::unique_ptr<RandomAccessFile> random_access_file;
  TF_EXPECT_OK(fs.NewRandomAccessFile("filename.txt", &random_access_file));

  // Use it and check the results.
  StringPiece result;
  char scratch[10];
  TF_EXPECT_OK(random_access_file->Read(0, 10, &result, scratch));
}

TEST(RetryingFileSystemTest, NewRandomAccessFile_SuccessWith3rdTry) {
  // Configure the mock base random access file.
  ExpectedCalls expected_file_calls(
      {std::make_tuple("Read", errors::Unavailable("Something is wrong")),
       std::make_tuple("Read", errors::Unavailable("Wrong again")),
       std::make_tuple("Read", Status::OK())});
  std::unique_ptr<RandomAccessFile> base_file(
      new MockRandomAccessFile(expected_file_calls));

  // Configure the mock base file system.
  ExpectedCalls expected_fs_calls(
      {std::make_tuple("NewRandomAccessFile", Status::OK())});
  std::unique_ptr<MockFileSystem> base_fs(
      new MockFileSystem(expected_fs_calls));
  base_fs->random_access_file_to_return = std::move(base_file);
  RetryingFileSystem<MockFileSystem> fs(
      std::move(base_fs), RetryConfig(0 /* init_delay_time_us */));

  // Retrieve the wrapped random access file.
  std::unique_ptr<RandomAccessFile> random_access_file;
  TF_EXPECT_OK(fs.NewRandomAccessFile("filename.txt", &random_access_file));

  // Use it and check the results.
  StringPiece result;
  char scratch[10];
  TF_EXPECT_OK(random_access_file->Read(0, 10, &result, scratch));
}

TEST(RetryingFileSystemTest, NewRandomAccessFile_AllRetriesFailed) {
  // Configure the mock base random access file.
  ExpectedCalls expected_file_calls = CreateRetriableErrors("Read", 11);
  std::unique_ptr<RandomAccessFile> base_file(
      new MockRandomAccessFile(expected_file_calls));

  // Configure the mock base file system.
  ExpectedCalls expected_fs_calls(
      {std::make_tuple("NewRandomAccessFile", Status::OK())});
  std::unique_ptr<MockFileSystem> base_fs(
      new MockFileSystem(expected_fs_calls));
  base_fs->random_access_file_to_return = std::move(base_file);
  RetryingFileSystem<MockFileSystem> fs(
      std::move(base_fs), RetryConfig(0 /* init_delay_time_us */));

  // Retrieve the wrapped random access file.
  std::unique_ptr<RandomAccessFile> random_access_file;
  TF_EXPECT_OK(fs.NewRandomAccessFile("filename.txt", &random_access_file));

  // Use it and check the results.
  StringPiece result;
  char scratch[10];
  const auto& status = random_access_file->Read(0, 10, &result, scratch);
  EXPECT_TRUE(
      str_util::StrContains(status.error_message(), "Retriable error #10"))
      << status;
}

TEST(RetryingFileSystemTest, NewRandomAccessFile_NoRetriesForSomeErrors) {
  // Configure the mock base random access file.
  ExpectedCalls expected_file_calls({
      std::make_tuple("Read",
                      errors::FailedPrecondition("Failed precondition")),
  });
  std::unique_ptr<RandomAccessFile> base_file(
      new MockRandomAccessFile(expected_file_calls));

  // Configure the mock base file system.
  ExpectedCalls expected_fs_calls(
      {std::make_tuple("NewRandomAccessFile", Status::OK())});
  std::unique_ptr<MockFileSystem> base_fs(
      new MockFileSystem(expected_fs_calls));
  base_fs->random_access_file_to_return = std::move(base_file);
  RetryingFileSystem<MockFileSystem> fs(
      std::move(base_fs), RetryConfig(0 /* init_delay_time_us */));

  // Retrieve the wrapped random access file.
  std::unique_ptr<RandomAccessFile> random_access_file;
  TF_EXPECT_OK(fs.NewRandomAccessFile("filename.txt", &random_access_file));

  // Use it and check the results.
  StringPiece result;
  char scratch[10];
  EXPECT_EQ("Failed precondition",
            random_access_file->Read(0, 10, &result, scratch).error_message());
}

TEST(RetryingFileSystemTest, NewWritableFile_ImmediateSuccess) {
  // Configure the mock base random access file.
  ExpectedCalls expected_file_calls({std::make_tuple("Sync", Status::OK()),
                                     std::make_tuple("Close", Status::OK())});
  std::unique_ptr<WritableFile> base_file(
      new MockWritableFile(expected_file_calls));

  // Configure the mock base file system.
  ExpectedCalls expected_fs_calls(
      {std::make_tuple("NewWritableFile", Status::OK())});
  std::unique_ptr<MockFileSystem> base_fs(
      new MockFileSystem(expected_fs_calls));
  base_fs->writable_file_to_return = std::move(base_file);
  RetryingFileSystem<MockFileSystem> fs(
      std::move(base_fs), RetryConfig(0 /* init_delay_time_us */));

  // Retrieve the wrapped writable file.
  std::unique_ptr<WritableFile> writable_file;
  TF_EXPECT_OK(fs.NewWritableFile("filename.txt", &writable_file));

  // Use it and check the results.
  TF_EXPECT_OK(writable_file->Sync());
}

TEST(RetryingFileSystemTest, NewWritableFile_SuccessWith3rdTry) {
  // Configure the mock base random access file.
  ExpectedCalls expected_file_calls(
      {std::make_tuple("Sync", errors::Unavailable("Something is wrong")),
       std::make_tuple("Sync", errors::Unavailable("Something is wrong again")),
       std::make_tuple("Sync", Status::OK()),
       std::make_tuple("Close", Status::OK())});
  std::unique_ptr<WritableFile> base_file(
      new MockWritableFile(expected_file_calls));

  // Configure the mock base file system.
  ExpectedCalls expected_fs_calls(
      {std::make_tuple("NewWritableFile", Status::OK())});
  std::unique_ptr<MockFileSystem> base_fs(
      new MockFileSystem(expected_fs_calls));
  base_fs->writable_file_to_return = std::move(base_file);
  RetryingFileSystem<MockFileSystem> fs(
      std::move(base_fs), RetryConfig(0 /* init_delay_time_us */));

  // Retrieve the wrapped writable file.
  std::unique_ptr<WritableFile> writable_file;
  TF_EXPECT_OK(fs.NewWritableFile("filename.txt", &writable_file));

  // Use it and check the results.
  TF_EXPECT_OK(writable_file->Sync());
}

TEST(RetryingFileSystemTest, NewWritableFile_SuccessWith3rdTry_ViaDestructor) {
  // Configure the mock base random access file.
  ExpectedCalls expected_file_calls(
      {std::make_tuple("Close", errors::Unavailable("Something is wrong")),
       std::make_tuple("Close",
                       errors::Unavailable("Something is wrong again")),
       std::make_tuple("Close", Status::OK())});
  std::unique_ptr<WritableFile> base_file(
      new MockWritableFile(expected_file_calls));

  // Configure the mock base file system.
  ExpectedCalls expected_fs_calls(
      {std::make_tuple("NewWritableFile", Status::OK())});
  std::unique_ptr<MockFileSystem> base_fs(
      new MockFileSystem(expected_fs_calls));
  base_fs->writable_file_to_return = std::move(base_file);
  RetryingFileSystem<MockFileSystem> fs(
      std::move(base_fs), RetryConfig(0 /* init_delay_time_us */));

  // Retrieve the wrapped writable file.
  std::unique_ptr<WritableFile> writable_file;
  TF_EXPECT_OK(fs.NewWritableFile("filename.txt", &writable_file));

  writable_file.reset();  // Trigger Close() via destructor.
}

TEST(RetryingFileSystemTest, NewAppendableFile_SuccessWith3rdTry) {
  // Configure the mock base random access file.
  ExpectedCalls expected_file_calls(
      {std::make_tuple("Sync", errors::Unavailable("Something is wrong")),
       std::make_tuple("Sync", errors::Unavailable("Something is wrong again")),
       std::make_tuple("Sync", Status::OK()),
       std::make_tuple("Close", Status::OK())});
  std::unique_ptr<WritableFile> base_file(
      new MockWritableFile(expected_file_calls));

  // Configure the mock base file system.
  ExpectedCalls expected_fs_calls(
      {std::make_tuple("NewAppendableFile", Status::OK())});
  std::unique_ptr<MockFileSystem> base_fs(
      new MockFileSystem(expected_fs_calls));
  base_fs->writable_file_to_return = std::move(base_file);
  RetryingFileSystem<MockFileSystem> fs(
      std::move(base_fs), RetryConfig(0 /* init_delay_time_us */));

  // Retrieve the wrapped appendable file.
  std::unique_ptr<WritableFile> writable_file;
  TF_EXPECT_OK(fs.NewAppendableFile("filename.txt", &writable_file));

  // Use it and check the results.
  TF_EXPECT_OK(writable_file->Sync());
}

TEST(RetryingFileSystemTest, NewWritableFile_AllRetriesFailed) {
  // Configure the mock base random access file.
  ExpectedCalls expected_file_calls = CreateRetriableErrors("Sync", 11);
  expected_file_calls.emplace_back(std::make_tuple("Close", Status::OK()));
  std::unique_ptr<WritableFile> base_file(
      new MockWritableFile(expected_file_calls));

  // Configure the mock base file system.
  ExpectedCalls expected_fs_calls(
      {std::make_tuple("NewWritableFile", Status::OK())});
  std::unique_ptr<MockFileSystem> base_fs(
      new MockFileSystem(expected_fs_calls));
  base_fs->writable_file_to_return = std::move(base_file);
  RetryingFileSystem<MockFileSystem> fs(
      std::move(base_fs), RetryConfig(0 /* init_delay_time_us */));

  // Retrieve the wrapped writable file.
  std::unique_ptr<WritableFile> writable_file;
  TF_EXPECT_OK(fs.NewWritableFile("filename.txt", &writable_file));

  // Use it and check the results.
  const auto& status = writable_file->Sync();
  EXPECT_TRUE(
      str_util::StrContains(status.error_message(), "Retriable error #10"))
      << status;
}

TEST(RetryingFileSystemTest,
     NewReadOnlyMemoryRegionFromFile_SuccessWith2ndTry) {
  ExpectedCalls expected_fs_calls(
      {std::make_tuple("NewReadOnlyMemoryRegionFromFile",
                       errors::Unavailable("Something is wrong")),
       std::make_tuple("NewReadOnlyMemoryRegionFromFile", Status::OK())});
  std::unique_ptr<MockFileSystem> base_fs(
      new MockFileSystem(expected_fs_calls));
  RetryingFileSystem<MockFileSystem> fs(
      std::move(base_fs), RetryConfig(0 /* init_delay_time_us */));

  std::unique_ptr<ReadOnlyMemoryRegion> result;
  TF_EXPECT_OK(fs.NewReadOnlyMemoryRegionFromFile("filename.txt", &result));
}

TEST(RetryingFileSystemTest, NewReadOnlyMemoryRegionFromFile_AllRetriesFailed) {
  ExpectedCalls expected_fs_calls =
      CreateRetriableErrors("NewReadOnlyMemoryRegionFromFile", 11);
  std::unique_ptr<MockFileSystem> base_fs(
      new MockFileSystem(expected_fs_calls));
  RetryingFileSystem<MockFileSystem> fs(
      std::move(base_fs), RetryConfig(0 /* init_delay_time_us */));

  std::unique_ptr<ReadOnlyMemoryRegion> result;
  const auto& status =
      fs.NewReadOnlyMemoryRegionFromFile("filename.txt", &result);
  EXPECT_TRUE(
      str_util::StrContains(status.error_message(), "Retriable error #10"))
      << status;
}

TEST(RetryingFileSystemTest, GetChildren_SuccessWith2ndTry) {
  ExpectedCalls expected_fs_calls(
      {std::make_tuple("GetChildren",
                       errors::Unavailable("Something is wrong")),
       std::make_tuple("GetChildren", Status::OK())});
  std::unique_ptr<MockFileSystem> base_fs(
      new MockFileSystem(expected_fs_calls));
  RetryingFileSystem<MockFileSystem> fs(
      std::move(base_fs), RetryConfig(0 /* init_delay_time_us */));

  std::vector<string> result;
  TF_EXPECT_OK(fs.GetChildren("gs://path", &result));
}

TEST(RetryingFileSystemTest, GetChildren_AllRetriesFailed) {
  ExpectedCalls expected_fs_calls = CreateRetriableErrors("GetChildren", 11);
  std::unique_ptr<MockFileSystem> base_fs(
      new MockFileSystem(expected_fs_calls));
  RetryingFileSystem<MockFileSystem> fs(
      std::move(base_fs), RetryConfig(0 /* init_delay_time_us */));

  std::vector<string> result;
  const auto& status = fs.GetChildren("gs://path", &result);
  EXPECT_TRUE(
      str_util::StrContains(status.error_message(), "Retriable error #10"))
      << status;
}

TEST(RetryingFileSystemTest, GetMatchingPaths_SuccessWith2ndTry) {
  ExpectedCalls expected_fs_calls(
      {std::make_tuple("GetMatchingPaths",
                       errors::Unavailable("Something is wrong")),
       std::make_tuple("GetMatchingPaths", Status::OK())});
  std::unique_ptr<MockFileSystem> base_fs(
      new MockFileSystem(expected_fs_calls));
  RetryingFileSystem<MockFileSystem> fs(
      std::move(base_fs), RetryConfig(0 /* init_delay_time_us */));

  std::vector<string> result;
  TF_EXPECT_OK(fs.GetMatchingPaths("gs://path/dir", &result));
}

TEST(RetryingFileSystemTest, GetMatchingPaths_AllRetriesFailed) {
  ExpectedCalls expected_fs_calls =
      CreateRetriableErrors("GetMatchingPaths", 11);
  std::unique_ptr<MockFileSystem> base_fs(
      new MockFileSystem(expected_fs_calls));
  RetryingFileSystem<MockFileSystem> fs(
      std::move(base_fs), RetryConfig(0 /* init_delay_time_us */));

  std::vector<string> result;
  const auto& status = fs.GetMatchingPaths("gs://path/dir", &result);
  EXPECT_TRUE(
      str_util::StrContains(status.error_message(), "Retriable error #10"))
      << status;
}

TEST(RetryingFileSystemTest, DeleteFile_SuccessWith2ndTry) {
  ExpectedCalls expected_fs_calls(
      {std::make_tuple("DeleteFile", errors::Unavailable("Something is wrong")),
       std::make_tuple("DeleteFile", Status::OK())});
  std::unique_ptr<MockFileSystem> base_fs(
      new MockFileSystem(expected_fs_calls));
  RetryingFileSystem<MockFileSystem> fs(
      std::move(base_fs), RetryConfig(0 /* init_delay_time_us */));

  std::vector<string> result;
  TF_EXPECT_OK(fs.DeleteFile("gs://path/file.txt"));
}

TEST(RetryingFileSystemTest, DeleteFile_AllRetriesFailed) {
  ExpectedCalls expected_fs_calls = CreateRetriableErrors("DeleteFile", 11);
  std::unique_ptr<MockFileSystem> base_fs(
      new MockFileSystem(expected_fs_calls));
  RetryingFileSystem<MockFileSystem> fs(
      std::move(base_fs), RetryConfig(0 /* init_delay_time_us */));

  std::vector<string> result;
  const auto& status = fs.DeleteFile("gs://path/file.txt");
  EXPECT_TRUE(
      str_util::StrContains(status.error_message(), "Retriable error #10"))
      << status;
}

TEST(RetryingFileSystemTest, CreateDir_SuccessWith2ndTry) {
  ExpectedCalls expected_fs_calls(
      {std::make_tuple("CreateDir", errors::Unavailable("Something is wrong")),
       std::make_tuple("CreateDir", Status::OK())});
  std::unique_ptr<MockFileSystem> base_fs(
      new MockFileSystem(expected_fs_calls));
  RetryingFileSystem<MockFileSystem> fs(
      std::move(base_fs), RetryConfig(0 /* init_delay_time_us */));

  std::vector<string> result;
  TF_EXPECT_OK(fs.CreateDir("gs://path/newdir"));
}

TEST(RetryingFileSystemTest, CreateDir_AllRetriesFailed) {
  ExpectedCalls expected_fs_calls = CreateRetriableErrors("CreateDir", 11);
  std::unique_ptr<MockFileSystem> base_fs(
      new MockFileSystem(expected_fs_calls));
  RetryingFileSystem<MockFileSystem> fs(
      std::move(base_fs), RetryConfig(0 /* init_delay_time_us */));

  std::vector<string> result;
  const auto& status = fs.CreateDir("gs://path/newdir");
  EXPECT_TRUE(
      str_util::StrContains(status.error_message(), "Retriable error #10"))
      << status;
}

TEST(RetryingFileSystemTest, DeleteDir_SuccessWith2ndTry) {
  ExpectedCalls expected_fs_calls(
      {std::make_tuple("DeleteDir", errors::Unavailable("Something is wrong")),
       std::make_tuple("DeleteDir", Status::OK())});
  std::unique_ptr<MockFileSystem> base_fs(
      new MockFileSystem(expected_fs_calls));
  RetryingFileSystem<MockFileSystem> fs(
      std::move(base_fs), RetryConfig(0 /* init_delay_time_us */));

  std::vector<string> result;
  TF_EXPECT_OK(fs.DeleteDir("gs://path/dir"));
}

TEST(RetryingFileSystemTest, DeleteDir_AllRetriesFailed) {
  ExpectedCalls expected_fs_calls = CreateRetriableErrors("DeleteDir", 11);
  std::unique_ptr<MockFileSystem> base_fs(
      new MockFileSystem(expected_fs_calls));
  RetryingFileSystem<MockFileSystem> fs(
      std::move(base_fs), RetryConfig(0 /* init_delay_time_us */));

  std::vector<string> result;
  const auto& status = fs.DeleteDir("gs://path/dir");
  EXPECT_TRUE(
      str_util::StrContains(status.error_message(), "Retriable error #10"))
      << status;
}

TEST(RetryingFileSystemTest, GetFileSize_SuccessWith2ndTry) {
  ExpectedCalls expected_fs_calls(
      {std::make_tuple("GetFileSize",
                       errors::Unavailable("Something is wrong")),
       std::make_tuple("GetFileSize", Status::OK())});
  std::unique_ptr<MockFileSystem> base_fs(
      new MockFileSystem(expected_fs_calls));
  RetryingFileSystem<MockFileSystem> fs(
      std::move(base_fs), RetryConfig(0 /* init_delay_time_us */));

  uint64 size;
  TF_EXPECT_OK(fs.GetFileSize("gs://path/file.txt", &size));
}

TEST(RetryingFileSystemTest, GetFileSize_AllRetriesFailed) {
  ExpectedCalls expected_fs_calls = CreateRetriableErrors("GetFileSize", 11);
  std::unique_ptr<MockFileSystem> base_fs(
      new MockFileSystem(expected_fs_calls));
  RetryingFileSystem<MockFileSystem> fs(
      std::move(base_fs), RetryConfig(0 /* init_delay_time_us */));

  uint64 size;
  const auto& status = fs.GetFileSize("gs://path/file.txt", &size);
  EXPECT_TRUE(
      str_util::StrContains(status.error_message(), "Retriable error #10"))
      << status;
}

TEST(RetryingFileSystemTest, RenameFile_SuccessWith2ndTry) {
  ExpectedCalls expected_fs_calls(
      {std::make_tuple("RenameFile", errors::Unavailable("Something is wrong")),
       std::make_tuple("RenameFile", Status::OK())});
  std::unique_ptr<MockFileSystem> base_fs(
      new MockFileSystem(expected_fs_calls));
  RetryingFileSystem<MockFileSystem> fs(
      std::move(base_fs), RetryConfig(0 /* init_delay_time_us */));

  TF_EXPECT_OK(fs.RenameFile("old_name", "new_name"));
}

TEST(RetryingFileSystemTest, RenameFile_AllRetriesFailed) {
  ExpectedCalls expected_fs_calls = CreateRetriableErrors("RenameFile", 11);
  std::unique_ptr<MockFileSystem> base_fs(
      new MockFileSystem(expected_fs_calls));
  RetryingFileSystem<MockFileSystem> fs(
      std::move(base_fs), RetryConfig(0 /* init_delay_time_us */));

  const auto& status = fs.RenameFile("old_name", "new_name");
  EXPECT_TRUE(
      str_util::StrContains(status.error_message(), "Retriable error #10"))
      << status;
}

TEST(RetryingFileSystemTest, Stat_SuccessWith2ndTry) {
  ExpectedCalls expected_fs_calls(
      {std::make_tuple("Stat", errors::Unavailable("Something is wrong")),
       std::make_tuple("Stat", Status::OK())});
  std::unique_ptr<MockFileSystem> base_fs(
      new MockFileSystem(expected_fs_calls));
  RetryingFileSystem<MockFileSystem> fs(
      std::move(base_fs), RetryConfig(0 /* init_delay_time_us */));

  FileStatistics stat;
  TF_EXPECT_OK(fs.Stat("file_name", &stat));
}

TEST(RetryingFileSystemTest, Stat_AllRetriesFailed) {
  ExpectedCalls expected_fs_calls = CreateRetriableErrors("Stat", 11);
  std::unique_ptr<MockFileSystem> base_fs(
      new MockFileSystem(expected_fs_calls));
  RetryingFileSystem<MockFileSystem> fs(
      std::move(base_fs), RetryConfig(0 /* init_delay_time_us */));

  FileStatistics stat;
  const auto& status = fs.Stat("file_name", &stat);
  EXPECT_TRUE(
      str_util::StrContains(status.error_message(), "Retriable error #10"))
      << status;
}

TEST(RetryingFileSystemTest, FileExists_AllRetriesFailed) {
  ExpectedCalls expected_fs_calls = CreateRetriableErrors("FileExists", 11);
  std::unique_ptr<MockFileSystem> base_fs(
      new MockFileSystem(expected_fs_calls));
  RetryingFileSystem<MockFileSystem> fs(
      std::move(base_fs), RetryConfig(0 /* init_delay_time_us */));

  const auto& status = fs.FileExists("file_name");
  EXPECT_TRUE(
      str_util::StrContains(status.error_message(), "Retriable error #10"))
      << status;
}

TEST(RetryingFileSystemTest, FileExists_SuccessWith2ndTry) {
  ExpectedCalls expected_fs_calls(
      {std::make_tuple("FileExists", errors::Unavailable("Something is wrong")),
       std::make_tuple("FileExists", Status::OK())});
  std::unique_ptr<MockFileSystem> base_fs(
      new MockFileSystem(expected_fs_calls));
  RetryingFileSystem<MockFileSystem> fs(
      std::move(base_fs), RetryConfig(0 /* init_delay_time_us */));

  TF_EXPECT_OK(fs.FileExists("gs://path/dir"));
}

TEST(RetryingFileSystemTest, IsDirectory_SuccessWith2ndTry) {
  ExpectedCalls expected_fs_calls(
      {std::make_tuple("IsDirectory",
                       errors::Unavailable("Something is wrong")),
       std::make_tuple("IsDirectory", Status::OK())});
  std::unique_ptr<MockFileSystem> base_fs(
      new MockFileSystem(expected_fs_calls));
  RetryingFileSystem<MockFileSystem> fs(
      std::move(base_fs), RetryConfig(0 /* init_delay_time_us */));

  TF_EXPECT_OK(fs.IsDirectory("gs://path/dir"));
}

TEST(RetryingFileSystemTest, IsDirectory_AllRetriesFailed) {
  ExpectedCalls expected_fs_calls = CreateRetriableErrors("IsDirectory", 11);
  std::unique_ptr<MockFileSystem> base_fs(
      new MockFileSystem(expected_fs_calls));
  RetryingFileSystem<MockFileSystem> fs(
      std::move(base_fs), RetryConfig(0 /* init_delay_time_us */));

  const auto& status = fs.IsDirectory("gs://path/dir");
  EXPECT_TRUE(
      str_util::StrContains(status.error_message(), "Retriable error #10"))
      << status;
}

TEST(RetryingFileSystemTest, DeleteRecursively_SuccessWith2ndTry) {
  ExpectedCalls expected_fs_calls(
      {std::make_tuple("DeleteRecursively",
                       errors::Unavailable("Something is wrong")),
       std::make_tuple("DeleteRecursively", Status::OK())});
  std::unique_ptr<MockFileSystem> base_fs(
      new MockFileSystem(expected_fs_calls));
  RetryingFileSystem<MockFileSystem> fs(
      std::move(base_fs), RetryConfig(0 /* init_delay_time_us */));
  int64 undeleted_files, undeleted_dirs;

  TF_EXPECT_OK(
      fs.DeleteRecursively("gs://path/dir", &undeleted_files, &undeleted_dirs));
}

TEST(RetryingFileSystemTest, DeleteRecursively_AllRetriesFailed) {
  ExpectedCalls expected_fs_calls =
      CreateRetriableErrors("DeleteRecursively", 11);
  std::unique_ptr<MockFileSystem> base_fs(
      new MockFileSystem(expected_fs_calls));
  RetryingFileSystem<MockFileSystem> fs(
      std::move(base_fs), RetryConfig(0 /* init_delay_time_us */));
  int64 undeleted_files, undeleted_dirs;

  const auto& status =
      fs.DeleteRecursively("gs://path/dir", &undeleted_files, &undeleted_dirs);
  EXPECT_TRUE(
      str_util::StrContains(status.error_message(), "Retriable error #10"))
      << status;
}

TEST(RetryingFileSystemTest, FlushCaches) {
  ExpectedCalls none;
  bool flushed = false;
  std::unique_ptr<MockFileSystem> base_fs(new MockFileSystem(none, &flushed));
  RetryingFileSystem<MockFileSystem> fs(
      std::move(base_fs), RetryConfig(0 /* init_delay_time_us */));
  fs.FlushCaches();
  EXPECT_TRUE(flushed);
}

}  // namespace
}  // namespace tensorflow
