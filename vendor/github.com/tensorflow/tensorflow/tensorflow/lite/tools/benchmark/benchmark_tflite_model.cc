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

#include "tensorflow/lite/tools/benchmark/benchmark_tflite_model.h"

#include <cstdarg>
#include <cstdlib>
#include <iostream>
#include <memory>
#include <string>
#include <unordered_set>
#include <vector>

#include "tensorflow/lite/kernels/register.h"
#include "tensorflow/lite/model.h"
#include "tensorflow/lite/op_resolver.h"
#include "tensorflow/lite/string_util.h"
#include "tensorflow/lite/tools/benchmark/logging.h"

#ifdef GEMMLOWP_PROFILING
#include "gemmlowp/profiling/profiler.h"
#endif

#ifdef TFLITE_CUSTOM_OPS_HEADER
void RegisterSelectedOps(::tflite::MutableOpResolver* resolver);
#endif

namespace tflite {
namespace benchmark {

void ProfilingListener::SetInterpreter(tflite::Interpreter* interpreter) {
  TFLITE_BENCHMARK_CHECK(interpreter);
  interpreter_ = interpreter;
  interpreter_->SetProfiler(&profiler_);
}

void ProfilingListener::OnSingleRunStart(RunType run_type) {
  if (run_type == REGULAR) {
    profiler_.Reset();
    profiler_.StartProfiling();
  }
}

void ProfilingListener::OnBenchmarkEnd(const BenchmarkResults& results) {
  if (has_profiles_) {
    TFLITE_LOG(INFO) << summarizer_.GetOutputString();
  }
}

void ProfilingListener::OnSingleRunEnd() {
  profiler_.StopProfiling();
  auto profile_events = profiler_.GetProfileEvents();
  has_profiles_ = !profile_events.empty();
  summarizer_.ProcessProfiles(profile_events, *interpreter_);
}

void GemmlowpProfilingListener::OnBenchmarkStart(
    const BenchmarkParams& params) {
#ifdef GEMMLOWP_PROFILING
  gemmlowp::RegisterCurrentThreadForProfiling();
  gemmlowp::StartProfiling();
#endif
}

void GemmlowpProfilingListener::OnBenchmarkEnd(
    const BenchmarkResults& results) {
#ifdef GEMMLOWP_PROFILING
  gemmlowp::FinishProfiling();
#endif
}

namespace {

std::vector<std::string> Split(const std::string& str, const char delim) {
  std::istringstream input(str);
  std::vector<std::string> results;
  std::string item;
  while (std::getline(input, item, delim)) {
    results.push_back(item);
  }
  return results;
}

template <typename T>
bool SplitAndParse(const std::string& str, char delim, std::vector<T>* values) {
  std::istringstream input(str);
  bool first = true;
  while (!input.eof()) {
    if (!first) {
      char c;
      input >> c;
      if (c != delim) {
        return false;
      }
    } else {
      first = false;
    }
    T val;
    input >> val;
    if (!input.eof() && !input.good()) {
      return false;
    }
    values->push_back(val);
  }
  return true;
}

template <typename T>
void FillRandomValue(T* ptr, const std::vector<int>& sizes,
                     const std::function<T()>& random_func) {
  int num_elements = 1;
  for (int dim : sizes) {
    num_elements *= dim;
  }
  for (int i = 0; i < num_elements; ++i) {
    *ptr++ = random_func();
  }
}

void FillRandomString(tflite::DynamicBuffer* buffer,
                      const std::vector<int>& sizes,
                      const std::function<string()>& random_func) {
  int num_elements = 1;
  for (int dim : sizes) {
    num_elements *= dim;
  }
  for (int i = 0; i < num_elements; ++i) {
    auto str = random_func();
    buffer->AddString(str.data(), str.length());
  }
}

bool PopulateInputLayerInfo(
    const string& names_string, const string& shapes_string,
    std::vector<BenchmarkTfLiteModel::InputLayerInfo>* info) {
  std::vector<std::string> names = Split(names_string, ',');
  std::vector<std::string> shapes = Split(shapes_string, ':');

  if (names.size() != shapes.size()) {
    TFLITE_LOG(ERROR) << "The number of items in"
                      << " --input_layer_shape (" << shapes_string << ", with "
                      << shapes.size() << " items)"
                      << " must match the number of items in"
                      << " --input_layer (" << names_string << ", with "
                      << names.size() << " items)."
                      << " For example --input_layer=input1,input2"
                      << " --input_layer_shape=1,224,224,4:1,20";
    return false;
  }

  for (int i = 0; i < names.size(); ++i) {
    info->push_back(BenchmarkTfLiteModel::InputLayerInfo());
    BenchmarkTfLiteModel::InputLayerInfo& input = info->back();

    input.name = names[i];

    TFLITE_BENCHMARK_CHECK(SplitAndParse(shapes[i], ',', &input.shape))
        << "Incorrect size string specified: " << shapes[i];
    for (int dim : input.shape) {
      if (dim == -1) {
        TFLITE_LOG(ERROR)
            << "Any unknown sizes in the shapes (-1's) must be replaced"
            << " with the size you want to benchmark with.";
        return false;
      }
    }
  }

  return true;
}

std::vector<int> TfLiteIntArrayToVector(const TfLiteIntArray* int_array) {
  std::vector<int> values;
  values.reserve(int_array->size);
  for (size_t i = 0; i < int_array->size; i++) {
    values.push_back(int_array->data[i]);
  }
  return values;
}

}  // namespace

BenchmarkParams BenchmarkTfLiteModel::DefaultParams() {
  BenchmarkParams default_params = BenchmarkModel::DefaultParams();
  default_params.AddParam("graph", BenchmarkParam::Create<std::string>(""));
  default_params.AddParam("input_layer",
                          BenchmarkParam::Create<std::string>(""));
  default_params.AddParam("input_layer_shape",
                          BenchmarkParam::Create<std::string>(""));
  default_params.AddParam("use_nnapi", BenchmarkParam::Create<bool>(false));
  return default_params;
}

BenchmarkTfLiteModel::BenchmarkTfLiteModel()
    : BenchmarkTfLiteModel(DefaultParams()) {}

BenchmarkTfLiteModel::BenchmarkTfLiteModel(BenchmarkParams params)
    : BenchmarkModel(std::move(params)) {
  AddListener(&profiling_listener_);
  AddListener(&gemmlowp_profiling_listener_);
}

std::vector<Flag> BenchmarkTfLiteModel::GetFlags() {
  std::vector<Flag> flags = BenchmarkTfLiteModel::BenchmarkModel::GetFlags();
  std::vector<Flag> specific_flags = {
      CreateFlag<std::string>("graph", &params_, "graph file name"),
      CreateFlag<std::string>("input_layer", &params_, "input layer names"),
      CreateFlag<std::string>("input_layer_shape", &params_,
                              "input layer shape"),
      CreateFlag<bool>("use_nnapi", &params_, "use nnapi api")};

  flags.insert(flags.end(), specific_flags.begin(), specific_flags.end());
  return flags;
}

void BenchmarkTfLiteModel::LogParams() {
  BenchmarkModel::LogParams();
  TFLITE_LOG(INFO) << "Graph: [" << params_.Get<std::string>("graph") << "]";
  TFLITE_LOG(INFO) << "Input layers: ["
                   << params_.Get<std::string>("input_layer") << "]";
  TFLITE_LOG(INFO) << "Input shapes: ["
                   << params_.Get<std::string>("input_layer_shape") << "]";
  TFLITE_LOG(INFO) << "Use nnapi : [" << params_.Get<bool>("use_nnapi") << "]";
}

bool BenchmarkTfLiteModel::ValidateParams() {
  if (params_.Get<std::string>("graph").empty()) {
    TFLITE_LOG(ERROR)
        << "Please specify the name of your TF Lite input file with --graph";
    return false;
  }
  return PopulateInputLayerInfo(params_.Get<std::string>("input_layer"),
                                params_.Get<std::string>("input_layer_shape"),
                                &inputs);
}

uint64_t BenchmarkTfLiteModel::ComputeInputBytes() {
  TFLITE_BENCHMARK_CHECK(interpreter);
  uint64_t total_input_bytes = 0;
  for (int input : interpreter->inputs()) {
    auto* t = interpreter->tensor(input);
    total_input_bytes += t->bytes;
  }
  return total_input_bytes;
}

void BenchmarkTfLiteModel::PrepareInputsAndOutputs() {
  auto interpreter_inputs = interpreter->inputs();
  // Set the values of the input tensors.
  for (int j = 0; j < interpreter_inputs.size(); ++j) {
    int i = interpreter_inputs[j];
    TfLiteTensor* t = interpreter->tensor(i);
    std::vector<int> sizes = TfLiteIntArrayToVector(t->dims);
    // TODO(ahentz): below we ignore the O-th dimension (number of batches).
    if (t->type == kTfLiteFloat32) {
      FillRandomValue<float>(
          interpreter->typed_tensor<float>(i),
          std::vector<int>(sizes.begin() + 1, sizes.end()),
          []() { return static_cast<float>(rand()) / RAND_MAX - 0.5f; });
    } else if (t->type == kTfLiteInt32) {
      // TODO(yunluli): This is currently only used for handling embedding input
      // for speech models. Generalize if necessary.
      FillRandomValue<int32_t>(
          interpreter->typed_tensor<int32_t>(i),
          std::vector<int32_t>(sizes.begin() + 1, sizes.end()),
          []() { return static_cast<int32_t>(rand()) % 100; });
    } else if (t->type == kTfLiteUInt8) {
      FillRandomValue<uint8_t>(
          interpreter->typed_tensor<uint8_t>(i),
          std::vector<int>(sizes.begin() + 1, sizes.end()),
          []() { return static_cast<uint8_t>(rand()) % 255; });
    } else if (t->type == kTfLiteInt8) {
      FillRandomValue<int8_t>(
          interpreter->typed_tensor<int8_t>(i),
          std::vector<int>(sizes.begin() + 1, sizes.end()),
          []() { return static_cast<int8_t>(rand()) % 255 - 127; });
    } else if (t->type == kTfLiteString) {
      tflite::DynamicBuffer buffer;
      FillRandomString(&buffer, sizes, []() {
        return "we're have some friends over saturday to hang out in the yard";
      });
      buffer.WriteToTensor(interpreter->tensor(i), /*new_shape=*/nullptr);
    } else {
      TFLITE_LOG(FATAL) << "Don't know how to populate tensor " << t->name
                        << " of type " << t->type;
    }
  }
}

void BenchmarkTfLiteModel::Init() {
  std::string graph = params_.Get<std::string>("graph");
  model = tflite::FlatBufferModel::BuildFromFile(graph.c_str());
  if (!model) {
    TFLITE_LOG(FATAL) << "Failed to mmap model " << graph;
  }
  TFLITE_LOG(INFO) << "Loaded model " << graph;
  model->error_reporter();
  TFLITE_LOG(INFO) << "resolved reporter";

#ifdef TFLITE_CUSTOM_OPS_HEADER
  tflite::MutableOpResolver resolver;
  RegisterSelectedOps(&resolver);
#else
  tflite::ops::builtin::BuiltinOpResolver resolver;
#endif

  tflite::InterpreterBuilder(*model, resolver)(&interpreter);
  if (!interpreter) {
    TFLITE_LOG(FATAL) << "Failed to construct interpreter";
  }
  profiling_listener_.SetInterpreter(interpreter.get());

  const int32_t num_threads = params_.Get<int32_t>("num_threads");

  if (num_threads != -1) {
    interpreter->SetNumThreads(num_threads);
  }

  bool use_nnapi = params_.Get<bool>("use_nnapi");

  interpreter->UseNNAPI(use_nnapi);
  ApplyDelegates();

  auto interpreter_inputs = interpreter->inputs();

  if (!inputs.empty()) {
    TFLITE_BENCHMARK_CHECK_EQ(inputs.size(), interpreter_inputs.size())
        << "Inputs mismatch: Model inputs #:" << interpreter_inputs.size()
        << " expected: " << inputs.size();
  }

  // TFLITE_BENCHMARK_CHECK that all names and types match
  for (int j = 0; j < inputs.size(); ++j) {
    const InputLayerInfo& input = inputs[j];
    int i = interpreter_inputs[j];
    TfLiteTensor* t = interpreter->tensor(i);
    TFLITE_BENCHMARK_CHECK_EQ(t->name, input.name)
        << "Tensor # " << i << " is named " << t->name << " but flags call it "
        << input.name;
  }

  // Resize all non-string tensors.
  for (int j = 0; j < inputs.size(); ++j) {
    const InputLayerInfo& input = inputs[j];
    int i = interpreter_inputs[j];
    TfLiteTensor* t = interpreter->tensor(i);
    if (t->type != kTfLiteString) {
      interpreter->ResizeInputTensor(i, input.shape);
    }
  }

  if (interpreter->AllocateTensors() != kTfLiteOk) {
    TFLITE_LOG(FATAL) << "Failed to allocate tensors!";
  }
}

void BenchmarkTfLiteModel::RunImpl() {
  if (interpreter->Invoke() != kTfLiteOk) {
    TFLITE_LOG(FATAL) << "Failed to invoke!";
  }
}

}  // namespace benchmark
}  // namespace tflite
