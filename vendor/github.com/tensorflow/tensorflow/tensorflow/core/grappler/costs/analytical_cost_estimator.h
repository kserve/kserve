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

#ifndef TENSORFLOW_CORE_GRAPPLER_COSTS_ANALYTICAL_COST_ESTIMATOR_H_
#define TENSORFLOW_CORE_GRAPPLER_COSTS_ANALYTICAL_COST_ESTIMATOR_H_

#include "tensorflow/core/grappler/costs/cost_estimator.h"
#include "tensorflow/core/grappler/costs/op_level_cost_estimator.h"
#include "tensorflow/core/grappler/costs/virtual_scheduler.h"
#include "tensorflow/core/grappler/grappler_item.h"
#include "tensorflow/core/lib/core/status.h"

namespace tensorflow {
class CostGraphDef;
class GraphDef;
}  // namespace tensorflow

namespace tensorflow {
namespace grappler {

class Cluster;
struct GrapplerItem;

// Estimate the cost of running a Grappler item based on the theoretical
// performance of the hardware that will run the model.
class AnalyticalCostEstimator : public CostEstimator {
 public:
  // Does not take ownership of cluster.
  AnalyticalCostEstimator(Cluster* cluster, bool use_static_shapes);
  // Does not take ownership of cluster or run_metadata
  //
  // When metadata is provided, step_stats and partition_graphs fields will
  // always be filled during PredictCosts, and the cost_graph field of metadata
  // will be filled only when cost_graph is not nullptr when invoking
  // PredictCosts.
  AnalyticalCostEstimator(Cluster* cluster,
                          std::unique_ptr<OpLevelCostEstimator> node_estimator,
                          std::unique_ptr<ReadyNodeManager> node_manager,
                          bool use_static_shapes, RunMetadata* run_metadata);
  ~AnalyticalCostEstimator() override {}

  // Initializes the estimator for the specified grappler item.
  // This implementation always returns OK.
  Status Initialize(const GrapplerItem& item) override;

  // Predict the performance of each node of the optimized graph and annotate
  // the CostGraphDef with the corresponding estimates. Also returns the
  // expected cost for the whole graph.
  Status PredictCosts(const GraphDef& optimized_graph, CostGraphDef* cost_graph,
                      Costs* cost) const override;

  const VirtualScheduler* GetScheduler() const { return scheduler_.get(); }

 private:
  Cluster* cluster_;
  GrapplerItem item_;
  std::unique_ptr<OpLevelCostEstimator> node_estimator_;
  std::unique_ptr<ReadyNodeManager> node_manager_;
  bool use_static_shapes_;
  std::unique_ptr<VirtualScheduler> scheduler_;

  RunMetadata* run_metadata_;
};

}  // end namespace grappler
}  // end namespace tensorflow

#endif  // TENSORFLOW_CORE_GRAPPLER_COSTS_ANALYTICAL_COST_ESTIMATOR_H_
