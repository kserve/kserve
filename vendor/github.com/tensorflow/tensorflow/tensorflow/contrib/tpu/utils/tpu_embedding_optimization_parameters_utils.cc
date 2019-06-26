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

#include "tensorflow/contrib/tpu/utils/tpu_embedding_optimization_parameters_utils.h"
#include "tensorflow/core/lib/core/errors.h"

namespace tensorflow {
namespace tpu {

string GetOptimizationAlgorithmName(OptimizationAlgorithm alg) {
  switch (alg) {
    case OptimizationAlgorithm::kAdagrad:
      return "Adagrad";
    case OptimizationAlgorithm::kStochasticGradientDescent:
      return "StochasticGradientDescent";
    case OptimizationAlgorithm::kFtrl:
      return "FTRL";
    case OptimizationAlgorithm::kAdam:
      return "ADAM";
    case OptimizationAlgorithm::kMomentum:
      return "Momentum";
    case OptimizationAlgorithm::kRmsProp:
      return "RMSProp";
    case OptimizationAlgorithm::kCenteredRmsProp:
      return "CenteredRMSProp";
    case OptimizationAlgorithm::kMdlAdagradLight:
      return "MDLAdagradLight";
    case OptimizationAlgorithm::kAdadelta:
      return "Adadelta";
    case OptimizationAlgorithm::kProximalAdagrad:
      return "ProximalAdagrad";
    case OptimizationAlgorithm::PARAMETERS_NOT_SET:
      return "*** Not set ***";
  }
}

string GetOptimizationAlgorithmFriendlyName(OptimizationAlgorithm alg) {
  switch (alg) {
    case OptimizationAlgorithm::kAdagrad:
      return "Adagrad";
    case OptimizationAlgorithm::kStochasticGradientDescent:
      return "stochastic gradient descent";
    case OptimizationAlgorithm::kFtrl:
      return "FTRL";
    case OptimizationAlgorithm::kAdam:
      return "ADAM";
    case OptimizationAlgorithm::kMomentum:
      return "Momentum";
    case OptimizationAlgorithm::kRmsProp:
      return "RMSProp";
    case OptimizationAlgorithm::kCenteredRmsProp:
      return "centered RMSProp";
    case OptimizationAlgorithm::kMdlAdagradLight:
      return "MDL Adagrad Light";
    case OptimizationAlgorithm::kAdadelta:
      return "Adadelta";
    case OptimizationAlgorithm::kProximalAdagrad:
      return "proximal Adagrad";
    case OptimizationAlgorithm::PARAMETERS_NOT_SET:
      return "unknown (not specified)";
  }
}

// Returns the number of optimization parameter vectors used by the optimization
// algorithm, excluding the weights themselves and assuming no gradient
// accumulation.
Status GetBaseAuxiliaryParameterCount(OptimizationAlgorithm alg, int* count) {
  switch (alg) {
    case OptimizationAlgorithm::kAdagrad:
      *count = 1;
      return Status::OK();
    case OptimizationAlgorithm::kStochasticGradientDescent:
      *count = 0;
      return Status::OK();
    case OptimizationAlgorithm::kFtrl:
      *count = 2;
      return Status::OK();
    case OptimizationAlgorithm::kAdam:
      *count = 2;
      return Status::OK();
    case OptimizationAlgorithm::kMomentum:
      *count = 1;
      return Status::OK();
    case OptimizationAlgorithm::kRmsProp:
      *count = 2;
      return Status::OK();
    case OptimizationAlgorithm::kCenteredRmsProp:
      *count = 3;
      return Status::OK();
    case OptimizationAlgorithm::kMdlAdagradLight:
      *count = 3;
      return Status::OK();
    case OptimizationAlgorithm::kAdadelta:
      *count = 2;
      return Status::OK();
    case OptimizationAlgorithm::kProximalAdagrad:
      *count = 1;
      return Status::OK();
    case OptimizationAlgorithm::PARAMETERS_NOT_SET:
      return errors::InvalidArgument("No optimization algorithm specified");
  }
}

Status GetGradientAccumulationSupport(OptimizationAlgorithm alg,
                                      GradientAccumulationSupport* support) {
  switch (alg) {
    case OptimizationAlgorithm::kAdagrad:
      *support = GradientAccumulationSupport::kSupported;
      return Status::OK();
    case OptimizationAlgorithm::kStochasticGradientDescent:
      *support = GradientAccumulationSupport::kUnnecessary;
      return Status::OK();
    default: {
      int auxiliary_parameter_count;
      TF_RETURN_IF_ERROR(
          GetBaseAuxiliaryParameterCount(alg, &auxiliary_parameter_count));
      *support = auxiliary_parameter_count + 1 <= kMaxAuxiliaryParameterCount
                     ? GradientAccumulationSupport::kSupported
                     : GradientAccumulationSupport::kNotSupported;
      return Status::OK();
    }
  }
}
namespace {
// Make a normal state variable specification.
StateVariableSpecification MakeStandardStateVariableSpecification(
    const string& name) {
  StateVariableSpecification result;
  result.set_name(name);
  result.mutable_user_defined();
  return result;
}
}  // namespace

Status GetOptimizationAlgorithmStateVariables(
    OptimizationAlgorithm alg, bool use_gradient_accumulation,
    std::vector<StateVariableSpecification>* state_variables) {
  // The first parameter set is always the weights themselves.
  state_variables->push_back(
      MakeStandardStateVariableSpecification("parameters"));
  // The order of the returned parameters needs to match the offsets used by
  // the algorithm implementations in test_util.cc and
  // address_handler_program_creator.cc.
  switch (alg) {
    case OptimizationAlgorithm::kAdagrad: {
      state_variables->push_back(
          MakeStandardStateVariableSpecification("accumulators"));
      break;
    }
    case OptimizationAlgorithm::kStochasticGradientDescent: {
      // None.
      break;
    }
    case OptimizationAlgorithm::kFtrl: {
      state_variables->push_back(
          MakeStandardStateVariableSpecification("accumulators"));
      state_variables->push_back(
          MakeStandardStateVariableSpecification("linears"));
      break;
    }
    case OptimizationAlgorithm::kAdam: {
      state_variables->push_back(
          MakeStandardStateVariableSpecification("momenta"));
      state_variables->push_back(
          MakeStandardStateVariableSpecification("velocities"));
      break;
    }
    case OptimizationAlgorithm::kMomentum: {
      state_variables->push_back(
          MakeStandardStateVariableSpecification("momenta"));
      break;
    }
    case OptimizationAlgorithm::kRmsProp: {
      state_variables->push_back(MakeStandardStateVariableSpecification("ms"));
      state_variables->push_back(MakeStandardStateVariableSpecification("mom"));
      break;
    }
    case OptimizationAlgorithm::kCenteredRmsProp: {
      state_variables->push_back(MakeStandardStateVariableSpecification("ms"));
      state_variables->push_back(MakeStandardStateVariableSpecification("mom"));
      state_variables->push_back(MakeStandardStateVariableSpecification("mg"));
      break;
    }
    case OptimizationAlgorithm::kMdlAdagradLight: {
      state_variables->push_back(
          MakeStandardStateVariableSpecification("accumulators"));
      state_variables->push_back(
          MakeStandardStateVariableSpecification("weights"));
      state_variables->push_back(
          MakeStandardStateVariableSpecification("benefits"));
      break;
    }
    case OptimizationAlgorithm::kAdadelta: {
      state_variables->push_back(
          MakeStandardStateVariableSpecification("accumulators"));
      state_variables->push_back(
          MakeStandardStateVariableSpecification("updates"));
      break;
    }
    case OptimizationAlgorithm::kProximalAdagrad: {
      state_variables->push_back(
          MakeStandardStateVariableSpecification("accumulators"));
      break;
    }
    case OptimizationAlgorithm::PARAMETERS_NOT_SET: {
      return errors::InvalidArgument("No optimization algorithm specified");
    }
  }
  // This needs to be last so that the save/restore ops do not need to know
  // about gradient accumulation.
  if (use_gradient_accumulation) {
    StateVariableSpecification gradient_acc;
    gradient_acc.set_name("gradient_accumulators");
    gradient_acc.mutable_fill_with_constant()->set_initial_value(
        kGradientAccumulatorInitialValue);
    state_variables->push_back(std::move(gradient_acc));
  }
  if (state_variables->size() > kMaxAuxiliaryParameterCount + 1) {
    return errors::InvalidArgument(
        "Optimization algorithm", GetOptimizationAlgorithmName(alg),
        "does not support gradient accumulation because it "
        "already has too many other accumulators");
  }
  return Status::OK();
}  // namespace tpu

std::vector<OptimizationAlgorithm> GetOptimizationAlgorithms() {
  return {
      OptimizationAlgorithm::kAdagrad,
      OptimizationAlgorithm::kStochasticGradientDescent,
      OptimizationAlgorithm::kFtrl,
      OptimizationAlgorithm::kAdam,
      OptimizationAlgorithm::kMomentum,
      OptimizationAlgorithm::kRmsProp,
      OptimizationAlgorithm::kCenteredRmsProp,
      OptimizationAlgorithm::kMdlAdagradLight,
      OptimizationAlgorithm::kAdadelta,
      OptimizationAlgorithm::kProximalAdagrad,
  };
}

}  // namespace tpu
}  // namespace tensorflow
