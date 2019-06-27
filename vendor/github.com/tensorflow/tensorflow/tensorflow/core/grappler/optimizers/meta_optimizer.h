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

#ifndef TENSORFLOW_CORE_GRAPPLER_OPTIMIZERS_META_OPTIMIZER_H_
#define TENSORFLOW_CORE_GRAPPLER_OPTIMIZERS_META_OPTIMIZER_H_

#include "tensorflow/core/framework/device_base.h"
#include "tensorflow/core/grappler/grappler_item.h"
#include "tensorflow/core/grappler/optimizers/graph_optimizer.h"
#include "tensorflow/core/lib/core/status.h"
#include "tensorflow/core/protobuf/config.pb.h"
#include "tensorflow/core/protobuf/rewriter_config.pb.h"

namespace tensorflow {
namespace grappler {

// Run the other grappler optimizers based on the specified rewriter config.
class MetaOptimizer : public GraphOptimizer {
 public:
  MetaOptimizer(DeviceBase* cpu_device, const ConfigProto& cfg);
  ~MetaOptimizer() override = default;

  string name() const override { return "meta_optimizer"; };

  Status Optimize(Cluster* cluster, const GrapplerItem& item,
                  GraphDef* optimized_graph) override;

  void PrintResult();

  void Feedback(Cluster* cluster, const GrapplerItem& item,
                const GraphDef& optimized_graph, double result) override;

 private:
  std::unique_ptr<GraphOptimizer> MakeNewOptimizer(
      const string& optimizer) const;

  // Initialize active optimizers from RewriterConfig toggles.
  Status InitializeOptimizers(
      std::vector<std::unique_ptr<GraphOptimizer>>* optimizers) const;
  // Initialize active optimizers from RewriterConfig optimizer names.
  Status InitializeOptimizersByName(
      std::vector<std::unique_ptr<GraphOptimizer>>* optimizers) const;
  // Initialize active optimizers from RewriterConfig.custom_optimizers.
  Status InitializeCustomGraphOptimizers(
      const std::set<string>& pre_initialized_optimizers,
      std::vector<std::unique_ptr<GraphOptimizer>>* optimizers) const;
  // Returns the config for a custom graph optimizer. Null if none was found.
  const RewriterConfig::CustomGraphOptimizer* GetCustomGraphOptimizerConfig(
      const string& name) const;

  // Run optimization pass over a single GrapplerItem. Meta optimizer might run
  // multiple such passes: 1) for the main graph 2) for the function library
  Status OptimizeGraph(Cluster* cluster, const GrapplerItem& item,
                       GraphDef* optimized_graph);

  DeviceBase* const cpu_device_;  // may be NULL
  ConfigProto config_proto_;
  RewriterConfig& cfg_;

  struct OptimizerResult {
    string optimizer_name;
    string result;
  };

  struct GraphOptimizationResult {
    explicit GraphOptimizationResult(const string& id) : id(id) {}
    string id;
    std::vector<OptimizerResult> results;
  };

  Status RunOptimizer(GraphOptimizer* optimizer, Cluster* cluster,
                      GrapplerItem* optimized_item, GraphDef* optimized_graph,
                      GraphOptimizationResult* optimization_result);

  std::vector<GraphOptimizationResult> optimization_results_;
};

bool MetaOptimizerEnabled(const ConfigProto& cfg);

// Run the meta optimizer.
//
// If <cpu_device> is non-null, it is the device to be used for executing ops
// during constant folding; if NULL, a new device is created for doing constant
// folding. For performance, it is recommended to pass in an existing cpu_device
// when possible.
Status RunMetaOptimizer(const GrapplerItem& item, const ConfigProto& cfg,
                        DeviceBase* cpu_device, Cluster* cluster,
                        GraphDef* optimized_graph);

}  // namespace grappler
}  // namespace tensorflow

#endif  // TENSORFLOW_CORE_GRAPPLER_OPTIMIZERS_META_OPTIMIZER_H_
