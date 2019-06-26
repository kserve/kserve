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

#ifndef TENSORFLOW_CONTRIB_TPU_UTILS_TPU_EMBEDDING_OPTIMIZATION_PARAMETERS_UTILS_H_
#define TENSORFLOW_CONTRIB_TPU_UTILS_TPU_EMBEDDING_OPTIMIZATION_PARAMETERS_UTILS_H_

#include <string>
#include "absl/base/casts.h"
#include "tensorflow/contrib/tpu/proto/optimization_parameters.pb.h"
#include "tensorflow/core/lib/core/status.h"

namespace tensorflow {
namespace tpu {

using OptimizationAlgorithm = OptimizationParameters::ParametersCase;

// Returns the name of the optimization algorithm.
string GetOptimizationAlgorithmName(OptimizationAlgorithm alg);

// Returns a user-friendly name for the optimization algorithm.
string GetOptimizationAlgorithmFriendlyName(OptimizationAlgorithm alg);

// Returns all supported optimization algorithms.
std::vector<OptimizationAlgorithm> GetOptimizationAlgorithms();

enum class GradientAccumulationSupport {
  // Accumulation cannot be used with this optimizer.
  kNotSupported,

  // Accumulation is unnecessary because optimizer application is commutative.
  kUnnecessary,

  // Accumulation is allowed and changes optimizer behavior.
  kSupported,
};

// Returns the number of optimization parameter vectors used by the optimization
// algorithm, excluding the weights themselves and assuming no gradient
// accumulation.
Status GetBaseAuxiliaryParameterCount(OptimizationAlgorithm alg, int *count);

// Returns whether (and how) an optimization algorithm supports gradient
// accumulation.
Status GetGradientAccumulationSupport(OptimizationAlgorithm alg,
                                      GradientAccumulationSupport *support);

// Returns the parameter specifications for the optimization algorithm (the main
// parameters first, followed by any auxiliary parameters such as Adagrad
// accumulators).
Status GetOptimizationAlgorithmStateVariables(
    OptimizationAlgorithm alg, bool use_gradient_accumulation,
    std::vector<StateVariableSpecification> *state_variables);

// Maximum value of auxiliar_parameter_count for any optimization algorithm.
static constexpr int kMaxAuxiliaryParameterCount = 3;

// Fill value for gradient accumulators. This is a denormal so that it will be
// flushed to zero on the current TPU platforms and needs to continue to have
// the following properties in the future:
//
// 1. Does not have the same bit pattern as a zero and can be distinguished from
// it using integer operations.
// 2. Treated as zero by floating-point arithmetic operations (at least addition
// and subtraction).
// 3. Cannot be produced by any floating-point arithmetic operation, including
// those involving itself.
//
// It does not need to compare equal or not equal to zero in floating point. We
// need to use a non-zero value here because some optimization algorithms are
// not no-ops on zero gradients, so we need to distinguish an accumulated
// gradient of zero from one that has been cleared after its gradients have
// already been applied to the parameters and accumulators.
const float kGradientAccumulatorInitialValue = absl::bit_cast<float, uint32>(1);

}  // namespace tpu
}  // namespace tensorflow

#endif  // TENSORFLOW_CONTRIB_TPU_UTILS_TPU_EMBEDDING_OPTIMIZATION_PARAMETERS_UTILS_H_
