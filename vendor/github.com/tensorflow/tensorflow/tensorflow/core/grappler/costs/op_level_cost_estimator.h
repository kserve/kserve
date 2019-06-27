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

#ifndef TENSORFLOW_CORE_GRAPPLER_COSTS_OP_LEVEL_COST_ESTIMATOR_H_
#define TENSORFLOW_CORE_GRAPPLER_COSTS_OP_LEVEL_COST_ESTIMATOR_H_

#include <functional>
#include <map>
#include <string>

#include "tensorflow/core/grappler/costs/cost_estimator.h"
#include "tensorflow/core/grappler/costs/op_context.h"
#include "tensorflow/core/grappler/costs/op_performance_data.pb.h"
#include "tensorflow/core/util/padding.h"

namespace tensorflow {
namespace grappler {

bool GetTensorShapeProtoFromTensorProto(const TensorProto& tensor_proto,
                                        TensorShapeProto* tensor_shape_proto);
TensorShapeProto MaybeGetMinimumShape(const TensorShapeProto& original_shape,
                                      int rank, bool* found_unknown_shapes);

class OpLevelCostEstimator {
 public:
  OpLevelCostEstimator();
  virtual ~OpLevelCostEstimator() {}

  virtual Costs PredictCosts(const OpContext& op_context) const;

  // Returns basic device performance info.
  virtual DeviceInfo GetDeviceInfo(const DeviceProperties& device) const;

 protected:
  // Predict cost of an op for which no accurate estimator is defined.
  Costs PredictCostOfAnUnknownOp(const OpContext& op_context) const;

  // Naive cost estimate based on the given operations count and total
  // input/output tensor sizes of the given op_info combined.
  Costs PredictOpCountBasedCost(double operations, const OpInfo& op_info) const;

  // Naive cost estimate based on the given operations count and the given total
  // io size in bytes. Sizes of op_info inputs and outputs are not taken into
  // consideration.
  Costs PredictOpCountBasedCost(double operations, double input_io_bytes,
                                double output_io_bytes,
                                const OpInfo& op_info) const;

  // This family of routines counts the number of operations to perform the
  // specified TensorFlow Op.
  struct MatMulDimensions {
    int m;
    int n;
    int k;
  };
  struct ConvolutionDimensions {
    int64 batch;      // Batch size.
    int64 ix;         // Input size x.
    int64 iy;         // Input size y.
    int64 iz;         // Input depth.
    int64 kx;         // Kernel x.
    int64 ky;         // Kernel y.
    int64 oz;         // Output depth.
    int64 ox;         // Output size x.
    int64 oy;         // Output size y.
    int64 sx;         // Stride x.
    int64 sy;         // Stride y.
    Padding padding;  // SAME or VALID.
  };
  int64 CountConv2DOperations(const OpInfo& op_features,
                              bool* found_unknown_shapes) const;
  int64 CountConv2DOperations(const OpInfo& op_features,
                              ConvolutionDimensions* conv_info,
                              bool* found_unknown_shapes) const;
  int64 CountMatMulOperations(const OpInfo& op_features,
                              bool* found_unknown_shapes) const;
  int64 CountMatMulOperations(const OpInfo& op_features,
                              MatMulDimensions* mat_mul,
                              bool* found_unknown_shapes) const;
  int64 CountBatchMatMulOperations(const OpInfo& op_features,
                                   bool* found_unknown_shapes) const;
  int64 CountConv2DBackpropInputOperations(const OpInfo& op_features,
                                           ConvolutionDimensions* conv_info,
                                           bool* found_unknown_shapes) const;
  int64 CountConv2DBackpropFilterOperations(const OpInfo& op_features,
                                            ConvolutionDimensions* conv_info,
                                            bool* found_unknown_shapes) const;

  // Calculate the element count of an input/output tensor.
  int64 CalculateTensorElementCount(const OpInfo::TensorProperties& tensor,
                                    bool* found_unknown_shapes) const;

  // Calculate the total size in bytes of an input/output tensor.
  int64 CalculateTensorSize(const OpInfo::TensorProperties& tensor,
                            bool* found_unknown_shapes) const;

  // Calculate the element count of the largest
  // input of specified TensorFlow op.
  int64 CalculateLargestInputCount(const OpInfo& op_features,
                                   bool* found_unknown_shapes) const;

  // Calculate the total size in bytes of the all
  // the inputs of specified TensorFlow op.
  int64 CalculateInputSize(const OpInfo& op_features,
                           bool* found_unknown_shapes) const;

  // Calculate the total size in bytes of the all
  // the outputs of specified TensorFlow op.
  int64 CalculateOutputSize(const OpInfo& op_features,
                            bool* found_unknown_shapes) const;

  // This family of routines predicts the costs to
  // perform the specified TensorFlow Op on the
  // device represented by a subclass. The default
  // implementation just divides the operations to
  // perform the op (from the "Count" routines,
  // above) by the device peak operations per
  // second.
  // Implementation of costs other than
  // execution_time is optional, depending on the
  // device.
  Costs PredictConv2D(const OpContext& op_context) const;
  Costs PredictCwiseOp(const OpContext& op_context) const;
  Costs PredictConv2DBackpropInput(const OpContext& op_context) const;
  Costs PredictConv2DBackpropFilter(const OpContext& op_context) const;
  Costs PredictFusedConv2DBiasActivation(const OpContext& op_context) const;
  Costs PredictMatMul(const OpContext& op_context) const;
  Costs PredictNoOp(const OpContext& op_context) const;
  Costs PredictIdentity(const OpContext& op_context) const;
  Costs PredictVariable(const OpContext& op_context) const;
  Costs PredictBatchMatMul(const OpContext& op_context) const;
  Costs PredictMetadata(const OpContext& op_context) const;
  Costs PredictGatherOrSlice(const OpContext& op_context) const;
  Costs PredictMaxPool(const OpContext& op_context) const;
  Costs PredictMaxPoolGrad(const OpContext& op_context) const;
  Costs PredictAvgPool(const OpContext& op_context) const;
  Costs PredictAvgPoolGrad(const OpContext& op_context) const;
  Costs PredictFusedBatchNorm(const OpContext& op_context) const;
  Costs PredictFusedBatchNormGrad(const OpContext& op_context) const;

  // Generic cost prediction method for fused operations.
  Costs PredictFusedOp(const OpContext& op_context,
                       const std::vector<OpContext>& fused_op_contexts) const;

  // Utility function for safe division. Returns 0
  // if rhs is 0 or negative.
  static double SafeDiv(const double lhs, const double rhs) {
    if (rhs > 0) {
      return lhs / rhs;
    } else {
      return 0.0;
    }
  }

  // For convolution and its grad ops.
  static ConvolutionDimensions ConvolutionDimensionsFromInputs(
      const TensorShapeProto& original_image_shape,
      const TensorShapeProto& original_filter_shape, const OpInfo& op_info,
      bool* found_unknown_shapes);

  // For Pooling, FusedBatchNorm, and their grad ops.
  static ConvolutionDimensions OpDimensionsFromInputs(
      const TensorShapeProto& original_image_shape, const OpInfo& op_info,
      bool* found_unknown_shapes);

  // Helper to construct child operation contexts for the component operations
  // of fused ops.
  static OpContext FusedChildContext(
      const OpContext& parent, const string& op_name,
      const OpInfo::TensorProperties& output,
      const std::vector<OpInfo::TensorProperties>& inputs);

  // Helper to construct tensor shapes.
  static OpInfo::TensorProperties DescribeTensor(
      DataType type, const std::vector<int64>& dims);

  // This method calculates the execution time depending on whether IO can
  // overlap with computation. It assumes the memory and the compute times have
  // already been calculated.
  void CombineCostsAndUpdateExecutionTime(Costs* costs) const;

 protected:
  std::map<string, int> elementwise_ops_;
  typedef std::function<Costs(const OpContext& op_context)> CostImpl;
  std::map<string, CostImpl> device_cost_impl_;
  // If true, assume compute and memory overlap; hence, the op cost is max of
  // compute_time and memory_time, insteaf of sum of those two.
  bool compute_memory_overlap_;

 private:
  friend class OpLevelCostEstimatorTest;
};

}  // end namespace grappler
}  // end namespace tensorflow
#endif  // TENSORFLOW_CORE_GRAPPLER_COSTS_OP_LEVEL_COST_ESTIMATOR_H_
