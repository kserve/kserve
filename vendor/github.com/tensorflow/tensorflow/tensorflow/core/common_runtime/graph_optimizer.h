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

#ifndef TENSORFLOW_CORE_COMMON_RUNTIME_GRAPH_OPTIMIZER_H_
#define TENSORFLOW_CORE_COMMON_RUNTIME_GRAPH_OPTIMIZER_H_

#include "tensorflow/core/framework/function.h"
#include "tensorflow/core/graph/graph.h"
#include "tensorflow/core/lib/core/status.h"
#include "tensorflow/core/platform/env.h"
#include "tensorflow/core/protobuf/config.pb.h"

namespace tensorflow {

class GraphOptimizer {
 public:
  GraphOptimizer(const OptimizerOptions& opts);
  ~GraphOptimizer();

  // Applies optimization passes specified in 'opts' to 'graph'.
  // Maybe replace *graph with a new graph object.  'device' is device
  // on which the 'graph' will execute. It's passed to the optimizers
  // so that they can respect constraints if any, that should be
  // respected.
  //
  // If shape_map is not null it maps from nodes in graph to partially-known
  // shapes of their outputs, and may be used, e.g., in the constant folding
  // pass. The use of shape_map implies that the mapping from node name to the
  // vector of partial shapes of its outputs is stable, i.e., no optimization
  // pass may replace a node with a different node of the same name that has a
  // different number of outputs, or outputs with different known shapes.
  // TODO(b/65453533) introduce a unique way to name nodes in a graph.
  //
  // If cse_consider_fn is not null then only nodes for which cse_consider_fn
  // returns true will be considered for CSE.
  // If cf_consider_fn is not null then only nodes for which cf_consider_fn
  // returns true will be considered for CF.
  void Optimize(
      FunctionLibraryRuntime* runtime, Env* env, Device* device,
      std::unique_ptr<Graph>* graph,
      const std::unordered_map<string, std::vector<PartialTensorShape>>*
          shape_map,
      const std::function<bool(const Node*)>& cse_consider_fn = nullptr,
      const std::function<bool(const Node*)>& cf_consider_fn = nullptr);

  const OptimizerOptions& options() { return opts_; }

 private:
  OptimizerOptions opts_;

  TF_DISALLOW_COPY_AND_ASSIGN(GraphOptimizer);
};

}  // end namespace tensorflow

#endif  // TENSORFLOW_CORE_COMMON_RUNTIME_GRAPH_OPTIMIZER_H_
