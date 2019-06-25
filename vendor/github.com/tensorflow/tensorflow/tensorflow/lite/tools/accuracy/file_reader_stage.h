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

#ifndef TENSORFLOW_LITE_TOOLS_ACCURACY_FILE_READER_STAGE_H_
#define TENSORFLOW_LITE_TOOLS_ACCURACY_FILE_READER_STAGE_H_

#include <string>

#include "tensorflow/lite/tools/accuracy/stage.h"

namespace tensorflow {
namespace metrics {
// A stage for reading a file into |string|.
// Inputs: a string tensor: |file_name|.
// Outputs: a string tensor: contents of |file_name|.
class FileReaderStage : public Stage {
 public:
  string name() const override { return "stage_filereader"; }
  string output_name() const override { return "stage_filereader_output"; }

  void AddToGraph(const Scope& scope, const Input& input) override;
};
}  //  namespace metrics
}  //  namespace tensorflow
#endif  // TENSORFLOW_LITE_TOOLS_ACCURACY_FILE_READER_STAGE_H_
