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

#ifndef TENSORFLOW_CONTRIB_TENSORRT_CONVERT_TRT_OPTIMIZATION_PASS_H_
#define TENSORFLOW_CONTRIB_TENSORRT_CONVERT_TRT_OPTIMIZATION_PASS_H_

#include <string>

#include "tensorflow/core/framework/graph.pb.h"
#include "tensorflow/core/grappler/optimizers/custom_graph_optimizer.h"
#include "tensorflow/core/platform/logging.h"

#if GOOGLE_CUDA
#if GOOGLE_TENSORRT

namespace tensorflow {
namespace tensorrt {
namespace convert {

class TRTOptimizationPass : public tensorflow::grappler::CustomGraphOptimizer {
 public:
  TRTOptimizationPass(const string& name = "TRTOptimizationPass")
      : name_(name),
        minimum_segment_size_(3),
        precision_mode_(0),
        maximum_batch_size_(-1),
        is_dynamic_op_(false),
        max_cached_batches_(1),
        max_workspace_size_bytes_(256LL << 20),
        use_calibration_(true) {
    VLOG(1) << "Constructing " << name_;
  }

  string name() const override { return name_; };

  tensorflow::Status Init(const tensorflow::RewriterConfig_CustomGraphOptimizer*
                              config = nullptr) override;

  tensorflow::Status Optimize(tensorflow::grappler::Cluster* cluster,
                              const tensorflow::grappler::GrapplerItem& item,
                              GraphDef* optimized_graph) override;

  void Feedback(tensorflow::grappler::Cluster* cluster,
                const tensorflow::grappler::GrapplerItem& item,
                const GraphDef& optimized_graph, double result) override;

  void PrintDebugInfo(tensorflow::grappler::Cluster* cluster,
                      const tensorflow::grappler::GrapplerItem& item);

 private:
  const string name_;
  int minimum_segment_size_;
  int precision_mode_;
  int maximum_batch_size_;
  bool is_dynamic_op_;
  std::vector<int> batches_;
  int max_cached_batches_;
  int64_t max_workspace_size_bytes_;
  bool use_calibration_;
};

}  // namespace convert
}  // namespace tensorrt
}  // namespace tensorflow

#endif  // GOOGLE_CUDA
#endif  // GOOGLE_TENSORRT
#endif  // TENSORFLOW_CONTRIB_TENSORRT_CONVERT_TRT_OPTIMIZATION_PASS_H_
