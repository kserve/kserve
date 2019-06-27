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

#include "tensorflow/lite/testing/init_tensorflow.h"
#include "tensorflow/lite/tools/benchmark/benchmark_tflite_model.h"
#include "tensorflow/lite/tools/benchmark/logging.h"

namespace tflite {
namespace benchmark {

int Main(int argc, char** argv) {
  ::tflite::InitTensorFlow();
#ifdef TFLITE_CUSTOM_OPS_HEADER
  TFLITE_LOG(INFO) << "STARTING with custom ops!";
#else
  TFLITE_LOG(INFO) << "STARTING!";
#endif
  BenchmarkTfLiteModel benchmark;
  BenchmarkLoggingListener listener;
  benchmark.AddListener(&listener);
  benchmark.Run(argc, argv);
  return 0;
}
}  // namespace benchmark
}  // namespace tflite

int main(int argc, char** argv) { return tflite::benchmark::Main(argc, argv); }
